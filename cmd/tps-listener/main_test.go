package main_test

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"
	"github.com/tedsuo/rata"

	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/cloudfoundry-incubator/tps"
)

var _ = Describe("TPS-Listener", func() {

	var httpClient *http.Client
	var requestGenerator *rata.RequestGenerator

	BeforeEach(func() {
		requestGenerator = rata.NewRequestGenerator(fmt.Sprintf("http://%s", listenerAddr), tps.Routes)
		httpClient = &http.Client{
			Transport: &http.Transport{},
		}
	})

	JustBeforeEach(func() {
		listener = ginkgomon.Invoke(runner)

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

		err := lrpBBS.DesireLRP(logger, desiredLRP)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if listener != nil {
			listener.Signal(os.Kill)
			Eventually(listener.Wait()).Should(Receive())
		}
	})

	Describe("GET /v1/actual_lrps/:guid", func() {
		Context("when the receptor is running", func() {
			JustBeforeEach(func() {
				lrpKey0 := models.NewActualLRPKey("some-process-guid", 0, "some-domain")
				instanceKey0 := models.NewActualLRPInstanceKey("some-instance-guid-0", "cell-id")

				err := lrpBBS.ClaimActualLRP(logger, lrpKey0, instanceKey0)
				Expect(err).NotTo(HaveOccurred())

				lrpKey1 := models.NewActualLRPKey("some-process-guid", 1, "some-domain")
				instanceKey1 := models.NewActualLRPInstanceKey("some-instance-guid-1", "cell-id")
				netInfo := models.NewActualLRPNetInfo("1.2.3.4", []models.PortMapping{
					{ContainerPort: 8080, HostPort: 65100},
				})
				err = lrpBBS.StartActualLRP(logger, lrpKey1, instanceKey1, netInfo)
				Expect(err).NotTo(HaveOccurred())
			})

			It("reports the state of the given process guid's instances", func() {
				getLRPs, err := requestGenerator.CreateRequest(
					tps.LRPStatus,
					rata.Params{"guid": "some-process-guid"},
					nil,
				)
				Expect(err).NotTo(HaveOccurred())

				response, err := httpClient.Do(getLRPs)
				Expect(err).NotTo(HaveOccurred())

				var lrpInstances []cc_messages.LRPInstance
				err = json.NewDecoder(response.Body).Decode(&lrpInstances)
				Expect(err).NotTo(HaveOccurred())

				Expect(lrpInstances).To(HaveLen(3))
				for i, _ := range lrpInstances {
					lrpInstances[i].Since = 0
				}

				Expect(lrpInstances).To(ContainElement(cc_messages.LRPInstance{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-0",
					Index:        0,
					State:        cc_messages.LRPInstanceStateStarting,
				}))

				Expect(lrpInstances).To(ContainElement(cc_messages.LRPInstance{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-1",
					Index:        1,
					State:        cc_messages.LRPInstanceStateRunning,
				}))

				Expect(lrpInstances).To(ContainElement(cc_messages.LRPInstance{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "",
					Index:        2,
					State:        cc_messages.LRPInstanceStateStarting,
				}))

			})
		})

		Context("when the receptor is not running", func() {
			BeforeEach(func() {
				ginkgomon.Kill(receptorRunner, 5)
			})

			It("returns 500", func() {
				getLRPs, err := requestGenerator.CreateRequest(
					tps.LRPStatus,
					rata.Params{"guid": "some-process-guid"},
					nil,
				)
				Expect(err).NotTo(HaveOccurred())

				response, err := httpClient.Do(getLRPs)
				Expect(err).NotTo(HaveOccurred())

				Expect(response.StatusCode).To(Equal(http.StatusInternalServerError))
			})
		})
	})

	Describe("GET /v1/actual_lrps/:guid/stats", func() {
		Context("when the receptor is running", func() {
			var trafficControllerProcess ifrit.Process

			JustBeforeEach(func() {
				lrpKey0 := models.NewActualLRPKey("some-process-guid", 0, "some-domain")
				instanceKey0 := models.NewActualLRPInstanceKey("some-instance-guid-0", "cell-id")

				err := lrpBBS.ClaimActualLRP(logger, lrpKey0, instanceKey0)
				Expect(err).NotTo(HaveOccurred())

				lrpKey1 := models.NewActualLRPKey("some-process-guid", 1, "some-domain")
				instanceKey1 := models.NewActualLRPInstanceKey("some-instance-guid-1", "cell-id")
				netInfo := models.NewActualLRPNetInfo("1.2.3.4", []models.PortMapping{
					{ContainerPort: 8080, HostPort: 65100},
				})
				err = lrpBBS.StartActualLRP(logger, lrpKey1, instanceKey1, netInfo)
				Expect(err).NotTo(HaveOccurred())
			})

			Context("when a DesiredLRP is not found", func() {
				It("returns a NotFound", func() {
					getLRPStats, err := requestGenerator.CreateRequest(
						tps.LRPStats,
						rata.Params{"guid": "some-bogus-guid"},
						nil,
					)
					Expect(err).ToNot(HaveOccurred())
					getLRPStats.Header.Add("Authorization", "I can do this.")

					response, err := httpClient.Do(getLRPStats)
					Expect(err).ToNot(HaveOccurred())
					Expect(response.StatusCode).To(Equal(http.StatusNotFound))
				})
			})

			Context("when the traffic controller is running", func() {
				BeforeEach(func() {
					message1 := marshalMessage(createContainerMetric("some-process-guid", 0, 3.0, 1024, 2048, 0))
					message2 := marshalMessage(createContainerMetric("some-process-guid", 1, 4.0, 1024, 2048, 0))
					message3 := marshalMessage(createContainerMetric("some-process-guid", 2, 5.0, 1024, 2048, 0))
					messages := [][]byte{message1, message2, message3}

					handler := NewHttpHandler(messages)
					httpServer := http_server.New(trafficControllerAddress, handler)
					trafficControllerProcess = ifrit.Invoke(sigmon.New(httpServer))
					Expect(trafficControllerProcess.Ready()).To(BeClosed())
				})

				AfterEach(func() {
					ginkgomon.Interrupt(trafficControllerProcess)
				})

				It("reports the state of the given process guid's instances", func() {
					getLRPStats, err := requestGenerator.CreateRequest(
						tps.LRPStats,
						rata.Params{"guid": "some-process-guid"},
						nil,
					)
					Expect(err).NotTo(HaveOccurred())
					getLRPStats.Header.Add("Authorization", "I can do this.")

					response, err := httpClient.Do(getLRPStats)
					Expect(err).NotTo(HaveOccurred())
					Expect(response.StatusCode).To(Equal(http.StatusOK))

					var lrpInstances []cc_messages.LRPInstance
					err = json.NewDecoder(response.Body).Decode(&lrpInstances)
					Expect(err).NotTo(HaveOccurred())

					Expect(lrpInstances).To(HaveLen(3))
					for i, _ := range lrpInstances {
						lrpInstances[i].Since = 0
					}

					Expect(lrpInstances).To(ContainElement(cc_messages.LRPInstance{
						ProcessGuid:  "some-process-guid",
						InstanceGuid: "some-instance-guid-0",
						Index:        0,
						State:        cc_messages.LRPInstanceStateStarting,
						Stats: &cc_messages.LRPInstanceStats{
							CpuPercentage: 0.03,
							MemoryBytes:   1024,
							DiskBytes:     2048,
						},
					}))

					Expect(lrpInstances).To(ContainElement(cc_messages.LRPInstance{
						ProcessGuid:  "some-process-guid",
						InstanceGuid: "some-instance-guid-1",
						Index:        1,
						State:        cc_messages.LRPInstanceStateRunning,
						Stats: &cc_messages.LRPInstanceStats{
							CpuPercentage: 0.04,
							MemoryBytes:   1024,
							DiskBytes:     2048,
						},
					}))

					Expect(lrpInstances).To(ContainElement(cc_messages.LRPInstance{
						ProcessGuid:  "some-process-guid",
						InstanceGuid: "",
						Index:        2,
						State:        cc_messages.LRPInstanceStateStarting,
						Stats: &cc_messages.LRPInstanceStats{
							CpuPercentage: 0.05,
							MemoryBytes:   1024,
							DiskBytes:     2048,
						},
					}))

				})
			})

			Context("when the traffic controller is not running", func() {
				It("reports the status with nil stats", func() {
					getLRPStats, err := requestGenerator.CreateRequest(
						tps.LRPStats,
						rata.Params{"guid": "some-process-guid"},
						nil,
					)
					Expect(err).NotTo(HaveOccurred())
					getLRPStats.Header.Add("Authorization", "I can do this.")

					response, err := httpClient.Do(getLRPStats)
					Expect(err).NotTo(HaveOccurred())
					Expect(response.StatusCode).To(Equal(http.StatusOK))

					var lrpInstances []cc_messages.LRPInstance
					err = json.NewDecoder(response.Body).Decode(&lrpInstances)
					Expect(err).NotTo(HaveOccurred())

					Expect(lrpInstances).To(HaveLen(3))

					for _, instance := range lrpInstances {
						Expect(instance.Stats).To(BeNil())
					}
				})
			})
		})

		Context("when the receptor is not running", func() {
			BeforeEach(func() {
				ginkgomon.Kill(receptorRunner, 5)
			})

			It("returns internal server error", func() {
				getLRPs, err := requestGenerator.CreateRequest(
					tps.LRPStatus,
					rata.Params{"guid": "some-process-guid"},
					nil,
				)
				Expect(err).NotTo(HaveOccurred())

				response, err := httpClient.Do(getLRPs)
				Expect(err).NotTo(HaveOccurred())

				Expect(response.StatusCode).To(Equal(http.StatusInternalServerError))
			})
		})
	})
})

func createContainerMetric(appId string, instanceIndex int32, cpuPercentage float64, memoryBytes uint64, diskByte uint64, timestamp int64) *events.Envelope {
	if timestamp == 0 {
		timestamp = time.Now().UnixNano()
	}

	cm := &events.ContainerMetric{
		ApplicationId: proto.String(appId),
		InstanceIndex: proto.Int32(instanceIndex),
		CpuPercentage: proto.Float64(cpuPercentage),
		MemoryBytes:   proto.Uint64(memoryBytes),
		DiskBytes:     proto.Uint64(diskByte),
	}

	return &events.Envelope{
		ContainerMetric: cm,
		EventType:       events.Envelope_ContainerMetric.Enum(),
		Origin:          proto.String("fake-origin-1"),
		Timestamp:       proto.Int64(timestamp),
	}
}

func marshalMessage(message *events.Envelope) []byte {
	data, err := proto.Marshal(message)
	if err != nil {
		log.Println(err.Error())
	}

	return data
}
