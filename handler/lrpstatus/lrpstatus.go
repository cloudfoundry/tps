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

	semaphore chan struct{}
}

func NewHandler(apiClient receptor.Client, maxInFlight int, logger lager.Logger) http.Handler {
	return &handler{
		apiClient: apiClient,
		logger:    logger,

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

	actual, err := handler.apiClient.ActualLRPsByProcessGuid(guid)
	if err != nil {
		lrpLogger.Error("failed-retrieving-lrp-info", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	instances := make([]cc_messages.LRPInstance, len(actual))
	for i, instance := range actual {
		instances[i] = cc_messages.LRPInstance{
			ProcessGuid:  instance.ProcessGuid,
			InstanceGuid: instance.InstanceGuid,
			Index:        uint(instance.Index),
			State:        cc_conv.StateFor(instance.State),
			Details:      instance.PlacementError,
			Since:        instance.Since,
		}
	}

	err = json.NewEncoder(w).Encode(instances)
	if err != nil {
		lrpLogger.Error("stream-response-failed", err, lager.Data{"guid": guid})
	}
}
