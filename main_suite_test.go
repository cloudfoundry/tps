package main_test

import (
	"fmt"
	"os"

	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/tps/integration/tpsrunner"
	"github.com/cloudfoundry/gunk/natsrunner"
	"github.com/cloudfoundry/gunk/timeprovider/faketimeprovider"
	"github.com/cloudfoundry/storeadapter"
	"github.com/cloudfoundry/storeadapter/storerunner/etcdstorerunner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/pivotal-golang/lager/lagertest"
	"github.com/tedsuo/ifrit"

	"testing"
	"time"
)

var store storeadapter.StoreAdapter
var bbs *Bbs.BBS
var timeProvider *faketimeprovider.FakeTimeProvider

var tpsAddr string
var tps ifrit.Process
var runner ifrit.Runner

var etcdRunner *etcdstorerunner.ETCDClusterRunner
var natsRunner *natsrunner.NATSRunner

var heartbeatInterval = 50 * time.Millisecond

var _ = SynchronizedBeforeSuite(func() []byte {
	tpsPath, err := gexec.Build("github.com/cloudfoundry-incubator/tps", "-race")
	Ω(err).ShouldNot(HaveOccurred())
	return []byte(tpsPath)
}, func(tpsPath []byte) {
	tpsAddr = fmt.Sprintf("127.0.0.1:%d", uint16(1518+GinkgoParallelNode()))
	etcdPort := 5001 + GinkgoParallelNode()
	natsPort := 4001 + GinkgoParallelNode()

	etcdRunner = etcdstorerunner.NewETCDClusterRunner(etcdPort, 1)

	store = etcdRunner.Adapter()
	timeProvider = faketimeprovider.New(time.Unix(0, 1138))
	bbs = Bbs.NewBBS(store, timeProvider, lagertest.NewTestLogger("test"))

	natsRunner = natsrunner.NewNATSRunner(natsPort)

	runner = tpsrunner.New(
		string(tpsPath),
		tpsAddr,
		[]string{fmt.Sprintf("http://127.0.0.1:%d", etcdPort)},
		[]string{fmt.Sprintf("127.0.0.1:%d", natsPort)},
		heartbeatInterval,
	)
})

func TestTPS(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TPS Suite")
}

var _ = BeforeEach(func() {
	startAll()
})

var _ = AfterEach(func() {
	stopAll()
})

var _ = SynchronizedAfterSuite(func() {
	stopAll()
}, func() {
	gexec.CleanupBuildArtifacts()
})

func startAll() {
	etcdRunner.Start()
	natsRunner.Start()
	tps = ifrit.Envoke(runner)
}

func stopAll() {
	if etcdRunner != nil {
		etcdRunner.Stop()
	}
	if natsRunner != nil {
		natsRunner.Stop()
	}
	if tps != nil {
		tps.Signal(os.Kill)
		Eventually(tps.Wait()).Should(Receive())
	}
}
