package cc_client_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestCcClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CcClient Suite")
}
