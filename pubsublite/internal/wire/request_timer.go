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

type requestTimerStatus int

const (
	requestTimerNew requestTimerStatus = iota
	requestTimerStopped
	requestTimerTriggered
)

// requestTimer bounds the duration of a request and executes `onTimeout` if
// the timer is triggered.
type requestTimer struct {
	onTimeout  func()
	timeoutErr error
	timer      *time.Timer
	mu         sync.Mutex
	status     requestTimerStatus
}

func newRequestTimer(duration time.Duration, onTimeout func(), timeoutErr error) *requestTimer {
	rt := &requestTimer{
		onTimeout:  onTimeout,
		timeoutErr: timeoutErr,
		status:     requestTimerNew,
	}
	rt.timer = time.AfterFunc(duration, rt.onTriggered)
	return rt
}

func (rt *requestTimer) onTriggered() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.status == requestTimerNew {
		rt.status = requestTimerTriggered
		rt.onTimeout()
	}
}

// Stop should be called upon a successful request to prevent the timer from
// expiring.
func (rt *requestTimer) Stop() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.status == requestTimerNew {
		rt.status = requestTimerStopped
		rt.timer.Stop()
	}
}

// ResolveError returns `timeoutErr` if the timer triggered, or otherwise
// `originalErr`.
func (rt *requestTimer) ResolveError(originalErr error) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.status == requestTimerTriggered {
		return rt.timeoutErr
	}
	return originalErr
}
