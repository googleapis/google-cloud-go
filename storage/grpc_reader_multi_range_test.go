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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
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
