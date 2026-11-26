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
	"time"

	gapic "cloud.google.com/go/storage/internal/apiv2"
	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"github.com/googleapis/gax-go/v2/callctx"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func peerAt(addr string) *peer.Peer {
	a, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		panic(err)
	}
	return &peer.Peer{Addr: a}
}

func newTestRetryMetrics(t *testing.T) (*clientMetrics, *sdkmetric.ManualReader) {
	t.Helper()
	mr := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	cm, _, err := initMetrics(context.Background(), "test-proj", &storageConfig{
		enableOtelMetrics: true,
		meterProvider:     provider,
	})
	if err != nil {
		t.Fatalf("initMetrics: %v", err)
	}
	return cm, mr
}

func collectRetryPoints(t *testing.T, mr *sdkmetric.ManualReader, filter map[string]string) []metricdata.DataPoint[int64] {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := mr.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	var out []metricdata.DataPoint[int64]
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "gcp.storage.client.retries" {
				continue
			}
			for _, dp := range m.Data.(metricdata.Sum[int64]).DataPoints {
				assertExactAttrKeys(t, m.Name, dp.Attributes,
					"error.type", "gcp.storage.operation", "network.path", "rpc.system.name", "server.address")
				match := true
				for k, want := range filter {
					if getAttr(dp, k) != want {
						match = false
						break
					}
				}
				if match {
					out = append(out, dp)
				}
			}
		}
	}
	return out
}

func sumRetryPoints(dps []metricdata.DataPoint[int64]) int64 {
	var total int64
	for _, dp := range dps {
		total += dp.Value
	}
	return total
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
		{[]string{"gccl-invocation-id/abc gccl-attempt-count/2", "gccl-attempt-count/1"}, retryInfo{"abc", 2}},
		{[]string{"gccl-attempt-count/x"}, retryInfo{}},
	} {
		if got := parseRetryInfo(tc.vals); got != tc.want {
			t.Errorf("parseRetryInfo(%q) = %+v, want %+v", tc.vals, got, tc.want)
		}
	}
	if (retryInfo{"a", 1}).isRetry() || !(retryInfo{"a", 2}).isRetry() {
		t.Error("isRetry: attempt 1 is not a retry, attempt 2 is")
	}
	mdCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(xGoogHeaderKey, "gccl-invocation-id/md-id gccl-attempt-count/4"))
	if got := grpcRetryInfo(mdCtx); got != (retryInfo{"md-id", 4}) {
		t.Errorf("grpcRetryInfo(metadata) = %+v, want {md-id 4}", got)
	}
}

func TestRetryTracker_ProgressResetAndEviction(t *testing.T) {
	var tr retryTracker
	// Healthy attempt 1 must not retain an entry in tr.entries.
	if r := tr.observe(retryInfo{"ok-inv", 1}, "OK"); r != "" {
		t.Fatalf("attempt 1 OK reason = %q, want empty", r)
	}
	if len(tr.entries) != 0 {
		t.Fatalf("len(tr.entries) after healthy attempt 1 = %d, want 0", len(tr.entries))
	}

	// Simulate gRPCWriter withProgressReset on attempt 2: 1 (fail) -> 2 (fail) -> 2 (OK).
	if r := tr.observe(retryInfo{"pr-inv", 1}, "UNAVAILABLE"); r != "" {
		t.Fatalf("attempt 1 reason = %q, want empty", r)
	}
	if r := tr.observe(retryInfo{"pr-inv", 2}, "DEADLINE_EXCEEDED"); r != "UNAVAILABLE" {
		t.Fatalf("attempt 2 reason = %q, want UNAVAILABLE", r)
	}
	if r := tr.observe(retryInfo{"pr-inv", 2}, "OK"); r != "DEADLINE_EXCEEDED" {
		t.Fatalf("reset attempt 2 reason = %q, want DEADLINE_EXCEEDED", r)
	}
}

type errAfterHeaderBody struct {
	err error
}

func (e *errAfterHeaderBody) Read([]byte) (int, error) { return 0, e.err }
func (e *errAfterHeaderBody) Close() error             { return nil }

