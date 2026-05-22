// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"sync"
	"testing"
)

func TestDiverterZeroAndOne(t *testing.T) {
	tests := []struct {
		name        string
		sessionLoad float64
		expectedRes bool
	}{
		{
			name:        "Load is zero",
			sessionLoad: 0.0,
			expectedRes: false,
		},
		{
			name:        "Load is negative",
			sessionLoad: -0.5,
			expectedRes: false,
		},
		{
			name:        "Load is one",
			sessionLoad: 1.0,
			expectedRes: true,
		},
		{
			name:        "Load is greater than one",
			sessionLoad: 1.5,
			expectedRes: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDiverter(tc.sessionLoad)
			for i := 0; i < 1000; i++ {
				if got := d.UseSession(); got != tc.expectedRes {
					t.Errorf("UseSession() = %v, want %v for load %f", got, tc.expectedRes, tc.sessionLoad)
				}
			}
		})
	}
}

func TestDiverterSetSessionLoad(t *testing.T) {
	d := NewDiverter(0.0)
	if got := d.UseSession(); got != false {
		t.Errorf("Expected initial UseSession() to be false, got %v", got)
	}

	d.SetSessionLoad(1.0)
	if got := d.UseSession(); got != true {
		t.Errorf("Expected UseSession() to be true after SetSessionLoad(1.0), got %v", got)
	}
}

func TestDiverterProbabilistic(t *testing.T) {
	load := 0.4
	d := NewDiverter(load)
	iterations := 10000
	trueCount := 0

	for i := 0; i < iterations; i++ {
		if d.UseSession() {
			trueCount++
		}
	}

	expected := int(float64(iterations) * load)
	tolerance := 300 // Allow a variance range of +/- 3% of total iterations

	if trueCount < expected-tolerance || trueCount > expected+tolerance {
		t.Errorf("Expected approximately %d true results (with tolerance %d), got %d", expected, tolerance, trueCount)
	}
}

func TestDiverterConcurrentAccess(t *testing.T) {
	d := NewDiverter(0.5)
	var wg sync.WaitGroup
	numWorkers := 20
	iterations := 1000

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if j%10 == 0 {
					d.SetSessionLoad(float64(workerID) / float64(numWorkers))
				}
				_ = d.UseSession()
			}
		}(i)
	}

	wg.Wait()
}
