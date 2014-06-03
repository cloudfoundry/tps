package main_test

import (
	"fmt"

	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/tps/integration/tpsrunner"
	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/gunk/timeprovider/faketimeprovider"
	"github.com/cloudfoundry/storeadapter"
	"github.com/cloudfoundry/storeadapter/storerunner/etcdstorerunner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"

	"testing"
	"time"
)

var runner ifrit.Runner
var tpsPort uint16

var etcdRunner *etcdstorerunner.ETCDClusterRunner
var store storeadapter.StoreAdapter
var bbs *Bbs.BBS
var timeProvider *faketimeprovider.FakeTimeProvider

var _ = BeforeEach(func() {
	tpsPath, err := gexec.Build("github.com/cloudfoundry-incubator/tps", "-race")
	Ω(err).ShouldNot(HaveOccurred())

	etcdPort := 5001 + GinkgoParallelNode()

	etcdRunner = etcdstorerunner.NewETCDClusterRunner(etcdPort, 1)

	store = etcdRunner.Adapter()

	logSink := gosteno.NewTestingSink()
	gosteno.Init(&gosteno.Config{
		Sinks: []gosteno.Sink{logSink},
	})
	logger := gosteno.NewLogger("the-logger")
	gosteno.EnterTestMode()

	timeProvider = faketimeprovider.New(time.Unix(0, 1138))
	bbs = Bbs.NewBBS(store, timeProvider, logger)

	tpsPort = uint16(1518 + GinkgoParallelNode())

	runner = tpsrunner.New(
		tpsPath,
		tpsPort,
		[]string{fmt.Sprintf("http://127.0.0.1:%d", etcdPort)},
	)
})

func TestTPS(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TPS Suite")
}
