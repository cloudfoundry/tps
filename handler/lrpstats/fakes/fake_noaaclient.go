// This file was generated by counterfeiter
package fakes

import (
	"sync"

	"github.com/cloudfoundry-incubator/tps/handler/lrpstats"
	"github.com/cloudfoundry/noaa/events"
)

type FakeNoaaClient struct {
	ContainerMetricsStub        func(appGuid string, authToken string) ([]*events.ContainerMetric, error)
	containerMetricsMutex       sync.RWMutex
	containerMetricsArgsForCall []struct {
		appGuid   string
		authToken string
	}
	containerMetricsReturns struct {
		result1 []*events.ContainerMetric
		result2 error
	}
	CloseStub        func() error
	closeMutex       sync.RWMutex
	closeArgsForCall []struct{}
	closeReturns struct {
		result1 error
	}
}

func (fake *FakeNoaaClient) ContainerMetrics(appGuid string, authToken string) ([]*events.ContainerMetric, error) {
	fake.containerMetricsMutex.Lock()
	fake.containerMetricsArgsForCall = append(fake.containerMetricsArgsForCall, struct {
		appGuid   string
		authToken string
	}{appGuid, authToken})
	fake.containerMetricsMutex.Unlock()
	if fake.ContainerMetricsStub != nil {
		return fake.ContainerMetricsStub(appGuid, authToken)
	} else {
		return fake.containerMetricsReturns.result1, fake.containerMetricsReturns.result2
	}
}

func (fake *FakeNoaaClient) ContainerMetricsCallCount() int {
	fake.containerMetricsMutex.RLock()
	defer fake.containerMetricsMutex.RUnlock()
	return len(fake.containerMetricsArgsForCall)
}

func (fake *FakeNoaaClient) ContainerMetricsArgsForCall(i int) (string, string) {
	fake.containerMetricsMutex.RLock()
	defer fake.containerMetricsMutex.RUnlock()
	return fake.containerMetricsArgsForCall[i].appGuid, fake.containerMetricsArgsForCall[i].authToken
}

func (fake *FakeNoaaClient) ContainerMetricsReturns(result1 []*events.ContainerMetric, result2 error) {
	fake.ContainerMetricsStub = nil
	fake.containerMetricsReturns = struct {
		result1 []*events.ContainerMetric
		result2 error
	}{result1, result2}
}

func (fake *FakeNoaaClient) Close() error {
	fake.closeMutex.Lock()
	fake.closeArgsForCall = append(fake.closeArgsForCall, struct{}{})
	fake.closeMutex.Unlock()
	if fake.CloseStub != nil {
		return fake.CloseStub()
	} else {
		return fake.closeReturns.result1
	}
}

func (fake *FakeNoaaClient) CloseCallCount() int {
	fake.closeMutex.RLock()
	defer fake.closeMutex.RUnlock()
	return len(fake.closeArgsForCall)
}

func (fake *FakeNoaaClient) CloseReturns(result1 error) {
	fake.CloseStub = nil
	fake.closeReturns = struct {
		result1 error
	}{result1}
}

var _ lrpstats.NoaaClient = new(FakeNoaaClient)
