package testrunner

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/tedsuo/ifrit/ginkgomon"
)

func New(bin string, listenAddr string, etcdCluster []string, natsAddresses []string, heartbeatInterval time.Duration) *ginkgomon.Runner {
	return ginkgomon.New(ginkgomon.Config{
		Name: "tps",
		Command: exec.Command(
			bin,
			"-etcdCluster", strings.Join(etcdCluster, ","),
			"-natsAddresses", strings.Join(natsAddresses, ","),
			"-heartbeatInterval", fmt.Sprintf("%s", heartbeatInterval),
			"-listenAddr", listenAddr,
		),
		StartCheck: "tps.started",
	})
}
