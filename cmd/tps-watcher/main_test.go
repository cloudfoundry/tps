package main_test

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"

	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/consuladapter"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs/shared"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
)

const watcherLockName = "tps_watcher_lock"

var _ = Describe("TPS", func() {
	startWatcher := func(check bool) (ifrit.Process, *ginkgomon.Runner) {
		if !check {
			runner.StartCheck = ""
		}

		return ginkgomon.Invoke(runner), runner
	}

	AfterEach(func() {
		if watcher != nil {
			watcher.Signal(os.Kill)
			Eventually(watcher.Wait()).Should(Receive())
		}
	})

	Describe("Crashed Apps", func() {
		var (
			ready  chan struct{}
			domain string
		)

		BeforeEach(func() {
			ready = make(chan struct{})
			fakeCC.RouteToHandler("POST", "/internal/apps/some-process-guid/crashed", func(res http.ResponseWriter, req *http.Request) {
				var appCrashed cc_messages.AppCrashedRequest

				bytes, err := ioutil.ReadAll(req.Body)
				Expect(err).NotTo(HaveOccurred())
				req.Body.Close()

				err = json.Unmarshal(bytes, &appCrashed)
				Expect(err).NotTo(HaveOccurred())

				Expect(appCrashed.CrashTimestamp).NotTo(BeZero())
				appCrashed.CrashTimestamp = 0

				Expect(appCrashed).To(Equal(cc_messages.AppCrashedRequest{
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
			watcher, _ = startWatcher(true)
			domain = cc_messages.AppLRPDomain

			desiredLRP := &models.DesiredLRP{
				Domain:      domain,
				ProcessGuid: "some-process-guid",
				Instances:   3,
				RootFs:      "some:rootfs",
				MemoryMb:    1024,
				DiskMb:      512,
				LogGuid:     "some-log-guid",
				Action: models.WrapAction(&models.RunAction{
					User: "me",
					Path: "ls",
				}),
			}

			err := bbsClient.DesireLRP(desiredLRP)
			Expect(err).NotTo(HaveOccurred())

			lrpKey1 := models.NewActualLRPKey("some-process-guid", 1, domain)
			instanceKey1 := models.NewActualLRPInstanceKey("some-instance-guid-1", "cell-id")
			netInfo := models.NewActualLRPNetInfo("1.2.3.4", models.NewPortMapping(65100, 8080))
			err = bbsClient.StartActualLRP(&lrpKey1, &instanceKey1, &netInfo)
			Expect(err).NotTo(HaveOccurred())

			// work around the fact that the event source has to sleep
			// see github.com/cloudfoundry/storeadapter/etcdstoreadapter/etcd_store_adapter.go
			time.Sleep(150 * time.Millisecond)

			bbsClient.CrashActualLRP(&lrpKey1, &instanceKey1, "out of memory")
		})

		It("POSTs to the CC that the application has crashed", func() {
			Eventually(ready, 5*time.Second).Should(BeClosed())
		})
	})

	Context("when the watcher loses the lock", func() {
		BeforeEach(func() {
			watcher, _ = startWatcher(true)
		})

		JustBeforeEach(func() {
			consulRunner.Reset()
		})

		AfterEach(func() {
			ginkgomon.Interrupt(watcher, 5)
		})

		It("exits with an error", func() {
			Eventually(watcher.Wait(), 5).Should(Receive(HaveOccurred()))
		})
	})

	Context("when the watcher initially does not have the lock", func() {
		var runner *ginkgomon.Runner
		var otherSession *consuladapter.Session

		BeforeEach(func() {
			otherSession = consulRunner.NewSession("other-Session")
			err := otherSession.AcquireLock(shared.LockSchemaPath(watcherLockName), []byte("something-else"))
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			watcher, runner = startWatcher(false)
		})

		AfterEach(func() {
			ginkgomon.Interrupt(watcher, 5)
		})

		It("does not start", func() {
			Consistently(runner.Buffer, 5*time.Second).ShouldNot(gbytes.Say("tps-watcher.started"))
		})

		Context("when the lock becomes available", func() {
			BeforeEach(func() {
				otherSession.Destroy()
			})

			It("is updated", func() {
				Eventually(runner.Buffer, 5*time.Second).Should(gbytes.Say("tps-watcher.started"))
			})
		})
	})
})
