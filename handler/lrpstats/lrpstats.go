package lrpstats

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/cloudfoundry-incubator/nsync/recipebuilder"
	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/tps/handler/lrpstatus"
	"github.com/cloudfoundry/sonde-go/events"
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

		if e, ok := err.(receptor.Error); ok && e.Type == receptor.DesiredLRPNotFound {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	actualLRPs, err := handler.receptorClient.ActualLRPsByProcessGuid(guid)
	if err != nil {
		handler.logger.Error("fetching-actual-lrp-info-failed", err, lager.Data{"ProcessGuid": guid})
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	metrics, err := handler.noaaClient.ContainerMetrics(desiredLRP.LogGuid, authorization)
	if err != nil {
		handler.logger.Error("container-metrics-failed", err, lager.Data{
			"ProcessGuid": guid,
			"LogGuid":     desiredLRP.LogGuid,
		})
	}

	metricsByInstanceIndex := make(map[uint]*cc_messages.LRPInstanceStats)
	currentTime := time.Now()
	for _, metric := range metrics {
		cpuPercentageAsDecimal := metric.GetCpuPercentage() / 100
		metricsByInstanceIndex[uint(metric.GetInstanceIndex())] = &cc_messages.LRPInstanceStats{
			Time:          currentTime,
			CpuPercentage: cpuPercentageAsDecimal,
			MemoryBytes:   metric.GetMemoryBytes(),
			DiskBytes:     metric.GetDiskBytes(),
		}
	}

	instances := lrpstatus.LRPInstances(actualLRPs,
		func(instance *cc_messages.LRPInstance, actual *receptor.ActualLRPResponse) {
			instance.Host = actual.Address
			instance.Port = getDefaultPort(actual.Ports)
			stats := metricsByInstanceIndex[uint(actual.Index)]
			instance.Stats = stats
		},
	)

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(w).Encode(instances)
	if err != nil {
		handler.logger.Error("stream-response-failed", err, lager.Data{"guid": guid})
	}
}

func getDefaultPort(mappings []receptor.PortMapping) uint16 {
	for _, mapping := range mappings {
		if mapping.ContainerPort == recipebuilder.DefaultPort {
			return mapping.HostPort
		}
	}

	return 0
}
