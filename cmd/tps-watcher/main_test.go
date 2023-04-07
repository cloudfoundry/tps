package main_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"code.cloudfoundry.org/bbs/events"
	"code.cloudfoundry.org/bbs/models"
	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/localip"
	"code.cloudfoundry.org/locket"
	locketconfig "code.cloudfoundry.org/locket/cmd/locket/config"
	locketrunner "code.cloudfoundry.org/locket/cmd/locket/testrunner"
	"code.cloudfoundry.org/locket/lock"
	locketmodels "code.cloudfoundry.org/locket/models"
	"code.cloudfoundry.org/runtimeschema/cc_messages"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

const watcherLockName = "tps_watcher_lock"

var _ = Describe("TPS", func() {
	var (
		domain        string
		locketRunner  ifrit.Runner
		locketProcess ifrit.Process
		locketAddress string
	)

	BeforeEach(func() {
		locketPort, err := localip.LocalPort()
		Expect(err).NotTo(HaveOccurred())

		dbName := fmt.Sprintf("locket_%d", GinkgoParallelProcess())
		connectionString := "postgres://locket:locket_pw@localhost"
		db, err := sql.Open("postgres", connectionString+"?sslmode=disable")
		Expect(err).NotTo(HaveOccurred())
		Expect(db.Ping()).NotTo(HaveOccurred())

		_, err = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
		Expect(err).NotTo(HaveOccurred())

		_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName))
		Expect(err).NotTo(HaveOccurred())

		locketBinName := "locket"
		locketAddress = fmt.Sprintf("localhost:%d", locketPort)
		locketRunner = locketrunner.NewLocketRunner(locketBinName, func(cfg *locketconfig.LocketConfig) {
			cfg.DatabaseConnectionString = connectionString + "/" + dbName
			cfg.DatabaseDriver = "postgres"
			cfg.ListenAddress = locketAddress
		})
		locketProcess = ginkgomon.Invoke(locketRunner)

		watcherConfig.ClientLocketConfig = locketrunner.ClientLocketConfig()
		watcherConfig.ClientLocketConfig.LocketAddress = locketAddress

		fakeBBS.AllowUnhandledRequests = true

		domain = cc_messages.AppLRPDomain
	})

	AfterEach(func() {
		ginkgomon.Interrupt(watcher, 5*time.Second)
		ginkgomon.Interrupt(locketProcess, 5*time.Second)

		if watcher != nil {
			watcher.Signal(os.Kill)
			Eventually(watcher.Wait()).Should(Receive())
		}
	})

	Describe("Crashed Apps", func() {
		var (
			ready chan struct{}
		)

		BeforeEach(func() {
			ready = make(chan struct{})
			fakeCC.RouteToHandler("POST", "/internal/v4/apps/some-process-guid/crashed", func(res http.ResponseWriter, req *http.Request) {
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
					CellID:          "cell-id",
					Reason:          "CRASHED",
					ExitDescription: "out of memory",
					CrashCount:      1,
				}))

				close(ready)
			})

			lrpKey := models.NewActualLRPKey("some-process-guid", 1, domain)
			instanceKey := models.NewActualLRPInstanceKey("some-instance-guid-1", "cell-id")
			netInfo := models.NewActualLRPNetInfo("1.2.3.4", "5.6.7.8", models.ActualLRPNetInfo_PreferredAddressHost, models.NewPortMapping(65100, 8080))
			beforeActualLRP := *models.NewRunningActualLRP(lrpKey, instanceKey, netInfo, 0)
			afterActualLRP := beforeActualLRP
			afterActualLRP.State = models.ActualLRPStateCrashed
			afterActualLRP.Since = int64(1)
			afterActualLRP.CrashCount = 1
			afterActualLRP.CrashReason = "out of memory"

			fakeBBS.RouteToHandler("POST", "/v1/events/lrp_instances.r1",
				func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Add("Content-Type", "text/event-stream; charset=utf-8")
					w.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate")
					w.Header().Add("Connection", "keep-alive")

					w.WriteHeader(http.StatusOK)

					flusher := w.(http.Flusher)
					flusher.Flush()
					closeNotifier := w.(http.CloseNotifier).CloseNotify()
					event := models.NewActualLRPCrashedEvent(&beforeActualLRP, &afterActualLRP)

					sseEvent, err := events.NewEventFromModelEvent(0, event)
					Expect(err).NotTo(HaveOccurred())

					err = sseEvent.Write(w)
					Expect(err).NotTo(HaveOccurred())

					flusher.Flush()

					<-closeNotifier
				},
			)
		})

		It("POSTs to the CC that the application has crashed", func() {
			Eventually(ready, 5*time.Second).Should(BeClosed())
		})
	})

	Describe("SqlLock", func() {
		Context("with invalid configuration", func() {
			Context("and the locket address is not configured", func() {
				BeforeEach(func() {
					watcherConfig.LocketAddress = ""
					disableStartCheck = true
				})

				It("exits with an error", func() {
					Eventually(runner).Should(gexec.Exit(2))
				})
			})
		})

		Context("with valid configuration", func() {
			It("acquires the lock in locket and becomes active", func() {
				Eventually(runner.Buffer, 5*time.Second).Should(gbytes.Say("tps-watcher.started"))
			})

			Context("and the locking server becomes unreachable after grabbing the lock", func() {
				JustBeforeEach(func() {
					Eventually(runner.Buffer, 5*time.Second).Should(gbytes.Say("tps-watcher.started"))

					ginkgomon.Interrupt(locketProcess, 5*time.Second)
				})

				It("exits after the TTL expires", func() {
					Eventually(runner, 17*time.Second).Should(gexec.Exit(1))
				})
			})

			Context("when the lock is not available", func() {
				var competingProcess ifrit.Process

				BeforeEach(func() {
					locketClient, err := locket.NewClient(logger, watcherConfig.ClientLocketConfig)
					Expect(err).NotTo(HaveOccurred())

					lockIdentifier := &locketmodels.Resource{
						Key:      "tps_watcher",
						Owner:    "Your worst enemy.",
						Value:    "Something",
						TypeCode: locketmodels.LOCK,
					}

					clock := clock.NewClock()
					competingRunner := lock.NewLockRunner(logger, locketClient, lockIdentifier, 5, clock, locket.RetryInterval)
					competingProcess = ginkgomon.Invoke(competingRunner)

					disableStartCheck = true
				})

				AfterEach(func() {
					ginkgomon.Interrupt(competingProcess, 5*time.Second)
				})

				It("does not become active", func() {
					Consistently(runner.Buffer, 5*time.Second).ShouldNot(gbytes.Say("tps-watcher.started"))
				})

				Context("and the lock becomes available", func() {
					JustBeforeEach(func() {
						Consistently(runner.Buffer, 5*time.Second).ShouldNot(gbytes.Say("tps-watcher.started"))

						ginkgomon.Interrupt(competingProcess, 5*time.Second)
					})

					It("grabs the lock and becomes active", func() {
						Eventually(runner.Buffer, 5*time.Second).Should(gbytes.Say("tps-watcher.started"))
					})
				})
			})
		})
	})
})
