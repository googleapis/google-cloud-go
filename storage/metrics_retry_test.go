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
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/googleapis/gax-go/v2/callctx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// peerAt returns a gRPC peer with the given address.
func peerAt(addr string) *peer.Peer {
	a, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		panic(err)
	}
	return &peer.Peer{Addr: a}
}

func TestParseRetryInfo(t *testing.T) {
	for _, tc := range []struct {
		vals []string
		want retryInfo
	}{
		{nil, retryInfo{}},
		{[]string{"gl-go/1.26 gccl/1.2"}, retryInfo{}},
		{[]string{"gl-go/1.26 gccl/1.2 gccl-invocation-id/abc gccl-attempt-count/3"}, retryInfo{"abc", 3}},
		{[]string{"gccl-invocation-id/abc gccl-attempt-count/1", "gccl-attempt-count/2"}, retryInfo{"abc", 2}},
		{[]string{"gccl-attempt-count/x"}, retryInfo{}},
	} {
		if got := parseRetryInfo(tc.vals); got != tc.want {
			t.Errorf("parseRetryInfo(%q) = %+v, want %+v", tc.vals, got, tc.want)
		}
	}
	if (retryInfo{"a", 1}).isRetry() || !(retryInfo{"a", 2}).isRetry() {
		t.Error("isRetry: attempt 1 is not a retry, attempt 2 is")
	}
}

func TestRetryTrackerObserve(t *testing.T) {
	var tr retryTracker
	steps := []struct {
		info      retryInfo
		errorType string
		want      string
	}{
		{retryInfo{"req1", 1}, "UNAVAILABLE", ""},            // first attempt fails
		{retryInfo{"req2", 1}, "RESOURCE_EXHAUSTED", ""},     // another request in flight fails
		{retryInfo{"req1", 2}, "UNAVAILABLE", "UNAVAILABLE"}, // retry of req1, fails again
		{retryInfo{"req2", 2}, "OK", "RESOURCE_EXHAUSTED"},   // retry of req2 succeeds
		{retryInfo{"req1", 3}, "OK", "UNAVAILABLE"},          // second retry of req1 succeeds
		{retryInfo{"req1", 1}, "OK", ""},                     // next chunk: count restarts, not a retry
		{retryInfo{"req3", 2}, "OK", errorTypeUnknown},       // retry whose failure was not observed
		{retryInfo{"", 2}, "OK", errorTypeUnknown},           // retry without an invocation id
		{retryInfo{"", 1}, "DEADLINE_EXCEEDED", ""},          // failure without an id is not remembered
	}
	for i, st := range steps {
		if got := tr.observe(st.info, st.errorType); got != st.want {
			t.Errorf("step %d observe(%+v, %s) = %q, want %q", i, st.info, st.errorType, got, st.want)
		}
	}
	if len(tr.last) != 0 {
		t.Errorf("tracker retained %v after all requests completed", tr.last)
	}
	var nilTracker *retryTracker
	if got := nilTracker.observe(retryInfo{"x", 2}, "OK"); got != errorTypeUnknown {
		t.Errorf("nil tracker retry reason = %q, want %q", got, errorTypeUnknown)
	}
	if got := nilTracker.observe(retryInfo{"x", 1}, "OK"); got != "" {
		t.Errorf("nil tracker first attempt reason = %q, want empty", got)
	}
}

// TestHTTPRetriesRecordedWithReason runs two requests of one operation through
// the metrics round tripper: the first fails with 503 and is retried, the
// second fails with 429 and is retried. Retries are attributed to the error
// that caused them, while first attempts and chunk restarts are not counted.
func TestHTTPRetriesRecordedWithReason(t *testing.T) {
	cm, mr := accuracyMetrics(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 3:
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			w.Write([]byte("ok"))
		}
	}))
	defer srv.Close()
	rt := &metricsRoundTripper{base: http.DefaultTransport, metrics: cm}
	ctx, record := cm.startOperation(context.Background(), "WriteObject", true)

	do := func(invocation string, attempt int) {
		req, _ := http.NewRequestWithContext(ctx, "PUT", srv.URL+"/upload/storage/v1/b/bucket/o", nil)
		req.Header.Set(xGoogHeaderKey, fmt.Sprintf("gl-go/1 gccl-invocation-id/%s gccl-attempt-count/%d", invocation, attempt))
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
		io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	do("req-a", 1) // 503
	do("req-a", 2) // retry, ok
	do("req-b", 1) // 429
	do("req-b", 2) // retry, ok
	do("req-b", 1) // next chunk of the same request: not a retry
	record(nil)

	if c, _ := metricPoints(t, mr, "gcp.storage.client.attempts", nil); c != 5 {
		t.Errorf("attempts = %d, want 5", c)
	}
	if c, _ := metricPoints(t, mr, "gcp.storage.client.retries", nil); c != 2 {
		t.Errorf("retries = %d, want 2", c)
	}
	for reason, want := range map[string]int64{"UNAVAILABLE": 1, "RESOURCE_EXHAUSTED": 1, "OK": 0} {
		if c, _ := metricPoints(t, mr, "gcp.storage.client.retries", map[string]string{"error.type": reason, "gcp.storage.operation": "WriteObject", "rpc.system.name": "http"}); c != want {
			t.Errorf("retries{error.type=%s} = %d, want %d", reason, c, want)
		}
	}
	if c, _ := metricPoints(t, mr, "gcp.storage.client.operations", nil); c != 1 {
		t.Errorf("operations = %d, want 1", c)
	}
}

