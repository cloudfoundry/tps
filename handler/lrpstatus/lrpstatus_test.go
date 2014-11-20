package lrpstatus_test

import (
	"net/http"
	"net/http/httptest"

	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/receptor/fake_receptor"
	. "github.com/cloudfoundry-incubator/tps/handler/lrpstatus"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("LRPStatus", func() {
	var (
		fakeClient *fake_receptor.FakeClient

		server        *httptest.Server
		fakeResponses chan chan []receptor.ActualLRPResponse
	)

	BeforeEach(func() {
		fakeClient = new(fake_receptor.FakeClient)

		handler := NewHandler(fakeClient, 1, lagertest.NewTestLogger("test"))
		server = httptest.NewServer(handler)
		fakeResponses = make(chan chan []receptor.ActualLRPResponse, 2)

		fakeClient.ActualLRPsByProcessGuidStub = func(string) ([]receptor.ActualLRPResponse, error) {
			return <-<-fakeResponses, nil
		}
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("rate limiting", func() {
		It("returns 503 if the limit is exceeded", func() {
			// hit it once, make fake receptor hang
			hangingResponse := make(chan []receptor.ActualLRPResponse, 1)
			immediateResponse := make(chan []receptor.ActualLRPResponse, 1)
			immediateResponse <- []receptor.ActualLRPResponse{}

			// ensure we stop hanging so server can close
			defer close(hangingResponse)

			fakeResponses <- hangingResponse
			fakeResponses <- immediateResponse

			firstResponseCh := make(chan *http.Response)
			go func() {
				defer GinkgoRecover()

				res, err := http.Get(server.URL)
				Ω(err).ShouldNot(HaveOccurred())

				firstResponseCh <- res
			}()

			Eventually(fakeClient.ActualLRPsByProcessGuidCallCount).Should(Equal(1))

			// hit it again, assert we get a 503
			resp, err := http.Get(server.URL)
			Ω(err).ShouldNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))

			// un-hang the fake client
			hangingResponse <- []receptor.ActualLRPResponse{}

			var firstResponse *http.Response
			Eventually(firstResponseCh).Should(Receive(&firstResponse))
			Expect(firstResponse.StatusCode).To(Equal(http.StatusOK))

			// hit it again, assert we don't get a 503
			resp, err = http.Get(server.URL)
			Ω(err).ShouldNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})
})
