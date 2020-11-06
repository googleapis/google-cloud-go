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
	"time"
)

// periodicTask manages a recurring background task.
type periodicTask struct {
	period  time.Duration
	ticker  *time.Ticker
	stop    chan struct{}
	stopped bool
	task    func()
}

func newPeriodicTask(period time.Duration, task func()) *periodicTask {
	return &periodicTask{
		ticker: time.NewTicker(period),
		stop:   make(chan struct{}),
		period: period,
		task:   task,
	}
}

// Start the polling goroutine.
func (pt *periodicTask) Start() {
	go pt.poll()
}

// Resume polling. The task is executed after the polling period.
func (pt *periodicTask) Resume() {
	pt.ticker.Reset(pt.period)
}

// Pause temporarily suspends the polling.
func (pt *periodicTask) Pause() {
	pt.ticker.Stop()
}

// Stop permanently stops the periodic task.
func (pt *periodicTask) Stop() {
	// Prevent a panic if the stop channel has already been closed.
	if !pt.stopped {
		close(pt.stop)
		pt.stopped = true
	}
}

func (pt *periodicTask) poll() {
	for {
		select {
		case <-pt.stop:
			// Ends the goroutine.
			return
		case <-pt.ticker.C:
			pt.task()
		}
	}
}
