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

	bigtablepb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
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

func TestDiverterApplySessionClientConfiguration(t *testing.T) {
	t.Run("nil config resets all methods to 0", func(t *testing.T) {
		d := NewDiverter(1.0)
		d.ApplySessionClientConfiguration(nil)
		for _, m := range []string{
			ScopeMethodReadRow,
			ScopeMethodMutateRow,
			ScopeMethodCheckAndMutateRow,
			ScopeMethodReadModifyWriteRow,
			ScopeMethodMutateRows,
		} {
			if got := d.SessionLoadForMethod(m); got != 0 {
				t.Errorf("SessionLoadForMethod(%q) = %v, want 0", m, got)
			}
			if d.UseSessionForMethod(m) {
				t.Errorf("UseSessionForMethod(%q) = true, want false", m)
			}
		}
	})

	t.Run("deprecated session_load applies only to ReadRow and MutateRow", func(t *testing.T) {
		d := NewDiverter(0.0)
		d.ApplySessionClientConfiguration(&bigtablepb.SessionClientConfiguration{
			SessionLoad: 1.0,
		})
		if got := d.SessionLoadForMethod("ReadRow"); got != 1.0 {
			t.Errorf("SessionLoadForMethod(ReadRow) = %v, want 1.0", got)
		}
		if got := d.SessionLoadForMethod("Bigtable.MutateRow"); got != 1.0 {
			t.Errorf("SessionLoadForMethod(MutateRow) = %v, want 1.0", got)
		}
		for _, m := range []string{
			ScopeMethodCheckAndMutateRow,
			ScopeMethodReadModifyWriteRow,
			ScopeMethodMutateRows,
		} {
			if got := d.SessionLoadForMethod(m); got != 0 {
				t.Errorf("SessionLoadForMethod(%q) = %v, want 0", m, got)
			}
			if d.UseSessionForMethod(m) {
				t.Errorf("UseSessionForMethod(%q) = true, want false", m)
			}
		}
	})

	t.Run("global_diversion overrides deprecated session_load for all methods", func(t *testing.T) {
		d := NewDiverter(0.0)
		d.ApplySessionClientConfiguration(&bigtablepb.SessionClientConfiguration{
			SessionLoad: 0.0,
			SessionDiversionConfiguration: &bigtablepb.SessionDiversionConfiguration{
				DiversionStrategy: &bigtablepb.SessionDiversionConfiguration_GlobalDiversion{
					GlobalDiversion: &bigtablepb.SessionScopeDiversionConfiguration{
						SessionLoad: 1.0,
					},
				},
			},
		})
		for _, m := range []string{
			ScopeMethodReadRow,
			ScopeMethodMutateRow,
			ScopeMethodCheckAndMutateRow,
			ScopeMethodReadModifyWriteRow,
			ScopeMethodMutateRows,
		} {
			if got := d.SessionLoadForMethod(m); got != 1.0 {
				t.Errorf("SessionLoadForMethod(%q) = %v, want 1.0", m, got)
			}
			if !d.UseSessionForMethod(m) {
				t.Errorf("UseSessionForMethod(%q) = false, want true", m)
			}
		}
	})

	t.Run("per_scope_diversion sets method-specific loads and overrides session_load", func(t *testing.T) {
		d := NewDiverter(0.0)
		d.ApplySessionClientConfiguration(&bigtablepb.SessionClientConfiguration{
			SessionLoad: 0.5,
			SessionDiversionConfiguration: &bigtablepb.SessionDiversionConfiguration{
				DiversionStrategy: &bigtablepb.SessionDiversionConfiguration_PerScopeDiversion_{
					PerScopeDiversion: &bigtablepb.SessionDiversionConfiguration_PerScopeDiversion{
						ScopeDiversions: map[string]*bigtablepb.SessionScopeDiversionConfiguration{
							"method:Bigtable.ReadRow":           {SessionLoad: 1.0},
							"method:Bigtable.MutateRow":         {SessionLoad: 0.0},
							"method:Bigtable.CheckAndMutateRow": {SessionLoad: 1.0},
							"method:Bigtable.MutateRows":        {SessionLoad: 0.25},
							// Unsupported scopes must be ignored:
							"table:my_table":                             {SessionLoad: 1.0},
							"method:Bigtable.ReadModifyWriteRow,table:t": {SessionLoad: 1.0},
						},
					},
				},
			},
		})

		wantLoads := map[string]float64{
			ScopeMethodReadRow:            1.0,
			ScopeMethodMutateRow:          0.0,
			ScopeMethodCheckAndMutateRow:  1.0,
			ScopeMethodReadModifyWriteRow: 0.0,
			ScopeMethodMutateRows:         0.25,
		}
		for scope, want := range wantLoads {
			if got := d.SessionLoadForMethod(scope); got != want {
				t.Errorf("SessionLoadForMethod(%q) = %v, want %v", scope, got, want)
			}
		}
		if !d.UseSessionForMethod("ReadRow") {
			t.Errorf("UseSessionForMethod(ReadRow) = false, want true")
		}
		if d.UseSessionForMethod("MutateRow") {
			t.Errorf("UseSessionForMethod(MutateRow) = true, want false")
		}
		if !d.UseSessionForMethod("CheckAndMutateRow") {
			t.Errorf("UseSessionForMethod(CheckAndMutateRow) = false, want true")
		}
		if d.UseSessionForMethod("ReadModifyWriteRow") {
			t.Errorf("UseSessionForMethod(ReadModifyWriteRow) = true, want false")
		}

		snap := d.Snapshot()
		if snap.ScopeLoads == nil {
			t.Fatal("Snapshot().ScopeLoads = nil, want non-nil map")
		}
		if got := snap.ScopeLoads[ScopeMethodCheckAndMutateRow]; got != 1.0 {
			t.Errorf("Snapshot().ScopeLoads[%q] = %v, want 1.0", ScopeMethodCheckAndMutateRow, got)
		}
		if _, exists := snap.ScopeLoads["table:my_table"]; exists {
			t.Errorf("Snapshot().ScopeLoads unexpectedly contains unsupported scope 'table:my_table'")
		}
	})
}

