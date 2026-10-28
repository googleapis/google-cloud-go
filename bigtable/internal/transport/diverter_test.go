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
				if got := d.UseSessionForMethod(ScopeMethodReadRow); got != tc.expectedRes {
					t.Errorf("UseSessionForMethod(ReadRow) = %v, want %v for load %f", got, tc.expectedRes, tc.sessionLoad)
				}
			}
		})
	}
}

func TestDiverterSetSessionLoad(t *testing.T) {
	d := NewDiverter(0.0)
	if got := d.UseSessionForMethod(ScopeMethodReadRow); got != false {
		t.Errorf("Expected initial UseSessionForMethod(ReadRow) to be false, got %v", got)
	}

	d.SetSessionLoad(1.0)
	if got := d.UseSessionForMethod(ScopeMethodReadRow); got != true {
		t.Errorf("Expected UseSessionForMethod(ReadRow) to be true after SetSessionLoad(1.0), got %v", got)
	}
}

func TestDiverterProbabilistic(t *testing.T) {
	load := 0.4
	d := NewDiverter(load)
	iterations := 10000
	trueCount := 0

	for i := 0; i < iterations; i++ {
		if d.UseSessionForMethod(ScopeMethodReadRow) {
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
				_ = d.UseSessionForMethod(ScopeMethodReadRow)
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

	t.Run("deprecated session_load applies only to ReadRow and MutateRow when diversion config is absent", func(t *testing.T) {
		d := NewDiverter(0.0)
		d.ApplySessionClientConfiguration(&bigtablepb.SessionClientConfiguration{
			SessionLoad: 1.0,
		})
		if got := d.SessionLoadForMethod(ScopeMethodReadRow); got != 1.0 {
			t.Errorf("SessionLoadForMethod(%q) = %v, want 1.0", ScopeMethodReadRow, got)
		}
		if got := d.SessionLoadForMethod(ScopeMethodMutateRow); got != 1.0 {
			t.Errorf("SessionLoadForMethod(%q) = %v, want 1.0", ScopeMethodMutateRow, got)
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

	t.Run("diversion config present ignores deprecated session_load completely", func(t *testing.T) {
		d := NewDiverter(0.0)
		d.ApplySessionClientConfiguration(&bigtablepb.SessionClientConfiguration{
			SessionLoad: 1.0,
			SessionDiversionConfiguration: &bigtablepb.SessionDiversionConfiguration{
				DiversionStrategy: &bigtablepb.SessionDiversionConfiguration_PerScopeDiversion_{
					PerScopeDiversion: &bigtablepb.SessionDiversionConfiguration_PerScopeDiversion{
						ScopeDiversions: map[string]*bigtablepb.SessionScopeDiversionConfiguration{
							ScopeMethodCheckAndMutateRow: {SessionLoad: 1.0},
						},
					},
				},
			},
		})
		if got := d.SessionLoadForMethod(ScopeMethodCheckAndMutateRow); got != 1.0 {
			t.Errorf("SessionLoadForMethod(%q) = %v, want 1.0", ScopeMethodCheckAndMutateRow, got)
		}
		for _, m := range []string{
			ScopeMethodReadRow,
			ScopeMethodMutateRow,
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

	t.Run("empty diversion config resolves to zero for all scopes", func(t *testing.T) {
		d := NewDiverter(1.0)
		d.ApplySessionClientConfiguration(&bigtablepb.SessionClientConfiguration{
			SessionLoad:                   1.0,
			SessionDiversionConfiguration: &bigtablepb.SessionDiversionConfiguration{},
		})
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

	t.Run("global_diversion sets uniform load across all served methods", func(t *testing.T) {
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

	t.Run("per_scope_diversion sets method-specific loads and ignores unserved scopes", func(t *testing.T) {
		d := NewDiverter(0.0)
		d.ApplySessionClientConfiguration(&bigtablepb.SessionClientConfiguration{
			SessionLoad: 0.5,
			SessionDiversionConfiguration: &bigtablepb.SessionDiversionConfiguration{
				DiversionStrategy: &bigtablepb.SessionDiversionConfiguration_PerScopeDiversion_{
					PerScopeDiversion: &bigtablepb.SessionDiversionConfiguration_PerScopeDiversion{
						ScopeDiversions: map[string]*bigtablepb.SessionScopeDiversionConfiguration{
							ScopeMethodReadRow:           {SessionLoad: 1.0},
							ScopeMethodMutateRow:         {SessionLoad: 0.0},
							ScopeMethodCheckAndMutateRow: {SessionLoad: 1.0},
							ScopeMethodMutateRows:        {SessionLoad: 0.25},
							// Unserved / unsupported scopes must be ignored:
							"method:Bigtable.SampleRowKeys":              {SessionLoad: 1.0},
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
		if !d.UseSessionForMethod(ScopeMethodReadRow) {
			t.Errorf("UseSessionForMethod(%q) = false, want true", ScopeMethodReadRow)
		}
		if d.UseSessionForMethod(ScopeMethodMutateRow) {
			t.Errorf("UseSessionForMethod(%q) = true, want false", ScopeMethodMutateRow)
		}
		if !d.UseSessionForMethod(ScopeMethodCheckAndMutateRow) {
			t.Errorf("UseSessionForMethod(%q) = false, want true", ScopeMethodCheckAndMutateRow)
		}
		if d.UseSessionForMethod(ScopeMethodReadModifyWriteRow) {
			t.Errorf("UseSessionForMethod(%q) = true, want false", ScopeMethodReadModifyWriteRow)
		}
		if got := d.SessionLoadForMethod("method:Bigtable.SampleRowKeys"); got != 0 {
			t.Errorf("SessionLoadForMethod(unserved SampleRowKeys) = %v, want 0", got)
		}
		// Shorthand method names without "method:Bigtable." prefix must not match.
		if got := d.SessionLoadForMethod("ReadRow"); got != 0 {
			t.Errorf("SessionLoadForMethod(\"ReadRow\") = %v, want 0 (canonical key required)", got)
		}

		snap := d.Snapshot()
		if len(snap.ScopeLoads) != len(wantLoads) {
			t.Fatalf("len(Snapshot().ScopeLoads) = %d, want %d", len(snap.ScopeLoads), len(wantLoads))
		}
		if got := snap.ScopeLoads[ScopeMethodCheckAndMutateRow]; got != 1.0 {
			t.Errorf("Snapshot().ScopeLoads[%q] = %v, want 1.0", ScopeMethodCheckAndMutateRow, got)
		}
		if _, exists := snap.ScopeLoads["table:my_table"]; exists {
			t.Errorf("Snapshot().ScopeLoads unexpectedly contains unsupported scope 'table:my_table'")
		}
		if _, exists := snap.ScopeLoads["method:Bigtable.SampleRowKeys"]; exists {
			t.Errorf("Snapshot().ScopeLoads unexpectedly contains unserved scope 'method:Bigtable.SampleRowKeys'")
		}
	})
}
