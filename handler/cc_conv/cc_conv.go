package cc_conv

import (
	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
)

func StateFor(state receptor.ActualLRPState) cc_messages.LRPInstanceState {
	switch state {
	case receptor.ActualLRPStateUnclaimed:
		return cc_messages.LRPInstanceStateStarting
	case receptor.ActualLRPStateClaimed:
		return cc_messages.LRPInstanceStateStarting
	case receptor.ActualLRPStateRunning:
		return cc_messages.LRPInstanceStateRunning
	case receptor.ActualLRPStateCrashed:
		return cc_messages.LRPInstanceStateCrashed
	default:
		return cc_messages.LRPInstanceStateUnknown
	}
}
