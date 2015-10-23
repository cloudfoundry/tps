package main

import (
	"crypto/tls"
	"flag"
	"net/http"
	"net/url"
	"os"

	"github.com/cloudfoundry-incubator/bbs"
	"github.com/cloudfoundry-incubator/cf-debug-server"
	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/tps/handler"
	"github.com/cloudfoundry/dropsonde"
	"github.com/cloudfoundry/noaa"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"
)

var listenAddr = flag.String(
	"listenAddr",
	"0.0.0.0:1518", // p and s's offset in the alphabet, do not change
	"listening address of api server",
)

var bbsAddress = flag.String(
	"bbsAddress",
	"",
	"Address to the BBS Server",
)

var trafficControllerURL = flag.String(
	"trafficControllerURL",
	"",
	"URL of TrafficController",
)

var skipSSLVerification = flag.Bool(
	"skipSSLVerification",
	true,
	"Skip SSL verification",
)

var maxInFlightRequests = flag.Int(
	"maxInFlightRequests",
	200,
	"number of requests to handle at a time; any more will receive 503",
)

var bbsCACert = flag.String(
	"bbsCACert",
	"",
	"path to certificate authority cert used for mutually authenticated TLS BBS communication",
)

var bbsClientCert = flag.String(
	"bbsClientCert",
	"",
	"path to client cert used for mutually authenticated TLS BBS communication",
)

var bbsClientKey = flag.String(
	"bbsClientKey",
	"",
	"path to client key used for mutually authenticated TLS BBS communication",
)

var bbsClientSessionCacheSize = flag.Int(
	"bbsClientSessionCacheSize",
	0,
	"Capacity of the ClientSessionCache option on the TLS configuration. If zero, golang's default will be used",
)

var bbsMaxIdleConnsPerHost = flag.Int(
	"bbsMaxIdleConnsPerHost",
	0,
	"Controls the maximum number of idle (keep-alive) connctions per host. If zero, golang's default will be used",
)

var bulkLRPStatusWorkers = flag.Int(
	"bulkLRPStatusWorkers",
	15,
	"Max concurrency for fetching bulk lrps",
)

const (
	dropsondeDestination = "localhost:3457"
	dropsondeOrigin      = "tps_listener"
)

func main() {
	cf_debug_server.AddFlags(flag.CommandLine)
	cf_lager.AddFlags(flag.CommandLine)
	flag.Parse()

	logger, reconfigurableSink := cf_lager.New("tps-listener")
	initializeDropsonde(logger)
	noaaClient := noaa.NewConsumer(*trafficControllerURL, &tls.Config{InsecureSkipVerify: *skipSSLVerification}, nil)
	defer noaaClient.Close()
	apiHandler := initializeHandler(logger, noaaClient, *maxInFlightRequests, initializeBBSClient(logger))

	members := grouper.Members{
		{"api", http_server.New(*listenAddr, apiHandler)},
	}

	if dbgAddr := cf_debug_server.DebugAddress(flag.CommandLine); dbgAddr != "" {
		members = append(grouper.Members{
			{"debug-server", cf_debug_server.Runner(dbgAddr, reconfigurableSink)},
		}, members...)
	}

	group := grouper.NewOrdered(os.Interrupt, members)

	monitor := ifrit.Invoke(sigmon.New(group))

	logger.Info("started")

	err := <-monitor.Wait()
	if err != nil {
		logger.Error("exited-with-failure", err)
		os.Exit(1)
	}

	logger.Info("exited")
}

func initializeDropsonde(logger lager.Logger) {
	err := dropsonde.Initialize(dropsondeDestination, dropsondeOrigin)
	if err != nil {
		logger.Error("failed to initialize dropsonde: %v", err)
	}
}

func initializeHandler(logger lager.Logger, noaaClient *noaa.Consumer, maxInFlight int, apiClient bbs.Client) http.Handler {
	apiHandler, err := handler.New(apiClient, noaaClient, maxInFlight, *bulkLRPStatusWorkers, logger)
	if err != nil {
		logger.Fatal("initialize-handler.failed", err)
	}

	return apiHandler
}

func initializeBBSClient(logger lager.Logger) bbs.Client {
	bbsURL, err := url.Parse(*bbsAddress)
	if err != nil {
		logger.Fatal("Invalid BBS URL", err)
	}

	if bbsURL.Scheme != "https" {
		return bbs.NewClient(*bbsAddress)
	}

	bbsClient, err := bbs.NewSecureClient(*bbsAddress, *bbsCACert, *bbsClientCert, *bbsClientKey, *bbsClientSessionCacheSize, *bbsMaxIdleConnsPerHost)
	if err != nil {
		logger.Fatal("Failed to configure secure BBS client", err)
	}
	return bbsClient
}
