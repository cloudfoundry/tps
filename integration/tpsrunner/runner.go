package tpsrunner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
)

type runner struct {
	bin         string
	listenAddr  string
	etcdCluster []string
}

func New(bin string, listenAddr string, etcdCluster []string) ifrit.Runner {
	return &runner{
		bin:         bin,
		listenAddr:  listenAddr,
		etcdCluster: etcdCluster,
	}
}

func (r *runner) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	session, err := gexec.Start(
		exec.Command(
			r.bin,
			"-etcdCluster", strings.Join(r.etcdCluster, ","),
			"-listenAddr", r.listenAddr,
		),
		ginkgo.GinkgoWriter,
		ginkgo.GinkgoWriter,
	)
	if err != nil {
		return err
	}

	gomega.Eventually(session).Should(gbytes.Say("tps.started"))

	close(ready)

dance:
	for {
		select {
		case sig := <-signals:
			session.Signal(sig)
		case <-session.Exited:
			break dance
		}
	}

	if session.ExitCode() == 0 {
		return nil
	}

	return fmt.Errorf("exit status %d", session.ExitCode())
}
