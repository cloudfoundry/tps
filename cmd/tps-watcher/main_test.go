package main_test

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/hashicorp/consul/consul/structs"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"

	"github.com/cloudfoundry-incubator/runtime-schema/bbs/shared"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
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
			watcher, _ = startWatcher(true)

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

			lrpKey1 := models.NewActualLRPKey("some-process-guid", 1, "some-domain")
			instanceKey1 := models.NewActualLRPInstanceKey("some-instance-guid-1", "cell-id")
			netInfo := models.NewActualLRPNetInfo("1.2.3.4", []models.PortMapping{
				{ContainerPort: 8080, HostPort: 65100},
			})
			err = bbs.StartActualLRP(logger, lrpKey1, instanceKey1, netInfo)
			Ω(err).ShouldNot(HaveOccurred())

			bbs.CrashActualLRP(logger, lrpKey1, instanceKey1, "out of memory")
		})

		It("POSTs to the CC that the application has crashed", func() {
			Eventually(ready).Should(BeClosed())
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

		BeforeEach(func() {
			consulRunner.WaitUntilReady()

			_, err := consulAdapter.AcquireAndMaintainLock(
				shared.LockSchemaPath(watcherLockName),
				[]byte("something-else"),
				structs.SessionTTLMin,
				nil)
			Ω(err).ShouldNot(HaveOccurred())
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
				err := consulAdapter.ReleaseAndDeleteLock(shared.LockSchemaPath(watcherLockName))
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("is updated", func() {
				Eventually(runner.Buffer, 5*time.Second).Should(gbytes.Say("tps-watcher.started"))
			})
		})
	})
})
