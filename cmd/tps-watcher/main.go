package main

import (
	"flag"
	"net/url"
	"os"

	"github.com/cloudfoundry-incubator/bbs"
	"github.com/cloudfoundry-incubator/cf-debug-server"
	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/consuladapter"
	"github.com/cloudfoundry-incubator/locket"
	"github.com/cloudfoundry-incubator/tps/cc_client"
	"github.com/cloudfoundry-incubator/tps/watcher"
	"github.com/cloudfoundry/dropsonde"
	"github.com/nu7hatch/gouuid"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/sigmon"
)

var bbsAddress = flag.String(
	"bbsAddress",
	"",
	"Address to the BBS Server",
)

var consulCluster = flag.String(
	"consulCluster",
	"",
	"comma-separated list of consul server URLs (scheme://ip:port)",
)

var lockTTL = flag.Duration(
	"lockTTL",
	locket.LockTTL,
	"TTL for service lock",
)

var lockRetryInterval = flag.Duration(
	"lockRetryInterval",
	locket.RetryInterval,
	"interval to wait before retrying a failed lock acquisition",
)

var ccBaseURL = flag.String(
	"ccBaseURL",
	"",
	"URI to acccess the Cloud Controller",
)

var ccUsername = flag.String(
	"ccUsername",
	"",
	"Basic auth username for CC internal API",
)

var ccPassword = flag.String(
	"ccPassword",
	"",
	"Basic auth password for CC internal API",
)

var skipCertVerify = flag.Bool(
	"skipCertVerify",
	false,
	"skip SSL certificate verification",
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

const (
	dropsondeOrigin      = "tps_watcher"
	dropsondeDestination = "localhost:3457"
)

func main() {
	cf_debug_server.AddFlags(flag.CommandLine)
	cf_lager.AddFlags(flag.CommandLine)
	flag.Parse()

	logger, reconfigurableSink := cf_lager.New("tps-watcher")
	initializeDropsonde(logger)

	lockMaintainer := initializeLockMaintainer(logger)

	ccClient := cc_client.NewCcClient(*ccBaseURL, *ccUsername, *ccPassword, *skipCertVerify)

	watcher := ifrit.RunFunc(func(signals <-chan os.Signal, ready chan<- struct{}) error {
		w, err := watcher.NewWatcher(logger, initializeBBSClient(logger), ccClient)
		if err != nil {
			return err
		}

		return w.Run(signals, ready)
	})

	members := grouper.Members{
		{"lock-maintainer", lockMaintainer},
		{"watcher", watcher},
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

func initializeLocket(logger lager.Logger) locket.Client {
	client, err := consuladapter.NewClient(*consulCluster)
	if err != nil {
		logger.Fatal("new-client-failed", err)
	}

	sessionMgr := consuladapter.NewSessionManager(client)
	consulSession, err := consuladapter.NewSession("tps-watcher", *lockTTL, client, sessionMgr)
	if err != nil {
		logger.Fatal("consul-session-failed", err)
	}

	return locket.NewClient(consulSession, clock.NewClock(), logger)
}

func initializeLockMaintainer(logger lager.Logger) ifrit.Runner {
	locketClient := initializeLocket(logger)

	uuid, err := uuid.NewV4()
	if err != nil {
		logger.Fatal("Couldn't generate uuid", err)
	}

	return locketClient.NewTpsWatcherLock(uuid.String(), *lockRetryInterval)
}

func initializeBBSClient(logger lager.Logger) bbs.Client {
	bbsURL, err := url.Parse(*bbsAddress)
	if err != nil {
		logger.Fatal("Invalid BBS URL", err)
	}

	if bbsURL.Scheme != "https" {
		return bbs.NewClient(*bbsAddress)
	}

	bbsClient, err := bbs.NewSecureClient(*bbsAddress, *bbsCACert, *bbsClientCert, *bbsClientKey)
	if err != nil {
		logger.Fatal("Failed to configure secure BBS client", err)
	}
	return bbsClient
}