func TestHTTPRetryMidBodyUnexpectedEOFAttribution(t *testing.T) {
	cm, mr := newTestRetryMetrics(t)
	ctx, record := cm.startOperation(context.Background(), "ReadObject", true)

	var attempt int
	rt := &metricsRoundTripper{
		base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			attempt++
			if attempt == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       &errAfterHeaderBody{err: io.ErrUnexpectedEOF},
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(http.NoBody),
			}, nil
		}),
		metrics: cm,
	}
	for n := 1; n <= 2; n++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://storage.googleapis.com/storage/v1/b/bkt/o/obj?alt=media", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		req.Header.Set(xGoogHeaderKey, fmt.Sprintf("gccl-invocation-id/body-eof gccl-attempt-count/%d", n))
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip(%d): %v", n, err)
		}
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
	}
	record(nil)

	if got := sumRetryPoints(collectRetryPoints(t, mr, map[string]string{
		"error.type":            "CONNECTION_ERROR",
		"gcp.storage.operation": "ReadObject",
		"network.path":          networkPathCloud,
		"rpc.system.name":       "http",
		"server.address":        "storage.googleapis.com",
	})); got != 1 {
		t.Errorf("retries{CONNECTION_ERROR} after mid-body UnexpectedEOF = %d, want 1", got)
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
		{retryInfo{"req1", 1}, "CONNECTION_ERROR", ""},       // restarted chunk fails on attempt 1
		{retryInfo{"req1", 2}, "OK", "CONNECTION_ERROR"},     // restarted chunk retries and gets its failure reason
		{retryInfo{"req3", 2}, "OK", "UNKNOWN"},              // retry whose failure was not observed
		{retryInfo{"", 2}, "OK", "UNKNOWN"},                  // retry without an invocation id
		{retryInfo{"", 1}, "DEADLINE_EXCEEDED", ""},          // failure without an id is not remembered
		// Multi-RPC attempt (writeLoop QueryWriteStatus + BidiWriteObject) sharing
		// one invocationID and attempt number:
		{retryInfo{"wl1", 1}, "TIMEOUT", ""},       // attempt 1 fails with stall timeout
		{retryInfo{"wl1", 2}, "OK", "TIMEOUT"},     // QueryWriteStatus succeeds on attempt 2
		{retryInfo{"wl1", 2}, "UNAVAILABLE", ""},   // BidiWriteObject on same attempt 2 does not duplicate retry
		{retryInfo{"wl1", 3}, "OK", "UNAVAILABLE"}, // attempt 3 receives BidiWriteObject failure reason
	}
	for i, st := range steps {
		if got := tr.observe(st.info, st.errorType); got != st.want {
			t.Errorf("step %d observe(%+v, %s) = %q, want %q", i, st.info, st.errorType, got, st.want)
		}
	}
	var nilTracker *retryTracker
	if got := nilTracker.observe(retryInfo{"x", 2}, "OK"); got != "UNKNOWN" {
		t.Errorf("nil tracker retry reason = %q, want UNKNOWN", got)
	}
	if got := nilTracker.observe(retryInfo{"x", 1}, "OK"); got != "" {
		t.Errorf("nil tracker first attempt reason = %q, want empty", got)
	}
}

func TestHTTPRetriesRecordedWithReason(t *testing.T) {
	cm, mr := newTestRetryMetrics(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 3:
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			_, _ = w.Write([]byte("ok"))
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
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
	}
	do("req-a", 1) // 503
	do("req-a", 2) // retry, ok
	do("req-b", 1) // 429
	do("req-b", 2) // retry, ok
	do("req-b", 1) // next chunk of the same request: not a retry
	record(nil)

	if got := sumRetryPoints(collectRetryPoints(t, mr, nil)); got != 2 {
		t.Errorf("retries = %d, want 2", got)
	}
	for reason, want := range map[string]int64{"UNAVAILABLE": 1, "RESOURCE_EXHAUSTED": 1, "OK": 0} {
		got := sumRetryPoints(collectRetryPoints(t, mr, map[string]string{
			"error.type":            reason,
			"gcp.storage.operation": "WriteObject",
			"rpc.system.name":       "http",
			"network.path":          networkPathCloud,
		}))
		if got != want {
			t.Errorf("retries{error.type=%s} = %d, want %d", reason, got, want)
		}
	}
}

func TestGRPCRetryRecordedWithReasonAndSlowStream(t *testing.T) {
	cm, mr := newTestRetryMetrics(t)
	ctx, record := cm.startOperation(context.Background(), "ReadObject", false)
	const method = "/google.storage.v2.Storage/ReadObject"
	attempt := func(n int, err error) {
		actx := callctx.SetHeaders(ctx, xGoogHeaderKey, fmt.Sprintf("gccl-invocation-id/inv gccl-attempt-count/%d", n))
		cm.recordRPC(actx, method, "dns:///storage.googleapis.com:443", 0.01, err, peerAt("74.125.126.207:443"))
	}
	attempt(1, status.Error(codes.Unavailable, "try again"))
	attempt(2, status.Error(codes.DeadlineExceeded, "slow"))
	attempt(3, nil)
	cm.recordRetryReason(ctx, "grpc", "WriteObject", "storage.googleapis.com", networkPathDirect, "SLOW_STREAM")
	record(nil)

	for _, tc := range []struct {
		filter map[string]string
		want   int64
	}{
		{map[string]string{"error.type": "UNAVAILABLE", "gcp.storage.operation": "ReadObject", "network.path": networkPathCloud, "rpc.system.name": "grpc", "server.address": "storage.googleapis.com"}, 1},
		{map[string]string{"error.type": "DEADLINE_EXCEEDED", "gcp.storage.operation": "ReadObject", "network.path": networkPathCloud, "rpc.system.name": "grpc", "server.address": "storage.googleapis.com"}, 1},
		{map[string]string{"error.type": "SLOW_STREAM", "gcp.storage.operation": "WriteObject", "network.path": networkPathDirect, "rpc.system.name": "grpc", "server.address": "storage.googleapis.com"}, 1},
	} {
		if got := sumRetryPoints(collectRetryPoints(t, mr, tc.filter)); got != tc.want {
			t.Errorf("retries%v = %d, want %d", tc.filter, got, tc.want)
		}
	}
}

