package lrpstatus

import (
	"encoding/json"
	"net/http"

	"github.com/cloudfoundry-incubator/bbs"
	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/tps/handler/cc_conv"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
)

type handler struct {
	apiClient bbs.Client
	clock     clock.Clock
	logger    lager.Logger
}

func NewHandler(apiClient bbs.Client, clk clock.Clock, logger lager.Logger) http.Handler {
	return &handler{
		apiClient: apiClient,
		clock:     clk,
		logger:    logger,
	}
}

func (handler *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	lrpLogger := handler.logger.Session("lrp-handler")

	guid := r.FormValue(":guid")

	actualLRPGroups, err := handler.apiClient.ActualLRPGroupsByProcessGuid(guid)
	if err != nil {
		lrpLogger.Error("failed-retrieving-lrp-info", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	instances := LRPInstances(actualLRPGroups,
		func(instance *cc_messages.LRPInstance, actual *models.ActualLRP) {
			instance.Details = actual.PlacementError
		},
		handler.clock,
	)

	err = json.NewEncoder(w).Encode(instances)
	if err != nil {
		lrpLogger.Error("stream-response-failed", err, lager.Data{"guid": guid})
	}
}

func LRPInstances(
	actualLRPGroups []*models.ActualLRPGroup,
	addInfo func(*cc_messages.LRPInstance, *models.ActualLRP),
	clk clock.Clock,
) []cc_messages.LRPInstance {
	instances := make([]cc_messages.LRPInstance, len(actualLRPGroups))
	for i, actualLRPGroup := range actualLRPGroups {
		actual, _ := actualLRPGroup.Resolve()

		instance := cc_messages.LRPInstance{
			ProcessGuid:  actual.ProcessGuid,
			InstanceGuid: actual.InstanceGuid,
			Index:        uint(actual.Index),
			Since:        actual.Since / 1e9,
			Uptime:       (clk.Now().UnixNano() - actual.Since) / 1e9,
			State:        cc_conv.StateFor(actual.State),
		}

		if addInfo != nil {
			addInfo(&instance, actual)
		}

		instances[i] = instance
	}

	return instances
}
