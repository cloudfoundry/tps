package watcher

import (
	"os"
	"sync/atomic"
	"time"

	"github.com/cloudfoundry-incubator/bbs"
	"github.com/cloudfoundry-incubator/bbs/events"
	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/tps/cc_client"
	"github.com/cloudfoundry/gunk/workpool"
	"github.com/pivotal-golang/lager"
)

type Watcher struct {
	bbsClient bbs.Client
	ccClient  cc_client.CcClient
	logger    lager.Logger

	pool *workpool.WorkPool
}

func NewWatcher(
	logger lager.Logger,
	bbsClient bbs.Client,
	ccClient cc_client.CcClient,
) (*Watcher, error) {
	workPool, err := workpool.NewWorkPool(500)
	if err != nil {
		return nil, err
	}

	return &Watcher{
		bbsClient: bbsClient,
		ccClient:  ccClient,
		logger:    logger,

		pool: workPool,
	}, nil
}

func (watcher *Watcher) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	logger := watcher.logger.Session("watcher")
	logger.Info("starting")

	close(ready)
	logger.Info("started")
	defer logger.Info("finished")

	eventChan := make(chan models.Event)

	var eventSource atomic.Value
	var stopEventSource int32

	go func() {
		var err error
		var es events.EventSource

		for {
			if atomic.LoadInt32(&stopEventSource) == 1 {
				return
			}

			logger.Info("subscribing-to-events")
			es, err = watcher.bbsClient.SubscribeToEvents()
			if err != nil {
				logger.Error("failed-subscribing-to-events", err)
				continue
			}

			eventSource.Store(es)

			var event models.Event
			for {
				event, err = es.Next()
				if err != nil {
					logger.Error("failed-getting-next-event", err)
					// wait a bit before retrying
					time.Sleep(time.Second)
					break
				}

				if event != nil {
					eventChan <- event
				}
			}
		}
	}()

	for {
		select {
		case event := <-eventChan:
			watcher.handleEvent(logger, event)

		case <-signals:
			logger.Info("stopping")
			atomic.StoreInt32(&stopEventSource, 1)
			if es := eventSource.Load(); es != nil {
				err := es.(events.EventSource).Close()
				if err != nil {
					logger.Error("failed-closing-event-source", err)
				}
			}
			return nil
		}
	}
}

func (watcher *Watcher) handleEvent(logger lager.Logger, event models.Event) {
	if changed, ok := event.(*models.ActualLRPChangedEvent); ok {
		after, _ := changed.After.Resolve()

		if after.Domain == cc_messages.AppLRPDomain {
			before, _ := changed.Before.Resolve()

			if after.CrashCount > before.CrashCount {
				logger.Info("app-crashed", lager.Data{
					"process-guid": after.ProcessGuid,
					"index":        after.Index,
				})

				guid := after.ProcessGuid
				appCrashed := cc_messages.AppCrashedRequest{
					Instance:        before.InstanceGuid,
					Index:           int(after.Index),
					Reason:          "CRASHED",
					ExitDescription: after.CrashReason,
					CrashCount:      int(after.CrashCount),
					CrashTimestamp:  after.Since,
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
	}
}
