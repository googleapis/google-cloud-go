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
	"bytes"
	"context"
	"hash/crc32"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/storage/experimental"
	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"github.com/google/go-cmp/cmp"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func newBufconnGRPCClient(t *testing.T, srv storagepb.StorageServer, extraOpts ...option.ClientOption) *Client {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	s := grpc.NewServer()
	storagepb.RegisterStorageServer(s, srv)
	go s.Serve(lis)
	t.Cleanup(s.Stop)

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	opts := []option.ClientOption{
		option.WithEndpoint("passthrough:///bufconn"),
		option.WithGRPCDialOption(grpc.WithContextDialer(dialer)),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
		internaloption.EnableDirectPath(false),
		WithDisabledClientMetrics(),
		WithGRPCBidiReads(),
	}
	opts = append(opts, extraOpts...)
	client, err := NewGRPCClient(t.Context(), opts...)
	if err != nil {
		t.Fatalf("NewGRPCClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func sendBidiReadRanges(stream storagepb.Storage_BidiReadObjectServer, data []byte, ranges []*storagepb.ReadRange) error {
	for _, rr := range ranges {
		end := rr.GetReadOffset() + rr.GetReadLength()
		if rr.GetReadLength() == 0 || end > int64(len(data)) {
			end = int64(len(data))
		}
		chunk := data[rr.GetReadOffset():end]
		crc := crc32.Checksum(chunk, crc32cTable)
		if err := stream.Send(&storagepb.BidiReadObjectResponse{
			ObjectDataRanges: []*storagepb.ObjectRangeData{{
				ChecksummedData: &storagepb.ChecksummedData{Content: chunk, Crc32C: &crc},
				ReadRange:       &storagepb.ReadRange{ReadOffset: rr.GetReadOffset(), ReadLength: int64(len(chunk)), ReadId: rr.GetReadId()},
				RangeEnd:        true,
			}},
		}); err != nil {
			return err
		}
	}
	return nil
}

type mrdStallOnOpenServer struct {
	storagepb.UnimplementedStorageServer
	data         []byte
	stallStreams int32
	streams      atomic.Int32
	mu           sync.Mutex
	xGoogHeaders []string
}

func (s *mrdStallOnOpenServer) BidiReadObject(stream storagepb.Storage_BidiReadObjectServer) error {
	md, _ := metadata.FromIncomingContext(stream.Context())
	xGoog := strings.Join(md.Get("x-goog-api-client"), " ")
	s.mu.Lock()
	s.xGoogHeaders = append(s.xGoogHeaders, xGoog)
	s.mu.Unlock()

	attempt := s.streams.Add(1)
	if _, err := stream.Recv(); err != nil {
		return err
	}
	stallLimit := s.stallStreams
	if stallLimit == 0 {
		stallLimit = 1
	}
	if attempt <= stallLimit {
		<-stream.Context().Done()
		return stream.Context().Err()
	}
	if err := stream.Send(&storagepb.BidiReadObjectResponse{
		Metadata:   &storagepb.Object{Size: int64(len(s.data)), Generation: 1},
		ReadHandle: &storagepb.BidiReadHandle{Handle: []byte("handle-1")},
	}); err != nil {
		return err
	}
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := sendBidiReadRanges(stream, s.data, req.GetReadRanges()); err != nil {
			return err
		}
	}
}

func TestMultiRangeDownloaderReadStallTimeout(t *testing.T) {
	data := []byte("0123456789abcdef0123456789abcdef")
	srv := &mrdStallOnOpenServer{data: data, stallStreams: 1}
	client := newBufconnGRPCClient(t, srv, experimental.WithReadStallTimeout(&experimental.ReadStallTimeoutConfig{
		TargetPercentile: 0.99,
		Min:              100 * time.Millisecond,
	}))
	client.SetRetry(WithBackoff(gax.Backoff{Initial: 10 * time.Millisecond}))

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	start := time.Now()
	mrd, err := client.Bucket("test-bucket").Object("test-object").NewMultiRangeDownloader(ctx)
	if err != nil {
		t.Fatalf("NewMultiRangeDownloader: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed >= 2*time.Second {
		t.Errorf("NewMultiRangeDownloader elapsed = %v, want < 2s", elapsed)
	}
	if got := srv.streams.Load(); got != 2 {
		t.Errorf("srv.streams.Load() = %d, want 2", got)
	}

	srv.mu.Lock()
	headers := append([]string(nil), srv.xGoogHeaders...)
	srv.mu.Unlock()
	if len(headers) != 2 {
		t.Fatalf("len(srv.xGoogHeaders) = %d, want 2", len(headers))
	}
	if !strings.Contains(headers[0], "gccl-invocation-id/") || !strings.Contains(headers[0], "gccl-attempt-count/1") {
		t.Errorf("headers[0] = %q, want gccl-invocation-id and gccl-attempt-count/1", headers[0])
	}
	if !strings.Contains(headers[1], "gccl-attempt-count/2") {
		t.Errorf("headers[1] = %q, want gccl-attempt-count/2", headers[1])
	}

	var buf bytes.Buffer
	var rangeErr error
	mrd.Add(&buf, 0, int64(len(data)), func(_, _ int64, err error) {
		rangeErr = err
	})
	mrd.Wait()
	if err := mrd.Close(); err != nil {
		t.Errorf("mrd.Close() = %v, want nil", err)
	}
	if rangeErr != nil {
		t.Fatalf("mrd.Add callback err = %v, want nil", rangeErr)
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Errorf("mrd.Add data = %q, want %q", buf.Bytes(), data)
	}
}

func TestMultiRangeDownloaderReadStallExhaustsMaxAttempts(t *testing.T) {
	data := []byte("0123456789abcdef0123456789abcdef")
	srv := &mrdStallOnOpenServer{data: data, stallStreams: 5}
	client := newBufconnGRPCClient(t, srv, experimental.WithReadStallTimeout(&experimental.ReadStallTimeoutConfig{
		TargetPercentile: 0.99,
		Min:              50 * time.Millisecond,
	}))
	client.SetRetry(
		WithBackoff(gax.Backoff{Initial: 5 * time.Millisecond, Max: 10 * time.Millisecond, Multiplier: 2}),
		WithMaxAttempts(2),
	)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	_, err := client.Bucket("test-bucket").Object("test-object").NewMultiRangeDownloader(ctx)
	if err == nil {
		t.Fatalf("NewMultiRangeDownloader() = nil, want error after exhausting MaxAttempts(2)")
	}
	wantSubstr := "retry failed after 2 attempts"
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("NewMultiRangeDownloader() err = %q, want it to contain %q", err.Error(), wantSubstr)
	}
	if got := srv.streams.Load(); got != 2 {
		t.Errorf("srv.streams.Load() = %d, want 2", got)
	}
}

type mrdReconnectServer struct {
	storagepb.UnimplementedStorageServer
	data            []byte
	delayBeforeFail time.Duration
	mu              sync.Mutex
	streams         int
	xGoogHeaders    []string
	progressStreams map[int]bool
	failStreams     map[int]bool
}

func (s *mrdReconnectServer) BidiReadObject(stream storagepb.Storage_BidiReadObjectServer) error {
	md, _ := metadata.FromIncomingContext(stream.Context())
	xGoog := strings.Join(md.Get("x-goog-api-client"), " ")

	s.mu.Lock()
	s.streams++
	streamNum := s.streams
	s.xGoogHeaders = append(s.xGoogHeaders, xGoog)
	sendProgress := s.progressStreams[streamNum]
	shouldFail := s.failStreams[streamNum]
	s.mu.Unlock()

	if _, err := stream.Recv(); err != nil {
		return err
	}

	if err := stream.Send(&storagepb.BidiReadObjectResponse{
		Metadata:   &storagepb.Object{Size: int64(len(s.data)), Generation: 1},
		ReadHandle: &storagepb.BidiReadHandle{Handle: []byte("handle-1")},
	}); err != nil {
		return err
	}

	req, err := stream.Recv()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}

	if sendProgress {
		ranges := req.GetReadRanges()
		if len(ranges) > 0 {
			rr := ranges[0]
			partLen := int64(8)
			if rr.GetReadLength() > 0 && rr.GetReadLength() < partLen {
				partLen = rr.GetReadLength()
			}
			chunk := s.data[rr.GetReadOffset() : rr.GetReadOffset()+partLen]
			crc := crc32.Checksum(chunk, crc32cTable)
			if err := stream.Send(&storagepb.BidiReadObjectResponse{
				ObjectDataRanges: []*storagepb.ObjectRangeData{{
					ChecksummedData: &storagepb.ChecksummedData{Content: chunk, Crc32C: &crc},
					ReadRange:       &storagepb.ReadRange{ReadOffset: rr.GetReadOffset(), ReadLength: partLen, ReadId: rr.GetReadId()},
					RangeEnd:        false,
				}},
			}); err != nil {
				return err
			}
		}
	}

	if shouldFail {
		if s.delayBeforeFail > 0 {
			time.Sleep(s.delayBeforeFail)
		}
		return status.Error(codes.Unavailable, "injected mid-stream unavailable")
	}

	if err := sendBidiReadRanges(stream, s.data, req.GetReadRanges()); err != nil {
		return err
	}

	for {
		if _, err := stream.Recv(); err != nil {
			return nil
		}
	}
}