// TestGRPCRetryRecordedWithReason verifies the gRPC path, where the attempt
// header is carried in the context by the retry loop.
func TestGRPCRetryRecordedWithReason(t *testing.T) {
	cm, mr := accuracyMetrics(t)
	ctx, record := cm.startOperation(context.Background(), "ReadObject", false)
	const method = "/google.storage.v2.Storage/ReadObject"
	attempt := func(n int, err error) {
		actx := callctx.SetHeaders(ctx, xGoogHeaderKey, fmt.Sprintf("gccl-invocation-id/inv gccl-attempt-count/%d", n))
		cm.recordRPC(actx, method, "dns:///storage.googleapis.com:443", 0.01, err, true, peerAt("74.125.126.207:443"))
	}
	attempt(1, status.Error(codes.Unavailable, "try again"))
	attempt(2, status.Error(codes.DeadlineExceeded, "slow"))
	attempt(3, nil)
	record(nil)

	if c, _ := metricPoints(t, mr, "gcp.storage.client.retries", map[string]string{"error.type": "UNAVAILABLE", "rpc.system.name": "grpc", "gcp.storage.operation": "ReadObject", "server.address": "storage.googleapis.com"}); c != 1 {
		t.Errorf("retries{UNAVAILABLE} = %d, want 1", c)
	}
	if c, _ := metricPoints(t, mr, "gcp.storage.client.retries", map[string]string{"error.type": "DEADLINE_EXCEEDED"}); c != 1 {
		t.Errorf("retries{DEADLINE_EXCEEDED} = %d, want 1", c)
	}
	if c, _ := metricPoints(t, mr, "gcp.storage.client.retries", nil); c != 2 {
		t.Errorf("retries = %d, want 2", c)
	}
}

// TestRetriesConcurrentRequests checks that concurrent requests of one
// operation (as in parallel composite uploads or multi-range downloads) are
// attributed independently. Run with -race.
func TestRetriesConcurrentRequests(t *testing.T) {
	cm, mr := accuracyMetrics(t)
	ctx, record := cm.startOperation(context.Background(), "WriteObject", false)
	const method = "/google.storage.v2.Storage/WriteObject"
	reasons := []codes.Code{codes.Unavailable, codes.ResourceExhausted, codes.Internal, codes.Aborted}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			inv := fmt.Sprintf("part-%d", i)
			code := reasons[i%len(reasons)]
			a1 := callctx.SetHeaders(ctx, xGoogHeaderKey, "gccl-invocation-id/"+inv+" gccl-attempt-count/1")
			cm.recordRPC(a1, method, "storage.googleapis.com:443", 0.01, status.Error(code, "fail"), true, peerAt("34.126.10.53:1371"))
			a2 := callctx.SetHeaders(ctx, xGoogHeaderKey, "gccl-invocation-id/"+inv+" gccl-attempt-count/2")
			cm.recordRPC(a2, method, "storage.googleapis.com:443", 0.01, nil, true, peerAt("34.126.10.53:1371"))
		}(i)
	}
	wg.Wait()
	record(nil)
	if c, _ := metricPoints(t, mr, "gcp.storage.client.retries", nil); c != 32 {
		t.Errorf("retries = %d, want 32", c)
	}
	for _, code := range reasons {
		if c, _ := metricPoints(t, mr, "gcp.storage.client.retries", map[string]string{"error.type": grpcCodeToString(code)}); c != 8 {
			t.Errorf("retries{%s} = %d, want 8", grpcCodeToString(code), c)
		}
	}
	if c, _ := metricPoints(t, mr, "gcp.storage.client.retries", map[string]string{"error.type": errorTypeUnknown}); c != 0 {
		t.Errorf("retries{UNKNOWN} = %d, want 0", c)
	}
	if c, _ := metricPoints(t, mr, "gcp.storage.client.retries", map[string]string{networkPathAttr: networkPathDirect}); c != 32 {
		t.Errorf("retries{network.path=directpath} = %d, want 32", c)
	}
}

