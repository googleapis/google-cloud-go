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
	"slices"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const reopenTestGeneration = 7

// failFirstStreamServer sends only object metadata on the first read stream
// and then fails it with a retryable error. Later streams serve the data.
// It records the generation requested by each stream.
type failFirstStreamServer struct {
	storagepb.UnimplementedStorageServer
	data []byte

	mu          sync.Mutex
	generations []int64
}

func (s *failFirstStreamServer) recordGeneration(gen int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generations = append(s.generations, gen)
	return len(s.generations)
}

func (s *failFirstStreamServer) gotGenerations() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.generations)
}

func (s *failFirstStreamServer) metadata() *storagepb.Object {
	return &storagepb.Object{Size: int64(len(s.data)), Generation: reopenTestGeneration}
}

func (s *failFirstStreamServer) ReadObject(req *storagepb.ReadObjectRequest, stream storagepb.Storage_ReadObjectServer) error {
	if s.recordGeneration(req.GetGeneration()) == 1 {
		if err := stream.Send(&storagepb.ReadObjectResponse{Metadata: s.metadata()}); err != nil {
			return err
		}
		return status.Error(codes.Unavailable, "transient")
	}
	chunk := s.data[req.GetReadOffset():]
	crc := crc32.Checksum(chunk, crc32cTable)
	return stream.Send(&storagepb.ReadObjectResponse{
		Metadata:        s.metadata(),
		ChecksummedData: &storagepb.ChecksummedData{Content: chunk, Crc32C: &crc},
		ContentRange:    &storagepb.ContentRange{Start: req.GetReadOffset(), End: int64(len(s.data)), CompleteLength: int64(len(s.data))},
	})
}

func (s *failFirstStreamServer) BidiReadObject(stream storagepb.Storage_BidiReadObjectServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}
	attempt := s.recordGeneration(req.GetReadObjectSpec().GetGeneration())
	if err := stream.Send(&storagepb.BidiReadObjectResponse{
		Metadata:   s.metadata(),
		ReadHandle: &storagepb.BidiReadHandle{Handle: []byte("handle")},
	}); err != nil {
		return err
	}
	if attempt == 1 {
		// Wait for ranges from the MultiRangeDownloader before failing so
		// that it has pending work to resend on the new stream.
		if len(req.GetReadRanges()) == 0 {
			if _, err := stream.Recv(); err != nil {
				return err
			}
		}
		return status.Error(codes.Unavailable, "transient")
	}
	for {
		if err := sendBidiReadRanges(stream, s.data, req.GetReadRanges()); err != nil {
			return err
		}
		if req, err = stream.Recv(); err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
	}
}

// TestGRPCReadReopenPinsGeneration checks that once a read learns the object
// generation, streams opened after a retryable error request that generation.
func TestGRPCReadReopenPinsGeneration(t *testing.T) {
	data := []byte("0123456789abcdef0123456789abcdef")

	for _, tc := range []struct {
		name string
		bidi bool
		read func(ctx context.Context, o *ObjectHandle) ([]byte, error)
	}{
		{
			name: "ReadObject",
			read: readAllWithReader,
		},
		{
			name: "BidiReadObject",
			bidi: true,
			read: readAllWithReader,
		},
		{
			name: "MultiRangeDownloader",
			bidi: true,
			read: func(ctx context.Context, o *ObjectHandle) ([]byte, error) {
				mrd, err := o.NewMultiRangeDownloader(ctx)
				if err != nil {
					return nil, err
				}
				var buf bytes.Buffer
				var rangeErr error
				mrd.Add(&buf, 0, int64(len(data)), func(_, _ int64, err error) {
					rangeErr = err
				})
				mrd.Wait()
				if err := mrd.Close(); err != nil {
					return nil, err
				}
				return buf.Bytes(), rangeErr
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := &failFirstStreamServer{data: data}
			client := newBufconnGRPCClient(t, srv)
			client.tc.(*grpcStorageClient).config.grpcBidiReads = tc.bidi
			client.SetRetry(WithBackoff(gax.Backoff{Initial: 10 * time.Millisecond}))

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			got, err := tc.read(ctx, client.Bucket("test-bucket").Object("test-object"))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Errorf("read data = %q, want %q", got, data)
			}
			gens := srv.gotGenerations()
			if len(gens) < 2 {
				t.Fatalf("server saw %d streams, want >= 2", len(gens))
			}
			if gens[0] != 0 {
				t.Errorf("first stream generation = %d, want 0", gens[0])
			}
			for i, gen := range gens[1:] {
				if gen != reopenTestGeneration {
					t.Errorf("stream %d generation = %d, want %d", i+2, gen, reopenTestGeneration)
				}
			}
		})
	}
}

func readAllWithReader(ctx context.Context, o *ObjectHandle) ([]byte, error) {
	r, err := o.NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
