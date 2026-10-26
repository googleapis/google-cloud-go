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
	client, err := NewGRPCClient(context.Background(), opts...)
	if err != nil {
		t.Fatalf("NewGRPCClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

type mrdStallOnOpenServer struct {
	storagepb.UnimplementedStorageServer
	data    []byte
	streams atomic.Int32
}

func (s *mrdStallOnOpenServer) BidiReadObject(stream storagepb.Storage_BidiReadObjectServer) error {
	attempt := s.streams.Add(1)
	if _, err := stream.Recv(); err != nil {
		return err
	}
	if attempt == 1 {
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
		for _, rr := range req.GetReadRanges() {
			end := rr.GetReadOffset() + rr.GetReadLength()
			if rr.GetReadLength() == 0 || end > int64(len(s.data)) {
				end = int64(len(s.data))
			}
			chunk := s.data[rr.GetReadOffset():end]
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
	}
}

func TestMultiRangeDownloaderReadStallTimeout(t *testing.T) {
	data := []byte("0123456789abcdef0123456789abcdef")
	srv := &mrdStallOnOpenServer{data: data}
	client := newBufconnGRPCClient(t, srv, experimental.WithReadStallTimeout(&experimental.ReadStallTimeoutConfig{
		TargetPercentile: 0.99,
		Min:              100 * time.Millisecond,
	}))
	client.SetRetry(WithBackoff(gax.Backoff{Initial: 10 * time.Millisecond}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	mrd, err := client.Bucket("test-bucket").Object("test-object").NewMultiRangeDownloader(ctx)
	if err != nil {
		t.Fatalf("NewMultiRangeDownloader: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed >= 2*time.Second {
		t.Errorf("NewMultiRangeDownloader took %v, want < 2s", elapsed)
	}
	if got := srv.streams.Load(); got != 2 {
		t.Errorf("got %d BidiReadObject streams, want 2", got)
	}

	var buf bytes.Buffer
	var rangeErr error
	mrd.Add(&buf, 0, int64(len(data)), func(_, _ int64, err error) {
		rangeErr = err
	})
	mrd.Wait()
	if err := mrd.Close(); err != nil {
		t.Errorf("mrd.Close: %v", err)
	}
	if rangeErr != nil {
		t.Fatalf("range callback error: %v", rangeErr)
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Errorf("got range data %q, want %q", buf.Bytes(), data)
	}
}

type mrdReconnectServer struct {
	storagepb.UnimplementedStorageServer
	data            []byte
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

	if streamNum == 1 {
		if err := stream.Send(&storagepb.BidiReadObjectResponse{
			Metadata:   &storagepb.Object{Size: int64(len(s.data)), Generation: 1},
			ReadHandle: &storagepb.BidiReadHandle{Handle: []byte("handle-1")},
		}); err != nil {
			return err
		}
	}

	req, err := stream.Recv()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}

	if sendProgress {
		if err := stream.Send(&storagepb.BidiReadObjectResponse{
			ReadHandle: &storagepb.BidiReadHandle{Handle: []byte("handle-refreshed")},
		}); err != nil {
			return err
		}
	}

	if shouldFail {
		return status.Error(codes.Unavailable, "injected mid-stream unavailable")
	}

	for _, rr := range req.GetReadRanges() {
		end := rr.GetReadOffset() + rr.GetReadLength()
		if rr.GetReadLength() == 0 || end > int64(len(s.data)) {
			end = int64(len(s.data))
		}
		chunk := s.data[rr.GetReadOffset():end]
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
	const initialBackoff = 100 * time.Millisecond
	client.SetRetry(
		WithBackoff(gax.Backoff{
			Initial:    initialBackoff,
			Max:        400 * time.Millisecond,
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
		t.Fatalf("got nil range error, want error after max attempts")
	}
	wantSubstr := "retry failed after 3 attempts"
	if !strings.Contains(rangeErr.Error(), wantSubstr) {
		t.Errorf("got range error %q, want it to contain %q", rangeErr.Error(), wantSubstr)
	}
	if elapsed < 5*time.Millisecond {
		t.Errorf("got elapsed %v, want >= 5ms for mid-stream backoff", elapsed)
	}

	retryMu.Lock()
	defer retryMu.Unlock()
	if len(gotContexts) != 3 {
		t.Fatalf("got %d RetryContext calls, want 3", len(gotContexts))
	}
	wantInvID := gotContexts[0].InvocationID
	if wantInvID == "" {
		t.Errorf("got empty InvocationID in RetryContext")
	}
	for i, rctx := range gotContexts {
		wantAttempt := i + 1
		if rctx.Attempt != wantAttempt {
			t.Errorf("gotContexts[%d].Attempt = %d, want %d", i, rctx.Attempt, wantAttempt)
		}
		if rctx.InvocationID != wantInvID {
			t.Errorf("gotContexts[%d].InvocationID = %q, want %q", i, rctx.InvocationID, wantInvID)
		}
		if rctx.Operation != "ReadObject" {
			t.Errorf("gotContexts[%d].Operation = %q, want %q", i, rctx.Operation, "ReadObject")
		}
		if rctx.Bucket != "my-bucket" {
			t.Errorf("gotContexts[%d].Bucket = %q, want %q", i, rctx.Bucket, "my-bucket")
		}
		if rctx.Object != "my-object" {
			t.Errorf("gotContexts[%d].Object = %q, want %q", i, rctx.Object, "my-object")
		}
	}

	srv.mu.Lock()
	headers := append([]string(nil), srv.xGoogHeaders...)
	srv.mu.Unlock()
	if len(headers) != 3 {
		t.Fatalf("got %d streams opened, want 3", len(headers))
	}
	wantAttempt2 := "gccl-invocation-id/" + wantInvID + " gccl-attempt-count/2"
	if !strings.Contains(headers[1], wantAttempt2) {
		t.Errorf("stream 2 x-goog-api-client = %q, want it to contain %q", headers[1], wantAttempt2)
	}
	wantAttempt3 := "gccl-invocation-id/" + wantInvID + " gccl-attempt-count/3"
	if !strings.Contains(headers[2], wantAttempt3) {
		t.Errorf("stream 3 x-goog-api-client = %q, want it to contain %q", headers[2], wantAttempt3)
	}
}

func TestMultiRangeDownloaderMidStreamReconnectResetOnProgress(t *testing.T) {
	data := []byte("0123456789abcdef0123456789abcdef")
	// Stream 1 fails; stream 2 sends a valid response (progress) then fails;
	// stream 3 fails; stream 4 succeeds. With MaxAttempts(3), the 3 mid-stream
	// failures only succeed if stream 2's valid response resets the counter.
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
	client.SetRetry(
		WithBackoff(gax.Backoff{Initial: 5 * time.Millisecond, Max: 20 * time.Millisecond, Multiplier: 2}),
		WithMaxAttempts(3),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
		t.Errorf("mrd.Close: %v", err)
	}
	if rangeErr != nil {
		t.Fatalf("got range error %v, want nil", rangeErr)
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Errorf("got range data %q, want %q", buf.Bytes(), data)
	}
	srv.mu.Lock()
	gotStreams := srv.streams
	srv.mu.Unlock()
	if gotStreams != 4 {
		t.Errorf("got %d streams, want 4", gotStreams)
	}
}

type mrdIdleDisconnectServer struct {
	storagepb.UnimplementedStorageServer
	data      []byte
	mu        sync.Mutex
	streams   int
	closeIdle []chan struct{}
	gateRange chan struct{}
}

func (s *mrdIdleDisconnectServer) triggerIdleClose(streamIdx int) {
	s.mu.Lock()
	ch := s.closeIdle[streamIdx]
	s.mu.Unlock()
	close(ch)
}

func (s *mrdIdleDisconnectServer) streamCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.streams
}

func (s *mrdIdleDisconnectServer) BidiReadObject(stream storagepb.Storage_BidiReadObjectServer) error {
	closeCh := make(chan struct{})
	s.mu.Lock()
	s.streams++
	s.closeIdle = append(s.closeIdle, closeCh)
	gate := s.gateRange
	s.mu.Unlock()

	if _, err := stream.Recv(); err != nil {
		return err
	}
	if err := stream.Send(&storagepb.BidiReadObjectResponse{
		Metadata:   &storagepb.Object{Size: int64(len(s.data)), Generation: 1},
		ReadHandle: &storagepb.BidiReadHandle{Handle: []byte("handle-idle")},
	}); err != nil {
		return err
	}

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
			for _, rr := range req.GetReadRanges() {
				end := rr.GetReadOffset() + rr.GetReadLength()
				if rr.GetReadLength() == 0 || end > int64(len(s.data)) {
					end = int64(len(s.data))
				}
				chunk := s.data[rr.GetReadOffset():end]
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
		}
	}
}

type inspectStreamsCmd struct {
	respC chan int
}

func (c *inspectStreamsCmd) apply(_ context.Context, m *multiRangeDownloaderManager) {
	c.respC <- len(m.streams)
}

func TestMultiRangeDownloaderIdleStreamLazyReconnectAndDownscale(t *testing.T) {
	data := []byte("0123456789abcdef0123456789abcdef")

	t.Run("SingleStreamLazyReconnect", func(t *testing.T) {
		srv := &mrdIdleDisconnectServer{data: data}
		client := newBufconnGRPCClient(t, srv)
		client.SetRetry(WithBackoff(gax.Backoff{Initial: 5 * time.Millisecond}))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
			t.Fatalf("first range error: %v", err1)
		}

		// Close the idle stream from the server and verify no eager reconnect happens while idle.
		srv.triggerIdleClose(0)
		time.Sleep(80 * time.Millisecond)
		if got := srv.streamCount(); got != 1 {
			t.Fatalf("got %d streams while idle after server close, want 1 (no eager reconnect)", got)
		}

		// Adding a new range must lazily reconnect the stream and complete the read.
		var buf2 bytes.Buffer
		var err2 error
		mrd.Add(&buf2, 16, 16, func(_, _ int64, err error) { err2 = err })
		mrd.Wait()
		if err := mrd.Close(); err != nil {
			t.Errorf("mrd.Close: %v", err)
		}
		if err2 != nil {
			t.Fatalf("second range error: %v", err2)
		}
		if !bytes.Equal(buf2.Bytes(), data[16:32]) {
			t.Errorf("got second range %q, want %q", buf2.Bytes(), data[16:32])
		}
		if got := srv.streamCount(); got != 2 {
			t.Errorf("got %d streams after lazy reconnect, want 2", got)
		}
	})

	t.Run("MultiStreamIdleDownscale", func(t *testing.T) {
		gate := make(chan struct{})
		srv := &mrdIdleDisconnectServer{data: data, gateRange: gate}
		client := newBufconnGRPCClient(t, srv)
		client.SetRetry(WithBackoff(gax.Backoff{Initial: 5 * time.Millisecond}))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

		// Wait for the second stream to be opened when stream 1 hits targetPendingRanges=1.
		deadline := time.Now().Add(2 * time.Second)
		for srv.streamCount() < 2 {
			if time.Now().After(deadline) {
				t.Fatalf("got %d streams, want scale-up to 2 streams", srv.streamCount())
			}
			time.Sleep(5 * time.Millisecond)
		}
		close(gate)
		mrd.Wait()
		if err1 != nil || err2 != nil {
			t.Fatalf("range errors: err1=%v, err2=%v", err1, err2)
		}

		// Close both idle streams from the server; excess stream (> minConnections) should be deleted
		// and neither stream should reconnect while idle.
		srv.triggerIdleClose(0)
		srv.triggerIdleClose(1)
		time.Sleep(80 * time.Millisecond)
		if got := srv.streamCount(); got != 2 {
			t.Fatalf("got %d streams while idle after closing both streams, want 2", got)
		}

		mgr := mrd.impl.(*multiRangeDownloaderManager)
		respC := make(chan int, 1)
		mgr.cmds <- &inspectStreamsCmd{respC: respC}
		if got := <-respC; got != 1 {
			t.Errorf("got %d streams in manager map after idle downscale, want 1", got)
		}
		if err := mrd.Close(); err != nil {
			t.Errorf("mrd.Close: %v", err)
		}
	})
}
