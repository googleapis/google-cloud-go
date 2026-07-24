/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"math"
	"testing"
	"time"
)

func TestEndpointScoreStateInitialization(t *testing.T) {
	state := &endpointScoreState{}
	state.update(100*time.Microsecond, time.Unix(100, 0))
	if got, want := state.scoreMicros, 100.0; got != want {
		t.Fatalf("scoreMicros = %v, want %v", got, want)
	}
}

func TestEndpointScoreStateTimeBasedAlpha(t *testing.T) {
	state := &endpointScoreState{}
	now := time.Unix(100, 0)
	state.update(100*time.Microsecond, now)
	state.update(200*time.Microsecond, now.Add(10*time.Second))

	alpha := 1 - math.Exp(-1)
	want := alpha*200 + (1-alpha)*100
	if got := state.scoreMicros; math.Abs(got-want) > 0.001 {
		t.Fatalf("scoreMicros = %v, want %v", got, want)
	}
}

func TestEndpointScoreStateRecordErrorPenalty(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	cfg := defaultEndpointRoutingConfig()
	cfg.now = clock.Now
	endpoint := &grpcChannelEndpoint{address: "server-a:443"}

	endpoint.recordLatency(cfg, 7, false, 100*time.Microsecond)
	endpoint.recordError(cfg, 7, false)

	endpoint.stateMu.Lock()
	got := endpoint.scores[endpointScoreKey{operationUID: 7, preferLeader: false}].scoreMicros
	endpoint.stateMu.Unlock()
	if got <= 100.0 {
		t.Fatalf("expected error penalty to increase score, got %v", got)
	}
}
