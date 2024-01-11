package watcher

import (
	"os"
	"time"

	"code.cloudfoundry.org/bbs"
	"code.cloudfoundry.org/bbs/events"
	"code.cloudfoundry.org/bbs/models"
	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/runtimeschema/cc_messages"
	"code.cloudfoundry.org/tps/cc_client"
	"code.cloudfoundry.org/workpool"
)

const DefaultRetryPauseInterval = time.Second

type Watcher struct {
	bbsClient          bbs.Client
	ccClient           cc_client.CcClient
	logger             lager.Logger
	retryPauseInterval time.Duration

	pool *workpool.WorkPool
}

func NewWatcher(
	logger lager.Logger,
	workPoolSize int,
	retryPauseInterval time.Duration,
	bbsClient bbs.Client,
	ccClient cc_client.CcClient,
) (*Watcher, error) {
	workPool, err := workpool.NewWorkPool(workPoolSize)
	if err != nil {
		return nil, err
	}

	return &Watcher{
		bbsClient:          bbsClient,
		ccClient:           ccClient,
		logger:             logger,
		retryPauseInterval: retryPauseInterval,
		pool:               workPool,
	}, nil
}

func (watcher *Watcher) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	logger := watcher.logger.Session("watcher")
	logger.Info("starting")
	defer logger.Info("finished")

	var subscription events.EventSource
	subscriptionChan := make(chan events.EventSource, 1)
	go subscribeToEvents(logger, watcher.bbsClient, subscriptionChan)

	eventChan := make(chan models.Event, 1)
	errorChan := make(chan error, 1)
	nextErrCount := 0

	close(ready)
	logger.Info("started")

	for {
		select {
		case subscription = <-subscriptionChan:
			if subscription != nil {
				go nextEvent(logger, subscription, eventChan, errorChan, watcher.retryPauseInterval)
			} else {
				go subscribeToEvents(logger, watcher.bbsClient, subscriptionChan)
			}

		case event := <-eventChan:
			if event != nil {
				watcher.handleEvent(logger, event)
			} else {
				nextErrCount += 1
				if nextErrCount > 2 {
					nextErrCount = 0
					go subscribeToEvents(logger, watcher.bbsClient, subscriptionChan)
					break
				}
			}
			go nextEvent(logger, subscription, eventChan, errorChan, watcher.retryPauseInterval)

		case err := <-errorChan:
			switch err {
			case events.ErrSourceClosed:
				logger.Debug("event-source-closed-resubscribe")
				go subscribeToEvents(logger, watcher.bbsClient, subscriptionChan)

			case events.ErrUnrecognizedEventType:
				logger.Debug("received-unexpected-event-type")
				go nextEvent(logger, subscription, eventChan, errorChan, watcher.retryPauseInterval)
			}

		case <-signals:
			logger.Info("stopping")
			if subscription != nil {
				err := subscription.Close()
				if err != nil {
					logger.Error("failed-closing-event-source", err)
				}
			}
			return nil
		}
	}
}

