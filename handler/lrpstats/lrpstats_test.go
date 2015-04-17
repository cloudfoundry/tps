package lrpstats_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/receptor/fake_receptor"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/tps/handler/lrpstats"
	"github.com/cloudfoundry-incubator/tps/handler/lrpstats/fakes"
	"github.com/cloudfoundry/noaa/events"
	"github.com/gogo/protobuf/proto"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
)

var _ = Describe("Stats", func() {
	const authorization = "something good"
	const guid = "my-guid"

	var (
		handler        http.Handler
		response       *httptest.ResponseRecorder
		request        *http.Request
		noaaClient     *fakes.FakeNoaaClient
		receptorClient *fake_receptor.FakeClient
		logger         *lagertest.TestLogger
	)

	BeforeEach(func() {
		var err error

		receptorClient = new(fake_receptor.FakeClient)
		noaaClient = &fakes.FakeNoaaClient{}
		logger = lagertest.NewTestLogger("test")
		handler = lrpstats.NewHandler(receptorClient, noaaClient, logger)
		response = httptest.NewRecorder()
		request, err = http.NewRequest("GET", "/v1/actual_lrps/:guid/stats", nil)
		Ω(err).ShouldNot(HaveOccurred())
	})

	JustBeforeEach(func() {
		handler.ServeHTTP(response, request)
	})

	Describe("Validation", func() {
		It("fails with a missing authorization header", func() {
			Ω(response.Code).Should(Equal(http.StatusUnauthorized))
		})

		Context("with an authorization header", func() {
			BeforeEach(func() {
				request.Header.Set("Authorization", authorization)
			})

			It("fails with no guid", func() {
				Ω(response.Code).Should(Equal(http.StatusBadRequest))
			})
		})
	})

	Describe("retrieve container metrics", func() {
		BeforeEach(func() {
			request.Header.Set("Authorization", authorization)
			request.Form = url.Values{}
			request.Form.Add(":guid", guid)

			noaaClient.ContainerMetricsReturns([]*events.ContainerMetric{
				{
					ApplicationId: proto.String("appId"),
					InstanceIndex: proto.Int32(5),
					CpuPercentage: proto.Float64(4),
					MemoryBytes:   proto.Uint64(1024),
					DiskBytes:     proto.Uint64(2048),
				},
			}, nil)

			receptorClient.ActualLRPsByProcessGuidReturns([]receptor.ActualLRPResponse{
				{
					Index:        5,
					State:        receptor.ActualLRPStateRunning,
					Since:        124578,
					InstanceGuid: "instanceId",
					ProcessGuid:  "appId",
				},
			}, nil)
		})

		It("returns a map of stats & status per index", func() {
			expectedLRPInstance := cc_messages.LRPInstance{
				ProcessGuid:  "appId",
				InstanceGuid: "instanceId",
				Index:        5,
				State:        cc_messages.LRPInstanceStateRunning,
				Since:        124578,
				Stats: &cc_messages.LRPInstanceStats{
					CpuPercentage: 4,
					MemoryBytes:   1024,
					DiskBytes:     2048,
				},
			}
			var stats []cc_messages.LRPInstance

			Ω(response.Code).Should(Equal(http.StatusOK))
			Ω(response.Header().Get("Content-Type")).Should(Equal("application/json"))
			err := json.Unmarshal(response.Body.Bytes(), &stats)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(stats).Should(ConsistOf(expectedLRPInstance))
		})

		It("calls ContainerMetrics", func() {
			Ω(noaaClient.ContainerMetricsCallCount()).Should(Equal(1))
			guid, token := noaaClient.ContainerMetricsArgsForCall(0)
			Ω(guid).Should(Equal(guid))
			Ω(token).Should(Equal(authorization))
		})

		Context("when ContainerMetrics fails", func() {
			BeforeEach(func() {
				noaaClient.ContainerMetricsReturns(nil, errors.New("bad stuff happened"))
			})

			It("responds with a 500", func() {
				Ω(response.Code).Should(Equal(http.StatusInternalServerError))
			})

			It("logs the failure", func() {
				Ω(logger).Should(Say("container-metrics-failed"))
			})
		})

		Context("when fetching actualLRPs fails", func() {
			BeforeEach(func() {
				receptorClient.ActualLRPsByProcessGuidReturns(nil, errors.New("bad stuff happened"))
			})

			It("responds with a 500", func() {
				Ω(response.Code).Should(Equal(http.StatusInternalServerError))
			})

			It("logs the failure", func() {
				Ω(logger).Should(Say("fetching-actual-lrp-info-failed"))
			})
		})

		It("calls Close", func() {
			Ω(noaaClient.CloseCallCount()).Should(Equal(1))
		})

		Context("when Close fails", func() {
			BeforeEach(func() {
				noaaClient.CloseReturns(errors.New("you failed"))
			})

			It("ignores the error and returns a 200", func() {
				Ω(response.Code).Should(Equal(200))
			})
		})
	})
})
