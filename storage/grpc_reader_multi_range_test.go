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
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net"
	"slices"
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
	"google.golang.org/protobuf/proto"
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
	data          []byte
	stallStreams  int32
	redirectFirst bool
	streams       atomic.Int32
	mu            sync.Mutex
	xGoogHeaders  []string
	specs         []*storagepb.BidiReadObjectSpec
}

func (s *mrdStallOnOpenServer) BidiReadObject(stream storagepb.Storage_BidiReadObjectServer) error {
	md, _ := metadata.FromIncomingContext(stream.Context())
	xGoog := strings.Join(md.Get("x-goog-api-client"), " ")
	s.mu.Lock()
	s.xGoogHeaders = append(s.xGoogHeaders, xGoog)
	s.mu.Unlock()

	attempt := s.streams.Add(1)
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.specs = append(s.specs, first.GetReadObjectSpec())
	s.mu.Unlock()
	if s.redirectFirst && attempt == 1 {
		st, err := status.New(codes.Aborted, "redirect").WithDetails(&storagepb.BidiReadObjectRedirectedError{
			RoutingToken: proto.String("token-1"),
			ReadHandle:   &storagepb.BidiReadHandle{Handle: []byte("redirect-handle")},
		})
		if err != nil {
			return err
		}
		return st.Err()
	}
	if attempt <= s.stallStreams {
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
	const stallMin = 150 * time.Millisecond
	srv := &mrdStallOnOpenServer{data: data, stallStreams: 1}
	client := newBufconnGRPCClient(t, srv, experimental.WithReadStallTimeout(&experimental.ReadStallTimeoutConfig{
		TargetPercentile: 0.99,
		Min:              stallMin,
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
	if elapsed < stallMin || elapsed >= 2*time.Second {
		t.Errorf("NewMultiRangeDownloader elapsed = %v, want in [%v, 2s)", elapsed, stallMin)
	}
	if got := srv.streams.Load(); got < 2 {
		t.Errorf("srv.streams.Load() = %d, want >= 2", got)
	}

	srv.mu.Lock()
	headers := append([]string(nil), srv.xGoogHeaders...)
	srv.mu.Unlock()
	if len(headers) < 2 {
		t.Fatalf("len(srv.xGoogHeaders) = %d, want >= 2", len(headers))
	}
	if !strings.Contains(headers[0], "gccl-invocation-id/") || !slices.Contains(strings.Fields(headers[0]), "gccl-attempt-count/1") {
		t.Errorf("headers[0] = %q, want gccl-invocation-id and gccl-attempt-count/1", headers[0])
	}
	lastHeader := headers[len(headers)-1]
	wantLastAttempt := fmt.Sprintf("gccl-attempt-count/%d", len(headers))
	if !slices.Contains(strings.Fields(lastHeader), wantLastAttempt) {
		t.Errorf("last header = %q, want %s", lastHeader, wantLastAttempt)
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

func TestMultiRangeDownloaderSendsConditions(t *testing.T) {
	for _, tc := range []struct {
		name  string
		conds Conditions
		want  *storagepb.BidiReadObjectSpec
	}{
		{
			name:  "match",
			conds: Conditions{GenerationMatch: 5, MetagenerationMatch: 7},
			want:  &storagepb.BidiReadObjectSpec{IfGenerationMatch: proto.Int64(5), IfMetagenerationMatch: proto.Int64(7)},
		},
		{
			name:  "not match",
			conds: Conditions{GenerationNotMatch: 5, MetagenerationNotMatch: 7},
			want:  &storagepb.BidiReadObjectSpec{IfGenerationNotMatch: proto.Int64(5), IfMetagenerationNotMatch: proto.Int64(7)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte("0123456789abcdef")
			// The redirect and the second connection each open a stream from a copy of the spec.
			srv := &mrdStallOnOpenServer{data: data, redirectFirst: true}
			client := newBufconnGRPCClient(t, srv)
			client.SetRetry(WithBackoff(gax.Backoff{Initial: 10 * time.Millisecond}))

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			mrd, err := client.Bucket("test-bucket").Object("test-object").If(tc.conds).NewMultiRangeDownloader(ctx, WithMinConnections(2))
			if err != nil {
				t.Fatalf("NewMultiRangeDownloader: %v", err)
			}
			// Any manager event lets it open the second connection.
			var buf bytes.Buffer
			mrd.Add(&buf, 0, int64(len(data)), func(int64, int64, error) {})
			mrd.Wait()
			for srv.streams.Load() < 3 && ctx.Err() == nil {
				time.Sleep(10 * time.Millisecond)
			}
			if err := mrd.Close(); err != nil {
				t.Errorf("mrd.Close() = %v, want nil", err)
			}

			srv.mu.Lock()
			specs := append([]*storagepb.BidiReadObjectSpec(nil), srv.specs...)
			srv.mu.Unlock()
			if len(specs) < 3 {
				t.Fatalf("len(srv.specs) = %d, want >= 3", len(specs))
			}
			for i, got := range specs {
				g := &storagepb.BidiReadObjectSpec{
					IfGenerationMatch:        got.IfGenerationMatch,
					IfGenerationNotMatch:     got.IfGenerationNotMatch,
					IfMetagenerationMatch:    got.IfMetagenerationMatch,
					IfMetagenerationNotMatch: got.IfMetagenerationNotMatch,
				}
				if !proto.Equal(g, tc.want) {
					t.Errorf("stream %d preconditions = %v, want %v", i, g, tc.want)
				}
			}
		})
	}
}

func TestMultiRangeDownloaderReadStallOnRedirect(t *testing.T) {
	data := []byte("0123456789abcdef0123456789abcdef")
	const stallMin = 150 * time.Millisecond
	// Stream 1 redirects, stream 2 (the redirect retry) stalls, stream 3 succeeds.
	srv := &mrdStallOnOpenServer{data: data, redirectFirst: true, stallStreams: 2}
	client := newBufconnGRPCClient(t, srv, experimental.WithReadStallTimeout(&experimental.ReadStallTimeoutConfig{
		TargetPercentile: 0.99,
		Min:              stallMin,
	}))
	client.SetRetry(WithBackoff(gax.Backoff{Initial: 10 * time.Millisecond}))

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	mrd, err := client.Bucket("test-bucket").Object("test-object").NewMultiRangeDownloader(ctx)
	if err != nil {
		t.Fatalf("NewMultiRangeDownloader: %v", err)
	}
	if err := mrd.Close(); err != nil {
		t.Errorf("mrd.Close() = %v, want nil", err)
	}
	if got := srv.streams.Load(); got < 3 {
		t.Errorf("srv.streams.Load() = %d, want >= 3", got)
	}
}

func TestMultiRangeDownloaderReadStallExhaustsMaxAttempts(t *testing.T) {
	data := []byte("0123456789abcdef0123456789abcdef")
	// stallStreams > maxAttempts ensures every attempt stalls.
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
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("NewMultiRangeDownloader() err = %v, want errors.Is(..., context.DeadlineExceeded)", err)
	}
	if got := srv.streams.Load(); got != 2 {
		t.Errorf("srv.streams.Load() = %d, want 2", got)
	}
}
