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

		watcherRunner = watcher.NewWatcher(logger, receptorClient, ccClient)

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
			before = receptor.ActualLRPResponse{ProcessGuid: "process-guid", InstanceGuid: "instance-guid", Index: 1, Since: 2, CrashCount: 0}
			after = receptor.ActualLRPResponse{ProcessGuid: "process-guid", InstanceGuid: "instance-guid", Index: 1, Since: 3, CrashCount: 0}
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

				It("calls AppCrashed", func() {
					Eventually(ccClient.AppCrashedCallCount).Should(Equal(1))
					guid, crashed, _ := ccClient.AppCrashedArgsForCall(0)
					Ω(guid).Should(Equal("process-guid"))
					Ω(crashed).Should(Equal(cc_messages.AppCrashedRequest{
						Instance:        "instance-guid",
						Index:           1,
						Reason:          "CRASHED",
						ExitDescription: "out of memory",
						CrashCount:      1,
						CrashTimestamp:  3,
					}))
				})
			})

			Context("and after < before", func() {
				BeforeEach(func() {
					before.CrashCount = 1
				})

				It("calls AppCrashed", func() {
					Eventually(ccClient.AppCrashedCallCount).Should(Equal(0))
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
