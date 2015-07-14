package bulklrpstatus_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/cloudfoundry-incubator/nsync/recipebuilder"
	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/receptor/fake_receptor"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/tps/handler/bulklrpstatus"
	"github.com/pivotal-golang/clock/fakeclock"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
)

var _ = Describe("Bulk Status", func() {
	const authorization = "something good"
	const guid1 = "my-guid1"
	const guid2 = "my-guid2"
	const logGuid1 = "log-guid1"
	const logGuid2 = "log-guid2"

	var (
		handler        http.Handler
		response       *httptest.ResponseRecorder
		request        *http.Request
		receptorClient *fake_receptor.FakeClient
		logger         *lagertest.TestLogger
		fakeClock      *fakeclock.FakeClock
	)

	BeforeEach(func() {
		var err error

		receptorClient = new(fake_receptor.FakeClient)
		logger = lagertest.NewTestLogger("test")
		fakeClock = fakeclock.NewFakeClock(time.Date(2008, 8, 8, 8, 8, 8, 8, time.UTC))
		handler = bulklrpstatus.NewHandler(receptorClient, fakeClock, logger)
		response = httptest.NewRecorder()
		url := "/v1/bulk_actual_lrp_status"
		request, err = http.NewRequest("GET", url, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	JustBeforeEach(func() {
		handler.ServeHTTP(response, request)
	})

	Describe("Validation", func() {
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
				query := request.URL.Query()
				query.Set("guids", fmt.Sprintf("%s,,%s", guid1, guid2))
				request.URL.RawQuery = query.Encode()
			})

			It("fails", func() {
				Expect(response.Code).To(Equal(http.StatusBadRequest))
			})
		})
	})

	Describe("retrieves instance state for lrps specified", func() {
		var expectedSinceTime, actualSinceTime int64

		BeforeEach(func() {
			expectedSinceTime = fakeClock.Now().Unix()
			actualSinceTime = fakeClock.Now().UnixNano()
			fakeClock.Increment(5 * time.Second)

			request.Header.Set("Authorization", authorization)

			query := request.URL.Query()
			query.Set("guids", fmt.Sprintf("%s,%s", guid1, guid2))
			request.URL.RawQuery = query.Encode()

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
			It("returns a map of status per index", func() {
				expectedLRPInstance1 := cc_messages.LRPInstance{
					ProcessGuid:  guid1,
					InstanceGuid: "instanceId",
					Index:        5,
					State:        cc_messages.LRPInstanceStateRunning,
					Since:        expectedSinceTime,
					Uptime:       5,
				}
				expectedLRPInstance2 := cc_messages.LRPInstance{
					ProcessGuid:  guid2,
					InstanceGuid: "instanceId",
					Index:        6,
					State:        cc_messages.LRPInstanceStateRunning,
					Since:        expectedSinceTime,
					Uptime:       5,
				}

				status := make(map[string][]cc_messages.LRPInstance)

				Expect(response.Code).To(Equal(http.StatusOK))
				Expect(response.Header().Get("Content-Type")).To(Equal("application/json"))

				err := json.Unmarshal(response.Body.Bytes(), &status)
				Expect(err).NotTo(HaveOccurred())

				Expect(status[guid1][0]).To(Equal(expectedLRPInstance1))
				Expect(status[guid2][0]).To(Equal(expectedLRPInstance2))
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
				status := make(map[string][]cc_messages.LRPInstance)

				Expect(response.Code).To(Equal(http.StatusOK))
				Expect(response.Header().Get("Content-Type")).To(Equal("application/json"))

				err := json.Unmarshal(response.Body.Bytes(), &status)
				Expect(err).NotTo(HaveOccurred())

				Expect(len(status)).To(Equal(1))
				Expect(status[guid2]).To(BeNil())
				Expect(logger).To(Say("fetching-actual-lrps-info-failed"))
			})
		})
	})
})
