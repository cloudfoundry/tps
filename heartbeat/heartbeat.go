package heartbeat

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/cloudfoundry/gunk/diegonats"
	"github.com/pivotal-golang/lager"
)

const HeatbeatSubject = "service.announce.tps"

type HeartbeatMessage struct {
	Addr string `json:"addr"`
	TTL  uint   `json:"ttl"`
}

type HeartbeatRunner struct {
	natsClient        diegonats.NATSClient
	natsAddresses     string
	natsUsername      string
	natsPassword      string
	heartbeatInterval time.Duration
	serviceAddress    string
	logger            lager.Logger
}

func New(natsClient diegonats.NATSClient, heartbeatInterval time.Duration, serviceAddress string, logger lager.Logger) *HeartbeatRunner {
	heartbeatLogger := logger.Session("heartbeater")
	return &HeartbeatRunner{
		natsClient:        natsClient,
		heartbeatInterval: heartbeatInterval,
		serviceAddress:    serviceAddress,
		logger:            heartbeatLogger,
	}
}

func (hr *HeartbeatRunner) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	ticker := time.NewTicker(hr.heartbeatInterval)
	heartbeatChan := make(chan error)

	close(ready)

	inflight := true
	go hr.heartbeat(heartbeatChan)

	for {
		select {
		case <-signals:
			return nil

		case err := <-heartbeatChan:
			inflight = false
			if err != nil {
				hr.logger.Error("failed", err)
				return err
			}

		case <-ticker.C:
			if inflight == true {
				continue
			}
			inflight = true
			go hr.heartbeat(heartbeatChan)
		}
	}
}

func (hr *HeartbeatRunner) heartbeat(heartbeatChan chan<- error) {
	msg := HeartbeatMessage{
		Addr: hr.serviceAddress,
		TTL:  ttlFromHeartbeatInterval(hr.heartbeatInterval),
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		heartbeatChan <- fmt.Errorf("could not marshal HeartbeatMessage: %s", err)
		return
	}

	hr.logger.Info("will-heartbeat")
	err = hr.natsClient.Publish(HeatbeatSubject, payload)
	if err != nil {
		heartbeatChan <- err
		return
	}

	hr.logger.Info("heartbeat")

	heartbeatChan <- nil
}

func ttlFromHeartbeatInterval(heartbeatInterval time.Duration) uint {
	heartbeatSecs := uint(heartbeatInterval / time.Second)
	if heartbeatSecs == 0 {
		heartbeatSecs = 1
	}
	return heartbeatSecs * 3
}
