package main

import (
	"flag"
	"net/http"
	"os"
	"strings"

	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/tps/handler"
	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/gunk/timeprovider"
	"github.com/cloudfoundry/storeadapter/etcdstoreadapter"
	"github.com/cloudfoundry/storeadapter/workerpool"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"
)

var etcdCluster = flag.String(
	"etcdCluster",
	"http://127.0.0.1:4001",
	"comma-separated list of etcd addresses (http://ip:port)",
)

var listenAddr = flag.String(
	"listenAddr",
	"0.0.0.0:1518", // p and s's offset in the alphabet, do not change
	"listening address of api server",
)

var syslogName = flag.String(
	"syslogName",
	"",
	"syslog name",
)

func main() {
	flag.Parse()

	logger := initializeLogger()
	bbs := initializeBbs(logger)
	apiHandler := initializeHandler(logger, bbs)

	process := grouper.EnvokeGroup(grouper.RunGroup{
		"api": http_server.New(*listenAddr, apiHandler),
	})

	monitor := ifrit.Envoke(sigmon.New(process))

	logger.Infof("tps.started")

	err := <-monitor.Wait()
	if err != nil {
		logger.Errord(map[string]interface{}{
			"error": err.Error(),
		}, "tps.exited")
		os.Exit(1)
	}

	logger.Info("tps.exited")
}

func initializeLogger() *gosteno.Logger {
	stenoConfig := &gosteno.Config{
		Sinks: []gosteno.Sink{
			gosteno.NewIOSink(os.Stdout),
		},
	}

	if *syslogName != "" {
		stenoConfig.Sinks = append(stenoConfig.Sinks, gosteno.NewSyslogSink(*syslogName))
	}

	gosteno.Init(stenoConfig)

	return gosteno.NewLogger("AppManager")
}

func initializeBbs(logger *gosteno.Logger) Bbs.TPSBBS {
	etcdAdapter := etcdstoreadapter.NewETCDStoreAdapter(
		strings.Split(*etcdCluster, ","),
		workerpool.NewWorkerPool(10),
	)

	err := etcdAdapter.Connect()
	if err != nil {
		logger.Fatalf("Error connecting to etcd: %s\n", err)
	}

	return Bbs.NewTPSBBS(etcdAdapter, timeprovider.NewTimeProvider(), logger)
}

func initializeHandler(logger *gosteno.Logger, bbs Bbs.TPSBBS) http.Handler {
	apiHandler, err := handler.New(bbs, logger)
	if err != nil {
		logger.Fatald(map[string]interface{}{
			"error": err.Error(),
		}, "tps.initialize-handler.failed")
	}

	return apiHandler
}