func TestMultiRangeDownloaderMidStreamReconnectBackoffAndMaxAttempts(t *testing.T) {
	data := []byte("0123456789abcdef0123456789abcdef")
	srv := &mrdReconnectServer{
		data: data,
		failStreams: map[int]bool{
			1: true,
			2: true,
			3: true,
		},
	}
	client := newBufconnGRPCClient(t, srv)

	var retryMu sync.Mutex
	var gotContexts []*RetryContext
	const initialBackoff = 20 * time.Millisecond
	client.SetRetry(
		WithBackoff(gax.Backoff{
			Initial:    initialBackoff,
			Max:        80 * time.Millisecond,
			Multiplier: 2,
		}),
		WithMaxAttempts(3),
		WithErrorFuncWithContext(func(err error, retryCtx *RetryContext) bool {
			if retryCtx != nil {
				cp := *retryCtx
				retryMu.Lock()
				gotContexts = append(gotContexts, &cp)
				retryMu.Unlock()
			}
			return ShouldRetry(err)
		}),
	)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	mrd, err := client.Bucket("my-bucket").Object("my-object").NewMultiRangeDownloader(ctx)
	if err != nil {
		t.Fatalf("NewMultiRangeDownloader: %v", err)
	}

	start := time.Now()
	var buf bytes.Buffer
	var rangeErr error
	mrd.Add(&buf, 0, int64(len(data)), func(_, _ int64, err error) {
		rangeErr = err
	})
	mrd.Wait()
	elapsed := time.Since(start)
	_ = mrd.Close()

	if rangeErr == nil {
		t.Fatalf("mrd.Add callback err = nil, want error after max attempts")
	}
	wantSubstr := "retry failed after 3 attempts"
	if !strings.Contains(rangeErr.Error(), wantSubstr) {
		t.Errorf("mrd.Add callback err = %q, want it to contain %q", rangeErr.Error(), wantSubstr)
	}
	if elapsed < time.Millisecond {
		t.Errorf("mrd.Wait() elapsed = %v, want >= 1ms for mid-stream backoff", elapsed)
	}

	retryMu.Lock()
	defer retryMu.Unlock()
	if len(gotContexts) != 3 {
		t.Fatalf("len(gotContexts) = %d, want 3", len(gotContexts))
	}
	wantInvID := gotContexts[0].InvocationID
	if wantInvID == "" {
		t.Errorf("gotContexts[0].InvocationID = \"\", want non-empty UUID")
	}
	wantContexts := []*RetryContext{
		{Attempt: 1, InvocationID: wantInvID, Operation: "ReadObject", Bucket: "my-bucket", Object: "my-object"},
		{Attempt: 2, InvocationID: wantInvID, Operation: "ReadObject", Bucket: "my-bucket", Object: "my-object"},
		{Attempt: 3, InvocationID: wantInvID, Operation: "ReadObject", Bucket: "my-bucket", Object: "my-object"},
	}
	if diff := cmp.Diff(wantContexts, gotContexts); diff != "" {
		t.Errorf("RetryContext mismatch (-want +got):\n%s", diff)
	}

	srv.mu.Lock()
	headers := append([]string(nil), srv.xGoogHeaders...)
	srv.mu.Unlock()
	if len(headers) != 3 {
		t.Fatalf("len(srv.xGoogHeaders) = %d, want 3", len(headers))
	}
	if !strings.Contains(headers[0], "gccl-invocation-id/") || !strings.Contains(headers[0], "gccl-attempt-count/1") {
		t.Errorf("headers[0] = %q, want gccl-invocation-id and gccl-attempt-count/1", headers[0])
	}
	wantAttempt2 := "gccl-invocation-id/" + wantInvID + " gccl-attempt-count/2"
	if !strings.Contains(headers[1], wantAttempt2) {
		t.Errorf("headers[1] = %q, want it to contain %q", headers[1], wantAttempt2)
	}
	wantAttempt3 := "gccl-invocation-id/" + wantInvID + " gccl-attempt-count/3"
	if !strings.Contains(headers[2], wantAttempt3) {
		t.Errorf("headers[2] = %q, want it to contain %q", headers[2], wantAttempt3)
	}
}

