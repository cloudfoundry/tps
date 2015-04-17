package handler

import (
	"net/http"

	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/tps"
	"github.com/cloudfoundry-incubator/tps/handler/lrpstats"
	"github.com/cloudfoundry-incubator/tps/handler/lrpstatus"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/rata"
)

func New(apiClient receptor.Client, noaaClient lrpstats.NoaaClient, maxInFlight int, logger lager.Logger) (http.Handler, error) {
	handlers := map[string]http.Handler{
		tps.LRPStatus: lrpstatus.NewHandler(apiClient, maxInFlight, logger),
		tps.LRPStats:  lrpstats.NewHandler(apiClient, noaaClient, logger),
	}

	return rata.NewRouter(tps.Routes, handlers)
}
