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
	"errors"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/pubsublite/internal/test"
)

const receiveStatusTimeout = 5 * time.Second

type testStatusChangeReceiver struct {
	// Status change notifications are fired asynchronously, so a channel receives
	// the statuses.
	statusC    chan serviceStatus
	lastStatus serviceStatus
	name       string
}

func newTestStatusChangeReceiver(name string) *testStatusChangeReceiver {
	return &testStatusChangeReceiver{
		statusC: make(chan serviceStatus, 1),
		name:    name,
	}
}

func (sr *testStatusChangeReceiver) Handle() interface{} { return sr }

func (sr *testStatusChangeReceiver) OnStatusChange(handle serviceHandle, status serviceStatus, err error) {
	sr.statusC <- status
}

func (sr *testStatusChangeReceiver) VerifyStatus(t *testing.T, want serviceStatus) {
	select {
	case status := <-sr.statusC:
		if status <= sr.lastStatus {
			t.Errorf("%s: Duplicate service status: %d, last status: %d", sr.name, status, sr.lastStatus)
		}
		if status != want {
			t.Errorf("%s: Got service status: %d, want: %d", sr.name, status, want)
		}
		sr.lastStatus = status
	case <-time.After(receiveStatusTimeout):
		t.Errorf("%s: Did not receive status within %v", sr.name, receiveStatusTimeout)
	}
}

func (sr *testStatusChangeReceiver) VerifyNoStatusChanges(t *testing.T) {
	select {
	case status := <-sr.statusC:
		t.Errorf("%s: Unexpected service status: %d", sr.name, status)
	default:
	}
}

type testService struct {
	receiver *testStatusChangeReceiver
	abstractService
}

func newTestService(name string) *testService {
	receiver := newTestStatusChangeReceiver(name)
	ts := &testService{receiver: receiver}
	ts.AddStatusChangeReceiver(receiver.Handle(), receiver.OnStatusChange)
	return ts
}

func (ts *testService) Start() { ts.UpdateStatus(serviceStarting, nil) }
func (ts *testService) Stop()  { ts.UpdateStatus(serviceTerminating, nil) }

func (ts *testService) UpdateStatus(targetStatus serviceStatus, err error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.unsafeUpdateStatus(targetStatus, err)
}

func TestServiceUpdateStatusIsLinear(t *testing.T) {
	err1 := errors.New("error1")
	err2 := errors.New("error2")

	service := newTestService("service")
	service.UpdateStatus(serviceStarting, nil)
	service.receiver.VerifyStatus(t, serviceStarting)

	service.UpdateStatus(serviceActive, nil)
	service.UpdateStatus(serviceActive, nil)
	service.receiver.VerifyStatus(t, serviceActive)

	service.UpdateStatus(serviceTerminating, err1)
	service.UpdateStatus(serviceStarting, nil)
	service.UpdateStatus(serviceTerminating, nil)
	service.receiver.VerifyStatus(t, serviceTerminating)

	service.UpdateStatus(serviceTerminated, err2)
	service.UpdateStatus(serviceTerminated, nil)
	service.receiver.VerifyStatus(t, serviceTerminated)

	// Verify that the first error is not clobbered by the second.
	if got, want := service.Error(), err1; !test.ErrorEqual(got, want) {
		t.Errorf("service.Error(): got (%v), want (%v)", got, want)
	}
}

func TestServiceCheckServiceStatus(t *testing.T) {
	for _, tc := range []struct {
		status  serviceStatus
		wantErr error
	}{
		{
			status:  serviceUninitialized,
			wantErr: ErrServiceUninitialized,
		},
		{
			status:  serviceStarting,
			wantErr: ErrServiceStarting,
		},
		{
			status:  serviceActive,
			wantErr: nil,
		},
		{
			status:  serviceTerminating,
			wantErr: ErrServiceStopped,
		},
		{
			status:  serviceTerminated,
			wantErr: ErrServiceStopped,
		},
	} {
		t.Run(fmt.Sprintf("Status=%v", tc.status), func(t *testing.T) {
			s := newTestService("service")
			s.UpdateStatus(tc.status, nil)
			if gotErr := s.unsafeCheckServiceStatus(); !test.ErrorEqual(gotErr, tc.wantErr) {
				t.Errorf("service.unsafeCheckServiceStatus(): got (%v), want (%v)", gotErr, tc.wantErr)
			}
		})
	}
}

