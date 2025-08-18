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
		for i := 0; i < maxRetryAttempts; i++ {
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
