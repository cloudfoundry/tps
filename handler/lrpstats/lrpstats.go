package lrpstats

import (
	"encoding/json"
	"net/http"

	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/tps/handler/cc_conv"
	"github.com/cloudfoundry/noaa/events"
	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter -o fakes/fake_noaaclient.go . NoaaClient
type NoaaClient interface {
	ContainerMetrics(appGuid string, authToken string) ([]*events.ContainerMetric, error)
	Close() error
}

type handler struct {
	receptorClient receptor.Client
	noaaClient     NoaaClient
	logger         lager.Logger
}

func NewHandler(receptorClient receptor.Client, noaaClient NoaaClient, logger lager.Logger) http.Handler {
	return &handler{receptorClient: receptorClient, noaaClient: noaaClient, logger: logger}
}

func (handler *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	authorization := r.Header.Get("Authorization")
	if authorization == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	guid := r.FormValue(":guid")
	if guid == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	desiredLRP, err := handler.receptorClient.GetDesiredLRP(guid)
	if err != nil {
		handler.logger.Error("fetching-desired-lrp-failed", err, lager.Data{"ProcessGuid": guid})
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	actualLRPs, err := handler.receptorClient.ActualLRPsByProcessGuid(guid)
	if err != nil {
		handler.logger.Error("fetching-actual-lrp-info-failed", err, lager.Data{"ProcessGuid": guid})
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	metrics, err := handler.noaaClient.ContainerMetrics(desiredLRP.LogGuid, authorization)
	defer handler.noaaClient.Close()
	if err != nil {
		handler.logger.Error("container-metrics-failed", err, lager.Data{
			"ProcessGuid": guid,
			"LogGuid":     desiredLRP.LogGuid,
		})
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	metricsByInstanceIndex := make(map[uint]*cc_messages.LRPInstanceStats)
	for _, metric := range metrics {
		cpuPercentageAsDecimal := metric.GetCpuPercentage() / 100
		metricsByInstanceIndex[uint(metric.GetInstanceIndex())] = &cc_messages.LRPInstanceStats{
			CpuPercentage: cpuPercentageAsDecimal,
			MemoryBytes:   metric.GetMemoryBytes(),
			DiskBytes:     metric.GetDiskBytes(),
		}
	}

	instances := make([]cc_messages.LRPInstance, len(actualLRPs))
	for i, instance := range actualLRPs {
		instances[i] = cc_messages.LRPInstance{
			ProcessGuid:  instance.ProcessGuid,
			InstanceGuid: instance.InstanceGuid,
			Index:        uint(instance.Index),
			Since:        instance.Since,
			State:        cc_conv.StateFor(instance.State),
			Stats:        metricsByInstanceIndex[uint(instance.Index)],
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(w).Encode(instances)
	if err != nil {
		handler.logger.Error("stream-response-failed", err, lager.Data{"guid": guid})
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
		return cc_messages.LRPInstanceStateCrashed
	default:
		logger.Error("unknown-state", nil, lager.Data{"state": state})
		return cc_messages.LRPInstanceStateUnknown
	}
}
