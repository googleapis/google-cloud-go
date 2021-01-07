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

package test

import (
	"fmt"
	"sync"
	"time"
)

// MsgTracker is a helper for checking whether a set of messages make a full
// round trip from publisher to subscriber.
//
// Add() registers published messages. Remove() should be called when messages
// are received by subscribers. Call Wait() to block until all tracked messages
// are received. The same MsgTracker instance can be reused to repeat this
// sequence for multiple test cycles.
//
// Add() and Remove() calls should not be interleaved.
type MsgTracker struct {
	msgMap map[string]bool
	done   chan struct{}
	mu     sync.Mutex
}

// NewMsgTracker creates a new message tracker.
func NewMsgTracker() *MsgTracker {
	return &MsgTracker{
		msgMap: make(map[string]bool),
		done:   make(chan struct{}, 1),
	}
}

// Add a set of tracked messages.
func (mt *MsgTracker) Add(msgs ...string) {
	mt.mu.Lock()
	defer mt.mu.Unlock()

	for _, msg := range msgs {
		mt.msgMap[msg] = true
	}
}

// Remove and return true if `msg` is tracked. Signals the `done` channel once
// all messages have been received.
func (mt *MsgTracker) Remove(msg string) bool {
	mt.mu.Lock()
	defer mt.mu.Unlock()

	if _, exists := mt.msgMap[msg]; exists {
		delete(mt.msgMap, msg)
		if len(mt.msgMap) == 0 {
			var void struct{}
			mt.done <- void
		}
		return true
	}
	return false
}

// Wait up to `timeout` to receive all tracked messages.
func (mt *MsgTracker) Wait(timeout time.Duration) error {
	mt.mu.Lock()
	totalCount := len(mt.msgMap)
	mt.mu.Unlock()

	select {
	case <-time.After(timeout):
		mt.mu.Lock()
		receivedCount := totalCount - len(mt.msgMap)
		err := fmt.Errorf("received %d of %d messages", receivedCount, totalCount)
		mt.msgMap = make(map[string]bool)
		mt.mu.Unlock()
		return err

	case <-mt.done:
		return nil
	}
}

// Empty returns true if there are no tracked messages remaining.
func (mt *MsgTracker) Empty() bool {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	return len(mt.msgMap) == 0
}

// Status returns an error if there are tracked messages remaining.
func (mt *MsgTracker) Status() error {
	mt.mu.Lock()
	defer mt.mu.Unlock()

	if len(mt.msgMap) == 0 {
		return nil
	}
	return fmt.Errorf("%d messages not received", len(mt.msgMap))
}
