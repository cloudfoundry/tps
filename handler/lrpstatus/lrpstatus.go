package lrpstatus

import (
	"encoding/json"
	"net/http"

	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/cloudfoundry/gosteno"

	"github.com/cloudfoundry-incubator/tps/api"
)

type handler struct {
	bbs    Bbs.TPSBBS
	logger *gosteno.Logger
}

func NewHandler(bbs Bbs.TPSBBS, logger *gosteno.Logger) http.Handler {
	return &handler{
		bbs:    bbs,
		logger: logger,
	}
}

func (handler *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	actual, err := handler.bbs.GetActualLRPsByProcessGuid(r.FormValue(":guid"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	instances := make([]api.LRPInstance, len(actual))
	for i, instance := range actual {
		instances[i] = api.LRPInstance{
			ProcessGuid:  instance.ProcessGuid,
			InstanceGuid: instance.InstanceGuid,
			Index:        uint(instance.Index),
			State:        handler.stateFor(instance.State),
			Since:        instance.Since,
		}

	}

	json.NewEncoder(w).Encode(instances)
}

func (handler *handler) stateFor(state models.ActualLRPState) string {
	switch state {
	case models.ActualLRPStateStarting:
		return "starting"
	case models.ActualLRPStateRunning:
		return "running"
	default:
		handler.logger.Errord(map[string]interface{}{
			"state": state,
		}, "tps.unknown-state")

		return "unknown"
	}
}
