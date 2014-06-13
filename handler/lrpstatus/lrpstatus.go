package lrpstatus

import (
	"encoding/json"
	"fmt"
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
	guid := r.FormValue(":guid")

	handler.logger.Infof("request for lrp %s received", guid)

	actual, err := handler.bbs.GetActualLRPsByProcessGuid(guid)
	if err != nil {
		handler.logger.Errord(map[string]interface{}{
			"error": err.Error(),
		}, "tps.lrps")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	handler.logger.Infof("retrieved lrp information for %s from bbs", guid)

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

	err = json.NewEncoder(w).Encode(instances)

	handler.logger.Infof("responding with lrp information for %s: %#v", guid, instances)

	if err != nil {
		handler.logger.Errord(map[string]interface{}{
			"error": fmt.Sprintf("failed to stream response for %s: %s", guid, err),
		}, "tps.lrps")
	}
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
