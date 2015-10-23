package handler_test

import (
	"net/http"
	"net/http/httptest"

	"github.com/cloudfoundry-incubator/bbs/fake_bbs"
	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/tps/handler"
	"github.com/cloudfoundry-incubator/tps/handler/lrpstats/fakes"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gogo/protobuf/proto"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Handler", func() {

	Describe("rate limiting", func() {

		var (
			noaaClient *fakes.FakeNoaaClient
			bbsClient  *fake_bbs.FakeClient

			logger *lagertest.TestLogger

			server                 *httptest.Server
			fakeActualLRPResponses chan []*models.ActualLRPGroup
			statsRequest           *http.Request
			statusRequest          *http.Request
			httpClient             *http.Client
		)

		BeforeEach(func() {
			var err error
			var httpHandler http.Handler

			httpClient = &http.Client{}
			logger = lagertest.NewTestLogger("test")
			bbsClient = new(fake_bbs.FakeClient)
			noaaClient = &fakes.FakeNoaaClient{}

			httpHandler, err = handler.New(bbsClient, noaaClient, 2, 15, logger)
			Expect(err).NotTo(HaveOccurred())

			server = httptest.NewServer(httpHandler)

			fakeActualLRPResponses = make(chan []*models.ActualLRPGroup, 2)

			bbsClient.DesiredLRPByProcessGuidStub = func(string) (*models.DesiredLRP, error) {
				return &models.DesiredLRP{}, nil
			}

			bbsClient.ActualLRPGroupsByProcessGuidStub = func(string) ([]*models.ActualLRPGroup, error) {
				return <-fakeActualLRPResponses, nil
			}

			noaaClient.ContainerMetricsReturns([]*events.ContainerMetric{
				{
					ApplicationId: proto.String("appId"),
					InstanceIndex: proto.Int32(0),
					CpuPercentage: proto.Float64(4),
					MemoryBytes:   proto.Uint64(1024),
					DiskBytes:     proto.Uint64(2048),
				},
			}, nil)

			statsRequest, err = http.NewRequest("GET", server.URL+"/v1/actual_lrps/some-guid/stats", nil)
			Expect(err).NotTo(HaveOccurred())
			statsRequest.Header.Set("Authorization", "something")

			statusRequest, err = http.NewRequest("GET", server.URL+"/v1/actual_lrps/some-guid", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			server.Close()
		})

		It("returns 503 if the limit is exceeded", func() {
			// hit both status and stats endpoints once, make fake bbs hang

			defer close(fakeActualLRPResponses)

			go func() {
				defer GinkgoRecover()

				res, err := httpClient.Do(statusRequest)
				Expect(err).NotTo(HaveOccurred())
				Expect(res.StatusCode).To(Equal(http.StatusOK))
			}()

			go func() {
				defer GinkgoRecover()

				res, err := httpClient.Do(statsRequest)
				Expect(err).NotTo(HaveOccurred())
				Expect(res.StatusCode).To(Equal(http.StatusOK))
			}()

			Eventually(bbsClient.ActualLRPGroupsByProcessGuidCallCount).Should(Equal(2))

			// hit it again, assert we get a 503
			resp, err := httpClient.Do(statusRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))

			resp, err = httpClient.Do(statsRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))

			// un-hang one http call
			fakeActualLRPResponses <- []*models.ActualLRPGroup{}

			go func() {
				defer GinkgoRecover()

				res, err := httpClient.Do(statsRequest)
				Expect(err).NotTo(HaveOccurred())
				Expect(res.StatusCode).To(Equal(http.StatusOK))
			}()

			fakeActualLRPResponses <- []*models.ActualLRPGroup{}
			fakeActualLRPResponses <- []*models.ActualLRPGroup{}

		})
	})
})