func TestServiceAddRemoveStatusChangeReceiver(t *testing.T) {
	receiver1 := newTestStatusChangeReceiver("receiver1")
	receiver2 := newTestStatusChangeReceiver("receiver2")
	receiver3 := newTestStatusChangeReceiver("receiver3")

	service := new(testService)
	service.AddStatusChangeReceiver(receiver1.Handle(), receiver1.OnStatusChange)
	service.AddStatusChangeReceiver(receiver2.Handle(), receiver2.OnStatusChange)
	service.AddStatusChangeReceiver(receiver3.Handle(), receiver3.OnStatusChange)

	t.Run("All receivers", func(t *testing.T) {
		service.UpdateStatus(serviceActive, nil)

		receiver1.VerifyStatus(t, serviceActive)
		receiver2.VerifyStatus(t, serviceActive)
		receiver3.VerifyStatus(t, serviceActive)
	})

	t.Run("receiver1 removed", func(t *testing.T) {
		service.RemoveStatusChangeReceiver(receiver1.Handle())
		service.UpdateStatus(serviceTerminating, nil)

		receiver1.VerifyNoStatusChanges(t)
		receiver2.VerifyStatus(t, serviceTerminating)
		receiver3.VerifyStatus(t, serviceTerminating)
	})

	t.Run("receiver2 removed", func(t *testing.T) {
		service.RemoveStatusChangeReceiver(receiver2.Handle())
		service.UpdateStatus(serviceTerminated, nil)

		receiver1.VerifyNoStatusChanges(t)
		receiver2.VerifyNoStatusChanges(t)
		receiver3.VerifyStatus(t, serviceTerminated)
	})
}

type testCompositeService struct {
	receiver *testStatusChangeReceiver
	compositeService
}

func newTestCompositeService(name string) *testCompositeService {
	receiver := newTestStatusChangeReceiver(name)
	ts := &testCompositeService{receiver: receiver}
	ts.AddStatusChangeReceiver(receiver.Handle(), receiver.OnStatusChange)
	ts.init()
	return ts
}

func (ts *testCompositeService) AddServices(services ...service) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.unsafeAddServices(services...)
}

func (ts *testCompositeService) RemoveService(service service) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.unsafeRemoveService(service)
}

func (ts *testCompositeService) DependenciesLen() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return len(ts.dependencies)
}

func (ts *testCompositeService) RemovedLen() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return len(ts.removed)
}

func TestCompositeServiceNormalStop(t *testing.T) {
	child1 := newTestService("child1")
	child2 := newTestService("child2")
	child3 := newTestService("child3")
	parent := newTestCompositeService("parent")
	if err := parent.AddServices(child1, child2); err != nil {
		t.Errorf("AddServices() got err: %v", err)
	}

	t.Run("Starting", func(t *testing.T) {
		wantState := serviceUninitialized
		if child1.Status() != wantState {
			t.Errorf("child1: current service status: got %d, want %d", child1.Status(), wantState)
		}
		if child2.Status() != wantState {
			t.Errorf("child2: current service status: got %d, want %d", child2.Status(), wantState)
		}

		parent.Start()

		child1.receiver.VerifyStatus(t, serviceStarting)
		child2.receiver.VerifyStatus(t, serviceStarting)
		parent.receiver.VerifyStatus(t, serviceStarting)

		// child3 is added after Start() and should be automatically started.
		if child3.Status() != wantState {
			t.Errorf("child3: current service status: got %d, want %d", child3.Status(), wantState)
		}
		if err := parent.AddServices(child3); err != nil {
			t.Errorf("AddServices() got err: %v", err)
		}
		child3.receiver.VerifyStatus(t, serviceStarting)
	})

	t.Run("Active", func(t *testing.T) {
		// parent service is active once all children are active.
		child1.UpdateStatus(serviceActive, nil)
		child2.UpdateStatus(serviceActive, nil)
		parent.receiver.VerifyNoStatusChanges(t)
		child3.UpdateStatus(serviceActive, nil)

		child1.receiver.VerifyStatus(t, serviceActive)
		child2.receiver.VerifyStatus(t, serviceActive)
		child3.receiver.VerifyStatus(t, serviceActive)
		parent.receiver.VerifyStatus(t, serviceActive)
		if err := parent.WaitStarted(); err != nil {
			t.Errorf("compositeService.WaitStarted() got err: %v", err)
		}
	})

	t.Run("Stopping", func(t *testing.T) {
		parent.Stop()

		child1.receiver.VerifyStatus(t, serviceTerminating)
		child2.receiver.VerifyStatus(t, serviceTerminating)
		child3.receiver.VerifyStatus(t, serviceTerminating)
		parent.receiver.VerifyStatus(t, serviceTerminating)

		// parent service is terminated once all children have terminated.
		child1.UpdateStatus(serviceTerminated, nil)
		child2.UpdateStatus(serviceTerminated, nil)
		parent.receiver.VerifyNoStatusChanges(t)
		child3.UpdateStatus(serviceTerminated, nil)

		child1.receiver.VerifyStatus(t, serviceTerminated)
		child2.receiver.VerifyStatus(t, serviceTerminated)
		child3.receiver.VerifyStatus(t, serviceTerminated)
		parent.receiver.VerifyStatus(t, serviceTerminated)
		if err := parent.WaitStopped(); err != nil {
			t.Errorf("compositeService.WaitStopped() got err: %v", err)
		}
	})
}

