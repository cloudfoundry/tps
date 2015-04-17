package handler

import (
	"net/http"

	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/tps"
	"github.com/cloudfoundry-incubator/tps/handler/lrpstats"
	"github.com/cloudfoundry-incubator/tps/handler/lrpstatus"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/rata"
)

func New(apiClient receptor.Client, noaaClient lrpstats.NoaaClient, maxInFlight int, logger lager.Logger) (http.Handler, error) {
	semaphore := make(chan struct{}, maxInFlight)

	handlers := map[string]http.Handler{
		tps.LRPStatus: tpsHandler{
			semaphore:       semaphore,
			delegateHandler: lrpstatus.NewHandler(apiClient, logger),
		},
		tps.LRPStats: tpsHandler{
			semaphore:       semaphore,
			delegateHandler: lrpstats.NewHandler(apiClient, noaaClient, logger),
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
