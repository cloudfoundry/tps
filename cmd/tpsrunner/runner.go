package tpsrunner

import (
	"os/exec"

	"github.com/tedsuo/ifrit/ginkgomon"
)

func NewListener(bin, listenAddr, bbsAddress, trafficControllerURL string) *ginkgomon.Runner {
	return ginkgomon.New(ginkgomon.Config{
		Name: "tps-listener",
		Command: exec.Command(
			bin,
			"-bbsAddress", bbsAddress,
			"-listenAddr", listenAddr,
			"-trafficControllerURL", trafficControllerURL,
			"-skipSSLVerification", "true",
		),
		StartCheck: "tps-listener.started",
	})
}

func NewWatcher(bin, diegoAPIURL, ccBaseURL, consulCluster string) *ginkgomon.Runner {
	return ginkgomon.New(ginkgomon.Config{
		Name: "tps-watcher",
		Command: exec.Command(
			bin,
			"-diegoAPIURL", diegoAPIURL,
			"-ccBaseURL", ccBaseURL,
			"-lockRetryInterval", "1s",
			"-consulCluster", consulCluster,
		),
		StartCheck: "tps-watcher.started",
	})
}
