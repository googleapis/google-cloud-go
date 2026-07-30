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

package debugview

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"cloud.google.com/go/bigtable"
	btransport "cloud.google.com/go/bigtable/internal/transport"
)

// fakeSessionProvider satisfies bigtable.SessionDebugProvider for the
// four session-backed views (sessionz / afez / flightz / loadz). Each
// per-view test file sets whichever field its view exercises; the other
// two methods return zero values.
type fakeSessionProvider struct {
	pools    []btransport.PoolSnapshot
	diverter btransport.DiverterSnapshot
	lb       []btransport.LoadBalancingSnapshot
}

func (f fakeSessionProvider) Snapshot() []btransport.PoolSnapshot { return f.pools }
func (f fakeSessionProvider) Diverter() btransport.DiverterSnapshot {
	return f.diverter
}
func (f fakeSessionProvider) LoadBalancingSnapshots() []btransport.LoadBalancingSnapshot {
	return f.lb
}

// fakeChannelProvider satisfies bigtable.ChannelDebugProvider for
// channelz tests.
type fakeChannelProvider struct {
	pools []bigtable.ChannelPoolDebug
}

func (f fakeChannelProvider) Snapshot() []bigtable.ChannelPoolDebug { return f.pools }

// fakeConfigProvider satisfies bigtable.ConfigDebugProvider for configz
// tests.
type fakeConfigProvider struct {
	snap btransport.ConfigSnapshot
}

func (f fakeConfigProvider) Snapshot() btransport.ConfigSnapshot { return f.snap }

// get is a tiny helper wrapping httptest to keep the per-view test
// files short. Every view's tests exercise Handler via the same shape.
func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// head returns the first n bytes of b as a string, with an ellipsis when
// truncated. Used by tests that need to include a body snippet in an
// error message without dumping the whole page.
func head(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
