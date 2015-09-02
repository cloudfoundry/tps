package cc_conv

import (
	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
)

func StateFor(state string) cc_messages.LRPInstanceState {
	switch state {
	case models.ActualLRPStateUnclaimed:
		return cc_messages.LRPInstanceStateStarting
	case models.ActualLRPStateClaimed:
		return cc_messages.LRPInstanceStateStarting
	case models.ActualLRPStateRunning:
		return cc_messages.LRPInstanceStateRunning
	case models.ActualLRPStateCrashed:
		return cc_messages.LRPInstanceStateCrashed
	default:
		return cc_messages.LRPInstanceStateUnknown
	}
}