func TestCompositeServiceErrorDuringStartup(t *testing.T) {
	child1 := newTestService("child1")
	child2 := newTestService("child2")
	parent := newTestCompositeService("parent")
	if err := parent.AddServices(child1, child2); err != nil {
		t.Errorf("AddServices() got err: %v", err)
	}

	t.Run("Starting", func(t *testing.T) {
		parent.Start()

		parent.receiver.VerifyStatus(t, serviceStarting)
		child1.receiver.VerifyStatus(t, serviceStarting)
		child2.receiver.VerifyStatus(t, serviceStarting)
	})

	t.Run("Terminating", func(t *testing.T) {
		// child1 now errors.
		wantErr := errors.New("err during startup")
		child1.UpdateStatus(serviceTerminated, wantErr)
		child1.receiver.VerifyStatus(t, serviceTerminated)

		// This causes parent and child2 to start terminating.
		parent.receiver.VerifyStatus(t, serviceTerminating)
		child2.receiver.VerifyStatus(t, serviceTerminating)

		// parent has terminated once child2 has terminated.
		child2.UpdateStatus(serviceTerminated, nil)
		child2.receiver.VerifyStatus(t, serviceTerminated)
		parent.receiver.VerifyStatus(t, serviceTerminated)
		if gotErr := parent.WaitStarted(); !test.ErrorEqual(gotErr, wantErr) {
			t.Errorf("compositeService.WaitStarted() got err: (%v), want err: (%v)", gotErr, wantErr)
		}
	})
}

func TestCompositeServiceErrorWhileActive(t *testing.T) {
	child1 := newTestService("child1")
	child2 := newTestService("child2")
	parent := newTestCompositeService("parent")
	if err := parent.AddServices(child1, child2); err != nil {
		t.Errorf("AddServices() got err: %v", err)
	}

	t.Run("Starting", func(t *testing.T) {
		parent.Start()

		child1.receiver.VerifyStatus(t, serviceStarting)
		child2.receiver.VerifyStatus(t, serviceStarting)
		parent.receiver.VerifyStatus(t, serviceStarting)
	})

	t.Run("Active", func(t *testing.T) {
		child1.UpdateStatus(serviceActive, nil)
		child2.UpdateStatus(serviceActive, nil)

		child1.receiver.VerifyStatus(t, serviceActive)
		child2.receiver.VerifyStatus(t, serviceActive)
		parent.receiver.VerifyStatus(t, serviceActive)
		if err := parent.WaitStarted(); err != nil {
			t.Errorf("compositeService.WaitStarted() got err: %v", err)
		}
	})

	t.Run("Terminating", func(t *testing.T) {
		// child2 now errors.
		wantErr := errors.New("err while active")
		child2.UpdateStatus(serviceTerminating, wantErr)
		child2.receiver.VerifyStatus(t, serviceTerminating)

		// This causes parent and child1 to start terminating.
		child1.receiver.VerifyStatus(t, serviceTerminating)
		parent.receiver.VerifyStatus(t, serviceTerminating)

		// parent has terminated once both children have terminated.
		child1.UpdateStatus(serviceTerminated, nil)
		child2.UpdateStatus(serviceTerminated, nil)
		child1.receiver.VerifyStatus(t, serviceTerminated)
		child2.receiver.VerifyStatus(t, serviceTerminated)
		parent.receiver.VerifyStatus(t, serviceTerminated)
		if gotErr := parent.WaitStopped(); !test.ErrorEqual(gotErr, wantErr) {
			t.Errorf("compositeService.WaitStopped() got err: (%v), want err: (%v)", gotErr, wantErr)
		}
	})
}

