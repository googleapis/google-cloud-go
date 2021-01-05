// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and

package wire

import (
	"fmt"
	"testing"
	"time"
)

func testReceiveSettings() ReceiveSettings {
	settings := DefaultReceiveSettings
	settings.Timeout = 5 * time.Second
	return settings
}

const serviceTestWaitTimeout = 30 * time.Second

// serviceTestProxy wraps a `service` and provides some convenience methods for
// testing.
type serviceTestProxy struct {
	t          *testing.T
	service    service
	name       string
	started    chan struct{}
	terminated chan struct{}
}

func (sp *serviceTestProxy) initAndStart(t *testing.T, s service, name string) {
	sp.t = t
	sp.service = s
	sp.name = name
	sp.started = make(chan struct{})
	sp.terminated = make(chan struct{})
	s.AddStatusChangeReceiver(nil, sp.onStatusChange)
	s.Start()
}

func (sp *serviceTestProxy) onStatusChange(_ serviceHandle, status serviceStatus, _ error) {
	if status == serviceActive {
		close(sp.started)
	}
	if status == serviceTerminated {
		close(sp.terminated)
	}
}

func (sp *serviceTestProxy) Start() { sp.service.Start() }
func (sp *serviceTestProxy) Stop()  { sp.service.Stop() }

// StartError waits for the service to start and returns the error.
func (sp *serviceTestProxy) StartError() error {
	select {
	case <-time.After(serviceTestWaitTimeout):
		return fmt.Errorf("%s did not start within %v", sp.name, serviceTestWaitTimeout)
	case <-sp.terminated:
		return sp.service.Error()
	case <-sp.started:
		return sp.service.Error()
	}
}

// FinalError waits for the service to terminate and returns the error.
func (sp *serviceTestProxy) FinalError() error {
	select {
	case <-time.After(serviceTestWaitTimeout):
		return fmt.Errorf("%s did not terminate within %v", sp.name, serviceTestWaitTimeout)
	case <-sp.terminated:
		return sp.service.Error()
	}
}

// StopVerifyNoError stops the service, waits for it to terminate and verifies
// that there is no error.
func (sp *serviceTestProxy) StopVerifyNoError() {
	sp.service.Stop()
	if gotErr := sp.FinalError(); gotErr != nil {
		sp.t.Errorf("%s final err: (%v), want: <nil>", sp.name, gotErr)
	}
}
