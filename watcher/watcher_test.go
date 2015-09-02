package watcher_test

import (
	"errors"
	"os"
	"sync/atomic"
	"time"

	"github.com/cloudfoundry-incubator/bbs/events"
	"github.com/cloudfoundry-incubator/bbs/events/eventfakes"
	"github.com/cloudfoundry-incubator/bbs/fake_bbs"
	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/bbs/models/test/model_helpers"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/tps/cc_client/fakes"
	"github.com/cloudfoundry-incubator/tps/watcher"
	"github.com/pivotal-golang/lager/lagertest"
	"github.com/tedsuo/ifrit"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
)

type EventHolder struct {
	event models.Event
}

var nilEventHolder = EventHolder{}

var _ = Describe("Watcher", func() {

	var (
		eventSource   *eventfakes.FakeEventSource
		bbsClient     *fake_bbs.FakeClient
		ccClient      *fakes.FakeCcClient
		watcherRunner *watcher.Watcher
		process       ifrit.Process

		logger *lagertest.TestLogger

		nextErr   atomic.Value
		nextEvent atomic.Value
	)

	BeforeEach(func() {
		eventSource = new(eventfakes.FakeEventSource)
		bbsClient = new(fake_bbs.FakeClient)
		bbsClient.SubscribeToEventsReturns(eventSource, nil)

		logger = lagertest.NewTestLogger("test")
		ccClient = new(fakes.FakeCcClient)

		var err error
		watcherRunner, err = watcher.NewWatcher(logger, bbsClient, ccClient)
		Expect(err).NotTo(HaveOccurred())

		nextErr = atomic.Value{}
		nextErr := nextErr
		nextEvent.Store(nilEventHolder)

		eventSource.CloseStub = func() error {
			nextErr.Store(errors.New("closed"))
			return nil
		}

		eventSource.NextStub = func() (models.Event, error) {
			time.Sleep(10 * time.Millisecond)
			if eventHolder := nextEvent.Load(); eventHolder != nilEventHolder {
				nextEvent.Store(nilEventHolder)

				eh := eventHolder.(EventHolder)
				if eh.event != nil {
					return eh.event, nil
				}
			}

			if err := nextErr.Load(); err != nil {
				return nil, err.(error)
			}

			return nil, nil
		}
	})

	JustBeforeEach(func() {
		process = ifrit.Invoke(watcherRunner)
	})

	AfterEach(func() {
		process.Signal(os.Interrupt)
		Eventually(process.Wait()).Should(Receive())
	})

	Describe("Actual LRP changes", func() {
		var before *models.ActualLRPGroup
		var after *models.ActualLRPGroup

		BeforeEach(func() {
			before = makeActualLRPGroup("process-guid", "instance-guid", 1, 2, 0, cc_messages.AppLRPDomain)
			after = makeActualLRPGroup("process-guid", "instance-guid", 1, 3, 0, cc_messages.AppLRPDomain)
		})

		JustBeforeEach(func() {
			nextEvent.Store(EventHolder{models.NewActualLRPChangedEvent(before, after)})
		})

		Context("when the crash count changes", func() {
			Context("and after > before", func() {
				BeforeEach(func() {
					after.Instance.CrashCount = 1
					after.Instance.CrashReason = "out of memory"
				})

				Context("and the application has the cc-app Domain", func() {
					It("calls AppCrashed", func() {
						Eventually(ccClient.AppCrashedCallCount).Should(Equal(1))
						guid, crashed, _ := ccClient.AppCrashedArgsForCall(0)
						Expect(guid).To(Equal("process-guid"))
						Expect(crashed).To(Equal(cc_messages.AppCrashedRequest{
							Instance:        "instance-guid",
							Index:           1,
							Reason:          "CRASHED",
							ExitDescription: "out of memory",
							CrashCount:      1,
							CrashTimestamp:  3,
						}))

						Expect(logger).To(Say("app-crashed"))
					})
				})

				Context("and the application does not have the cc-app Domain", func() {
					var otherBefore *models.ActualLRPGroup
					var otherAfter *models.ActualLRPGroup

					BeforeEach(func() {
						otherBefore = makeActualLRPGroup("other-process-guid", "instance-guid", 1, 2, 0, "")
						otherAfter = makeActualLRPGroup("other-process-guid", "instance-guid", 1, 3, 1, "")

						event := EventHolder{models.NewActualLRPChangedEvent(before, after)}
						otherEvent := EventHolder{models.NewActualLRPChangedEvent(otherBefore, otherAfter)}
						events := []EventHolder{otherEvent, event}

						eventSource.NextStub = func() (models.Event, error) {
							var e EventHolder
							time.Sleep(10 * time.Millisecond)
							if len(events) == 0 {
								return nil, nil
							}
							e, events = events[0], events[1:]
							return e.event, nil
						}
					})

					It("does not call AppCrashed", func() {
						Eventually(ccClient.AppCrashedCallCount).Should(Equal(1))
						buffer := logger.Buffer()
						Expect(buffer).To(Say("process-guid"))
						Expect(buffer).NotTo(Say("other-process-guid"))
					})
				})
			})

			Context("and after < before", func() {
				BeforeEach(func() {
					before.Instance.CrashCount = 1
				})

				It("does not call AppCrashed", func() {
					Consistently(ccClient.AppCrashedCallCount).Should(Equal(0))
				})
			})
		})

		Context("when the crash count does not change", func() {
			It("does not call AppCrashed", func() {
				Eventually(ccClient.AppCrashedCallCount).Should(Equal(0))
			})
		})
	})

	Describe("Unrecognized events", func() {
		Context("when its not ActualLRPChanged event", func() {

			BeforeEach(func() {
				nextEvent.Store(EventHolder{&models.ActualLRPCreatedEvent{}})
			})

			It("does not emit any more messages", func() {
				Consistently(ccClient.AppCrashedCallCount).Should(Equal(0))
			})
		})
	})

	Context("when the event source returns an error", func() {
		var subscribeErr error

		BeforeEach(func() {
			subscribeErr = models.ErrUnknownError

			bbsClient.SubscribeToEventsStub = func() (events.EventSource, error) {
				if bbsClient.SubscribeToEventsCallCount() == 1 {
					return eventSource, nil
				}
				return nil, subscribeErr
			}

			eventSource.NextStub = func() (models.Event, error) {
				return nil, errors.New("next-error")
			}
		})

		It("re-subscribes", func() {
			Eventually(bbsClient.SubscribeToEventsCallCount, 2*time.Second).Should(BeNumerically(">", 1))
		})

		Context("when re-subscribing fails", func() {
			It("retries", func() {
				Consistently(process.Wait()).ShouldNot(Receive())
			})
		})
	})

	Describe("interrupting the process", func() {
		It("should be possible to SIGINT the route emitter", func() {
			process.Signal(os.Interrupt)
			Eventually(process.Wait()).Should(Receive())
		})
	})

})

func makeActualLRPGroup(processGuid, instanceGuid string, index, since, crashCount int32, domain string) *models.ActualLRPGroup {
	lrp := model_helpers.NewValidActualLRP(processGuid, index)
	lrp.InstanceGuid = instanceGuid
	lrp.Since = int64(since)
	lrp.CrashCount = crashCount
	lrp.Domain = domain

	return &models.ActualLRPGroup{Instance: lrp}
}