func TestMultiRangeDownloaderMidStreamReconnectResetOnProgress(t *testing.T) {
	data := []byte("0123456789abcdef0123456789abcdef")
	// Stream 1 fails before any range data; stream 2 sends 8 bytes of partial range
	// data (progress) then fails; stream 3 fails before any range data; stream 4
	// delivers the remaining 24 bytes and succeeds. With MaxAttempts(3), the 3
	// mid-stream failures only succeed because stream 2's partial range data resets
	// the reconnect attempt counter and starts a fresh InvocationID.
	srv := &mrdReconnectServer{
		data: data,
		progressStreams: map[int]bool{
			2: true,
		},
		failStreams: map[int]bool{
			1: true,
			2: true,
			3: true,
		},
	}
	client := newBufconnGRPCClient(t, srv)

	var retryMu sync.Mutex
	var gotContexts []*RetryContext
	client.SetRetry(
		WithBackoff(gax.Backoff{Initial: 5 * time.Millisecond, Max: 20 * time.Millisecond, Multiplier: 2}),
		WithMaxAttempts(3),
		WithErrorFuncWithContext(func(err error, retryCtx *RetryContext) bool {
			if retryCtx != nil {
				cp := *retryCtx
				retryMu.Lock()
				gotContexts = append(gotContexts, &cp)
				retryMu.Unlock()
			}
			return ShouldRetry(err)
		}),
	)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	mrd, err := client.Bucket("my-bucket").Object("my-object").NewMultiRangeDownloader(ctx)
	if err != nil {
		t.Fatalf("NewMultiRangeDownloader: %v", err)
	}

	var buf bytes.Buffer
	var rangeErr error
	mrd.Add(&buf, 0, int64(len(data)), func(_, _ int64, err error) {
		rangeErr = err
	})
	mrd.Wait()
	if err := mrd.Close(); err != nil {
		t.Errorf("mrd.Close() = %v, want nil", err)
	}
	if rangeErr != nil {
		t.Fatalf("mrd.Add callback err = %v, want nil", rangeErr)
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Errorf("mrd.Add data = %q, want %q", buf.Bytes(), data)
	}

	retryMu.Lock()
	defer retryMu.Unlock()
	if len(gotContexts) != 3 {
		t.Fatalf("len(gotContexts) = %d, want 3", len(gotContexts))
	}
	invID1 := gotContexts[0].InvocationID
	invID2 := gotContexts[1].InvocationID
	if invID1 == "" || invID2 == "" {
		t.Fatalf("got empty InvocationID: invID1=%q, invID2=%q", invID1, invID2)
	}
	if invID1 == invID2 {
		t.Errorf("gotContexts[1].InvocationID = %q, want new InvocationID after range data progress", invID2)
	}
	wantContexts := []*RetryContext{
		{Attempt: 1, InvocationID: invID1, Operation: "ReadObject", Bucket: "my-bucket", Object: "my-object"},
		{Attempt: 1, InvocationID: invID2, Operation: "ReadObject", Bucket: "my-bucket", Object: "my-object"},
		{Attempt: 2, InvocationID: invID2, Operation: "ReadObject", Bucket: "my-bucket", Object: "my-object"},
	}
	if diff := cmp.Diff(wantContexts, gotContexts); diff != "" {
		t.Errorf("RetryContext mismatch (-want +got):\n%s", diff)
	}

	srv.mu.Lock()
	headers := append([]string(nil), srv.xGoogHeaders...)
	srv.mu.Unlock()
	if len(headers) != 4 {
		t.Fatalf("len(srv.xGoogHeaders) = %d, want 4", len(headers))
	}
	wantStream2 := "gccl-invocation-id/" + invID1 + " gccl-attempt-count/2"
	if !strings.Contains(headers[1], wantStream2) {
		t.Errorf("headers[1] = %q, want it to contain %q", headers[1], wantStream2)
	}
	wantStream3 := "gccl-invocation-id/" + invID2 + " gccl-attempt-count/2"
	if !strings.Contains(headers[2], wantStream3) {
		t.Errorf("headers[2] = %q, want it to contain %q", headers[2], wantStream3)
	}
	wantStream4 := "gccl-invocation-id/" + invID2 + " gccl-attempt-count/3"
	if !strings.Contains(headers[3], wantStream4) {
		t.Errorf("headers[3] = %q, want it to contain %q", headers[3], wantStream4)
	}
}

