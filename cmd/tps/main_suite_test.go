package main_test

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudfoundry-incubator/consuladapter"
	receptorrunner "github.com/cloudfoundry-incubator/receptor/cmd/receptor/testrunner"
	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/tps/cmd/tps/testrunner"
	"github.com/cloudfoundry/storeadapter"
	"github.com/cloudfoundry/storeadapter/storerunner/etcdstorerunner"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/ghttp"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager/lagertest"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"

	"testing"
)

var (
	receptorPath string
	receptorPort int

	etcdPort int

	consulPort    int
	consulRunner  consuladapter.ClusterRunner
	consulAdapter consuladapter.Adapter

	tpsPort int
	tpsAddr string
	tps     ifrit.Process
	runner  *ginkgomon.Runner

	tpsPath string

	fakeCC         *ghttp.Server
	etcdRunner     *etcdstorerunner.ETCDClusterRunner
	receptorRunner ifrit.Process
	store          storeadapter.StoreAdapter
	bbs            *Bbs.BBS
	logger         *lagertest.TestLogger
)

func TestTPS(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TPS Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	tps, err := gexec.Build("github.com/cloudfoundry-incubator/tps/cmd/tps", "-race")
	Ω(err).ShouldNot(HaveOccurred())

	receptor, err := gexec.Build("github.com/cloudfoundry-incubator/receptor/cmd/receptor", "-race")
	Ω(err).ShouldNot(HaveOccurred())

	payload, err := json.Marshal(map[string]string{
		"tps":      tps,
		"receptor": receptor,
	})
	Ω(err).ShouldNot(HaveOccurred())

	return payload
}, func(payload []byte) {
	binaries := map[string]string{}

	err := json.Unmarshal(payload, &binaries)
	Ω(err).ShouldNot(HaveOccurred())

	etcdPort = 5001 + GinkgoParallelNode()
	receptorPort = 6001 + GinkgoParallelNode()
	tpsPort = 1518 + GinkgoParallelNode()

	etcdRunner = etcdstorerunner.NewETCDClusterRunner(etcdPort, 1)
	tpsPath = string(binaries["tps"])
	receptorPath = string(binaries["receptor"])
	store = etcdRunner.Adapter()

	consulPort = 9001 + config.GinkgoConfig.ParallelNode*consuladapter.PortOffsetLength
	consulRunner = consuladapter.NewClusterRunner(
		consulPort,
		1,
		"http",
	)

	logger = lagertest.NewTestLogger("test")
})

var _ = BeforeEach(func() {
	etcdRunner.Start()
	consulRunner.Start()

	bbs = Bbs.NewBBS(store, consulRunner.NewAdapter(), clock.NewClock(), logger)

	receptor := receptorrunner.New(receptorPath, receptorrunner.Args{
		Address:       fmt.Sprintf("127.0.0.1:%d", receptorPort),
		EtcdCluster:   strings.Join(etcdRunner.NodeURLS(), ","),
		ConsulCluster: strings.Join(consulRunner.Addresses(), ","),
	})
	receptor.StartCheck = "receptor.started"
	receptorRunner = ginkgomon.Invoke(receptor)

	fakeCC = ghttp.NewServer()

	tpsAddr = fmt.Sprintf("127.0.0.1:%d", uint16(tpsPort))

	runner = testrunner.New(
		string(tpsPath),
		tpsAddr,
		fmt.Sprintf("http://127.0.0.1:%d", receptorPort),
		fmt.Sprintf(fakeCC.URL()),
	)
})

var _ = AfterEach(func() {
	fakeCC.Close()
	ginkgomon.Kill(receptorRunner, 5)
	etcdRunner.Stop()
	consulRunner.Stop()
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	gexec.CleanupBuildArtifacts()
})
