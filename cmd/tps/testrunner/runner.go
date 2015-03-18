package testrunner

import (
	"os/exec"

	"github.com/tedsuo/ifrit/ginkgomon"
)

func New(bin string, listenAddr string, diegoAPIURL string, ccBaseURL string) *ginkgomon.Runner {
	return ginkgomon.New(ginkgomon.Config{
		Name: "tps",
		Command: exec.Command(
			bin,
			"-diegoAPIURL", diegoAPIURL,
			"-listenAddr", listenAddr,
			"-ccBaseURL", ccBaseURL,
		),
		StartCheck: "tps.started",
	})
}