func TestRetriesConcurrentRequests(t *testing.T) {
	cm, mr := newTestRetryMetrics(t)
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
			cm.recordRPC(a1, method, "storage.googleapis.com:443", 0.01, status.Error(code, "fail"), peerAt("34.126.10.53:1371"))
			a2 := callctx.SetHeaders(ctx, xGoogHeaderKey, "gccl-invocation-id/"+inv+" gccl-attempt-count/2")
			cm.recordRPC(a2, method, "storage.googleapis.com:443", 0.01, nil, peerAt("34.126.10.53:1371"))
		}(i)
	}
	wg.Wait()
	record(nil)
	if got := sumRetryPoints(collectRetryPoints(t, mr, map[string]string{networkPathAttr: networkPathDirect})); got != 32 {
		t.Errorf("retries{network.path=directpath} = %d, want 32", got)
	}
	for _, code := range reasons {
		if got := sumRetryPoints(collectRetryPoints(t, mr, map[string]string{"error.type": grpcCodeToString(code)})); got != 8 {
			t.Errorf("retries{%s} = %d, want 8", grpcCodeToString(code), got)
		}
	}
}

type slowChunkRotateServer struct {
	storagepb.UnimplementedStorageServer
	streams atomic.Int32
}

func (s *slowChunkRotateServer) StartResumableWrite(_ context.Context, _ *storagepb.StartResumableWriteRequest) (*storagepb.StartResumableWriteResponse, error) {
	return &storagepb.StartResumableWriteResponse{UploadId: "upid-slow"}, nil
}

func (s *slowChunkRotateServer) BidiWriteObject(stream storagepb.Storage_BidiWriteObjectServer) error {
	streamNum := s.streams.Add(1)
	var offset int64
	for chunkIdx := 0; ; chunkIdx++ {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if streamNum == 1 && chunkIdx == 1 {
			// Delay the second chunk ack past maxDelay (50ms) so chunkDelay deterministically triggers rotation.
			time.Sleep(60 * time.Millisecond)
		}
		offset += int64(len(req.GetChecksummedData().GetContent()))
		if req.GetFinishWrite() {
			return stream.Send(&storagepb.BidiWriteObjectResponse{
				WriteStatus: &storagepb.BidiWriteObjectResponse_Resource{
					Resource: &storagepb.Object{Name: "obj", Size: offset},
				},
			})
		}
		if req.GetFlush() {
			if err := stream.Send(&storagepb.BidiWriteObjectResponse{
				WriteStatus: &storagepb.BidiWriteObjectResponse_PersistedSize{PersistedSize: offset},
			}); err != nil {
				return err
			}
		}
	}
}

func TestConnectSlowStreamRotationEmitsRetry(t *testing.T) {
	cm, mr := newTestRetryMetrics(t)
	srv := &slowChunkRotateServer{}
	lis := bufconn.Listen(1024 * 1024)
	gsrv := grpc.NewServer()
	storagepb.RegisterStorageServer(gsrv, srv)
	go func() { _ = gsrv.Serve(lis) }()
	t.Cleanup(gsrv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	raw, err := gapic.NewClient(context.Background(), option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("gapic.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })

	ctx, record := cm.startOperation(context.Background(), "WriteObject", false)
	metricsStateFromContext(ctx).setTarget("storage.googleapis.com:443")

	sender := &gRPCResumableBidiWriteBufferSender{
		raw:               raw,
		bucket:            "projects/_/buckets/bkt",
		startWriteRequest: &storagepb.StartResumableWriteRequest{},
		chunkDelay: &writeChunkDelay{
			minDelay: 2 * time.Millisecond,
			maxDelay: 50 * time.Millisecond,
		},
	}

	requests := make(chan gRPCBidiWriteRequest, 4)
	requestAcks := make(chan struct{}, 1)
	completions := make(chan gRPCBidiWriteCompletion, 4)
	sender.connect(ctx, gRPCBufSenderChans{requests: requests, requestAcks: requestAcks, completions: completions})

	for i, finish := range []bool{false, false, false, true} {
		requests <- gRPCBidiWriteRequest{offset: int64(i * 4), buf: []byte("abcd"), flush: true, finishWrite: finish}
		if _, ok := <-completions; !ok {
			t.Fatalf("completions closed early at chunk %d (err=%v)", i, sender.err())
		}
	}
	close(requests)
	for range completions {
	}
	record(nil)

	if srv.streams.Load() != 2 {
		t.Fatalf("BidiWriteObject streams opened = %d, want 2 (rotation)", srv.streams.Load())
	}
	if got := sumRetryPoints(collectRetryPoints(t, mr, map[string]string{
		"error.type":            "SLOW_STREAM",
		"gcp.storage.operation": "WriteObject",
		"network.path":          networkPathCloud,
		"rpc.system.name":       "grpc",
		"server.address":        "storage.googleapis.com",
	})); got != 1 {
		t.Errorf("retries{SLOW_STREAM} = %d, want 1", got)
	}
}