func TestCompositeServiceRemoveService(t *testing.T) {
	child1 := newTestService("child1")
	child2 := newTestService("child2")
	parent := newTestCompositeService("parent")
	if err := parent.AddServices(child1, child2); err != nil {
		t.Errorf("AddServices() got err: %v", err)
	}

	t.Run("Starting", func(t *testing.T) {
		parent.Start()

		child1.receiver.VerifyStatus(t, serviceStarting)
		child2.receiver.VerifyStatus(t, serviceStarting)
		parent.receiver.VerifyStatus(t, serviceStarting)
	})

	t.Run("Active", func(t *testing.T) {
		child1.UpdateStatus(serviceActive, nil)
		child2.UpdateStatus(serviceActive, nil)

		child1.receiver.VerifyStatus(t, serviceActive)
		child2.receiver.VerifyStatus(t, serviceActive)
		parent.receiver.VerifyStatus(t, serviceActive)
	})

	t.Run("Remove service", func(t *testing.T) {
		// Removing child1 should stop it, but leave everything else active.
		parent.RemoveService(child1)

		if got, want := parent.DependenciesLen(), 1; got != want {
			t.Errorf("compositeService.dependencies: got len %d, want %d", got, want)
		}
		if got, want := parent.RemovedLen(), 1; got != want {
			t.Errorf("compositeService.removed: got len %d, want %d", got, want)
		}

		child1.receiver.VerifyStatus(t, serviceTerminating)
		child2.receiver.VerifyNoStatusChanges(t)
		parent.receiver.VerifyNoStatusChanges(t)

		// After child1 has terminated, it should be removed.
		child1.UpdateStatus(serviceTerminated, nil)

		child1.receiver.VerifyStatus(t, serviceTerminated)
		child2.receiver.VerifyNoStatusChanges(t)
		parent.receiver.VerifyNoStatusChanges(t)
	})

	t.Run("Terminating", func(t *testing.T) {
		// Now stop the composite service.
		parent.Stop()

		child2.receiver.VerifyStatus(t, serviceTerminating)
		parent.receiver.VerifyStatus(t, serviceTerminating)

		child2.UpdateStatus(serviceTerminated, nil)

		child2.receiver.VerifyStatus(t, serviceTerminated)
		parent.receiver.VerifyStatus(t, serviceTerminated)
		if err := parent.WaitStopped(); err != nil {
			t.Errorf("compositeService.WaitStopped() got err: %v", err)
		}

		if got, want := parent.DependenciesLen(), 1; got != want {
			t.Errorf("compositeService.dependencies: got len %d, want %d", got, want)
		}
		if got, want := parent.RemovedLen(), 0; got != want {
			t.Errorf("compositeService.removed: got len %d, want %d", got, want)
		}
	})
}

