package bulklrpstats

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/cloudfoundry-incubator/nsync/recipebuilder"
	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/tps/handler/lrpstatus"
	"github.com/cloudfoundry/gunk/workpool"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"
)

const MAX_STAT_WORKPOOL_SIZE = 15

var processGuidPattern = regexp.MustCompile(`^([a-zA-Z0-9_-]+,)*[a-zA-Z0-9_-]+$`)

//go:generate counterfeiter -o fakes/fake_noaaclient.go . NoaaClient
type NoaaClient interface {
	ContainerMetrics(appGuid string, authToken string) ([]*events.ContainerMetric, error)
	Close() error
}

type handler struct {
	receptorClient receptor.Client
	noaaClient     NoaaClient
	clock          clock.Clock
	logger         lager.Logger
}

func NewHandler(receptorClient receptor.Client, noaaClient NoaaClient, clk clock.Clock, logger lager.Logger) http.Handler {
	return &handler{receptorClient: receptorClient, noaaClient: noaaClient, clock: clk, logger: logger}
}

func (handler *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	authorization := r.Header.Get("Authorization")
	if authorization == "" {
		handler.logger.Error("failed-authorization", nil)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	guidParameter := r.FormValue("guids")
	if !processGuidPattern.Match([]byte(guidParameter)) {
		handler.logger.Error("failed-parsing-guids", nil, lager.Data{"guid-parameter": guidParameter})
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	guids := strings.Split(guidParameter, ",")
	works := []func(){}

	statsBundle := make(map[string][]cc_messages.LRPInstance)
	statsLock := sync.Mutex{}

	for _, processGuid := range guids {
		works = append(works, handler.getStatsForLRPWorkFunction(processGuid, authorization, &statsLock, statsBundle))
	}

	throttler, err := workpool.NewThrottler(MAX_STAT_WORKPOOL_SIZE, works)
	if err != nil {
		handler.logger.Error("failed-constructing-throttler", err, lager.Data{"max-workers": MAX_STAT_WORKPOOL_SIZE, "num-works": len(works)})
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	throttler.Work()

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(w).Encode(statsBundle)
	if err != nil {
		handler.logger.Error("stream-response-failed", err, nil)
	}
}

func (handler *handler) getStatsForLRPWorkFunction(processGuid string, authorization string, statsLock *sync.Mutex, statsBundle map[string][]cc_messages.LRPInstance) func() {
	return func() {
		desiredLRP, err := handler.receptorClient.GetDesiredLRP(processGuid)

		if err != nil {
			handler.logger.Error("fetching-a-desired-lrp-failed", err, lager.Data{"ProcessGuid": processGuid})
			return
		}

		actualLRPs, err := handler.receptorClient.ActualLRPsByProcessGuid(desiredLRP.ProcessGuid)
		if err != nil {
			handler.logger.Error("fetching-actual-lrps-info-failed", err, lager.Data{"ProcessGuid": processGuid})
			return
		}

		metrics, err := handler.noaaClient.ContainerMetrics(desiredLRP.LogGuid, authorization)
		if err != nil {
			handler.logger.Error("getting-container-metrics-failed", err, lager.Data{
				"ProcessGuid": desiredLRP.ProcessGuid,
				"LogGuid":     desiredLRP.LogGuid,
			})
		}

		metricsByInstanceIndex := make(map[uint]*cc_messages.LRPInstanceStats)
		currentTime := handler.clock.Now()
		for _, metric := range metrics {
			cpuPercentageAsDecimal := metric.GetCpuPercentage() / 100

			disk := uint64(0)
			if metric.GetDiskBytes() > 1024*1024 {
				disk = metric.GetDiskBytes() - 1024*1024
			}

			metricsByInstanceIndex[uint(metric.GetInstanceIndex())] = &cc_messages.LRPInstanceStats{
				Time:          currentTime,
				CpuPercentage: cpuPercentageAsDecimal,
				MemoryBytes:   metric.GetMemoryBytes(),
				DiskBytes:     disk,
			}
		}

		instances := lrpstatus.LRPInstances(actualLRPs,
			func(instance *cc_messages.LRPInstance, actual *receptor.ActualLRPResponse) {
				instance.Host = actual.Address
				instance.Port = getDefaultPort(actual.Ports)
				stats := metricsByInstanceIndex[uint(actual.Index)]
				instance.Stats = stats
			},
			handler.clock,
		)

		statsLock.Lock()
		statsBundle[desiredLRP.ProcessGuid] = instances
		statsLock.Unlock()
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
