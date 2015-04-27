package lrpstatus_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/receptor/fake_receptor"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/runtime-schema/diego_errors"
	"github.com/cloudfoundry-incubator/tps/handler/lrpstatus"
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

		handler := lrpstatus.NewHandler(fakeClient, lagertest.NewTestLogger("test"))
		server = httptest.NewServer(handler)
		fakeResponses = make(chan chan []receptor.ActualLRPResponse, 2)

		fakeClient.ActualLRPsByProcessGuidStub = func(string) ([]receptor.ActualLRPResponse, error) {
			return <-<-fakeResponses, nil
		}
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("Instance state", func() {
		BeforeEach(func() {
			fakeClient.ActualLRPsByProcessGuidStub = func(string) ([]receptor.ActualLRPResponse, error) {
				return []receptor.ActualLRPResponse{
					{Index: 0, State: receptor.ActualLRPStateUnclaimed},
					{Index: 1, State: receptor.ActualLRPStateClaimed},
					{Index: 2, State: receptor.ActualLRPStateRunning},
					{Index: 3, State: receptor.ActualLRPStateCrashed, PlacementError: diego_errors.CELL_MISMATCH_MESSAGE},
				}, nil
			}
		})

		It("returns instance state", func() {
			res, err := http.Get(server.URL)
			Ω(err).ShouldNot(HaveOccurred())

			response := []cc_messages.LRPInstance{}
			err = json.NewDecoder(res.Body).Decode(&response)
			res.Body.Close()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(response).Should(HaveLen(3))
			Ω(response[0].State).Should(Equal(cc_messages.LRPInstanceStateStarting))
			Ω(response[1].State).Should(Equal(cc_messages.LRPInstanceStateRunning))
			Ω(response[2].State).Should(Equal(cc_messages.LRPInstanceStateCrashed))
			Ω(response[2].Details).Should(Equal(diego_errors.CELL_MISMATCH_MESSAGE))
		})
	})
})
