package watcher

import (
	"os"
	"sync/atomic"

	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/tps/cc_client"
	"github.com/cloudfoundry/gunk/workpool"
	"github.com/pivotal-golang/lager"
)

type Watcher struct {
	receptorClient receptor.Client
	ccClient       cc_client.CcClient
	logger         lager.Logger

	pool *workpool.WorkPool
}

func NewWatcher(
	logger lager.Logger,
	receptorClient receptor.Client,
	ccClient cc_client.CcClient,
) *Watcher {
	return &Watcher{
		receptorClient: receptorClient,
		ccClient:       ccClient,
		logger:         logger.Session("watcher"),

		pool: workpool.NewWorkPool(500),
	}
}

func (watcher *Watcher) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	watcher.logger.Info("starting")

	close(ready)
	watcher.logger.Info("started")
	defer watcher.logger.Info("finished")

	eventChan := make(chan receptor.Event)

	var eventSource atomic.Value
	var stopEventSource int32

	go func() {
		var err error
		var es receptor.EventSource

		for {
			if atomic.LoadInt32(&stopEventSource) == 1 {
				return
			}

			es, err = watcher.receptorClient.SubscribeToEvents()
			if err != nil {
				watcher.logger.Error("failed-subscribing-to-events", err)
				continue
			}

			eventSource.Store(es)

			var event receptor.Event
			for {
				event, err = es.Next()
				if err != nil {
					watcher.logger.Error("failed-getting-next-event", err)
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
			watcher.handleEvent(watcher.logger, event)

		case <-signals:
			watcher.logger.Info("stopping")
			atomic.StoreInt32(&stopEventSource, 1)
			if es := eventSource.Load(); es != nil {
				err := es.(receptor.EventSource).Close()
				if err != nil {
					watcher.logger.Error("failed-closing-event-source", err)
				}
			}
			return nil
		}
	}
}

func (watcher *Watcher) handleEvent(logger lager.Logger, event receptor.Event) {
	if changed, ok := event.(receptor.ActualLRPChangedEvent); ok {
		if changed.After.CrashCount > changed.Before.CrashCount {
			logger.Info("app-crashed", lager.Data{
				"process-guid": changed.After.ProcessGuid,
				"index":        changed.After.Index,
			})

			guid := changed.After.ProcessGuid
			appCrashed := cc_messages.AppCrashedRequest{
				Instance:        changed.Before.InstanceGuid,
				Index:           changed.After.Index,
				Reason:          "CRASHED",
				ExitDescription: changed.After.CrashReason,
				CrashCount:      changed.After.CrashCount,
				CrashTimestamp:  changed.After.Since,
			}

			watcher.pool.Submit(func() {
				err := watcher.ccClient.AppCrashed(guid, appCrashed, logger)
				if err != nil {
					logger.Info("failed-app-crashed", lager.Data{
						"process-guid": guid,
						"index":        changed.After.Index,
						"error":        err,
					})
				}
			})
		}
	}
}
