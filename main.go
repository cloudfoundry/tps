package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cloudfoundry-incubator/cf-debug-server"
	"github.com/cloudfoundry-incubator/cf-lager"
	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/tps/handler"
	"github.com/cloudfoundry-incubator/tps/heartbeat"
	"github.com/cloudfoundry/dropsonde/autowire"
	"github.com/cloudfoundry/gunk/group_runner"
	"github.com/cloudfoundry/gunk/natsclientrunner"
	"github.com/cloudfoundry/gunk/timeprovider"
	"github.com/cloudfoundry/storeadapter/etcdstoreadapter"
	"github.com/cloudfoundry/storeadapter/workerpool"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"
)

var listenAddr = flag.String(
	"listenAddr",
	"0.0.0.0:1518", // p and s's offset in the alphabet, do not change
	"listening address of api server",
)

var etcdCluster = flag.String(
	"etcdCluster",
	"http://127.0.0.1:4001",
	"comma-separated list of etcd addresses (http://ip:port)",
)

var natsAddresses = flag.String(
	"natsAddresses",
	"127.0.0.1:4222",
	"comma-separated list of NATS addresses (ip:port)",
)

var natsUsername = flag.String(
	"natsUsername",
	"nats",
	"Username to connect to nats",
)

var natsPassword = flag.String(
	"natsPassword",
	"nats",
	"Password for nats user",
)

var heartbeatInterval = flag.Duration(
	"heartbeatInterval",
	60*time.Second,
	"the interval, in seconds, between heartbeats for maintaining presence",
)

func main() {
	flag.Parse()

	logger := cf_lager.New("tps")
	bbs := initializeBbs(logger)
	apiHandler := initializeHandler(logger, bbs)

	natsClient := natsclientrunner.NewClient(*natsAddresses, *natsUsername, *natsPassword)
	cf_debug_server.Run()

	group := group_runner.New([]group_runner.Member{
		{"natsClient", natsclientrunner.New(natsClient, logger)},
		{"heartbeat", heartbeat.New(
			natsClient,
			*heartbeatInterval,
			fmt.Sprintf("http://%s", *listenAddr),
			logger)},
		{"api", http_server.New(*listenAddr, apiHandler)},
	})

	monitor := ifrit.Envoke(sigmon.New(group))

	logger.Info("started")

	err := <-monitor.Wait()
	if err != nil {
		logger.Error("exited", err)
		os.Exit(1)
	}

	logger.Info("exited")
	os.Exit(0)
}

func initializeBbs(logger lager.Logger) Bbs.TPSBBS {
	etcdAdapter := etcdstoreadapter.NewETCDStoreAdapter(
		strings.Split(*etcdCluster, ","),
		workerpool.NewWorkerPool(10),
	)

	err := etcdAdapter.Connect()
	if err != nil {
		logger.Fatal("failed-to-connect-to-etcd", err)
	}

	return Bbs.NewTPSBBS(etcdAdapter, timeprovider.NewTimeProvider(), logger)
}

func initializeHandler(logger lager.Logger, bbs Bbs.TPSBBS) http.Handler {
	apiHandler, err := handler.New(bbs, logger)
	if err != nil {
		logger.Fatal("initialize-handler.failed", err)
	}

	return autowire.InstrumentedHandler(apiHandler)
}
