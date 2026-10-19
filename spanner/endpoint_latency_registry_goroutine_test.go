// Copyright 2026 Google LLC
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
// limitations under the License.

package spanner

import (
	"bytes"
	"context"
	"runtime"
	"testing"

	"google.golang.org/api/option"
)

func TestEndpointLatencyRegistryDoesNotStartCleanupGoroutine(t *testing.T) {
	assertNoEndpointLatencyRegistryCleanupGoroutine(t)

	client, err := NewClientWithConfig(
		context.Background(),
		"projects/p/instances/i/databases/d",
		ClientConfig{},
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("NewClientWithConfig: %v", err)
	}
	client.Close()

	assertNoEndpointLatencyRegistryCleanupGoroutine(t)
}

func assertNoEndpointLatencyRegistryCleanupGoroutine(t *testing.T) {
	t.Helper()

	buf := make([]byte, 64<<10)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			if bytes.Contains(buf[:n], []byte("endpointLatencyRegistry).runCleanup")) {
				t.Error("endpoint latency registry cleanup goroutine is running")
			}
			return
		}
		buf = make([]byte, 2*len(buf))
	}
}
