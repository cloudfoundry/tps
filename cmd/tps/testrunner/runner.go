package testrunner

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/tedsuo/ifrit/ginkgomon"
)

func New(bin string, listenAddr string, diegoAPIURL string, natsAddresses []string, heartbeatInterval time.Duration) *ginkgomon.Runner {
	return ginkgomon.New(ginkgomon.Config{
		Name: "tps",
		Command: exec.Command(
			bin,
			"-diegoAPIURL", diegoAPIURL,
			"-natsAddresses", strings.Join(natsAddresses, ","),
			"-heartbeatInterval", fmt.Sprintf("%s", heartbeatInterval),
			"-listenAddr", listenAddr,
		),
		StartCheck: "tps.started",
	})
}
