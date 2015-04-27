package lrpstatus

import (
	"encoding/json"
	"net/http"

	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/tps/handler/cc_conv"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
)

type handler struct {
	apiClient receptor.Client
	logger    lager.Logger
}

func NewHandler(apiClient receptor.Client, logger lager.Logger) http.Handler {
	return &handler{
		apiClient: apiClient,
		logger:    logger,
	}
}

func (handler *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	lrpLogger := handler.logger.Session("lrp-handler")

	guid := r.FormValue(":guid")

	actual, err := handler.apiClient.ActualLRPsByProcessGuid(guid)
	if err != nil {
		lrpLogger.Error("failed-retrieving-lrp-info", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	instances := make([]cc_messages.LRPInstance, 0, len(actual))
	for _, instance := range actual {
		if instance.State == receptor.ActualLRPStateUnclaimed {
			continue
		}
		instances = append(instances, cc_messages.LRPInstance{
			ProcessGuid:  instance.ProcessGuid,
			InstanceGuid: instance.InstanceGuid,
			Index:        uint(instance.Index),
			State:        cc_conv.StateFor(instance.State),
			Details:      instance.PlacementError,
			Since:        instance.Since,
		})
	}

	err = json.NewEncoder(w).Encode(instances)
	if err != nil {
		lrpLogger.Error("stream-response-failed", err, lager.Data{"guid": guid})
	}
}
