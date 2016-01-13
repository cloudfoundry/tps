package handler

import (
	"net/http"

	"github.com/cloudfoundry-incubator/bbs"
	"github.com/cloudfoundry-incubator/tps"
	"github.com/cloudfoundry-incubator/tps/handler/bulklrpstatus"
	"github.com/cloudfoundry-incubator/tps/handler/lrpstats"
	"github.com/cloudfoundry-incubator/tps/handler/lrpstatus"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/rata"
)

func New(apiClient bbs.Client, noaaClient lrpstats.NoaaClient, maxInFlight, bulkLRPStatusWorkers int, logger lager.Logger) (http.Handler, error) {
	semaphore := make(chan struct{}, maxInFlight)
	clock := clock.NewClock()

	handlers := map[string]http.Handler{
		tps.LRPStatus: tpsHandler{
			semaphore:       semaphore,
			delegateHandler: LogWrap(lrpstatus.NewHandler(apiClient, clock, logger), logger),
		},
		tps.LRPStats: tpsHandler{
			semaphore:       semaphore,
			delegateHandler: LogWrap(lrpstats.NewHandler(apiClient, noaaClient, clock, logger), logger),
		},
		tps.BulkLRPStatus: tpsHandler{
			semaphore:       semaphore,
			delegateHandler: LogWrap(bulklrpstatus.NewHandler(apiClient, clock, bulkLRPStatusWorkers, logger), logger),
		},
	}

	return rata.NewRouter(tps.Routes, handlers)
}

type tpsHandler struct {
	semaphore       chan struct{}
	delegateHandler http.Handler
}

func (handler tpsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	select {
	case handler.semaphore <- struct{}{}:
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	defer func() {
		<-handler.semaphore
	}()

	handler.delegateHandler.ServeHTTP(w, r)
}
