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
	period time.Duration
	task   func()
	ticker *time.Ticker
	stop   chan struct{}
}

func newPeriodicTask(period time.Duration, task func()) *periodicTask {
	return &periodicTask{
		period: period,
		task:   task,
	}
}

// Start the polling goroutine. No-op if the goroutine is already running.
// The task is executed after the polling period.
func (pt *periodicTask) Start() {
	if pt.ticker != nil || pt.period <= 0 {
		return
	}

	pt.ticker = time.NewTicker(pt.period)
	pt.stop = make(chan struct{})
	go pt.poll(pt.ticker, pt.stop)
}

// Stop/pause the periodic task.
func (pt *periodicTask) Stop() {
	if pt.ticker == nil {
		return
	}

	pt.ticker.Stop()
	close(pt.stop)

	pt.ticker = nil
	pt.stop = nil
}

func (pt *periodicTask) poll(ticker *time.Ticker, stop chan struct{}) {
	for {
		// stop has higher priority.
		select {
		case <-stop:
			return // Ends the goroutine.
		default:
		}

		select {
		case <-stop:
			return // Ends the goroutine.
		case <-ticker.C:
			pt.task()
		}
	}
}