func TestMultiRangeDownloaderMaxRetryDuration(t *testing.T) {
	data := []byte("0123456789abcdef0123456789abcdef")
	srv := &mrdReconnectServer{
		data:            data,
		delayBeforeFail: 15 * time.Millisecond,
		failStreams: map[int]bool{
			1: true,
			2: true,
			3: true,
		},
	}
	client := newBufconnGRPCClient(t, srv)
	client.SetRetry(
		WithBackoff(gax.Backoff{Initial: 5 * time.Millisecond, Max: 10 * time.Millisecond, Multiplier: 1.5}),
		WithMaxRetryDuration(20*time.Millisecond),
	)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	mrd, err := client.Bucket("my-bucket").Object("my-object").NewMultiRangeDownloader(ctx)
	if err != nil {
		t.Fatalf("NewMultiRangeDownloader: %v", err)
	}

	var buf bytes.Buffer
	var rangeErr error
	mrd.Add(&buf, 0, int64(len(data)), func(_, _ int64, err error) {
		rangeErr = err
	})
	mrd.Wait()
	_ = mrd.Close()

	if rangeErr == nil {
		t.Fatalf("mrd.Add callback err = nil, want maxRetryDuration error")
	}
	wantSubstr := "retry deadline of 20ms reached"
	if !strings.Contains(rangeErr.Error(), wantSubstr) {
		t.Errorf("mrd.Add callback err = %q, want it to contain %q", rangeErr.Error(), wantSubstr)
	}
}

