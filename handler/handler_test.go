package handler_test

import (
	"net/http"
	"net/http/httptest"

	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/receptor/fake_receptor"
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
			noaaClient     *fakes.FakeNoaaClient
			receptorClient *fake_receptor.FakeClient

			logger *lagertest.TestLogger

			server                 *httptest.Server
			fakeActualLRPResponses chan []receptor.ActualLRPResponse
			statsRequest           *http.Request
			statusRequest          *http.Request
			httpClient             *http.Client
		)

		BeforeEach(func() {
			var err error
			var httpHandler http.Handler

			httpClient = &http.Client{}
			logger = lagertest.NewTestLogger("test")
			receptorClient = new(fake_receptor.FakeClient)
			noaaClient = &fakes.FakeNoaaClient{}

			httpHandler, err = handler.New(receptorClient, noaaClient, 2, logger)
			Expect(err).NotTo(HaveOccurred())

			server = httptest.NewServer(httpHandler)

			fakeActualLRPResponses = make(chan []receptor.ActualLRPResponse, 2)

			receptorClient.ActualLRPsByProcessGuidStub = func(string) ([]receptor.ActualLRPResponse, error) {
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
			// hit both status and stats endpoints once, make fake receptor hang

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

			Eventually(receptorClient.ActualLRPsByProcessGuidCallCount).Should(Equal(2))

			// hit it again, assert we get a 503
			resp, err := httpClient.Do(statusRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))

			resp, err = httpClient.Do(statsRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))

			// un-hang one http call
			fakeActualLRPResponses <- []receptor.ActualLRPResponse{}

			go func() {
				defer GinkgoRecover()

				res, err := httpClient.Do(statsRequest)
				Expect(err).NotTo(HaveOccurred())
				Expect(res.StatusCode).To(Equal(http.StatusOK))
			}()

			fakeActualLRPResponses <- []receptor.ActualLRPResponse{}
			fakeActualLRPResponses <- []receptor.ActualLRPResponse{}

		})
	})

})
