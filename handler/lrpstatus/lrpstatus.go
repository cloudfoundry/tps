package lrpstatus

import (
	"encoding/json"
	"net/http"

	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/tps"
)

type handler struct {
	bbs    Bbs.TPSBBS
	logger lager.Logger

	semaphore chan struct{}
}

func NewHandler(bbs Bbs.TPSBBS, maxInFlight int, logger lager.Logger) http.Handler {
	return &handler{
		bbs:    bbs,
		logger: logger,

		semaphore: make(chan struct{}, maxInFlight),
	}
}

func (handler *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	select {
	case handler.semaphore <- struct{}{}:
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	defer func() {
		<-handler.semaphore
	}()

	lrpLogger := handler.logger.Session("lrp-handler")

	guid := r.FormValue(":guid")

	actual, err := handler.bbs.ActualLRPsByProcessGuid(guid)
	if err != nil {
		lrpLogger.Error("failed-retrieving-bbs-info", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	instances := make([]tps.LRPInstance, len(actual))
	for i, instance := range actual {
		instances[i] = tps.LRPInstance{
			ProcessGuid:  instance.ProcessGuid,
			InstanceGuid: instance.InstanceGuid,
			Index:        uint(instance.Index),
			State:        stateFor(instance.State, lrpLogger),
			Since:        instance.Since,
		}
	}

	err = json.NewEncoder(w).Encode(instances)
	if err != nil {
		lrpLogger.Error("stream-response-failed", err, lager.Data{"guid": guid})
	}
}

func stateFor(state models.ActualLRPState, logger lager.Logger) string {
	switch state {
	case models.ActualLRPStateStarting:
		return "starting"
	case models.ActualLRPStateRunning:
		return "running"
	default:
		logger.Error("unknown-state", nil, lager.Data{"state": state})
		return "unknown"
	}
}
