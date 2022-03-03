// Copyright 2021 Google LLC
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
	"sync"
	"time"
)

// minDuration returns the minimum of two durations.
func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

type timerStatus int

const (
	timerActive timerStatus = iota
	timerStopped
	timerTriggered
)

// requestTimer is a one-shot timer used to bound the duration of a request. It
// executes `onTimeout` if the timeout expires.
type requestTimer struct {
	onTimeout  func()
	timeoutErr error
	timer      *time.Timer
	mu         sync.Mutex
	status     timerStatus
}

func newRequestTimer(timeout time.Duration, onTimeout func(), timeoutErr error) *requestTimer {
	rt := &requestTimer{
		onTimeout:  onTimeout,
		timeoutErr: timeoutErr,
		status:     timerActive,
	}
	rt.timer = time.AfterFunc(timeout, rt.onTriggered)
	return rt
}

func (rt *requestTimer) onTriggered() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.status == timerActive {
		rt.status = timerTriggered
		rt.onTimeout()
	}
}

// Stop should be called upon a successful request to prevent the timer from
// expiring.
func (rt *requestTimer) Stop() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.status == timerActive {
		rt.status = timerStopped
		rt.timer.Stop()
	}
}

// ResolveError returns `timeoutErr` if the timer triggered, or otherwise
// `originalErr`.
func (rt *requestTimer) ResolveError(originalErr error) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.status == timerTriggered {
		return rt.timeoutErr
	}
	return originalErr
}

// streamIdleTimer is an approximate timer used to detect idle streams.
// `onTimeout` may be called up to (timeout / pollDivisor) after `timeout` has
// expired.
type streamIdleTimer struct {
	timeout   time.Duration
	onTimeout func()
	task      *periodicTask
	mu        sync.Mutex
	status    timerStatus
	startTime time.Time
}

const (
	pollDivisor     = 4
	maxPollInterval = time.Minute
)

// newStreamIdleTimer creates an unstarted timer.
func newStreamIdleTimer(timeout time.Duration, onTimeout func()) *streamIdleTimer {
	st := &streamIdleTimer{
		timeout:   timeout,
		onTimeout: onTimeout,
		status:    timerStopped,
	}
	st.task = newPeriodicTask(minDuration(timeout/pollDivisor, maxPollInterval), st.onPoll)
	st.task.Start()
	return st
}

// Restart the timer. Should be called when there is stream activity.
func (st *streamIdleTimer) Restart() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.status = timerActive
	st.startTime = time.Now()
}

// Stop the timer to prevent it from expiring.
func (st *streamIdleTimer) Stop() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.status = timerStopped
}

// Shutdown should be called when the timer is no longer used.
func (st *streamIdleTimer) Shutdown() {
	st.Stop()
	st.task.Stop()
}

func (st *streamIdleTimer) onPoll() {
	timeoutExpired := func() bool {
		st.mu.Lock()
		defer st.mu.Unlock()
		// Note: time.Since() uses monotonic clock readings.
		if st.status == timerActive && time.Since(st.startTime) > st.timeout {
			st.status = timerTriggered
			return true
		}
		return false
	}()
	if timeoutExpired {
		st.onTimeout()
	}
}
