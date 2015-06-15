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

	actuals, err := handler.apiClient.ActualLRPsByProcessGuid(guid)
	if err != nil {
		lrpLogger.Error("failed-retrieving-lrp-info", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	instances := LRPInstances(actuals,
		func(instance *cc_messages.LRPInstance, actual *receptor.ActualLRPResponse) {
			instance.Details = actual.PlacementError
		},
	)

	err = json.NewEncoder(w).Encode(instances)
	if err != nil {
		lrpLogger.Error("stream-response-failed", err, lager.Data{"guid": guid})
	}
}

func LRPInstances(
	actualLRPs []receptor.ActualLRPResponse,
	addInfo func(*cc_messages.LRPInstance, *receptor.ActualLRPResponse),
) []cc_messages.LRPInstance {
	instances := make([]cc_messages.LRPInstance, len(actualLRPs))
	for i, actual := range actualLRPs {
		instance := cc_messages.LRPInstance{
			ProcessGuid:  actual.ProcessGuid,
			InstanceGuid: actual.InstanceGuid,
			Index:        uint(actual.Index),
			Since:        actual.Since / 1e9,
			State:        cc_conv.StateFor(actual.State),
		}

		if addInfo != nil {
			addInfo(&instance, &actual)
		}

		instances[i] = instance
	}

	return instances
}
