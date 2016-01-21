package main_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/cloudfoundry-incubator/bbs"
	bbstestrunner "github.com/cloudfoundry-incubator/bbs/cmd/bbs/testrunner"
	"github.com/cloudfoundry-incubator/consuladapter/consulrunner"
	"github.com/cloudfoundry-incubator/tps/cmd/tpsrunner"
	"github.com/cloudfoundry/storeadapter/storerunner/etcdstorerunner"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/ghttp"
	"github.com/pivotal-golang/lager/lagertest"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"

	"testing"
)

var (
	trafficControllerAddress string
	trafficControllerPort    int
	trafficControllerURL     string

	etcdPort int

	consulRunner *consulrunner.ClusterRunner

	listenerPort int
	listenerAddr string
	listener     ifrit.Process
	runner       *ginkgomon.Runner

	listenerPath string

	fakeCC     *ghttp.Server
	etcdRunner *etcdstorerunner.ETCDClusterRunner

	bbsClient bbs.Client
	logger    *lagertest.TestLogger
	bbsPath   string
	bbsURL    *url.URL

	bbsArgs          bbstestrunner.Args
	bbsRunner        *ginkgomon.Runner
	bbsProcess       ifrit.Process
	auctioneerServer *ghttp.Server
)

func TestTPS(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TPS-Listener Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	tps, err := gexec.Build("github.com/cloudfoundry-incubator/tps/cmd/tps-listener", "-race")
	Expect(err).NotTo(HaveOccurred())

	bbs, err := gexec.Build("github.com/cloudfoundry-incubator/bbs/cmd/bbs", "-race")
	Expect(err).NotTo(HaveOccurred())

	payload, err := json.Marshal(map[string]string{
		"listener": tps,
		"bbs":      bbs,
	})
	Expect(err).NotTo(HaveOccurred())

	return payload
}, func(payload []byte) {
	binaries := map[string]string{}

	err := json.Unmarshal(payload, &binaries)
	Expect(err).NotTo(HaveOccurred())

	etcdPort = 5001 + GinkgoParallelNode()
	listenerPort = 1518 + GinkgoParallelNode()

	trafficControllerPort = 7001 + GinkgoParallelNode()*2
	trafficControllerAddress = fmt.Sprintf("127.0.0.1:%d", trafficControllerPort)
	trafficControllerURL = fmt.Sprintf("ws://%s", trafficControllerAddress)

	etcdRunner = etcdstorerunner.NewETCDClusterRunner(etcdPort, 1, nil)

	listenerPath = string(binaries["listener"])

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

	bbsClient = bbs.NewClient(bbsURL.String())

	auctioneerServer = ghttp.NewServer()
	auctioneerServer.UnhandledRequestStatusCode = http.StatusAccepted
	auctioneerServer.AllowUnhandledRequests = true

	bbsArgs = bbstestrunner.Args{
		Address:           bbsAddress,
		AdvertiseURL:      bbsURL.String(),
		AuctioneerAddress: auctioneerServer.URL(),
		EtcdCluster:       strings.Join(etcdRunner.NodeURLS(), ","),
		ConsulCluster:     consulRunner.ConsulCluster(),

		EncryptionKeys: []string{"label:key"},
		ActiveKeyLabel: "label",
	}

	etcdRunner.Start()
	consulRunner.Start()
	consulRunner.WaitUntilReady()
})

var _ = BeforeEach(func() {
	etcdRunner.Reset()
	consulRunner.Reset()

	bbsRunner = bbstestrunner.New(bbsPath, bbsArgs)
	bbsProcess = ginkgomon.Invoke(bbsRunner)

	fakeCC = ghttp.NewServer()

	listenerAddr = fmt.Sprintf("127.0.0.1:%d", uint16(listenerPort))

	runner = tpsrunner.NewListener(
		string(listenerPath),
		listenerAddr,
		bbsURL.String(),
		trafficControllerURL,
		consulRunner.URL(),
	)
})

var _ = AfterEach(func() {
	ginkgomon.Kill(bbsProcess)
	fakeCC.Close()
})

var _ = SynchronizedAfterSuite(func() {
	etcdRunner.Stop()
	consulRunner.Stop()
	auctioneerServer.Close()
}, func() {
	gexec.CleanupBuildArtifacts()
})