type streamStateSnapshot struct {
	totalStreams        int
	activeSessions      int
	reconnectingStreams int
}

type inspectStreamsCmd struct {
	respC chan streamStateSnapshot
}

func (c *inspectStreamsCmd) apply(_ context.Context, m *multiRangeDownloaderManager) {
	snap := streamStateSnapshot{totalStreams: len(m.streams)}
	for _, s := range m.streams {
		if s.session != nil {
			snap.activeSessions++
		}
		if s.reconnecting {
			snap.reconnectingStreams++
		}
	}
	c.respC <- snap
}

func waitForStreamState(t *testing.T, mrd *MultiRangeDownloader, want streamStateSnapshot) {
	t.Helper()
	mgr := mrd.impl.(*multiRangeDownloaderManager)
	deadline := time.Now().Add(2 * time.Second)
	var last streamStateSnapshot
	for time.Now().Before(deadline) {
		respC := make(chan streamStateSnapshot, 1)
		select {
		case mgr.cmds <- &inspectStreamsCmd{respC: respC}:
			last = <-respC
			if last == want {
				return
			}
		case <-mgr.ctx.Done():
			t.Fatalf("waitForStreamState(%+v): manager context done; last state = %+v", want, last)
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("waitForStreamState timed out: got %+v, want %+v", last, want)
}

func TestMultiRangeDownloaderCloseCancelsReconnectBackoff(t *testing.T) {
	data := []byte("0123456789abcdef0123456789abcdef")
	srv := &mrdReconnectServer{
		data:        data,
		failStreams: map[int]bool{1: true},
	}
	client := newBufconnGRPCClient(t, srv)
	client.SetRetry(WithBackoff(gax.Backoff{Initial: 10 * time.Second, Max: 10 * time.Second, Multiplier: 1}))

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	mrd, err := client.Bucket("my-bucket").Object("my-object").NewMultiRangeDownloader(ctx)
	if err != nil {
		t.Fatalf("NewMultiRangeDownloader: %v", err)
	}

	var buf bytes.Buffer
	doneC := make(chan error, 1)
	mrd.Add(&buf, 0, int64(len(data)), func(_, _ int64, err error) {
		doneC <- err
	})

	waitForStreamState(t, mrd, streamStateSnapshot{totalStreams: 1, activeSessions: 0, reconnectingStreams: 1})

	closeStart := time.Now()
	if err := mrd.Close(); err != nil {
		t.Errorf("mrd.Close() = %v, want nil", err)
	}
	closeElapsed := time.Since(closeStart)
	if closeElapsed >= time.Second {
		t.Errorf("mrd.Close() elapsed = %v, want < 1s when canceling backoff sleep", closeElapsed)
	}

	select {
	case rangeErr := <-doneC:
		if rangeErr == nil {
			t.Errorf("mrd.Add callback err = nil, want non-nil error on Close")
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for mrd.Add callback after mrd.Close()")
	}
}

type mrdIdleDisconnectServer struct {
	storagepb.UnimplementedStorageServer
	data        []byte
	closeErr    error
	mu          sync.Mutex
	streams     int
	closeIdle   []chan struct{}
	streamGates map[int]chan struct{}
}

func (s *mrdIdleDisconnectServer) triggerIdleClose(t *testing.T, streamIdx int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		if streamIdx < len(s.closeIdle) {
			ch := s.closeIdle[streamIdx]
			s.mu.Unlock()
			close(ch)
			return
		}
		s.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("triggerIdleClose(%d): timed out waiting for server stream; got %d streams", streamIdx, s.streamCount())
}

func (s *mrdIdleDisconnectServer) streamCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.streams
}

func (s *mrdIdleDisconnectServer) BidiReadObject(stream storagepb.Storage_BidiReadObjectServer) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	if err := stream.Send(&storagepb.BidiReadObjectResponse{
		Metadata:   &storagepb.Object{Size: int64(len(s.data)), Generation: 1},
		ReadHandle: &storagepb.BidiReadHandle{Handle: []byte("handle-idle")},
	}); err != nil {
		return err
	}

	closeCh := make(chan struct{})
	s.mu.Lock()
	streamIdx := s.streams
	s.streams++
	s.closeIdle = append(s.closeIdle, closeCh)
	var gate chan struct{}
	if s.streamGates != nil {
		gate = s.streamGates[streamIdx]
	}
	closeErr := s.closeErr
	s.mu.Unlock()

	reqCh := make(chan *storagepb.BidiReadObjectRequest, 10)
	errCh := make(chan error, 1)
	go func() {
		for {
			req, err := stream.Recv()
			if err != nil {
				errCh <- err
				return
			}
			reqCh <- req
		}
	}()

	for {
		select {
		case <-closeCh:
			if closeErr == io.EOF {
				return nil
			}
			if closeErr != nil {
				return closeErr
			}
			return status.Error(codes.Unavailable, "idle stream closed by server")
		case <-stream.Context().Done():
			return stream.Context().Err()
		case err := <-errCh:
			if err == io.EOF {
				return nil
			}
			return err
		case req := <-reqCh:
			if gate != nil {
				select {
				case <-gate:
				case <-stream.Context().Done():
					return stream.Context().Err()
				}
			}
			if err := sendBidiReadRanges(stream, s.data, req.GetReadRanges()); err != nil {
				return err
			}
		}
	}
}

func TestMultiRangeDownloaderSingleStreamLazyReconnect(t *testing.T) {
	data := []byte("0123456789abcdef0123456789abcdef")

	for _, tc := range []struct {
		name     string
		closeErr error
	}{
		{name: "Unavailable", closeErr: status.Error(codes.Unavailable, "idle stream closed by server")},
		{name: "EOF", closeErr: io.EOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := &mrdIdleDisconnectServer{data: data, closeErr: tc.closeErr}
			client := newBufconnGRPCClient(t, srv)

			var errorFuncCalls atomic.Int32
			client.SetRetry(
				WithBackoff(gax.Backoff{Initial: 5 * time.Millisecond}),
				WithErrorFuncWithContext(func(err error, _ *RetryContext) bool {
					errorFuncCalls.Add(1)
					return ShouldRetry(err)
				}),
			)

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			mrd, err := client.Bucket("b").Object("o").NewMultiRangeDownloader(ctx)
			if err != nil {
				t.Fatalf("NewMultiRangeDownloader: %v", err)
			}

			var buf1 bytes.Buffer
			var err1 error
			mrd.Add(&buf1, 0, 16, func(_, _ int64, err error) { err1 = err })
			mrd.Wait()
			if err1 != nil {
				t.Fatalf("mrd.Add(first) err = %v, want nil", err1)
			}

			// Close the idle stream from the server and wait for the session to become dormant.
			srv.triggerIdleClose(t, 0)
			waitForStreamState(t, mrd, streamStateSnapshot{totalStreams: 1, activeSessions: 0, reconnectingStreams: 0})
			if got := srv.streamCount(); got != 1 {
				t.Fatalf("srv.streamCount() = %d while idle after server close, want 1", got)
			}
			if got := errorFuncCalls.Load(); got != 0 {
				t.Errorf("errorFuncCalls.Load() = %d on idle stream close, want 0", got)
			}

			// Wait() on a dormant idle downloader should return immediately.
			mrd.Wait()

			// Adding a new range must lazily reconnect the stream and complete the read.
			var buf2 bytes.Buffer
			var err2 error
			mrd.Add(&buf2, 16, 16, func(_, _ int64, err error) { err2 = err })
			mrd.Wait()
			if err := mrd.Close(); err != nil {
				t.Errorf("mrd.Close() = %v, want nil", err)
			}
			if err2 != nil {
				t.Fatalf("mrd.Add(second) err = %v, want nil", err2)
			}
			if !bytes.Equal(buf2.Bytes(), data[16:32]) {
				t.Errorf("mrd.Add(second) data = %q, want %q", buf2.Bytes(), data[16:32])
			}
			if got := srv.streamCount(); got != 2 {
				t.Errorf("srv.streamCount() = %d after lazy reconnect, want 2", got)
			}
		})
	}
}