func (watcher *Watcher) handleEvent(logger lager.Logger, event models.Event) {
	if crashed, ok := event.(*models.ActualLRPCrashedEvent); ok {
		if crashed.ActualLRPKey.Domain == cc_messages.AppLRPDomain {
			logger.Info("app-crashed", lager.Data{
				"process-guid": crashed.ActualLRPKey.ProcessGuid,
				"index":        crashed.ActualLRPKey.Index,
			})

			guid := crashed.ActualLRPKey.ProcessGuid
			cellId := crashed.ActualLRPInstanceKey.CellId
			appCrashed := cc_messages.AppCrashedRequest{
				Instance:        crashed.ActualLRPInstanceKey.InstanceGuid,
				Index:           int(crashed.ActualLRPKey.Index),
				CellID:          cellId,
				Reason:          "CRASHED",
				ExitDescription: crashed.CrashReason,
				CrashCount:      int(crashed.CrashCount),
				CrashTimestamp:  crashed.Since,
			}

			watcher.pool.Submit(func() {
				logger := logger.WithData(lager.Data{
					"process-guid": guid,
					"index":        appCrashed.Index,
				})
				logger.Info("recording-app-crashed")
				err := watcher.ccClient.AppCrashed(guid, appCrashed, logger)
				if err != nil {
					logger.Error("failed-recording-app-crashed", err)
				}
			})
		}
	}

	if removed, ok := event.(*models.ActualLRPInstanceRemovedEvent); ok {
		key := removed.ActualLrp.ActualLRPKey
		if removed.ActualLrp.Presence == models.ActualLRP_Evacuating && key.Domain == cc_messages.AppLRPDomain {
			instanceKey := removed.ActualLrp.ActualLRPInstanceKey

			logger.Info("app-evacuating", lager.Data{
				"process-guid": key.ProcessGuid,
				"index":        key.Index,
			})

			appRescheduling := cc_messages.AppReschedulingRequest{
				Instance: instanceKey.InstanceGuid,
				Index:    int(key.Index),
				CellID:   instanceKey.CellId,
				Reason:   "Cell is being evacuated",
			}

			watcher.pool.Submit(func() {
				logger := logger.WithData(lager.Data{
					"process-guid": key.ProcessGuid,
					"index":        key.Index,
				})
				logger.Info("recording-evacuating-app-instance")
				err := watcher.ccClient.AppRescheduling(key.ProcessGuid, appRescheduling, logger)
				if err != nil {
					logger.Error("failed-recording-evacuating-app-instance", err)
				}
			})
		}
	}

	if changedEvent, ok := event.(*models.ActualLRPInstanceChangedEvent); ok {
		key := changedEvent.ActualLRPKey

		before := changedEvent.Before
		after := changedEvent.After
		if key.Domain == cc_messages.AppLRPDomain {
			changed, newValue := calculateRoutableChange(before.RoutableExists(), after.RoutableExists(), before.GetRoutable(), after.GetRoutable())
			if changed {

				logger.Info("app-readiness-changed", lager.Data{
					"process-guid": key.ProcessGuid,
					"index":        key.Index,
				})

				AppReadinessChanged := cc_messages.AppReadinessChangedRequest{
					Instance: changedEvent.ActualLRPInstanceKey.InstanceGuid,
					Index:    int(key.Index),
					CellID:   changedEvent.ActualLRPInstanceKey.CellId,
					Ready:    newValue,
				}

				watcher.pool.Submit(func() {
					logger := logger.WithData(lager.Data{
						"process-guid": key.ProcessGuid,
						"index":        key.Index,
					})
					logger.Info("recording-app-readiness-changed")
					err := watcher.ccClient.AppReadinessChanged(key.ProcessGuid, AppReadinessChanged, logger)
					if err != nil {
						logger.Error("failed-recording-app-readiness-changed", err)
					}
				})
			}
		}
	}
}

func calculateRoutableChange(beforeSet, afterSet, beforeValue, afterValue bool) (hasChanged, newValue bool) {
	// If routable is not set for either the before or after do not emit an
	// event.
	if !beforeSet && !afterSet {
		return false, false
	}

	// If routable is not set before, but is set after, emit an event
	// regardless of the after value.
	if !beforeSet && afterSet {
		return true, afterValue
	}

	// If routable was set before, but not after only emit event if it was not
	// ready before.
	if beforeSet && !afterSet {
		return !beforeValue, true
	}

	// If routable is set in both the before and after and they don't match,
	// then emit an event.
	if beforeValue != afterValue {
		return true, afterValue
	}

	// If routable is set in both the before and after, and the value has not
	// changed, do not emit an event.
	return false, false
}

func subscribeToEvents(logger lager.Logger, bbsClient bbs.Client, subscriptionChan chan<- events.EventSource) {
	logger.Info("subscribing-to-events")
	eventSource, err := bbsClient.SubscribeToInstanceEvents(logger)
	if err != nil {
		logger.Error("failed-subscribing-to-events", err)
		subscriptionChan <- nil
	} else {
		logger.Info("subscribed-to-events")
		subscriptionChan <- eventSource
	}
}

func nextEvent(logger lager.Logger, es events.EventSource, eventChan chan<- models.Event, errorChan chan<- error, retryPauseInterval time.Duration) {
	event, err := es.Next()

	switch err {
	case nil:
		eventChan <- event

	case events.ErrSourceClosed:
		logger.Error("failed-getting-next-event", err)
		errorChan <- err

	case events.ErrUnrecognizedEventType:
		logger.Error("failed-getting-next-event", err)
		errorChan <- err

	default:
		logger.Error("failed-getting-next-event", err)
		// wait a bit before retrying
		time.Sleep(retryPauseInterval)
		eventChan <- nil
	}
}