func TestCompositeServiceTree(t *testing.T) {
	leaf1 := newTestService("leaf1")
	leaf2 := newTestService("leaf2")
	intermediate1 := newTestCompositeService("intermediate1")
	if err := intermediate1.AddServices(leaf1, leaf2); err != nil {
		t.Errorf("intermediate1.AddServices() got err: %v", err)
	}

	leaf3 := newTestService("leaf3")
	leaf4 := newTestService("leaf4")
	intermediate2 := newTestCompositeService("intermediate2")
	if err := intermediate2.AddServices(leaf3, leaf4); err != nil {
		t.Errorf("intermediate2.AddServices() got err: %v", err)
	}

	root := newTestCompositeService("root")
	if err := root.AddServices(intermediate1, intermediate2); err != nil {
		t.Errorf("root.AddServices() got err: %v", err)
	}
	wantErr := errors.New("fail")

	t.Run("Starting", func(t *testing.T) {
		// Start trickles down the tree.
		root.Start()

		leaf1.receiver.VerifyStatus(t, serviceStarting)
		leaf2.receiver.VerifyStatus(t, serviceStarting)
		leaf3.receiver.VerifyStatus(t, serviceStarting)
		leaf4.receiver.VerifyStatus(t, serviceStarting)
		intermediate1.receiver.VerifyStatus(t, serviceStarting)
		intermediate2.receiver.VerifyStatus(t, serviceStarting)
		root.receiver.VerifyStatus(t, serviceStarting)
	})

	t.Run("Active", func(t *testing.T) {
		// serviceActive notification trickles up the tree.
		leaf1.UpdateStatus(serviceActive, nil)
		leaf2.UpdateStatus(serviceActive, nil)
		leaf3.UpdateStatus(serviceActive, nil)
		leaf4.UpdateStatus(serviceActive, nil)

		leaf1.receiver.VerifyStatus(t, serviceActive)
		leaf2.receiver.VerifyStatus(t, serviceActive)
		leaf3.receiver.VerifyStatus(t, serviceActive)
		leaf4.receiver.VerifyStatus(t, serviceActive)
		intermediate1.receiver.VerifyStatus(t, serviceActive)
		intermediate2.receiver.VerifyStatus(t, serviceActive)
		root.receiver.VerifyStatus(t, serviceActive)
		if err := root.WaitStarted(); err != nil {
			t.Errorf("compositeService.WaitStarted() got err: %v", err)
		}
	})

	t.Run("Leaf fails", func(t *testing.T) {
		leaf1.UpdateStatus(serviceTerminated, wantErr)
		leaf1.receiver.VerifyStatus(t, serviceTerminated)

		// Leaf service failure should trickle up the tree and across to all other
		// leaves, causing them all to start terminating.
		leaf2.receiver.VerifyStatus(t, serviceTerminating)
		leaf3.receiver.VerifyStatus(t, serviceTerminating)
		leaf4.receiver.VerifyStatus(t, serviceTerminating)
		intermediate1.receiver.VerifyStatus(t, serviceTerminating)
		intermediate2.receiver.VerifyStatus(t, serviceTerminating)
		root.receiver.VerifyStatus(t, serviceTerminating)
	})

	t.Run("Terminated", func(t *testing.T) {
		// serviceTerminated notification trickles up the tree.
		leaf2.UpdateStatus(serviceTerminated, nil)
		leaf3.UpdateStatus(serviceTerminated, nil)
		leaf4.UpdateStatus(serviceTerminated, nil)

		leaf2.receiver.VerifyStatus(t, serviceTerminated)
		leaf3.receiver.VerifyStatus(t, serviceTerminated)
		leaf4.receiver.VerifyStatus(t, serviceTerminated)
		intermediate1.receiver.VerifyStatus(t, serviceTerminated)
		intermediate2.receiver.VerifyStatus(t, serviceTerminated)
		root.receiver.VerifyStatus(t, serviceTerminated)

		if gotErr := root.WaitStopped(); !test.ErrorEqual(gotErr, wantErr) {
			t.Errorf("compositeService.WaitStopped() got err: (%v), want err: (%v)", gotErr, wantErr)
		}
	})
}

func TestCompositeServiceAddServicesErrors(t *testing.T) {
	child1 := newTestService("child1")
	parent := newTestCompositeService("parent")
	if err := parent.AddServices(child1); err != nil {
		t.Errorf("AddServices(child1) got err: %v", err)
	}

	child2 := newTestService("child2")
	child2.Start()
	if gotErr, wantErr := parent.AddServices(child2), errChildServiceStarted; !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("AddServices(child2) got err: (%v), want err: (%v)", gotErr, wantErr)
	}

	parent.Stop()
	child3 := newTestService("child3")
	if gotErr, wantErr := parent.AddServices(child3), ErrServiceStopped; !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("AddServices(child3) got err: (%v), want err: (%v)", gotErr, wantErr)
	}
}
