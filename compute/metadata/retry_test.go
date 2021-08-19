// Copyright 2021 Google LLC
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

package metadata

import (
	"io"
	"testing"
	"time"
)

type constantBackoff struct{}

func (b constantBackoff) Pause() time.Duration { return 100 }

type errTemp struct{}

func (e errTemp) Error() string { return "temporary error" }

func (e errTemp) Temporary() bool { return true }

type errWrapped struct {
	e error
}

func (e errWrapped) Error() string { return "unwrap me to get more context" }

func (e errWrapped) Unwrap() error { return e.e }

func TestMetadataRetryer(t *testing.T) {
	t.Run("retry on 500", func(t *testing.T) {
		retryer := metadataRetryer{bo: constantBackoff{}}
		delay, shouldRetry := retryer.Retry(500, nil)
		if !shouldRetry {
			t.Fatal("retryer.Retry(500, nil) = false, want true")
		}
		if delay != 100 {
			t.Fatalf("retryer.Retry(500, nil) = %d, want 100", delay)
		}
	})
	t.Run("don't retry 400", func(t *testing.T) {
		retryer := metadataRetryer{bo: constantBackoff{}}
		delay, shouldRetry := retryer.Retry(400, io.EOF)
		if shouldRetry {
			t.Fatal("retryer.Retry(400, io.EOF) = true, want false")
		}
		if delay != 0 {
			t.Fatalf("retryer.Retry(400, io.EOF) = %d, want 0", delay)
		}
	})
	t.Run("retry on io.ErrUnexpectedEOF", func(t *testing.T) {
		retryer := metadataRetryer{bo: constantBackoff{}}
		_, shouldRetry := retryer.Retry(400, io.ErrUnexpectedEOF)
		if !shouldRetry {
			t.Fatal("retryer.Retry(400, io.ErrUnexpectedEOF) = false, want true")
		}
	})
	t.Run("retry on temporary error", func(t *testing.T) {
		retryer := metadataRetryer{bo: constantBackoff{}}
		err := errTemp{}
		_, shouldRetry := retryer.Retry(400, err)
		if !shouldRetry {
			t.Fatal("retryer.Retry(400, err) = false, want true")
		}
	})
	t.Run("retry on wrapped temporary error", func(t *testing.T) {
		retryer := metadataRetryer{bo: constantBackoff{}}
		err := errWrapped{errTemp{}}
		_, shouldRetry := retryer.Retry(400, err)
		if !shouldRetry {
			t.Fatal("retryer.Retry(400, err) = false, want true")
		}
	})
	t.Run("don't retry on wrapped io.EOF", func(t *testing.T) {
		retryer := metadataRetryer{bo: constantBackoff{}}
		err := errWrapped{io.EOF}
		_, shouldRetry := retryer.Retry(400, err)
		if shouldRetry {
			t.Fatal("retryer.Retry(400, err) = true, want false")
		}
	})
	t.Run("stop retry after 5 attempts", func(t *testing.T) {
		retryer := metadataRetryer{bo: constantBackoff{}}
		for i := 1; i <= 6; i++ {
			_, shouldRetry := retryer.Retry(500, nil)
			if i == 6 {
				if shouldRetry {
					t.Fatal("an error should only be retried 5 times")
				}
				break
			}
			if !shouldRetry {
				t.Fatalf("retryer.Retry(500, nil) = false, want true")
			}
		}
	})
}
