package bulklrpstats_test

import (
	"encoding/json"
	"errors"
	"fmt"
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
		url := "/v1/actual_lrps_bulk/stats"
		request, err = http.NewRequest("GET", url, nil)
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

			Context("with no process guids", func() {
				It("fails with missing process guids", func() {
					Expect(response.Code).To(Equal(http.StatusBadRequest))
				})
			})

			Context("with malformed process guids", func() {
				BeforeEach(func() {
					request.Form = url.Values{}
					request.Form.Add("guids", fmt.Sprintf("%s,,%s", guid1, guid2))
				})

				It("fails", func() {
					Expect(response.Code).To(Equal(http.StatusBadRequest))
				})
			})
		})
	})

	Describe("retrieve container metrics", func() {
		var expectedSinceTime, actualSinceTime int64

		BeforeEach(func() {
			expectedSinceTime = fakeClock.Now().Unix()
			actualSinceTime = fakeClock.Now().UnixNano()
			fakeClock.Increment(5 * time.Second)

			request.Header.Set("Authorization", authorization)

			query := request.URL.Query()
			query.Set("guids", fmt.Sprintf("%s,%s", guid1, guid2))
			request.URL.RawQuery = query.Encode()

			noaaClient.ContainerMetricsStub = func(appGuid string, authToken string) ([]*events.ContainerMetric, error) {
				if appGuid == logGuid1 {
					return []*events.ContainerMetric{
						{
							ApplicationId: proto.String("appId1"),
							InstanceIndex: proto.Int32(5),
							CpuPercentage: proto.Float64(4),
							MemoryBytes:   proto.Uint64(1024),
							DiskBytes:     proto.Uint64(2048 * 1024),
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
							DiskBytes:     proto.Uint64(2048 * 1024),
						},
					}, nil
				}
				return nil, nil
			}

			receptorClient.GetDesiredLRPStub = func(processGuid string) (receptor.DesiredLRPResponse, error) {
				if processGuid == guid1 {
					return receptor.DesiredLRPResponse{
						LogGuid:     logGuid1,
						ProcessGuid: guid1,
					}, nil
				} else if processGuid == guid2 {
					return receptor.DesiredLRPResponse{
						LogGuid:     logGuid2,
						ProcessGuid: guid2,
					}, nil
				}
				return receptor.DesiredLRPResponse{}, errors.New("UNEXPECTED GUID YO")
			}

			receptorClient.ActualLRPsByProcessGuidStub = func(processGuid string) ([]receptor.ActualLRPResponse, error) {
				if processGuid == guid1 {
					return []receptor.ActualLRPResponse{
						{
							Index:   5,
							State:   receptor.ActualLRPStateRunning,
							Since:   actualSinceTime,
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
					}, nil
				} else if processGuid == guid2 {
					return []receptor.ActualLRPResponse{
						{
							Index:   6,
							State:   receptor.ActualLRPStateRunning,
							Since:   actualSinceTime,
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
					}, nil
				} else {
					return nil, errors.New("WHAT?")
				}
			}
		})

		Context("when the LRPs have been running for a while", func() {
			It("returns a map of stats & status per index in the correct units", func() {
				expectedLRPInstance1 := cc_messages.LRPInstance{
					ProcessGuid:  guid1,
					InstanceGuid: "instanceId",
					Index:        5,
					State:        cc_messages.LRPInstanceStateRunning,
					Host:         "host1",
					Port:         1234,
					Since:        expectedSinceTime,
					Uptime:       5,
					Stats: &cc_messages.LRPInstanceStats{
						Time:          time.Unix(0, 0),
						CpuPercentage: 0.04,
						MemoryBytes:   1024,
						DiskBytes:     1024 * 1024,
					},
				}
				expectedLRPInstance2 := cc_messages.LRPInstance{
					ProcessGuid:  guid2,
					InstanceGuid: "instanceId",
					Index:        6,
					State:        cc_messages.LRPInstanceStateRunning,
					Host:         "host2",
					Port:         1235,
					Since:        expectedSinceTime,
					Uptime:       5,
					Stats: &cc_messages.LRPInstanceStats{
						Time:          time.Unix(0, 0),
						CpuPercentage: 0.04,
						MemoryBytes:   1024,
						DiskBytes:     1024 * 1024,
					},
				}

				stats := make(map[string][]cc_messages.LRPInstance)

				Expect(response.Code).To(Equal(http.StatusOK))
				Expect(response.Header().Get("Content-Type")).To(Equal("application/json"))
				err := json.Unmarshal(response.Body.Bytes(), &stats)
				Expect(err).NotTo(HaveOccurred())
				Expect(stats[guid1][0].Stats.Time).NotTo(BeZero())
				expectedLRPInstance1.Stats.Time = stats[guid1][0].Stats.Time
				expectedLRPInstance2.Stats.Time = stats[guid2][0].Stats.Time
				Expect(stats[guid1][0]).To(Equal(expectedLRPInstance1))
				Expect(stats[guid2][0]).To(Equal(expectedLRPInstance2))
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

		Context("when fetching ContainerMetrics fails", func() {
			BeforeEach(func() {
				noaaClient.ContainerMetricsReturns(nil, errors.New("bad stuff happened"))
			})

			It("responds with empty stats", func() {
				expectedLRPInstances := map[string][]cc_messages.LRPInstance{}
				expectedLRPInstances[guid1] = []cc_messages.LRPInstance{
					{
						ProcessGuid:  guid1,
						InstanceGuid: "instanceId",
						Index:        5,
						State:        cc_messages.LRPInstanceStateRunning,
						Host:         "host1",
						Port:         1234,
						Since:        expectedSinceTime,
						Uptime:       5,
						Stats:        nil,
					},
				}
				expectedLRPInstances[guid2] = []cc_messages.LRPInstance{
					{
						ProcessGuid:  guid2,
						InstanceGuid: "instanceId",
						Index:        6,
						State:        cc_messages.LRPInstanceStateRunning,
						Host:         "host2",
						Port:         1235,
						Since:        expectedSinceTime,
						Uptime:       5,
						Stats:        nil,
					},
				}

				stats := make(map[string][]cc_messages.LRPInstance)
				Expect(response.Code).To(Equal(http.StatusOK))
				Expect(response.Header().Get("Content-Type")).To(Equal("application/json"))
				err := json.Unmarshal(response.Body.Bytes(), &stats)
				Expect(err).NotTo(HaveOccurred())
				Expect(stats).To(Equal(expectedLRPInstances))
			})

			It("logs the failure", func() {
				Expect(logger).To(Say("container-metrics-failed"))
			})
		})

		Context("when fetching one of the desiredLRPs fails", func() {
			BeforeEach(func() {
				receptorClient.GetDesiredLRPStub = func(processGuid string) (receptor.DesiredLRPResponse, error) {
					if processGuid == guid1 {
						return receptor.DesiredLRPResponse{
							LogGuid:     logGuid1,
							ProcessGuid: guid1,
						}, nil
					} else if processGuid == guid2 {
						return receptor.DesiredLRPResponse{}, receptor.Error{Type: receptor.DesiredLRPNotFound}
					}
					return receptor.DesiredLRPResponse{}, errors.New("UNEXPECTED GUID YO")
				}
			})

			It("it is excluded from the result", func() {
				stats := make(map[string][]cc_messages.LRPInstance)

				Expect(response.Code).To(Equal(http.StatusOK))
				Expect(response.Header().Get("Content-Type")).To(Equal("application/json"))

				err := json.Unmarshal(response.Body.Bytes(), &stats)
				Expect(err).NotTo(HaveOccurred())

				Expect(len(stats)).To(Equal(1))
				Expect(stats[guid2]).To(BeNil())
			})
		})

		Context("when fetching one of the actualLRPs fails", func() {
			BeforeEach(func() {
				receptorClient.ActualLRPsByProcessGuidStub = func(processGuid string) ([]receptor.ActualLRPResponse, error) {
					if processGuid == guid1 {
						return []receptor.ActualLRPResponse{
							{
								ProcessGuid: guid1,
							},
						}, nil
					} else if processGuid == guid2 {
						return []receptor.ActualLRPResponse{}, errors.New("boom")
					}
					return []receptor.ActualLRPResponse{}, errors.New("UNEXPECTED GUID YO")
				}
			})

			It("it is excluded from the result and logs the failure", func() {
				stats := make(map[string][]cc_messages.LRPInstance)

				Expect(response.Code).To(Equal(http.StatusOK))
				Expect(response.Header().Get("Content-Type")).To(Equal("application/json"))

				err := json.Unmarshal(response.Body.Bytes(), &stats)
				Expect(err).NotTo(HaveOccurred())

				Expect(len(stats)).To(Equal(1))
				Expect(stats[guid2]).To(BeNil())
				Expect(logger).To(Say("fetching-actual-lrps-info-failed"))
			})
		})
	})
})
