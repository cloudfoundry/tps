package main_test

import (
	"fmt"

	"github.com/cloudfoundry-incubator/tps/cmd/tps/testrunner"
	"github.com/cloudfoundry/gunk/diegonats"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/ghttp"
	"github.com/pivotal-golang/clock/fakeclock"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"

	"testing"
	"time"
)

var clock *fakeclock.FakeClock

var tpsAddr string
var tps ifrit.Process
var runner *ginkgomon.Runner

var natsPort int
var gnatsdRunner ifrit.Process
var natsClient diegonats.NATSClient
var receptorServer *ghttp.Server

var heartbeatInterval = 50 * time.Millisecond
var tpsBinPath string

var _ = SynchronizedBeforeSuite(func() []byte {
	synchronizedTpsBinPath, err := gexec.Build("github.com/cloudfoundry-incubator/tps/cmd/tps", "-race")
	Ω(err).ShouldNot(HaveOccurred())
	return []byte(synchronizedTpsBinPath)
}, func(synchronizedTpsBinPath []byte) {
	tpsBinPath = string(synchronizedTpsBinPath)
})

func TestTPS(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TPS Suite")
}

var _ = BeforeEach(func() {
	tpsAddr = fmt.Sprintf("127.0.0.1:%d", uint16(1518+GinkgoParallelNode()))
	natsPort = 4001 + GinkgoParallelNode()

	clock = fakeclock.NewFakeClock(time.Unix(0, 1138))
	receptorServer = ghttp.NewServer()

	runner = testrunner.New(
		string(tpsBinPath),
		tpsAddr,
		receptorServer.URL(),
		[]string{fmt.Sprintf("127.0.0.1:%d", natsPort)},
		heartbeatInterval,
	)

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
	gnatsdRunner, natsClient = diegonats.StartGnatsd(natsPort)
}

func stopAll() {
	ginkgomon.Kill(gnatsdRunner)
}
