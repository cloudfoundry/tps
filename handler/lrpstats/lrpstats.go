package lrpstats

import (
	"encoding/json"
	"net/http"

	"github.com/cloudfoundry-incubator/bbs"
	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/nsync/recipebuilder"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/tps/handler/lrpstatus"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter -o fakes/fake_noaaclient.go . NoaaClient
type NoaaClient interface {
	ContainerMetrics(appGuid string, authToken string) ([]*events.ContainerMetric, error)
	Close() error
}

type handler struct {
	bbsClient  bbs.Client
	noaaClient NoaaClient
	clock      clock.Clock
	logger     lager.Logger
}

func NewHandler(bbsClient bbs.Client, noaaClient NoaaClient, clk clock.Clock, logger lager.Logger) http.Handler {
	return &handler{bbsClient: bbsClient, noaaClient: noaaClient, clock: clk, logger: logger}
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

	desiredLRP, err := handler.bbsClient.DesiredLRPByProcessGuid(guid)
	if err != nil {
		handler.logger.Error("fetching-desired-lrp-failed", err, lager.Data{"ProcessGuid": guid})
		switch models.ConvertError(err).Type {
		case models.Error_ResourceNotFound:
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	actualLRPs, err := handler.bbsClient.ActualLRPGroupsByProcessGuid(guid)
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
	currentTime := handler.clock.Now()
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
		func(instance *cc_messages.LRPInstance, actual *models.ActualLRP) {
			instance.Host = actual.Address
			instance.Port = getDefaultPort(actual.Ports)
			stats := metricsByInstanceIndex[uint(actual.Index)]
			instance.Stats = stats
		},
		handler.clock,
	)

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(w).Encode(instances)
	if err != nil {
		handler.logger.Error("stream-response-failed", err, lager.Data{"guid": guid})
	}
}

func getDefaultPort(mappings []*models.PortMapping) uint16 {
	for _, mapping := range mappings {
		if mapping.ContainerPort == recipebuilder.DefaultPort {
			return uint16(mapping.HostPort)
		}
	}

	return 0
}
