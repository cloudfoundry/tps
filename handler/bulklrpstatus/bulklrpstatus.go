package bulklrpstatus

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/cloudfoundry-incubator/bbs"
	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/tps/handler/lrpstatus"
	"github.com/cloudfoundry/gunk/workpool"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"
)

const MAX_STAT_WORKPOOL_SIZE = 15

var processGuidPattern = regexp.MustCompile(`^([a-zA-Z0-9_-]+,)*[a-zA-Z0-9_-]+$`)

type handler struct {
	bbsClient bbs.Client
	clock     clock.Clock
	logger    lager.Logger
}

func NewHandler(bbsClient bbs.Client, clk clock.Clock, logger lager.Logger) http.Handler {
	return &handler{bbsClient: bbsClient, clock: clk, logger: logger}
}

func (handler *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	guidParameter := r.FormValue("guids")
	if !processGuidPattern.Match([]byte(guidParameter)) {
		handler.logger.Error("failed-parsing-guids", nil, lager.Data{"guid-parameter": guidParameter})
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	guids := strings.Split(guidParameter, ",")
	works := []func(){}

	statusBundle := make(map[string][]cc_messages.LRPInstance)
	statusLock := sync.Mutex{}

	for _, processGuid := range guids {
		works = append(works, handler.getStatusForLRPWorkFunction(processGuid, &statusLock, statusBundle))
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

	err = json.NewEncoder(w).Encode(statusBundle)
	if err != nil {
		handler.logger.Error("stream-response-failed", err, nil)
	}
}

func (handler *handler) getStatusForLRPWorkFunction(processGuid string, statusLock *sync.Mutex, statusBundle map[string][]cc_messages.LRPInstance) func() {
	return func() {
		logger := handler.logger.Session("fetch-lrp-instances", lager.Data{"process-guid": processGuid})
		logger.Info("start")
		defer logger.Info("complete")
		actualLRPGroups, err := handler.bbsClient.ActualLRPGroupsByProcessGuid(processGuid)
		if err != nil {
			logger.Error("fetching-actual-lrps-info-failed", err)
			return
		}

		instances := lrpstatus.LRPInstances(actualLRPGroups,
			func(instance *cc_messages.LRPInstance, actual *models.ActualLRP) {
				instance.Details = actual.PlacementError
			},
			handler.clock,
		)

		statusLock.Lock()
		statusBundle[processGuid] = instances
		statusLock.Unlock()
	}
}
