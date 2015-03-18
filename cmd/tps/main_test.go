package main_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit/ginkgomon"
	"github.com/tedsuo/rata"

	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	api "github.com/cloudfoundry-incubator/tps"
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

		desiredLRP := models.DesiredLRP{
			Domain:      "some-domain",
			ProcessGuid: "some-process-guid",
			Instances:   3,
			Stack:       "some-stack",
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
		if tps != nil {
			tps.Signal(os.Kill)
			Eventually(tps.Wait()).Should(Receive())
		}
	})

	Describe("GET /lrps/:guid", func() {
		Context("when the receptor is running", func() {
			JustBeforeEach(func() {
				lrpKey0 := models.NewActualLRPKey("some-process-guid", 0, "some-domain")
				instanceKey0 := models.NewActualLRPInstanceKey("some-instance-guid-0", "cell-id")

				err := bbs.ClaimActualLRP(logger, lrpKey0, instanceKey0)
				Ω(err).ShouldNot(HaveOccurred())

				lrpKey1 := models.NewActualLRPKey("some-process-guid", 1, "some-domain")
				instanceKey1 := models.NewActualLRPInstanceKey("some-instance-guid-1", "cell-id")
				netInfo := models.NewActualLRPNetInfo("1.2.3.4", []models.PortMapping{
					{ContainerPort: 8080, HostPort: 65100},
				})
				err = bbs.StartActualLRP(logger, lrpKey1, instanceKey1, netInfo)
				Ω(err).ShouldNot(HaveOccurred())
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

				Ω(lrpInstances).Should(HaveLen(3))
				for i, _ := range lrpInstances {
					lrpInstances[i].Since = 0
				}

				Ω(lrpInstances).Should(ContainElement(cc_messages.LRPInstance{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-0",
					Index:        0,
					State:        cc_messages.LRPInstanceStateStarting,
				}))

				Ω(lrpInstances).Should(ContainElement(cc_messages.LRPInstance{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-1",
					Index:        1,
					State:        cc_messages.LRPInstanceStateRunning,
				}))

				Ω(lrpInstances).Should(ContainElement(cc_messages.LRPInstance{
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
					Instance:   "some-instance-guid-1",
					Index:      1,
					Reason:     "CRASHED",
					CrashCount: 1,
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

			bbs.CrashActualLRP(logger, lrpKey1, instanceKey1)
		})

		It("POSTs to the CC that the application has crashed", func() {
			Eventually(ready).Should(BeClosed())
		})
	})
})
