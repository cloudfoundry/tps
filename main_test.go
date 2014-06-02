package main_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"syscall"

	"github.com/cloudfoundry-incubator/runtime-schema/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/router"

	"github.com/cloudfoundry-incubator/tps/api"
)

var _ = Describe("TPS", func() {
	var tps ifrit.Process

	var httpClient *http.Client
	var requestGenerator *router.RequestGenerator

	BeforeEach(func() {
		tps = ifrit.Envoke(runner)

		httpClient = &http.Client{
			Transport: &http.Transport{},
		}

		requestGenerator = router.NewRequestGenerator(fmt.Sprintf("http://127.0.0.1:%d", tpsPort), api.Routes)
	})

	AfterEach(func() {
		tps.Signal(syscall.SIGTERM)
		Eventually(tps.Wait(), 5).Should(Receive(BeNil()))
	})

	Describe("GET /lrps/:guid", func() {
		Context("when etcd is running", func() {
			BeforeEach(func() {
				etcdRunner.Start()
			})

			AfterEach(func() {
				etcdRunner.Stop()
			})

			BeforeEach(func() {
				bbs.ReportActualLRPAsStarting(models.ActualLRP{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-1",

					Index: 0,

					State: models.ActualLRPStateStarting,
				})

				bbs.ReportActualLRPAsRunning(models.ActualLRP{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-2",

					Index: 1,

					State: models.ActualLRPStateRunning,
				})

				bbs.ReportActualLRPAsRunning(models.ActualLRP{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-3",

					Index: 2,

					State: models.ActualLRPStateRunning,
				})

				bbs.ReportActualLRPAsRunning(models.ActualLRP{
					ProcessGuid:  "some-other-process-guid",
					InstanceGuid: "some-instance-guid-3",

					Index: 0,

					State: models.ActualLRPStateRunning,
				})
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

					State: "starting",
				}))

				Ω(lrpInstances).Should(ContainElement(api.LRPInstance{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-2",

					Index: 1,

					State: "running",
				}))

				Ω(lrpInstances).Should(ContainElement(api.LRPInstance{
					ProcessGuid:  "some-process-guid",
					InstanceGuid: "some-instance-guid-3",

					Index: 2,

					State: "running",
				}))
			})
		})

		Context("when etcd is not running", func() {
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
})
