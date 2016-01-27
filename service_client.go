package tps

import (
	"time"

	"github.com/cloudfoundry-incubator/consuladapter"
	"github.com/cloudfoundry-incubator/locket"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/ifrit"
)

const TPSWatcherLockSchemaKey = "tps_watcher_lock"

func TPSWatcherLockSchemaPath() string {
	return locket.LockSchemaPath(TPSWatcherLockSchemaKey)
}

type ServiceClient interface {
	NewTPSWatcherLockRunner(logger lager.Logger, bulkerID string, retryInterval, lockTTL time.Duration) ifrit.Runner
}

type serviceClient struct {
	consulClient consuladapter.Client
	clock        clock.Clock
}

func NewServiceClient(consulClient consuladapter.Client, clock clock.Clock) ServiceClient {
	return serviceClient{
		consulClient: consulClient,
		clock:        clock,
	}
}

func (c serviceClient) NewTPSWatcherLockRunner(logger lager.Logger, emitterID string, retryInterval, lockTTL time.Duration) ifrit.Runner {
	return locket.NewLock(logger, c.consulClient, TPSWatcherLockSchemaPath(), []byte(emitterID), c.clock, retryInterval, lockTTL)
}
