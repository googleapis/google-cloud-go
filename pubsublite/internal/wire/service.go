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
	"sync"
)

// serviceStatus specifies the current status of the service. The order of the
// values reflects the lifecycle of services. Note that some statuses may be
// skipped.
type serviceStatus int

const (
	// Service has not been started.
	serviceUninitialized serviceStatus = iota
	// Service is starting up.
	serviceStarting
	// Service is active and accepting new data. Note that the underlying stream
	// may be reconnecting due to retryable errors.
	serviceActive
	// Service is gracefully shutting down by flushing all pending data. No new
	// data is accepted.
	serviceTerminating
	// Service has terminated. No new data is accepted.
	serviceTerminated
)

// serviceHandle is used to compare pointers to service instances.
type serviceHandle interface{}

// serviceStatusChangeFunc notifies the parent of service status changes.
// `serviceTerminating` and `serviceTerminated` have an associated error. This
// error may be nil if the user called Stop().
type serviceStatusChangeFunc func(serviceHandle, serviceStatus, error)

// service is the interface that must be implemented by services (essentially
// gRPC client stream wrappers, e.g. subscriber, publisher) that can be
// dependencies of a compositeService.
type service interface {
	Start()
	Stop()

	// Methods below are implemented by abstractService.
	AddStatusChangeReceiver(serviceHandle, serviceStatusChangeFunc)
	RemoveStatusChangeReceiver(serviceHandle)
	Handle() serviceHandle
	Status() serviceStatus
	Error() error
}

// abstractService can be embedded into other structs to provide common
// functionality for managing service status and status change receivers.
type abstractService struct {
	mu                    sync.Mutex
	statusChangeReceivers []*statusChangeReceiver
	status                serviceStatus
	// The error that cause the service to terminate.
	err error
}

type statusChangeReceiver struct {
	handle         serviceHandle // For removing the receiver.
	onStatusChange serviceStatusChangeFunc
}

func (as *abstractService) AddStatusChangeReceiver(handle serviceHandle, onStatusChange serviceStatusChangeFunc) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.statusChangeReceivers = append(
		as.statusChangeReceivers,
		&statusChangeReceiver{handle, onStatusChange})
}

func (as *abstractService) RemoveStatusChangeReceiver(handle serviceHandle) {
	as.mu.Lock()
	defer as.mu.Unlock()

	for i := len(as.statusChangeReceivers) - 1; i >= 0; i-- {
		r := as.statusChangeReceivers[i]
		if r.handle == handle {
			// Swap with last element, erase last element and truncate the slice.
			lastIdx := len(as.statusChangeReceivers) - 1
			if i != lastIdx {
				as.statusChangeReceivers[i] = as.statusChangeReceivers[lastIdx]
			}
			as.statusChangeReceivers[lastIdx] = nil
			as.statusChangeReceivers = as.statusChangeReceivers[:lastIdx]
		}
	}
}

// Handle identifies this service instance, even when there are multiple layers
// of embedding.
func (as *abstractService) Handle() serviceHandle {
	return as
}

func (as *abstractService) Error() error {
	as.mu.Lock()
	defer as.mu.Unlock()
	return as.err
}

func (as *abstractService) Status() serviceStatus {
	as.mu.Lock()
	defer as.mu.Unlock()
	return as.status
}

func (as *abstractService) unsafeCheckServiceStatus() error {
	switch {
	case as.status == serviceUninitialized:
		return ErrServiceUninitialized
	case as.status == serviceStarting:
		return ErrServiceStarting
	case as.status >= serviceTerminating:
		return ErrServiceStopped
	default:
		return nil
	}
}

// unsafeUpdateStatus assumes the service is already holding a mutex when
// called, as it often needs to be atomic with other operations.
func (as *abstractService) unsafeUpdateStatus(targetStatus serviceStatus, err error) bool {
	if as.status >= targetStatus {
		// Already at the same or later stage of the service lifecycle.
		return false
	}

	as.status = targetStatus
	if as.err == nil {
		// Prevent clobbering original error.
		as.err = err
	}

	for _, receiver := range as.statusChangeReceivers {
		// Notify in a goroutine to prevent deadlocks if the receiver is holding a
		// locked mutex.
		go receiver.onStatusChange(as.Handle(), as.status, as.err)
	}
	return true
}

type closeable interface {
	Close() error
}

type apiClients []closeable

func (ac apiClients) Close() (retErr error) {
	for _, c := range ac {
		if err := c.Close(); retErr == nil {
			retErr = err
		}
	}
	return
}

var errChildServiceStarted = errors.New("pubsublite: dependent service must not be started")

