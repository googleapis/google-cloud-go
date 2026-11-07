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
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
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

// mrdImmediateReconnectErrorServer fails stream 1 after receiving the range
// request, and fails stream 2 (the async reconnect stream) immediately after
// receiving the initial BidiReadObjectSpec—before the client's eventLoop may
// have processed reconnectStreamCmd. Stream 3 succeeds.
type mrdImmediateReconnectErrorServer struct {
	storagepb.UnimplementedStorageServer
	data    []byte
	streams atomic.Int32
}

func (s *mrdImmediateReconnectErrorServer) BidiReadObject(stream storagepb.Storage_BidiReadObjectServer) error {
	streamNum := s.streams.Add(1)
	if _, err := stream.Recv(); err != nil {
		return err
	}
	if streamNum == 2 {
		return status.Error(codes.Unavailable, "immediate error on reconnecting stream")
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
	if streamNum == 1 {
		return status.Error(codes.Unavailable, "mid-stream error on initial stream")
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

func TestMultiRangeDownloaderSessionReadyGate(t *testing.T) {
	data := []byte("0123456789abcdef0123456789abcdef")
	srv := &mrdImmediateReconnectErrorServer{data: data}
	client := newBufconnGRPCClient(t, srv)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	mrd, err := client.Bucket("test-bucket").Object("test-object").NewMultiRangeDownloader(ctx)
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
	if got := srv.streams.Load(); got != 3 {
		t.Errorf("srv.streams.Load() = %d, want 3", got)
	}
}
