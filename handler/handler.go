package handler

import (
	"net/http"

	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/tps/api"
	"github.com/cloudfoundry-incubator/tps/handler/lrpstatus"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/rata"
)

func New(bbs Bbs.TPSBBS, maxInFlight int, logger lager.Logger) (http.Handler, error) {
	handlers := map[string]http.Handler{
		api.LRPStatus: lrpstatus.NewHandler(bbs, maxInFlight, logger),
	}

	return rata.NewRouter(api.Routes, handlers)
}
