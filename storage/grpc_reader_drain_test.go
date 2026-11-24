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
	"net"
	"testing"
	"time"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/mem"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

// stallAfterDataServer sends one message containing all of data and then
// blocks until the stream is cancelled, without ever sending trailers. This is
// a stream that is complete from the reader's point of view (seen == size) but
// never reaches EOF, which the storage testbench cannot produce.
type stallAfterDataServer struct {
	storagepb.UnimplementedStorageServer
	data []byte
	// cancelled is closed when the handler observes that the stream's context
	// is done.
	cancelled chan struct{}
}

func (s *stallAfterDataServer) ReadObject(_ *storagepb.ReadObjectRequest, stream storagepb.Storage_ReadObjectServer) error {
	crc := crc32.Checksum(s.data, crc32cTable)
	if err := stream.Send(&storagepb.ReadObjectResponse{
		ChecksummedData: &storagepb.ChecksummedData{Content: s.data, Crc32C: &crc},
		Metadata:        &storagepb.Object{Size: int64(len(s.data))},
	}); err != nil {
		return err
	}
	<-stream.Context().Done()
	close(s.cancelled)
	return stream.Context().Err()
}

func (s *stallAfterDataServer) BidiReadObject(stream storagepb.Storage_BidiReadObjectServer) error {
	crc := crc32.Checksum(s.data, crc32cTable)
	if err := stream.Send(&storagepb.BidiReadObjectResponse{
		Metadata: &storagepb.Object{Size: int64(len(s.data))},
		ObjectDataRanges: []*storagepb.ObjectRangeData{{
			ChecksummedData: &storagepb.ChecksummedData{Content: s.data, Crc32C: &crc},
			ReadRange:       &storagepb.ReadRange{ReadOffset: 0, ReadLength: int64(len(s.data)), ReadId: defaultReadID},
			RangeEnd:        true,
		}},
	}); err != nil {
		return err
	}
	<-stream.Context().Done()
	close(s.cancelled)
	return stream.Context().Err()
}

// TestCloseDrainTimeoutCancelsStalledStream checks that when a completed stream
// never delivers its trailers, Close gives up after defaultStreamDrainTimeout
// and cancels the stream.
func TestCloseDrainTimeoutCancelsStalledStream(t *testing.T) {
	const size = 64 << 10
	data := bytes.Repeat([]byte("0123456789abcdef"), size/16)

	for _, tc := range []struct {
		name string
		// open starts a read stream, receives the one data message and returns a
		// reader for which seen == size.
		open func(t *testing.T, ctx context.Context, cancel context.CancelFunc, client storagepb.StorageClient) interface{ Close() error }
	}{
		{
			name: "ReadObject",
			open: func(t *testing.T, ctx context.Context, cancel context.CancelFunc, client storagepb.StorageClient) interface{ Close() error } {
				stream, err := client.ReadObject(ctx, &storagepb.ReadObjectRequest{Bucket: "projects/_/buckets/b", Object: "o"}, grpc.ForceCodecV2(bytesCodecReadObject{}))
				if err != nil {
					t.Fatalf("ReadObject: %v", err)
				}
				resp := &storagepb.ReadObjectResponse{}
				recvDataMessage(t, stream, resp)
				checkChecksummedData(t, resp.GetChecksummedData(), data)
				return &gRPCReadObjectReader{
					stream:   stream,
					cancel:   cancel,
					size:     size,
					seen:     size,
					settings: &settings{},
				}
			},
		},
		{
			name: "BidiReadObject",
			open: func(t *testing.T, ctx context.Context, cancel context.CancelFunc, client storagepb.StorageClient) interface{ Close() error } {
				stream, err := client.BidiReadObject(ctx, grpc.ForceCodecV2(bytesCodecV2{}))
				if err != nil {
					t.Fatalf("BidiReadObject: %v", err)
				}
				if err := stream.CloseSend(); err != nil {
					t.Fatalf("CloseSend: %v", err)
				}
				resp := &storagepb.BidiReadObjectResponse{}
				recvDataMessage(t, stream, resp)
				if len(resp.GetObjectDataRanges()) != 1 {
					t.Fatalf("got %d object data ranges, want 1", len(resp.GetObjectDataRanges()))
				}
				checkChecksummedData(t, resp.GetObjectDataRanges()[0].GetChecksummedData(), data)
				return &gRPCReader{
					stream:    stream,
					cancel:    cancel,
					size:      size,
					seen:      size,
					finalized: true,
					settings:  &settings{},
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := &stallAfterDataServer{data: data, cancelled: make(chan struct{})}
			client := newBufconnStorageClient(t, srv)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			r := tc.open(t, ctx, cancel, client)

			start := time.Now()
			if err := r.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			elapsed := time.Since(start)
			if elapsed < defaultStreamDrainTimeout || elapsed >= defaultStreamDrainTimeout+500*time.Millisecond {
				t.Errorf("Close took %v; want in [%v, %v)", elapsed, defaultStreamDrainTimeout, defaultStreamDrainTimeout+500*time.Millisecond)
			}

			select {
			case <-srv.cancelled:
			case <-time.After(time.Second):
				t.Fatalf("server handler did not observe stream cancellation")
			}

			start = time.Now()
			if err := r.Close(); err != nil {
				t.Fatalf("second Close: %v", err)
			}
			if elapsed := time.Since(start); elapsed >= 50*time.Millisecond {
				t.Errorf("second Close took %v; want it to return immediately", elapsed)
			}
		})
	}
}

// newBufconnStorageClient serves srv on an in-memory listener and returns a
// client connected to it.
func newBufconnStorageClient(t *testing.T, srv storagepb.StorageServer) storagepb.StorageClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	s := grpc.NewServer()
	storagepb.RegisterStorageServer(s, srv)
	go s.Serve(lis)
	t.Cleanup(s.Stop)

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return storagepb.NewStorageClient(conn)
}

// recvDataMessage receives one raw message from stream, the same way the
// readers do, and unmarshals it into resp.
func recvDataMessage(t *testing.T, stream grpc.ClientStream, resp proto.Message) {
	t.Helper()
	var databufs mem.BufferSlice
	if err := stream.RecvMsg(&databufs); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}
	defer databufs.Free()
	if err := proto.Unmarshal(databufs.Materialize(), resp); err != nil {
		t.Fatalf("proto.Unmarshal: %v", err)
	}
}

func checkChecksummedData(t *testing.T, cd *storagepb.ChecksummedData, want []byte) {
	t.Helper()
	if !bytes.Equal(cd.GetContent(), want) {
		t.Fatalf("got %d bytes of content, want %d matching bytes", len(cd.GetContent()), len(want))
	}
	if got, want := cd.GetCrc32C(), crc32.Checksum(want, crc32cTable); got != want {
		t.Fatalf("got crc32c %d, want %d", got, want)
	}
}
