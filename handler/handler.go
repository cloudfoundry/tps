package handler

import (
	"net/http"

	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/tps/api"
	"github.com/cloudfoundry-incubator/tps/handler/lrpstatus"
	"github.com/cloudfoundry/gosteno"
	"github.com/tedsuo/rata"
)

func New(bbs Bbs.TPSBBS, logger *gosteno.Logger) (http.Handler, error) {
	handlers := map[string]http.Handler{
		api.LRPStatus: lrpstatus.NewHandler(bbs, logger),
	}

	return rata.NewRouter(api.Routes, handlers)
}
