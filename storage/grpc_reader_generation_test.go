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

// generationServer serves object reads and records the generation requested
// by each stream. The object metadata always reports reopenTestGeneration.
type generationServer struct {
	storagepb.UnimplementedStorageServer
	data []byte
	// failFirst makes the first stream send only metadata and then fail
	// with a retryable error.
	failFirst bool
	// notFoundOnRetry makes every stream after the first fail with NotFound.
	notFoundOnRetry bool
	// waitForSecondStream makes the first bidi stream hold back range data
	// until a second stream has been opened.
	waitForSecondStream bool

	mu           sync.Mutex
	generations  []int64
	secondStream chan struct{}
}

func newGenerationServer(data []byte) *generationServer {
	return &generationServer{data: data, secondStream: make(chan struct{})}
}

// recordStream records the generation requested by a new stream and returns
// the 1-based number of that stream.
func (s *generationServer) recordStream(gen int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generations = append(s.generations, gen)
	if len(s.generations) == 2 {
		close(s.secondStream)
	}
	return len(s.generations)
}

func (s *generationServer) gotGenerations() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.generations)
}

func (s *generationServer) metadata() *storagepb.Object {
	return &storagepb.Object{Size: int64(len(s.data)), Generation: reopenTestGeneration}
}

func (s *generationServer) ReadObject(req *storagepb.ReadObjectRequest, stream storagepb.Storage_ReadObjectServer) error {
	n := s.recordStream(req.GetGeneration())
	if n > 1 && s.notFoundOnRetry {
		return status.Error(codes.NotFound, "object not found")
	}
	if n == 1 && s.failFirst {
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

func (s *generationServer) BidiReadObject(stream storagepb.Storage_BidiReadObjectServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}
	n := s.recordStream(req.GetReadObjectSpec().GetGeneration())
	if n > 1 && s.notFoundOnRetry {
		return status.Error(codes.NotFound, "object not found")
	}
	if err := stream.Send(&storagepb.BidiReadObjectResponse{
		Metadata:   s.metadata(),
		ReadHandle: &storagepb.BidiReadHandle{Handle: []byte("handle")},
	}); err != nil {
		return err
	}
	if n == 1 && s.failFirst {
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
		if n == 1 && s.waitForSecondStream && len(req.GetReadRanges()) > 0 {
			select {
			case <-s.secondStream:
			case <-stream.Context().Done():
				return stream.Context().Err()
			}
		}
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

// readWithMRD returns a function that reads all of an object's data with a
// MultiRangeDownloader created with opts.
func readWithMRD(opts ...MRDOption) func(context.Context, *ObjectHandle) ([]byte, error) {
	return func(ctx context.Context, o *ObjectHandle) ([]byte, error) {
		mrd, err := o.NewMultiRangeDownloader(ctx, opts...)
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		var rangeErr error
		mrd.Add(&buf, 0, mrd.Attrs.Size, func(_, _ int64, err error) {
			rangeErr = err
		})
		mrd.Wait()
		if err := mrd.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), rangeErr
	}
}

// TestGRPCReadReopenPinsGeneration checks that once a read learns the object
// generation, streams opened later request that generation, and that a
// generation set by the user is always used.
func TestGRPCReadReopenPinsGeneration(t *testing.T) {
	data := []byte("0123456789abcdef0123456789abcdef")

	for _, tc := range []struct {
		name string
		bidi bool
		// userGen is the generation set on the ObjectHandle, or 0 for none.
		userGen int64
		// extraStream opens a second stream instead of failing the first.
		extraStream bool
		read        func(ctx context.Context, o *ObjectHandle) ([]byte, error)
	}{
		{
			name: "ReadObject",
			read: readObject,
		},
		{
			name: "BidiReadObject",
			bidi: true,
			read: readObject,
		},
		{
			name: "MultiRangeDownloader",
			bidi: true,
			read: readWithMRD(),
		},
		{
			name:        "MultiRangeDownloaderExtraStream",
			bidi:        true,
			extraStream: true,
			read:        readWithMRD(WithMinConnections(2)),
		},
		{
			name:    "ReadObjectUserGeneration",
			userGen: 3,
			read:    readObject,
		},
		{
			name:    "BidiReadObjectUserGeneration",
			bidi:    true,
			userGen: 3,
			read:    readObject,
		},
		{
			name:    "MultiRangeDownloaderUserGeneration",
			bidi:    true,
			userGen: 3,
			read:    readWithMRD(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newGenerationServer(data)
			srv.failFirst = !tc.extraStream
			srv.waitForSecondStream = tc.extraStream
			client := newBufconnGRPCClient(t, srv)
			client.tc.(*grpcStorageClient).config.grpcBidiReads = tc.bidi
			client.SetRetry(WithBackoff(gax.Backoff{Initial: 10 * time.Millisecond}))

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			o := client.Bucket("test-bucket").Object("test-object")
			if tc.userGen != 0 {
				o = o.Generation(tc.userGen)
			}
			got, err := tc.read(ctx, o)
			if err != nil {
				t.Fatalf("tc.read() = %v, want nil", err)
			}
			if !bytes.Equal(got, data) {
				t.Errorf("tc.read() data = %q, want %q", got, data)
			}

			wantFirst, wantLater := int64(0), int64(reopenTestGeneration)
			if tc.userGen != 0 {
				wantFirst, wantLater = tc.userGen, tc.userGen
			}
			gens := srv.gotGenerations()
			if len(gens) < 2 {
				t.Fatalf("server saw %d streams, want >= 2", len(gens))
			}
			if gens[0] != wantFirst {
				t.Errorf("stream 1 generation = %d, want %d", gens[0], wantFirst)
			}
			for i, gen := range gens[1:] {
				if gen != wantLater {
					t.Errorf("stream %d generation = %d, want %d", i+2, gen, wantLater)
				}
			}
		})
	}
}

// TestGRPCReadReopenNotFound checks that a NotFound error when reopening a
// read is reported as ErrObjectNotExist.
func TestGRPCReadReopenNotFound(t *testing.T) {
	data := []byte("0123456789abcdef0123456789abcdef")

	for _, tc := range []struct {
		name string
		bidi bool
		read func(ctx context.Context, o *ObjectHandle) ([]byte, error)
	}{
		{name: "ReadObject", read: readObject},
		{name: "BidiReadObject", bidi: true, read: readObject},
		{name: "MultiRangeDownloader", bidi: true, read: readWithMRD()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newGenerationServer(data)
			srv.failFirst = true
			srv.notFoundOnRetry = true
			client := newBufconnGRPCClient(t, srv)
			client.tc.(*grpcStorageClient).config.grpcBidiReads = tc.bidi
			client.SetRetry(WithBackoff(gax.Backoff{Initial: 10 * time.Millisecond}))

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			_, err := tc.read(ctx, client.Bucket("test-bucket").Object("test-object"))
			if !errors.Is(err, ErrObjectNotExist) {
				t.Errorf("tc.read() = %v, want %v", err, ErrObjectNotExist)
			}
		})
	}
}
