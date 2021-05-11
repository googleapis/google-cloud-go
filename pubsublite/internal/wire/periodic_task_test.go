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
	"sync/atomic"
	"testing"
	"time"
)

func TestPeriodicTask(t *testing.T) {
	const pollInterval = 10 * time.Millisecond
	var callCount int32
	values := make(chan int32)
	task := func() {
		values <- atomic.AddInt32(&callCount, 1)
	}
	ptask := newPeriodicTask(pollInterval, task)
	defer ptask.Stop()

	t.Run("Start", func(t *testing.T) {
		ptask.Start()
		ptask.Start() // Tests duplicate start

		got := <-values

		// Attempt to immediately stop the task after the first run.
		// Note: if this test is still flaky, pollInterval can be increased.
		ptask.Stop()

		if want := int32(1); got != want {
			t.Errorf("got %d, want %d", got, want)
		}
	})

	t.Run("Stop", func(t *testing.T) {
		ptask.Stop() // Tests duplicate stop (also called in Start above)

		// Wait at least the poll interval to ensure the task did not run.
		time.Sleep(2 * pollInterval)
		select {
		case got := <-values:
			t.Errorf("got unexpected value %d", got)
		default:
		}
	})

	t.Run("Restart", func(t *testing.T) {
		ptask.Start()

		if got, want := <-values, int32(2); got != want {
			t.Errorf("got %d, want %d", got, want)
		}
	})
}