func TestNetworkPathClassification(t *testing.T) {
	for addr, want := range map[string]string{
		"34.126.10.53:1371":          networkPathDirect, // DirectPath IPv4 range 34.126.0.0/18
		"34.126.63.255:443":          networkPathDirect,
		"34.126.64.1:443":            networkPathCloud,  // just outside the /18
		"[2001:4860:8040::1]:443":    networkPathDirect, // DirectPath IPv6 range
		"[2001:4860:8080::1]:443":    networkPathCloud,
		"74.125.126.207:443":         networkPathCloud,  // Google Front End
		"[::ffff:34.126.1.1]:443":    networkPathDirect, // IPv4-mapped IPv6
		"127.0.0.1:8888":             networkPathCloud,  // emulator
		"storage.googleapis.com:443": networkPathCloud,  // not an IP
	} {
		if got := networkPathOfAddr(addr); got != want {
			t.Errorf("networkPathOfAddr(%q) = %q, want %q", addr, got, want)
		}
	}
	if got := networkPathOfPeer(nil); got != networkPathUnknown {
		t.Errorf("networkPathOfPeer(nil) = %q, want %q", got, networkPathUnknown)
	}
	if got := networkPathOfPeer(&peer.Peer{}); got != networkPathUnknown {
		t.Errorf("networkPathOfPeer(no addr) = %q, want %q", got, networkPathUnknown)
	}
}

// TestNetworkPathOnAttemptMetrics checks that the path of the connection that
// served each gRPC attempt is recorded on the attempt-scoped metrics, that an
// attempt without a connection is "unknown", and that HTTP attempts are
// always "cloudpath".
func TestNetworkPathOnAttemptMetrics(t *testing.T) {
	cm, mr := accuracyMetrics(t)
	ctx, record := cm.startOperation(context.Background(), "GetObject", false)
	const method = "/google.storage.v2.Storage/GetObject"
	cm.recordRPC(ctx, method, "google-c2p:///storage.googleapis.com", 0.01, nil, true, peerAt("34.126.8.42:10106"))
	cm.recordRPC(ctx, method, "google-c2p:///storage.googleapis.com", 0.01, nil, true, peerAt("74.125.126.207:443"))
	cm.recordRPC(ctx, method, "google-c2p:///storage.googleapis.com", 0.01, status.Error(codes.Unavailable, "dial"), false, nil)
	record(nil)

	for _, m := range []string{"gcp.storage.client.attempts", "rpc.client.call.duration"} {
		for path, want := range map[string]int64{networkPathDirect: 1, networkPathCloud: 1, networkPathUnknown: 1} {
			if c, _ := metricPoints(t, mr, m, map[string]string{networkPathAttr: path, "server.address": "storage.googleapis.com"}); c != want {
				t.Errorf("%s{network.path=%s} = %d, want %d", m, path, c, want)
			}
		}
	}
	if c, _ := metricPoints(t, mr, "gcp.storage.client.ttfb", map[string]string{networkPathAttr: networkPathDirect}); c != 1 {
		t.Errorf("ttfb{directpath} = %d, want 1", c)
	}
	if c, _ := metricPoints(t, mr, "gcp.storage.client.errors", map[string]string{networkPathAttr: networkPathUnknown, "error.type": "UNAVAILABLE"}); c != 1 {
		t.Errorf("errors{unknown,UNAVAILABLE} = %d, want 1", c)
	}
	// Operation-scoped metrics do not carry the attribute.
	if c, _ := metricPoints(t, mr, "gcp.storage.client.operations", map[string]string{networkPathAttr: networkPathDirect}); c != 0 {
		t.Errorf("operations carry network.path (%d)", c)
	}
	if c, _ := metricPoints(t, mr, "gcp.storage.client.operations", nil); c != 1 {
		t.Errorf("operations = %d, want 1", c)
	}

	// HTTP: always cloudpath.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer srv.Close()
	rt := &metricsRoundTripper{base: http.DefaultTransport, metrics: cm}
	hctx, hrecord := cm.startOperation(context.Background(), "GetObject", true)
	req, _ := http.NewRequestWithContext(hctx, "GET", srv.URL+"/storage/v1/b/b/o/o", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()
	hrecord(nil)
	for _, m := range []string{"gcp.storage.client.attempts", "http.client.request.duration", "gcp.storage.client.ttfb"} {
		if c, _ := metricPoints(t, mr, m, map[string]string{"rpc.system.name": "http", networkPathAttr: networkPathCloud}); c != 1 {
			t.Errorf("%s{http,cloudpath} = %d, want 1", m, c)
		}
	}
}
