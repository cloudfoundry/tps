package main_test

import (
	"code.cloudfoundry.org/diego-logging-client/testhelpers"
	"code.cloudfoundry.org/lager/v3/lagertest"
	"code.cloudfoundry.org/tps/cmd/tpsrunner"
	"encoding/json"
	_ "github.com/lib/pq"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
	"testing"
	"time"

	tpsconfig "code.cloudfoundry.org/tps/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/ghttp"
)

var (
	watcher           ifrit.Process
	runner            *ginkgomon.Runner
	disableStartCheck bool

	watcherPath string

	watcherConfig tpsconfig.WatcherConfig

	fakeCC  *ghttp.Server
	fakeBBS *ghttp.Server
	logger  *lagertest.TestLogger

	testIngressServer                                       *testhelpers.TestIngressServer
	metronCAFile, metronServerCertFile, metronServerKeyFile string
)

func TestTPS(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TPS-Watcher Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	tps, err := gexec.Build("../tps-watcher", "-race")
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

	watcherPath = binaries["watcher"]

	logger = lagertest.NewTestLogger("test")
})

var _ = BeforeEach(func() {
	fakeCC = ghttp.NewServer()
	fakeBBS = ghttp.NewServer()

	watcherConfig = tpsconfig.DefaultWatcherConfig()
	watcherConfig.BBSAddress = fakeBBS.URL()
	watcherConfig.CCBaseUrl = fakeCC.URL()
	watcherConfig.LagerConfig.LogLevel = "debug"
	watcherConfig.CCClientCert = "../../fixtures/watcher_cc_client.crt"
	watcherConfig.CCClientKey = "../../fixtures/watcher_cc_client.key"
	watcherConfig.CCCACert = "../../fixtures/watcher_cc_ca.crt"

	disableStartCheck = false

	var err error
	metronCAFile = "../../fixtures/metron_ca.crt"
	metronServerCertFile = "../../fixtures/metron_client.crt"
	metronServerKeyFile = "../../fixtures/metron_client.key"
	testIngressServer, _ = testhelpers.NewTestIngressServer(metronServerCertFile, metronServerKeyFile, metronCAFile)
	Expect(err).NotTo(HaveOccurred())
	_ = testIngressServer.Start()
})

var _ = JustBeforeEach(func() {
	runner = tpsrunner.NewWatcher(watcherPath, watcherConfig)
	if disableStartCheck {
		runner.StartCheck = ""
	}
	watcher = ginkgomon.Invoke(runner)
	time.Sleep(1 * time.Second)
})

var _ = AfterEach(func() {
	fakeCC.Close()
	fakeBBS.Close()
	testIngressServer.Stop()
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	gexec.CleanupBuildArtifacts()
})
