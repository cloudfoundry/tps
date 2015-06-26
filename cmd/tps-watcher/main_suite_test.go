package main_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	bbstestrunner "github.com/cloudfoundry-incubator/bbs/cmd/bbs/testrunner"
	"github.com/cloudfoundry-incubator/consuladapter/consulrunner"
	receptorrunner "github.com/cloudfoundry-incubator/receptor/cmd/receptor/testrunner"
	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/tps/cmd/tpsrunner"
	"github.com/cloudfoundry/storeadapter"
	"github.com/cloudfoundry/storeadapter/storerunner/etcdstorerunner"
	"github.com/pivotal-golang/clock"
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
	receptorPath string
	receptorPort int

	etcdPort int

	consulRunner *consulrunner.ClusterRunner

	watcher ifrit.Process
	runner  *ginkgomon.Runner

	watcherPath string

	fakeCC         *ghttp.Server
	etcdRunner     *etcdstorerunner.ETCDClusterRunner
	receptorRunner ifrit.Process
	store          storeadapter.StoreAdapter
	bbs            *Bbs.BBS
	logger         *lagertest.TestLogger
	bbsPath        string
	bbsURL         *url.URL
)

var bbsArgs bbstestrunner.Args
var bbsRunner *ginkgomon.Runner
var bbsProcess ifrit.Process

func TestTPS(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TPS-Watcher Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	tps, err := gexec.Build("github.com/cloudfoundry-incubator/tps/cmd/tps-watcher", "-race")
	Expect(err).NotTo(HaveOccurred())

	receptor, err := gexec.Build("github.com/cloudfoundry-incubator/receptor/cmd/receptor", "-race")
	Expect(err).NotTo(HaveOccurred())

	bbs, err := gexec.Build("github.com/cloudfoundry-incubator/bbs/cmd/bbs", "-race")
	Expect(err).NotTo(HaveOccurred())

	payload, err := json.Marshal(map[string]string{
		"watcher":  tps,
		"receptor": receptor,
		"bbs":      bbs,
	})
	Expect(err).NotTo(HaveOccurred())

	return payload
}, func(payload []byte) {
	binaries := map[string]string{}

	err := json.Unmarshal(payload, &binaries)
	Expect(err).NotTo(HaveOccurred())

	etcdPort = 5001 + GinkgoParallelNode()
	receptorPort = 6001 + GinkgoParallelNode()*2

	etcdRunner = etcdstorerunner.NewETCDClusterRunner(etcdPort, 1, nil)

	watcherPath = string(binaries["watcher"])
	receptorPath = string(binaries["receptor"])
	store = etcdRunner.Adapter(nil)

	consulRunner = consulrunner.NewClusterRunner(
		9001+config.GinkgoConfig.ParallelNode*consulrunner.PortOffsetLength,
		1,
		"http",
	)

	logger = lagertest.NewTestLogger("test")

	bbsPath = string(binaries["bbs"])
	bbsAddress := fmt.Sprintf("127.0.0.1:%d", 13000+GinkgoParallelNode())

	bbsURL = &url.URL{
		Scheme: "http",
		Host:   bbsAddress,
	}

	bbsArgs = bbstestrunner.Args{
		Address:     bbsAddress,
		EtcdCluster: strings.Join(etcdRunner.NodeURLS(), ","),
	}
})

var _ = BeforeEach(func() {
	etcdRunner.Start()

	consulRunner.Start()
	consulRunner.WaitUntilReady()

	bbsRunner = bbstestrunner.New(bbsPath, bbsArgs)
	bbsProcess = ginkgomon.Invoke(bbsRunner)

	taskHandlerAddress := fmt.Sprintf("127.0.0.1:%d", receptorPort+1)
	bbs = Bbs.NewBBS(store, consulRunner.NewSession("a-session"), "http://"+taskHandlerAddress, clock.NewClock(), logger)

	receptor := receptorrunner.New(receptorPath, receptorrunner.Args{
		Address:            fmt.Sprintf("127.0.0.1:%d", receptorPort),
		BBSAddress:         bbsURL.String(),
		TaskHandlerAddress: taskHandlerAddress,
		EtcdCluster:        strings.Join(etcdRunner.NodeURLS(), ","),
		ConsulCluster:      consulRunner.ConsulCluster(),
	})
	receptorRunner = ginkgomon.Invoke(receptor)

	fakeCC = ghttp.NewServer()

	runner = tpsrunner.NewWatcher(
		string(watcherPath),
		fmt.Sprintf("http://127.0.0.1:%d", receptorPort),
		fmt.Sprintf(fakeCC.URL()),
		consulRunner.ConsulCluster(),
	)
})

var _ = AfterEach(func() {
	ginkgomon.Kill(bbsProcess)
	fakeCC.Close()
	ginkgomon.Kill(receptorRunner, 5)
	etcdRunner.Stop()
	consulRunner.Stop()
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	gexec.CleanupBuildArtifacts()
})
