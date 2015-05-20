package watcher_test

import (
	"errors"
	"os"
	"sync/atomic"
	"time"

	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/receptor/fake_receptor"
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
	event receptor.Event
}

var nilEventHolder = EventHolder{}

var _ = Describe("Watcher", func() {

	var (
		eventSource    *fake_receptor.FakeEventSource
		receptorClient *fake_receptor.FakeClient
		ccClient       *fakes.FakeCcClient
		watcherRunner  *watcher.Watcher
		process        ifrit.Process

		logger *lagertest.TestLogger

		nextErr   atomic.Value
		nextEvent atomic.Value
	)

	BeforeEach(func() {
		eventSource = new(fake_receptor.FakeEventSource)
		receptorClient = new(fake_receptor.FakeClient)
		receptorClient.SubscribeToEventsReturns(eventSource, nil)

		logger = lagertest.NewTestLogger("test")
		ccClient = new(fakes.FakeCcClient)

		var err error
		watcherRunner, err = watcher.NewWatcher(logger, receptorClient, ccClient)
		Expect(err).NotTo(HaveOccurred())

		nextErr = atomic.Value{}
		nextErr := nextErr
		nextEvent.Store(nilEventHolder)

		eventSource.CloseStub = func() error {
			nextErr.Store(errors.New("closed"))
			return nil
		}

		eventSource.NextStub = func() (receptor.Event, error) {
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
		var before receptor.ActualLRPResponse
		var after receptor.ActualLRPResponse

		BeforeEach(func() {
			before = receptor.ActualLRPResponse{ProcessGuid: "process-guid", InstanceGuid: "instance-guid", Index: 1, Since: 2, CrashCount: 0, Domain: cc_messages.AppLRPDomain}
			after = receptor.ActualLRPResponse{ProcessGuid: "process-guid", InstanceGuid: "instance-guid", Index: 1, Since: 3, CrashCount: 0, Domain: cc_messages.AppLRPDomain}
		})

		JustBeforeEach(func() {
			nextEvent.Store(EventHolder{receptor.NewActualLRPChangedEvent(before, after)})
		})

		Context("when the crash count changes", func() {
			Context("and after > before", func() {
				BeforeEach(func() {
					after.CrashCount = 1
					after.CrashReason = "out of memory"
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
					var otherBefore receptor.ActualLRPResponse
					var otherAfter receptor.ActualLRPResponse

					BeforeEach(func() {
						otherBefore = receptor.ActualLRPResponse{ProcessGuid: "other-process-guid", InstanceGuid: "instance-guid", Index: 1, Since: 2, CrashCount: 0}
						otherAfter = receptor.ActualLRPResponse{ProcessGuid: "other-process-guid", InstanceGuid: "instance-guid", Index: 1, Since: 3, CrashCount: 1}

						event := EventHolder{receptor.NewActualLRPChangedEvent(before, after)}
						otherEvent := EventHolder{receptor.NewActualLRPChangedEvent(otherBefore, otherAfter)}
						events := []EventHolder{otherEvent, event}

						eventSource.NextStub = func() (receptor.Event, error) {
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
					before.CrashCount = 1
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
				nextEvent.Store(EventHolder{receptor.ActualLRPCreatedEvent{}})
			})

			It("does not emit any more messages", func() {
				Consistently(ccClient.AppCrashedCallCount).Should(Equal(0))
			})
		})
	})

	Context("when the event source returns an error", func() {
		var subscribeErr error

		BeforeEach(func() {
			subscribeErr = errors.New("subscribe-error")

			receptorClient.SubscribeToEventsStub = func() (receptor.EventSource, error) {
				if receptorClient.SubscribeToEventsCallCount() == 1 {
					return eventSource, nil
				}
				return nil, subscribeErr
			}

			eventSource.NextStub = func() (receptor.Event, error) {
				return nil, errors.New("next-error")
			}
		})

		It("re-subscribes", func() {
			Eventually(receptorClient.SubscribeToEventsCallCount).Should(BeNumerically(">", 1))
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
