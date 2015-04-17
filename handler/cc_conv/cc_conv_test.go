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
			Ω(StateFor(receptor.ActualLRPStateUnclaimed)).Should(Equal(cc_messages.LRPInstanceStateStarting))
			Ω(StateFor(receptor.ActualLRPStateClaimed)).Should(Equal(cc_messages.LRPInstanceStateStarting))
			Ω(StateFor(receptor.ActualLRPStateRunning)).Should(Equal(cc_messages.LRPInstanceStateRunning))
			Ω(StateFor(receptor.ActualLRPStateCrashed)).Should(Equal(cc_messages.LRPInstanceStateCrashed))
			Ω(StateFor("foobar")).Should(Equal(cc_messages.LRPInstanceStateUnknown))
		})
	})
})