func TestMultiRangeDownloaderMultiStreamIdleDownscale(t *testing.T) {
	data := []byte("0123456789abcdef0123456789abcdef")
	gate0 := make(chan struct{})
	srv := &mrdIdleDisconnectServer{
		data: data,
		streamGates: map[int]chan struct{}{
			0: gate0,
		},
	}
	client := newBufconnGRPCClient(t, srv)
	client.SetRetry(WithBackoff(gax.Backoff{Initial: 5 * time.Millisecond}))

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	params := &newMultiRangeDownloaderParams{
		bucket:              "b",
		object:              "o",
		gen:                 defaultGen,
		minConnections:      1,
		maxConnections:      2,
		targetPendingRanges: 1,
	}
	mrd, err := client.tc.NewMultiRangeDownloader(ctx, params)
	if err != nil {
		t.Fatalf("NewMultiRangeDownloader: %v", err)
	}

	var buf1, buf2 bytes.Buffer
	var err1, err2 error
	mrd.Add(&buf1, 0, 16, func(_, _ int64, err error) { err1 = err })
	mrd.Add(&buf2, 16, 16, func(_, _ int64, err error) { err2 = err })

	// Stream 0 is blocked on gate0 at capacity, so adding a second range scales up to 2 streams.
	waitForStreamState(t, mrd, streamStateSnapshot{totalStreams: 2, activeSessions: 2, reconnectingStreams: 0})

	// Close idle excess stream 1 while stream 0 is still at capacity on gate0;
	// stream 1 should be removed without triggering addNewStream while unsentRequests is empty.
	srv.triggerIdleClose(t, 1)
	waitForStreamState(t, mrd, streamStateSnapshot{totalStreams: 1, activeSessions: 1, reconnectingStreams: 0})
	if got := srv.streamCount(); got != 2 {
		t.Fatalf("srv.streamCount() = %d after closing idle excess stream 1, want 2", got)
	}

	// Unblock stream 0 so it finishes buf1 and buf2 and becomes idle, then close stream 0 from the server.
	close(gate0)
	mrd.Wait()
	if err1 != nil || err2 != nil {
		t.Fatalf("mrd.Add errors: err1=%v, err2=%v; want nil", err1, err2)
	}

	srv.triggerIdleClose(t, 0)
	waitForStreamState(t, mrd, streamStateSnapshot{totalStreams: 1, activeSessions: 0, reconnectingStreams: 0})
	if got := srv.streamCount(); got != 2 {
		t.Fatalf("srv.streamCount() = %d while idle after downscale, want 2", got)
	}

	// Adding a third range after idle downscale lazily reconnects the remaining stream without opening a 4th stream.
	var buf3 bytes.Buffer
	var err3 error
	mrd.Add(&buf3, 0, 32, func(_, _ int64, err error) { err3 = err })
	mrd.Wait()
	if err := mrd.Close(); err != nil {
		t.Errorf("mrd.Close() = %v, want nil", err)
	}
	if err3 != nil {
		t.Fatalf("mrd.Add(third) err = %v, want nil", err3)
	}
	if !bytes.Equal(buf3.Bytes(), data) {
		t.Errorf("mrd.Add(third) data = %q, want %q", buf3.Bytes(), data)
	}
	if got := srv.streamCount(); got != 3 {
		t.Errorf("srv.streamCount() = %d after post-downscale lazy reconnect, want 3", got)
	}
}
