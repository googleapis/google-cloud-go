// Copyright 2025 Google LLC
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

package retry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

type temporaryError struct {
	msg string
}

func (e *temporaryError) Error() string {
	return e.msg
}

func (e *temporaryError) Temporary() bool {
	return true
}

func TestShouldRetry(t *testing.T) {
	tests := []struct {
		name   string
		status int
		err    error
		want   bool
	}{
		{
			name:   "200 OK",
			status: http.StatusOK,
			err:    nil,
			want:   false,
		},
		{
			name:   "500 Internal Server Error",
			status: http.StatusInternalServerError,
			err:    nil,
			want:   true,
		},
		{
			name:   "503 Service Unavailable",
			status: http.StatusServiceUnavailable,
			err:    nil,
			want:   true,
		},
		{
			name:   "404 Not Found",
			status: http.StatusNotFound,
			err:    nil,
			want:   false,
		},
		{
			name:   "Unexpected EOF",
			status: 0,
			err:    io.ErrUnexpectedEOF,
			want:   true,
		},
		{
			name:   "Temporary Error",
			status: 0,
			err:    &temporaryError{msg: "temporary"},
			want:   true,
		},
		{
			name:   "Non-Temporary Error",
			status: 0,
			err:    errors.New("non-temporary"),
			want:   false,
		},
		{
			name:   "Wrapped Temporary Error",
			status: 0,
			err:    fmt.Errorf("wrapped: %w", &temporaryError{msg: "temporary"}),
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRetry(tt.status, tt.err); got != tt.want {
				t.Errorf("shouldRetry() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetryer_Retry(t *testing.T) {
	t.Run("stops after max attempts", func(t *testing.T) {
		r := New()
		for i := 0; i < r.maxAttempts; i++ {
			if _, ok := r.Retry(500, nil); !ok {
				t.Errorf("Retry() should have returned true on attempt %d", i)
			}
		}
		if _, ok := r.Retry(500, nil); ok {
			t.Error("Retry() should have returned false after max attempts")
		}
	})

	t.Run("no retry on success", func(t *testing.T) {
		r := New()
		if _, ok := r.Retry(200, nil); ok {
			t.Error("Retry() should have returned false on 200 OK")
		}
	})
}

func TestDefaultBackoff_Pause(t *testing.T) {
	b := &defaultBackoff{
		cur: 100 * time.Millisecond,
		max: 1 * time.Second,
		mul: 2,
	}

	d1 := b.Pause()
	if d1 > 100*time.Millisecond {
		t.Errorf("Pause() got %v, want <= 100ms", d1)
	}
	if b.cur != 200*time.Millisecond {
		t.Errorf("b.cur got %v, want 200ms", b.cur)
	}

	d2 := b.Pause()
	if d2 > 200*time.Millisecond {
		t.Errorf("Pause() got %v, want <= 200ms", d2)
	}
	if b.cur != 400*time.Millisecond {
		t.Errorf("b.cur got %v, want 400ms", b.cur)
	}

	// Test that it doesn't exceed max
	b.cur = 1 * time.Second
	b.Pause()
	if b.cur > 1*time.Second {
		t.Errorf("b.cur got %v, want <= 1s", b.cur)
	}
}

func TestNewWithOptions(t *testing.T) {
	t.Run("Full options", func(t *testing.T) {
		opts := &Options{
			Initial:     200 * time.Millisecond,
			Max:         500 * time.Millisecond,
			Multiplier:  1.5,
			MaxAttempts: 3,
		}
		r := NewWithOptions(opts)

		if r.maxAttempts != 3 {
			t.Errorf("maxAttempts = %d, want 3", r.maxAttempts)
		}

		b, ok := r.bo.(*defaultBackoff)
		if !ok {
			t.Fatalf("backoff is not defaultBackoff")
		}
		if b.cur != 200*time.Millisecond {
			t.Errorf("Initial = %v, want 200ms", b.cur)
		}
		if b.max != 500*time.Millisecond {
			t.Errorf("Max = %v, want 500ms", b.max)
		}
		if b.mul != 1.5 {
			t.Errorf("Multiplier = %v, want 1.5", b.mul)
		}
	})

	t.Run("Partial options (defaults)", func(t *testing.T) {
		// Provide only partial options, zeros should become defaults.
		opts := &Options{
			Initial: 0, // Should default to 100ms
			Max:     0, // Should default to 30s
			// Multiplier 0 -> Should default to 2.0
			// MaxAttempts 0 -> Should default to 5
		}
		r := NewWithOptions(opts)

		if r.maxAttempts != 5 {
			t.Errorf("maxAttempts = %d, want 5 (default)", r.maxAttempts)
		}

		b, ok := r.bo.(*defaultBackoff)
		if !ok {
			t.Fatalf("backoff is not defaultBackoff")
		}
		if b.cur != 100*time.Millisecond {
			t.Errorf("Initial = %v, want 100ms (default)", b.cur)
		}
		if b.max != 30*time.Second {
			t.Errorf("Max = %v, want 30s (default)", b.max)
		}
		if b.mul != 2.0 {
			t.Errorf("Multiplier = %v, want 2.0 (default)", b.mul)
		}
	})
}

func TestSleep_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel context immediately
	cancel()

	// Sleep for a long time. It should return immediately because of the canceled context.
	err := Sleep(ctx, 10*time.Hour)
	if err != context.Canceled {
		t.Errorf("Sleep() error = %v, want %v", err, context.Canceled)
	}
}

