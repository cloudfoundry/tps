package tpsrunner

import (
	"encoding/json"
	"io/ioutil"
	"os/exec"

	"code.cloudfoundry.org/tps/config"
	. "github.com/onsi/gomega"

	"github.com/tedsuo/ifrit/ginkgomon"
)

func NewListener(bin, listenAddr, bbsAddress, trafficControllerURL, consulCluster string) *ginkgomon.Runner {
	configFile, err := ioutil.TempFile("", "listener_config")
	Expect(err).NotTo(HaveOccurred())

	listenerConfig := config.DefaultListenerConfig()
	listenerConfig.BBSAddress = bbsAddress
	listenerConfig.ListenAddress = listenAddr
	listenerConfig.LagerConfig.LogLevel = "debug"
	listenerConfig.ConsulCluster = consulCluster
	listenerConfig.TrafficControllerURL = trafficControllerURL

	listenerJSON, err := json.Marshal(listenerConfig)
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(configFile.Name(), listenerJSON, 0644)
	Expect(err).NotTo(HaveOccurred())

	return ginkgomon.New(ginkgomon.Config{
		Name:       "tps-listener",
		StartCheck: "tps-listener.started",
		Command: exec.Command(
			bin,
			"-configPath", configFile.Name(),
		),
	})
}

func NewWatcher(bin, bbsAddress, ccBaseURL, consulCluster string) *ginkgomon.Runner {
	return ginkgomon.New(ginkgomon.Config{
		Name: "tps-watcher",
		Command: exec.Command(
			bin,
			"-bbsAddress", bbsAddress,
			"-ccBaseURL", ccBaseURL,
			"-lockRetryInterval", "1s",
			"-consulCluster", consulCluster,
		),
		StartCheck: "tps-watcher.started",
	})
}
