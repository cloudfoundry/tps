package main_test

import (
	"encoding/json"
	"fmt"

	"github.com/cloudfoundry-incubator/consuladapter/consulrunner"
	"github.com/cloudfoundry-incubator/tps/cmd/tpsrunner"
	"github.com/pivotal-golang/lager/lagertest"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/ghttp"

	"testing"
)

var (
	consulRunner *consulrunner.ClusterRunner

	watcher ifrit.Process
	runner  *ginkgomon.Runner

	watcherPath string

	fakeCC  *ghttp.Server
	fakeBBS *ghttp.Server
	logger  *lagertest.TestLogger
)

func TestTPS(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TPS-Watcher Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	tps, err := gexec.Build("github.com/cloudfoundry-incubator/tps/cmd/tps-watcher", "-race")
	Expect(err).NotTo(HaveOccurred())

	payload, err := json.Marshal(map[string]string{
		"watcher": tps,
	})
	Expect(err).NotTo(HaveOccurred())

	return payload
}, func(payload []byte) {
	binaries := map[string]string{}

	err := json.Unmarshal(payload, &binaries)
	Expect(err).NotTo(HaveOccurred())

	watcherPath = string(binaries["watcher"])

	consulRunner = consulrunner.NewClusterRunner(
		9001+config.GinkgoConfig.ParallelNode*consulrunner.PortOffsetLength,
		1,
		"http",
	)

	logger = lagertest.NewTestLogger("test")
})

var _ = BeforeEach(func() {
	consulRunner.Start()
	consulRunner.WaitUntilReady()

	fakeCC = ghttp.NewServer()
	fakeBBS = ghttp.NewServer()

	runner = tpsrunner.NewWatcher(
		string(watcherPath),
		fakeBBS.URL(),
		fmt.Sprintf(fakeCC.URL()),
		consulRunner.ConsulCluster(),
	)
})

var _ = AfterEach(func() {
	fakeCC.Close()
	fakeBBS.Close()
	consulRunner.Stop()
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	gexec.CleanupBuildArtifacts()
})
