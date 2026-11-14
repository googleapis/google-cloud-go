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

package storage

import (
	"testing"
	"time"
)

const testChunk16MiB = 16 * 1024 * 1024

func TestNewWriteChunkDelay(t *testing.T) {
	for _, tc := range []struct {
		desc      string
		chunkSize int
		timeout   time.Duration
		wantNil   bool
		wantMin   time.Duration
		wantMax   time.Duration
	}{
		{desc: "zero timeout disables", chunkSize: testChunk16MiB, timeout: 0, wantNil: true},
		{desc: "negative timeout disables", chunkSize: testChunk16MiB, timeout: -time.Second, wantNil: true},
		{desc: "16 MiB floor is 50ms + 160ms", chunkSize: testChunk16MiB, timeout: 5 * time.Second, wantMin: 210 * time.Millisecond, wantMax: 5 * time.Second},
		{desc: "256 KiB floor is close to the base", chunkSize: 256 * 1024, timeout: 5 * time.Second, wantMin: 52500 * time.Microsecond, wantMax: 5 * time.Second},
		{desc: "floor is capped at half the timeout", chunkSize: testChunk16MiB, timeout: 100 * time.Millisecond, wantMin: 50 * time.Millisecond, wantMax: 100 * time.Millisecond},
		{desc: "tiny timeout keeps min < max", chunkSize: testChunk16MiB, timeout: time.Microsecond, wantMin: 500 * time.Nanosecond, wantMax: time.Microsecond},
		{desc: "1ns timeout falls back to 1ms/2ms", chunkSize: testChunk16MiB, timeout: time.Nanosecond, wantMin: time.Millisecond, wantMax: 2 * time.Millisecond},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			d := newWriteChunkDelay(tc.chunkSize, tc.timeout)
			if tc.wantNil {
				if d != nil {
					t.Fatalf("newWriteChunkDelay(%d, %v) = %+v, want nil", tc.chunkSize, tc.timeout, d)
				}
				return
			}
			if d == nil {
				t.Fatalf("newWriteChunkDelay(%d, %v) = nil, want non-nil", tc.chunkSize, tc.timeout)
			}
			if d.minDelay != tc.wantMin || d.maxDelay != tc.wantMax {
				t.Errorf("bounds = [%v, %v], want [%v, %v]", d.minDelay, d.maxDelay, tc.wantMin, tc.wantMax)
			}
			if got := d.current(); got != tc.wantMin {
				t.Errorf("initial threshold = %v, want minDelay %v", got, tc.wantMin)
			}
		})
	}
}

func TestWriteChunkDelay_NilIsDisabled(t *testing.T) {
	var d *writeChunkDelay
	d.resetWarmUp()
	if d.recordLatency(time.Hour) {
		t.Error("nil recordLatency reported exceeded")
	}
	if d.exceeds(time.Hour) {
		t.Error("nil exceeds reported true")
	}
}

func TestWriteChunkDelay_ResetWarmUp(t *testing.T) {
	d := newWriteChunkDelay(testChunk16MiB, 10*time.Second)
	d.recordLatency(400 * time.Millisecond) // threshold = 800ms

	// After rotating to a new stream, resetWarmUp prevents a cold first chunk
	// (2s) on the new stream from immediately triggering another rotation while
	// still updating the EWMA baseline.
	beforeReset := d.current()
	d.resetWarmUp()
	if d.exceeds(2 * time.Second) {
		t.Fatal("exceeds(2s) must be false during new-stream warm-up")
	}
	if d.recordLatency(2 * time.Second) {
		t.Fatal("first chunk after resetWarmUp must not report exceeded")
	}
	if got := d.current(); got <= beforeReset {
		t.Fatalf("new-stream warm-up chunk did not update EWMA threshold: %v -> %v", beforeReset, got)
	}
	if !d.exceeds(d.current() + time.Millisecond) {
		t.Fatal("exceeds must be active again after new-stream warm-up completes")
	}
}

func TestWriteChunkDelay_WarmUp(t *testing.T) {
	d := newWriteChunkDelay(testChunk16MiB, 5*time.Second) // floor 210ms

	if d.exceeds(time.Hour) {
		t.Fatal("exceeds must be false before the first sample")
	}
	if d.recordLatency(80 * time.Millisecond) {
		t.Fatal("first sample must not report exceeded")
	}
	// 2 * 80ms < floor, so the threshold stays at the floor.
	if got, want := d.current(), 210*time.Millisecond; got != want {
		t.Fatalf("threshold after fast warm-up = %v, want %v", got, want)
	}
	if d.exceeds(210 * time.Millisecond) {
		t.Error("exceeds at exactly the threshold must be false")
	}
	if !d.exceeds(211 * time.Millisecond) {
		t.Error("exceeds just above the threshold must be true")
	}
}

func TestWriteChunkDelay_ThresholdTracksBaseline(t *testing.T) {
	d := newWriteChunkDelay(testChunk16MiB, 10*time.Second)

	// A cold 2s first chunk seeds a 4s threshold without rotating.
	if d.recordLatency(2 * time.Second) {
		t.Fatal("cold first chunk must not report exceeded")
	}
	if got, want := d.current(), 4*time.Second; got != want {
		t.Fatalf("threshold after 2s warm-up = %v, want %v", got, want)
	}

	// Healthy 400ms chunks pull the baseline down within a handful of samples.
	for i := 0; i < 10; i++ {
		if d.recordLatency(400 * time.Millisecond) {
			t.Fatalf("healthy chunk %d reported exceeded", i+2)
		}
	}
	if got := d.current(); got >= 1500*time.Millisecond {
		t.Fatalf("threshold after ten 400ms chunks = %v, want < 1.5s", got)
	}

	// A step to 1.5s is now detected...
	if !d.recordLatency(1500 * time.Millisecond) {
		t.Fatal("step to 1.5s must report exceeded")
	}
	// ...and exceeded is judged against the threshold before the update: the
	// 1.5s sample has already raised the threshold, so the same latency no
	// longer exceeds it on the next call.
	before := d.current()
	for i := 0; i < 10; i++ {
		d.recordLatency(1500 * time.Millisecond)
	}
	if got := d.current(); got <= before {
		t.Fatalf("threshold did not rise under a sustained slowdown: %v -> %v", before, got)
	}
	if d.exceeds(1500 * time.Millisecond) {
		t.Fatal("sustained 1.5s chunks must become the new normal")
	}
}

func TestWriteChunkDelay_ClampsToTimeout(t *testing.T) {
	d := newWriteChunkDelay(testChunk16MiB, 3*time.Second)
	d.recordLatency(2 * time.Second) // 2x would be 4s
	if got, want := d.current(), 3*time.Second; got != want {
		t.Fatalf("threshold = %v, want clamp to maxDelay %v", got, want)
	}
	for i := 0; i < 20; i++ {
		d.recordLatency(10 * time.Second)
	}
	if got, want := d.current(), 3*time.Second; got != want {
		t.Fatalf("threshold = %v after huge samples, want maxDelay %v", got, want)
	}
	for i := 0; i < 200; i++ {
		d.recordLatency(time.Millisecond)
	}
	if got, want := d.current(), d.minDelay; got != want {
		t.Fatalf("threshold = %v after tiny samples, want minDelay %v", got, want)
	}
}