// compositeService can be embedded into other structs to manage child services.
// It implements the service interface and can itself be a dependency of another
// compositeService.
//
// If one child service terminates due to a permanent failure, all other child
// services are stopped. Child services can be added and removed dynamically.
type compositeService struct {
	// Used to block until all dependencies have started or terminated.
	waitStarted    chan struct{}
	waitTerminated chan struct{}

	// Current dependencies.
	dependencies map[serviceHandle]service
	// Removed dependencies that are in the process of terminating.
	removed map[serviceHandle]service

	// Dependencies to close when the compositeService has terminated.
	toClose closeable

	abstractService
}

// init must be called after creation of the derived struct.
func (cs *compositeService) init() {
	cs.waitStarted = make(chan struct{})
	cs.waitTerminated = make(chan struct{})
	cs.dependencies = make(map[serviceHandle]service)
	cs.removed = make(map[serviceHandle]service)
}

// Start up dependencies.
func (cs *compositeService) Start() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.abstractService.unsafeUpdateStatus(serviceStarting, nil) {
		for _, s := range cs.dependencies {
			s.Start()
		}
	}
}

// WaitStarted waits for all dependencies to start.
func (cs *compositeService) WaitStarted() error {
	<-cs.waitStarted
	return cs.Error()
}

// Stop all dependencies.
func (cs *compositeService) Stop() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.unsafeInitiateShutdown(serviceTerminating, nil)
}

// WaitStopped waits for all dependencies to stop.
func (cs *compositeService) WaitStopped() error {
	<-cs.waitTerminated
	return cs.Error()
}

func (cs *compositeService) unsafeAddServices(services ...service) error {
	if cs.status >= serviceTerminating {
		return ErrServiceStopped
	}

	for _, s := range services {
		// Adding dependent services which have already started not currently
		// supported. Requires updating logic to handle the compositeService state.
		if s.Status() > serviceUninitialized {
			return errChildServiceStarted
		}

		s.AddStatusChangeReceiver(cs.Handle(), cs.onServiceStatusChange)
		cs.dependencies[s.Handle()] = s
		if cs.status > serviceUninitialized {
			s.Start()
		}
	}
	return nil
}

func (cs *compositeService) unsafeRemoveService(remove service) {
	if _, present := cs.dependencies[remove.Handle()]; !present {
		return
	}
	delete(cs.dependencies, remove.Handle())
	// The service will be completely removed after it has terminated.
	cs.removed[remove.Handle()] = remove
	if remove.Status() < serviceTerminating {
		remove.Stop()
	}
}

func (cs *compositeService) unsafeInitiateShutdown(targetStatus serviceStatus, err error) {
	if cs.unsafeUpdateStatus(targetStatus, err) {
		for _, s := range cs.dependencies {
			if s.Status() < serviceTerminating {
				s.Stop()
			}
		}
	}
}

func (cs *compositeService) unsafeUpdateStatus(targetStatus serviceStatus, err error) (ret bool) {
	previousStatus := cs.status
	if ret = cs.abstractService.unsafeUpdateStatus(targetStatus, err); ret {
		// Note: the waitStarted channel must be closed when the service fails to
		// start.
		if previousStatus < serviceActive && targetStatus >= serviceActive {
			close(cs.waitStarted)
		}
		if targetStatus == serviceTerminated {
			if cs.toClose != nil {
				cs.toClose.Close()
			}
			close(cs.waitTerminated)
		}
	}
	return
}

func (cs *compositeService) onServiceStatusChange(handle serviceHandle, status serviceStatus, err error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if removedService, present := cs.removed[handle]; present {
		if status == serviceTerminated {
			removedService.RemoveStatusChangeReceiver(cs.Handle())
			delete(cs.removed, handle)
		}
	}

	// Note: we cannot rely on the service not being in the `removed` map to
	// determine whether it is an active dependency. The notification may be for a
	// service that is no longer in cs.removed or cs.dependencies, because status
	// changes are notified asynchronously and may be received out of order.
	_, isDependency := cs.dependencies[handle]

	// If a single service terminates, stop them all, but allow the others to
	// flush pending data. Ignore removed services that are stopping.
	shouldTerminate := status >= serviceTerminating && isDependency
	numStarted := 0
	numTerminated := 0

	for _, s := range cs.dependencies {
		if shouldTerminate && s.Status() < serviceTerminating {
			s.Stop()
		}
		if s.Status() >= serviceActive {
			numStarted++
		}
		if s.Status() == serviceTerminated {
			numTerminated++
		}
	}

	switch {
	case numTerminated == len(cs.dependencies) && len(cs.removed) == 0:
		cs.unsafeUpdateStatus(serviceTerminated, err)
	case shouldTerminate:
		cs.unsafeUpdateStatus(serviceTerminating, err)
	case numStarted == len(cs.dependencies):
		cs.unsafeUpdateStatus(serviceActive, err)
	}
}
