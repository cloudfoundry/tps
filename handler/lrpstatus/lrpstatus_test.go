package lrpstatus_test

import (
	"net/http"
	"net/http/httptest"

	"github.com/cloudfoundry-incubator/runtime-schema/bbs/fake_bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	. "github.com/cloudfoundry-incubator/tps/handler/lrpstatus"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("LRPStatus", func() {
	var (
		fakeBBS *fake_bbs.FakeTPSBBS

		server        *httptest.Server
		fakeResponses chan chan []models.ActualLRP
	)

	BeforeEach(func() {
		fakeBBS = new(fake_bbs.FakeTPSBBS)

		handler := NewHandler(fakeBBS, 1, lagertest.NewTestLogger("test"))
		server = httptest.NewServer(handler)
		fakeResponses = make(chan chan []models.ActualLRP, 2)

		fakeBBS.ActualLRPsByProcessGuidStub = func(string) ([]models.ActualLRP, error) {
			return <-<-fakeResponses, nil
		}
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("rate limiting", func() {
		It("returns 503 if the limit is exceeded", func() {
			// hit it once, make fake BBS hang
			hangingResponse := make(chan []models.ActualLRP, 1)
			immediateResponse := make(chan []models.ActualLRP, 1)
			immediateResponse <- []models.ActualLRP{}

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

			Eventually(fakeBBS.ActualLRPsByProcessGuidCallCount).Should(Equal(1))

			// hit it again, assert we get a 503
			resp, err := http.Get(server.URL)
			Ω(err).ShouldNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))

			// un-hang the fake BBS
			hangingResponse <- []models.ActualLRP{}

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
