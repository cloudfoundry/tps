package main_test

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit/ginkgomon"

	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
)

var _ = Describe("TPS", func() {
	JustBeforeEach(func() {
		watcher = ginkgomon.Invoke(runner)

		desiredLRP := models.DesiredLRP{
			Domain:      "some-domain",
			ProcessGuid: "some-process-guid",
			Instances:   3,
			RootFS:      "some:rootfs",
			MemoryMB:    1024,
			DiskMB:      512,
			LogGuid:     "some-log-guid",
			Action: &models.RunAction{
				Path: "ls",
			},
		}

		err := bbs.DesireLRP(logger, desiredLRP)
		Ω(err).ShouldNot(HaveOccurred())

	})

	AfterEach(func() {
		if watcher != nil {
			watcher.Signal(os.Kill)
			Eventually(watcher.Wait()).Should(Receive())
		}
	})

	Describe("Crashed Apps", func() {
		var ready chan struct{}

		BeforeEach(func() {
			ready = make(chan struct{})
			fakeCC.RouteToHandler("POST", "/internal/apps/some-process-guid/crashed", func(res http.ResponseWriter, req *http.Request) {
				var appCrashed cc_messages.AppCrashedRequest

				bytes, err := ioutil.ReadAll(req.Body)
				Ω(err).ShouldNot(HaveOccurred())
				req.Body.Close()

				err = json.Unmarshal(bytes, &appCrashed)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(appCrashed.CrashTimestamp).ShouldNot(BeZero())
				appCrashed.CrashTimestamp = 0

				Ω(appCrashed).Should(Equal(cc_messages.AppCrashedRequest{
					Instance:        "some-instance-guid-1",
					Index:           1,
					Reason:          "CRASHED",
					ExitDescription: "out of memory",
					CrashCount:      1,
				}))
				close(ready)
			})
		})

		JustBeforeEach(func() {
			lrpKey1 := models.NewActualLRPKey("some-process-guid", 1, "some-domain")
			instanceKey1 := models.NewActualLRPInstanceKey("some-instance-guid-1", "cell-id")
			netInfo := models.NewActualLRPNetInfo("1.2.3.4", []models.PortMapping{
				{ContainerPort: 8080, HostPort: 65100},
			})
			err := bbs.StartActualLRP(logger, lrpKey1, instanceKey1, netInfo)
			Ω(err).ShouldNot(HaveOccurred())

			bbs.CrashActualLRP(logger, lrpKey1, instanceKey1, "out of memory")
		})

		It("POSTs to the CC that the application has crashed", func() {
			Eventually(ready).Should(BeClosed())
		})
	})
})
