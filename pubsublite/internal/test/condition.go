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

package test

import (
	"testing"
	"time"
)

// Condition allows tests to wait for some event to occur, or check that it has
// not occurred.
type Condition struct {
	name string
	done chan struct{}
}

// NewCondition creates a new condition.
func NewCondition(name string) *Condition {
	return &Condition{
		name: name,
		done: make(chan struct{}),
	}
}

// SetDone marks the condition as done.
func (c *Condition) SetDone() {
	close(c.done)
}

// WaitUntilDone waits up to the specified duration for the condition to be
// marked done.
func (c *Condition) WaitUntilDone(t *testing.T, duration time.Duration) {
	t.Helper()

	select {
	case <-time.After(duration):
		t.Errorf("Condition(%q): timed out after waiting %v", c.name, duration)
	case <-c.done:
	}
}

// VerifyNotDone checks that the condition is not done.
func (c *Condition) VerifyNotDone(t *testing.T) {
	t.Helper()

	select {
	case <-c.done:
		t.Errorf("Condition(%q): is done, expected not done", c.name)
	default:
	}
}
