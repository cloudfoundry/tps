package main_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/cloudfoundry/yagnats"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/router"

	"github.com/cloudfoundry-incubator/tps/api"
	"github.com/cloudfoundry-incubator/tps/heartbeat"
)

var _ = Describe("TPS", func() {

	var httpClient *http.Client
	var requestGenerator *router.RequestGenerator
	var natsClient yagnats.NATSClient

	BeforeEach(func() {
		natsClient = natsRunner.MessageBus
		requestGenerator = router.NewRequestGenerator(fmt.Sprintf("http://%s", tpsAddr), api.Routes)
		httpClient = &http.Client{
			Transport: &http.Transport{},
		}
	})

	Describe("GET /lrps/:guid", func() {
		Context("when etcd is running", func() {
			BeforeEach(func() {
				bbs.ReportActualLRPAsStarting(models.ActualLRP{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-1",

					Index: 0,

					State: models.ActualLRPStateStarting,
				}, "executor-id")

				bbs.ReportActualLRPAsRunning(models.ActualLRP{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-2",

					Index: 1,

					State: models.ActualLRPStateRunning,
				}, "executor-id")

				bbs.ReportActualLRPAsRunning(models.ActualLRP{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-3",

					Index: 2,

					State: models.ActualLRPStateRunning,
				}, "executor-id")

				bbs.ReportActualLRPAsRunning(models.ActualLRP{
					ProcessGuid:  "some-other-process-guid",
					InstanceGuid: "some-instance-guid-3",

					Index: 0,

					State: models.ActualLRPStateRunning,
				}, "executor-id")
			})

			It("reports the state of the given process guid's instances", func() {
				getLRPs, err := requestGenerator.RequestForHandler(
					api.LRPStatus,
					router.Params{"guid": "some-process-guid"},
					nil,
				)
				Ω(err).ShouldNot(HaveOccurred())

				response, err := httpClient.Do(getLRPs)
				Ω(err).ShouldNot(HaveOccurred())

				var lrpInstances []api.LRPInstance
				err = json.NewDecoder(response.Body).Decode(&lrpInstances)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(lrpInstances).Should(HaveLen(3))

				Ω(lrpInstances).Should(ContainElement(api.LRPInstance{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-1",

					Index: 0,
					Since: timeProvider.Time().UnixNano(),

					State: "starting",
				}))

				Ω(lrpInstances).Should(ContainElement(api.LRPInstance{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-2",

					Index: 1,
					Since: timeProvider.Time().UnixNano(),

					State: "running",
				}))

				Ω(lrpInstances).Should(ContainElement(api.LRPInstance{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-3",

					Index: 2,
					Since: timeProvider.Time().UnixNano(),

					State: "running",
				}))
			})
		})

		Context("when etcd is not running", func() {
			BeforeEach(func() {
				etcdRunner.Stop()
			})

			It("returns 500", func() {
				getLRPs, err := requestGenerator.RequestForHandler(
					api.LRPStatus,
					router.Params{"guid": "some-process-guid"},
					nil,
				)
				Ω(err).ShouldNot(HaveOccurred())

				response, err := httpClient.Do(getLRPs)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(response.StatusCode).Should(Equal(http.StatusInternalServerError))
			})
		})
	})

	Context("when the NATS server is running", func() {
		var tpsNatsSubject = "service.announce.tps"
		var announceMsg chan *yagnats.Message

		BeforeEach(func() {
			announceMsg = make(chan *yagnats.Message)
			natsClient.Subscribe(tpsNatsSubject, func(msg *yagnats.Message) {
				announceMsg <- msg
			})
		})

		AfterEach(func() {
			natsClient.UnsubscribeAll(tpsNatsSubject)
		})

		It("heartbeats announcement messages at the predefined interval", func() {
			Eventually(announceMsg, heartbeatInterval+time.Second).Should(Receive())
			Eventually(announceMsg, heartbeatInterval+time.Second).Should(Receive())
		})

		Describe("published HeartbeatMessage", func() {
			var heartbeatMsg heartbeat.HeartbeatMessage

			BeforeEach(func(done Done) {
				heartbeatMsg = heartbeat.HeartbeatMessage{}
				msg := <-announceMsg
				err := json.Unmarshal(msg.Payload, &heartbeatMsg)
				Ω(err).ShouldNot(HaveOccurred())
				close(done)
			})

			It("contains the correct tps address", func() {
				Ω(heartbeatMsg.Addr).Should(Equal(fmt.Sprintf("http://%s", tpsAddr)))
			})

			It("a ttl 3 times longer than the heartbeatInterval, in seconds", func() {
				Ω(heartbeatMsg.TTL).Should(Equal(uint(3)))
			})
		})
	})

	Context("when the NATS server is down while starting up", func() {
		BeforeEach(func() {
			tps.Signal(os.Kill)
			Eventually(tps.Wait()).Should(Receive(nil))

			natsRunner.KillWithFire()

			tps = ifrit.Envoke(runner)
		})

		It("exits imediately", func() {
			Eventually(tps.Wait()).Should(Receive())
		})
	})

	Context("when the NATS server goes down after startup", func() {
		BeforeEach(func() {
			natsRunner.KillWithFire()
			time.Sleep(50 * time.Millisecond)
		})

		It("does not exit", func() {
			Consistently(tps.Wait()).ShouldNot(Receive())
		})

		It("exits when we send a signal", func() {
			tps.Signal(syscall.SIGINT)
			Eventually(tps.Wait()).Should(Receive())
		})
	})
})
