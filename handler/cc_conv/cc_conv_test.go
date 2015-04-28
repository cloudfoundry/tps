package cc_conv

import (
	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CC Conversion Tools", func() {
	Describe("StateFor", func() {
		It("converts state from ActualLRPState to cc_messages LRPInstanceState", func() {
			Expect(StateFor(receptor.ActualLRPStateUnclaimed)).To(Equal(cc_messages.LRPInstanceStateStarting))
			Expect(StateFor(receptor.ActualLRPStateClaimed)).To(Equal(cc_messages.LRPInstanceStateStarting))
			Expect(StateFor(receptor.ActualLRPStateRunning)).To(Equal(cc_messages.LRPInstanceStateRunning))
			Expect(StateFor(receptor.ActualLRPStateCrashed)).To(Equal(cc_messages.LRPInstanceStateCrashed))
			Expect(StateFor("foobar")).To(Equal(cc_messages.LRPInstanceStateUnknown))
		})
	})
})
