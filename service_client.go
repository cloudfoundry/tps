package tps

import (
	"time"

	"github.com/cloudfoundry-incubator/consuladapter"
	"github.com/cloudfoundry-incubator/locket"
	"github.com/cloudfoundry-incubator/locket/maintainer"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/ifrit"
)

const TPSWatcherLockSchemaKey = "tps_watcher_lock"

func TPSWatcherLockSchemaPath() string {
	return locket.LockSchemaPath(TPSWatcherLockSchemaKey)
}

type ServiceClient interface {
	NewTPSWatcherLockRunner(logger lager.Logger, bulkerID string, retryInterval time.Duration) ifrit.Runner
}

type serviceClient struct {
	session *consuladapter.Session
	clock   clock.Clock
}

func NewServiceClient(session *consuladapter.Session, clock clock.Clock) ServiceClient {
	return serviceClient{session, clock}
}

func (c serviceClient) NewTPSWatcherLockRunner(logger lager.Logger, emitterID string, retryInterval time.Duration) ifrit.Runner {
	return maintainer.NewLock(c.session, TPSWatcherLockSchemaPath(), []byte(emitterID), c.clock, retryInterval, logger)
}
