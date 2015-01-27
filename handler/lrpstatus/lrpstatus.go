package lrpstatus

import (
	"encoding/json"
	"net/http"

	"github.com/cloudfoundry-incubator/receptor"
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
			State:        stateFor(instance.State, lrpLogger),
			Since:        instance.Since,
		}
	}

	err = json.NewEncoder(w).Encode(instances)
	if err != nil {
		lrpLogger.Error("stream-response-failed", err, lager.Data{"guid": guid})
	}
}

func stateFor(state receptor.ActualLRPState, logger lager.Logger) cc_messages.LRPInstanceState {
	switch state {
	case receptor.ActualLRPStateUnclaimed:
		return cc_messages.LRPInstanceStateStarting
	case receptor.ActualLRPStateClaimed:
		return cc_messages.LRPInstanceStateStarting
	case receptor.ActualLRPStateRunning:
		return cc_messages.LRPInstanceStateRunning
	case receptor.ActualLRPStateCrashed:
		return cc_messages.LRPInstanceStateFlapping
	default:
		logger.Error("unknown-state", nil, lager.Data{"state": state})
		return cc_messages.LRPInstanceStateUnknown
	}
}
