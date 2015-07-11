package bulklrpstats_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/cloudfoundry-incubator/nsync/recipebuilder"
	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/receptor/fake_receptor"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/tps/handler/bulklrpstats"
	"github.com/cloudfoundry-incubator/tps/handler/lrpstats/fakes"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gogo/protobuf/proto"
	"github.com/pivotal-golang/clock/fakeclock"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
)

var _ = Describe("Stats", func() {
	const authorization = "something good"
	const guid1 = "my-guid1"
	const guid2 = "my-guid2"
	const logGuid1 = "log-guid1"
	const logGuid2 = "log-guid2"

	var (
		handler        http.Handler
		response       *httptest.ResponseRecorder
		request        *http.Request
		noaaClient     *fakes.FakeNoaaClient
		receptorClient *fake_receptor.FakeClient
		logger         *lagertest.TestLogger
		fakeClock      *fakeclock.FakeClock
	)

	BeforeEach(func() {
		var err error

		receptorClient = new(fake_receptor.FakeClient)
		noaaClient = &fakes.FakeNoaaClient{}
		logger = lagertest.NewTestLogger("test")
		fakeClock = fakeclock.NewFakeClock(time.Date(2008, 8, 8, 8, 8, 8, 8, time.UTC))
		handler = bulklrpstats.NewHandler(receptorClient, noaaClient, fakeClock, logger)
		response = httptest.NewRecorder()
		request, err = http.NewRequest("GET", "/v1/actual_lrps_bulk/stats", nil)
		Expect(err).NotTo(HaveOccurred())
	})

	JustBeforeEach(func() {
		handler.ServeHTTP(response, request)
	})

	Describe("Validation", func() {
		It("fails with a missing authorization header", func() {
			Expect(response.Code).To(Equal(http.StatusUnauthorized))
		})

		Context("with an authorization header", func() {
			BeforeEach(func() {
				request.Header.Set("Authorization", authorization)
			})
		})
	})

	Describe("retrieve container metrics", func() {
		BeforeEach(func() {
			request.Header.Set("Authorization", authorization)
			request.Form = url.Values{}

			noaaClient.ContainerMetricsStub = func(appGuid string, authToken string) ([]*events.ContainerMetric, error) {
				if appGuid == logGuid1 {
					return []*events.ContainerMetric{
						{
							ApplicationId: proto.String("appId1"),
							InstanceIndex: proto.Int32(5),
							CpuPercentage: proto.Float64(4),
							MemoryBytes:   proto.Uint64(1024),
							DiskBytes:     proto.Uint64(2048),
						},
					}, nil
				}
				if appGuid == logGuid2 {
					return []*events.ContainerMetric{
						{
							ApplicationId: proto.String("appId2"),
							InstanceIndex: proto.Int32(6),
							CpuPercentage: proto.Float64(4),
							MemoryBytes:   proto.Uint64(1024),
							DiskBytes:     proto.Uint64(2048),
						},
					}, nil
				}
				return nil, nil
			}

			receptorClient.DesiredLRPsReturns([]receptor.DesiredLRPResponse{
				{
					LogGuid:     logGuid1,
					ProcessGuid: guid1,
				},
				{
					LogGuid:     logGuid2,
					ProcessGuid: guid2,
				},
			}, nil)

			receptorClient.ActualLRPsReturns([]receptor.ActualLRPResponse{
				{
					Index:   5,
					State:   receptor.ActualLRPStateRunning,
					Since:   fakeClock.Now().UnixNano(),
					Address: "host1",
					Ports: []receptor.PortMapping{
						{
							ContainerPort: 7890,
							HostPort:      5432,
						},
						{
							ContainerPort: recipebuilder.DefaultPort,
							HostPort:      1234,
						}},
					InstanceGuid: "instanceId",
					ProcessGuid:  guid1,
				},
				{
					Index:   6,
					State:   receptor.ActualLRPStateRunning,
					Since:   fakeClock.Now().UnixNano(),
					Address: "host2",
					Ports: []receptor.PortMapping{
						{
							ContainerPort: 7891,
							HostPort:      5433,
						},
						{
							ContainerPort: recipebuilder.DefaultPort,
							HostPort:      1235,
						}},
					InstanceGuid: "instanceId",
					ProcessGuid:  guid2,
				},
			}, nil)
		})

		Context("when the LRPs have been running for a while", func() {
			var expectedSinceTime int64

			BeforeEach(func() {
				expectedSinceTime = fakeClock.Now().Unix()
				fakeClock.Increment(5 * time.Second)
			})

			It("returns a map of stats & status per index in the correct units", func() {
				expectedLRPInstances := []cc_messages.LRPInstance{
					{
						ProcessGuid:  guid1,
						InstanceGuid: "instanceId",
						Index:        5,
						State:        cc_messages.LRPInstanceStateRunning,
						Host:         "host",
						Port:         1234,
						Since:        expectedSinceTime,
						Uptime:       5,
						Stats: &cc_messages.LRPInstanceStats{
							Time:          time.Unix(0, 0),
							CpuPercentage: 0.04,
							MemoryBytes:   1024,
							DiskBytes:     2048,
						},
					},
					{
						ProcessGuid:  guid2,
						InstanceGuid: "instanceId",
						Index:        6,
						State:        cc_messages.LRPInstanceStateRunning,
						Host:         "host",
						Port:         1235,
						Since:        expectedSinceTime,
						Uptime:       5,
						Stats: &cc_messages.LRPInstanceStats{
							Time:          time.Unix(0, 0),
							CpuPercentage: 0.04,
							MemoryBytes:   1024,
							DiskBytes:     2048,
						},
					},
				}
				var stats []cc_messages.LRPInstance

				Expect(response.Code).To(Equal(http.StatusOK))
				Expect(response.Header().Get("Content-Type")).To(Equal("application/json"))
				err := json.Unmarshal(response.Body.Bytes(), &stats)
				Expect(err).NotTo(HaveOccurred())
				Expect(stats[0].Stats.Time).NotTo(BeZero())
				expectedLRPInstances[0].Stats.Time = stats[0].Stats.Time
				expectedLRPInstances[1].Stats.Time = stats[0].Stats.Time
				Expect(stats).To(ConsistOf(expectedLRPInstances))
			})
		})

		It("calls ContainerMetrics", func() {
			Expect(noaaClient.ContainerMetricsCallCount()).To(Equal(2))
			expectedLogGuid1, token := noaaClient.ContainerMetricsArgsForCall(0)
			expectedLogGuid2, token := noaaClient.ContainerMetricsArgsForCall(1)
			Expect(expectedLogGuid1).To(Equal(logGuid1))
			Expect(expectedLogGuid2).To(Equal(logGuid2))
			Expect(token).To(Equal(authorization))
		})

		Context("when ContainerMetrics fails", func() {
			BeforeEach(func() {
				noaaClient.ContainerMetricsReturns(nil, errors.New("bad stuff happened"))
			})

			It("responds with empty stats", func() {
				expectedLRPInstances := []cc_messages.LRPInstance{
					{
						ProcessGuid:  guid1,
						InstanceGuid: "instanceId",
						Index:        5,
						State:        cc_messages.LRPInstanceStateRunning,
						Host:         "host",
						Port:         1234,
						Since:        fakeClock.Now().Unix(),
						Uptime:       5,
						Stats:        nil,
					},
					{
						ProcessGuid:  guid2,
						InstanceGuid: "instanceId",
						Index:        6,
						State:        cc_messages.LRPInstanceStateRunning,
						Host:         "host",
						Port:         1235,
						Since:        fakeClock.Now().Unix(),
						Uptime:       5,
						Stats:        nil,
					},
				}

				var stats []cc_messages.LRPInstance
				Expect(response.Code).To(Equal(http.StatusOK))
				Expect(response.Header().Get("Content-Type")).To(Equal("application/json"))
				err := json.Unmarshal(response.Body.Bytes(), &stats)
				Expect(err).NotTo(HaveOccurred())
				Expect(stats).To(ConsistOf(expectedLRPInstances))
			})

			It("logs the failure", func() {
				Expect(logger).To(Say("container-metrics-failed"))
			})
		})

		Context("when fetching the desiredLRPs fails", func() {
			Context("when the desiredLRPs are not found", func() {
				BeforeEach(func() {
					receptorClient.DesiredLRPsReturns([]receptor.DesiredLRPResponse{}, receptor.Error{Type: receptor.DesiredLRPNotFound})
				})

				It("responds with a 404", func() {
					Expect(response.Code).To(Equal(http.StatusNotFound))
				})
			})

			Context("when another type of error occurs", func() {
				BeforeEach(func() {
					receptorClient.DesiredLRPsReturns([]receptor.DesiredLRPResponse{}, errors.New("some error"))
				})

				It("responds with a 500", func() {
					Expect(response.Code).To(Equal(http.StatusInternalServerError))
				})
			})
		})

		Context("when fetching actualLRPs fails", func() {
			BeforeEach(func() {
				receptorClient.ActualLRPsReturns(nil, errors.New("bad stuff happened"))
			})

			It("responds with a 500", func() {
				Expect(response.Code).To(Equal(http.StatusInternalServerError))
			})

			It("logs the failure", func() {
				Expect(logger).To(Say("fetching-actual-lrps-info-failed"))
			})
		})
	})
})
