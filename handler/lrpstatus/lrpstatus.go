package lrpstatus

import (
	"encoding/json"
	"net/http"

	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/tps/api"
)

type handler struct {
	bbs    Bbs.TPSBBS
	logger lager.Logger
}

func NewHandler(bbs Bbs.TPSBBS, logger lager.Logger) http.Handler {
	return &handler{
		bbs:    bbs,
		logger: logger,
	}
}

func (handler *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	lrpLogger := handler.logger.Session("lrp-handler")

	guid := r.FormValue(":guid")

	actual, err := handler.bbs.GetActualLRPsByProcessGuid(guid)
	if err != nil {
		lrpLogger.Error("failed-retrieving-bbs-info", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	instances := make([]api.LRPInstance, len(actual))
	for i, instance := range actual {
		instances[i] = api.LRPInstance{
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
