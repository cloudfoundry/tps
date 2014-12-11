package main_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/apcera/nats"
	"github.com/cloudfoundry-incubator/receptor"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit/ginkgomon"
	"github.com/tedsuo/rata"

	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	api "github.com/cloudfoundry-incubator/tps"
	"github.com/cloudfoundry-incubator/tps/heartbeat"
)

var _ = Describe("TPS", func() {

	var httpClient *http.Client
	var requestGenerator *rata.RequestGenerator

	BeforeEach(func() {
		requestGenerator = rata.NewRequestGenerator(fmt.Sprintf("http://%s", tpsAddr), api.Routes)
		httpClient = &http.Client{
			Transport: &http.Transport{},
		}
	})

	JustBeforeEach(func() {
		tps = ginkgomon.Invoke(runner)
	})

	AfterEach(func() {
		if tps != nil {
			tps.Signal(os.Kill)
			Eventually(tps.Wait()).Should(Receive())
		}
	})

	Describe("GET /lrps/:guid", func() {
		Context("when the receptor is running", func() {
			BeforeEach(func() {
				receptorServer.RouteToHandler("GET", "/v1/actual_lrps/some-process-guid", func(w http.ResponseWriter, req *http.Request) {
					json.NewEncoder(w).Encode([]receptor.ActualLRPResponse{
						{
							ProcessGuid:  "some-process-guid",
							InstanceGuid: "some-instance-guid-1",
							Domain:       "some-domain",
							Index:        0,
							Since:        1,
							State:        receptor.ActualLRPStateClaimed,
						},
						{
							ProcessGuid:  "some-process-guid",
							InstanceGuid: "some-instance-guid-2",
							Domain:       "some-domain",
							Index:        1,
							Since:        2,
							State:        receptor.ActualLRPStateRunning,
						},
						{
							ProcessGuid:  "some-process-guid",
							InstanceGuid: "some-instance-guid-3",
							Domain:       "some-domain",
							Index:        2,
							Since:        3,
							State:        receptor.ActualLRPStateUnclaimed,
						},
						{
							ProcessGuid:  "some-process-guid",
							InstanceGuid: "some-instance-guid-4",
							Domain:       "some-domain",
							Index:        3,
							Since:        4,
							State:        "",
						},
					})
				})
			})

			AfterEach(func() {
				receptorServer.Close()
			})

			It("reports the state of the given process guid's instances", func() {
				getLRPs, err := requestGenerator.CreateRequest(
					api.LRPStatus,
					rata.Params{"guid": "some-process-guid"},
					nil,
				)
				Ω(err).ShouldNot(HaveOccurred())

				response, err := httpClient.Do(getLRPs)
				Ω(err).ShouldNot(HaveOccurred())

				var lrpInstances []cc_messages.LRPInstance
				err = json.NewDecoder(response.Body).Decode(&lrpInstances)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(lrpInstances).Should(HaveLen(4))

				Ω(lrpInstances).Should(ContainElement(cc_messages.LRPInstance{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-1",
					Index:        0,
					Since:        1,
					State:        cc_messages.LRPInstanceStateStarting,
				}))

				Ω(lrpInstances).Should(ContainElement(cc_messages.LRPInstance{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-2",
					Index:        1,
					Since:        2,
					State:        cc_messages.LRPInstanceStateRunning,
				}))

				Ω(lrpInstances).Should(ContainElement(cc_messages.LRPInstance{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-3",
					Index:        2,
					Since:        3,
					State:        cc_messages.LRPInstanceStateUnknown,
				}))

				Ω(lrpInstances).Should(ContainElement(cc_messages.LRPInstance{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-4",
					Index:        3,
					Since:        4,
					State:        cc_messages.LRPInstanceStateUnknown,
				}))
			})
		})

		Context("when the receptor is not running", func() {
			BeforeEach(func() {
				receptorServer.Close()
			})

			It("returns 500", func() {
				getLRPs, err := requestGenerator.CreateRequest(
					api.LRPStatus,
					rata.Params{"guid": "some-process-guid"},
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
		var announceMsg chan *nats.Msg
		var subscription *nats.Subscription

		BeforeEach(func() {
			announceMsg = make(chan *nats.Msg)
			var err error
			subscription, err = natsClient.Subscribe(tpsNatsSubject, func(msg *nats.Msg) {
				announceMsg <- msg
			})
			Ω(err).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			err := natsClient.Unsubscribe(subscription)
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("heartbeats announcement messages at the predefined interval", func() {
			Eventually(announceMsg, heartbeatInterval+time.Second).Should(Receive())
			Eventually(announceMsg, heartbeatInterval+time.Second).Should(Receive())
		})

		Describe("published HeartbeatMessage", func() {
			var heartbeatMsg heartbeat.HeartbeatMessage

			JustBeforeEach(func(done Done) {
				heartbeatMsg = heartbeat.HeartbeatMessage{}
				msg := <-announceMsg
				err := json.Unmarshal(msg.Data, &heartbeatMsg)
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
			runner.StartCheck = ""
			ginkgomon.Kill(gnatsdRunner)
		})

		It("does not exit", func() {
			Consistently(tps.Wait()).ShouldNot(Receive())
		})

		It("exits when we send a signal", func() {
			tps.Signal(syscall.SIGINT)
			Eventually(tps.Wait()).Should(Receive())
		})
	})

	Context("when the NATS server goes down after startup", func() {
		JustBeforeEach(func() {
			ginkgomon.Kill(gnatsdRunner)
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
