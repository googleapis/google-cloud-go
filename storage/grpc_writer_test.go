// Copyright 2025 Google LLC
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
	"sync"
	"testing"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/protobuf/proto"
)

func TestGetObjectChecksums(t *testing.T) {
	tests := []struct {
		name                string
		fullObjectChecksum  func() uint32
		finishWrite         bool
		sendCRC32C          bool
		takeoverWriter      bool
		disableAutoChecksum bool
		attrs               *ObjectAttrs
		want                *storagepb.ObjectChecksums
	}{
		{
			name:        "finishWrite is false",
			finishWrite: false,
			want:        nil,
		},
		{
			name:        "sendCRC32C is true, attrs have CRC32C",
			finishWrite: true,
			sendCRC32C:  true,
			attrs:       &ObjectAttrs{CRC32C: 123},
			want: &storagepb.ObjectChecksums{
				Crc32C: proto.Uint32(123),
			},
		},
		{
			name:                "disableCRC32C is true and sendCRC32C is true",
			finishWrite:         true,
			sendCRC32C:          true,
			disableAutoChecksum: true,
			attrs:               &ObjectAttrs{CRC32C: 123},
			want: &storagepb.ObjectChecksums{
				Crc32C: proto.Uint32(123),
			},
		},
		{
			name:                "disableCRC32C is true and sendCRC32C is false",
			finishWrite:         true,
			sendCRC32C:          false,
			disableAutoChecksum: true,
			want:                nil,
		},
		{
			name:                "CRC32C enabled, no user-provided checksum",
			fullObjectChecksum:  func() uint32 { return 456 },
			finishWrite:         true,
			sendCRC32C:          false,
			disableAutoChecksum: false,
			attrs:               &ObjectAttrs{},
			want: &storagepb.ObjectChecksums{
				Crc32C: proto.Uint32(456),
			},
		},
		// TODO(b/461982277): remove this testcase once checksums for takeover writer is implemented
		{
			name:           "takeover writer should return nil",
			finishWrite:    true,
			takeoverWriter: true,
			want:           nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getObjectChecksums(&getObjectChecksumsParams{
				disableAutoChecksum: tt.disableAutoChecksum,
				sendCRC32C:          tt.sendCRC32C,
				objectAttrs:         tt.attrs,
				fullObjectChecksum:  tt.fullObjectChecksum,
				finishWrite:         tt.finishWrite,
				takeoverWriter:      tt.takeoverWriter,
			})
			if !proto.Equal(got, tt.want) {
				t.Errorf("getObjectChecksums() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGRPCWriter_OneShot_ZeroCopy(t *testing.T) {
	// One-shot mode (ChunkSize = 0) must bypass buffering even for small data.
	data := []byte("small-payload-for-oneshot")

	mockSender := &mockZeroCopySender{}
	w := &gRPCWriter{
		buf:           nil,
		chunkSize:     0,
		writeQuantum:  256 * 1024,
		sendableUnits: 1,
		writesChan:    make(chan gRPCWriterCommand, 1),
		donec:         make(chan struct{}),
		streamSender:  mockSender,
		settings:      &settings{},
		forceOneShot:  true, // Enable one-shot mode.
	}
	w.progress = func(int64) {}
	w.setObj = func(*ObjectAttrs) {}
	w.setSize = func(int64) {}

	go func() {
		w.writeLoop(context.Background())
		close(w.donec)
	}()

	n, err := w.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("Short write")
	}

	w.Close()
	mockSender.wg.Wait()

	mockSender.mu.Lock()
	defer mockSender.mu.Unlock()

	dataRequests := filterDataRequests(mockSender.requests)

	if len(dataRequests) != 1 {
		t.Fatalf("Expected 1 data request, got %d", len(dataRequests))
	}

	// Verify that the small payload was sent directly from the user's slice
	// without being copied into the internal buffer.
	if &dataRequests[0].buf[0] != &data[0] {
		t.Errorf("OneShot: Expected zero-copy for small payload, but buffer was copied")
	}
}

func TestGRPCWriter_DirtyBuffer_CopyFallback(t *testing.T) {
	chunkSize := 100
	part1 := make([]byte, 50)
	part2 := make([]byte, 50)

	mockSender := &mockZeroCopySender{}
	w := &gRPCWriter{
		buf:           nil,
		chunkSize:     chunkSize,
		writeQuantum:  chunkSize,
		sendableUnits: 1,
		writesChan:    make(chan gRPCWriterCommand, 1),
		donec:         make(chan struct{}),
		streamSender:  mockSender,
		settings:      &settings{},
	}
	// Initialize required callbacks.
	w.progress = func(int64) {}
	w.setObj = func(*ObjectAttrs) {}
	w.setSize = func(int64) {}

	go func() {
		w.writeLoop(context.Background())
		close(w.donec)
	}()

	w.Write(part1)
	w.Write(part2)

	w.Close()
	mockSender.wg.Wait()

	mockSender.mu.Lock()
	defer mockSender.mu.Unlock()

	dataRequests := filterDataRequests(mockSender.requests)

	if len(dataRequests) != 1 {
		t.Fatalf("Expected 1 combined data request, got %d", len(dataRequests))
	}

	// Verify that the internal buffer was used (copy fallback) because the
	// individual writes were too small to trigger the zero-copy path.
	sentBuf := dataRequests[0].buf

	if &sentBuf[0] == &part1[0] {
		t.Errorf("Expected copy (buffering), but got zero-copy of part1")
	}
	if &sentBuf[0] == &part2[0] {
		t.Errorf("Expected copy (buffering), but got zero-copy of part2")
	}

	if len(sentBuf) != 100 {
		t.Errorf("Expected 100 bytes sent, got %d", len(sentBuf))
	}
}

func TestGRPCWriter_ZeroCopyOptimization(t *testing.T) {
	chunkSize := 256 * 1024
	// Data size is 2 full chunks + 100 bytes.
	dataSize := (chunkSize * 2) + 100
	data := make([]byte, dataSize)
	data[0] = 1
	data[chunkSize] = 2

	mockSender := &mockZeroCopySender{}
	w := &gRPCWriter{
		buf:           nil,
		chunkSize:     chunkSize,
		writeQuantum:  chunkSize,
		sendableUnits: 10,
		writesChan:    make(chan gRPCWriterCommand, 1),
		donec:         make(chan struct{}),
		streamSender:  mockSender,
		settings:      &settings{},
	}
	w.progress = func(int64) {}
	w.setObj = func(*ObjectAttrs) {}
	w.setSize = func(int64) {}

	go func() {
		w.writeLoop(context.Background())
		close(w.donec)
	}()

	w.Write(data)
	w.Close()
	mockSender.wg.Wait()

	mockSender.mu.Lock()
	defer mockSender.mu.Unlock()

	dataRequests := filterDataRequests(mockSender.requests)

	// Expect 3 requests: two zero-copy full chunks and one copied tail.
	if len(dataRequests) != 3 {
		t.Fatalf("Expected 3 data requests, got %d", len(dataRequests))
	}

	// Verify zero-copy on the first chunk.
	if &dataRequests[0].buf[0] != &data[0] {
		t.Errorf("Chunk 1: Zero-copy optimization failed (buffer copied)")
	}

	// Verify zero-copy on the second chunk.
	if &dataRequests[1].buf[0] != &data[chunkSize] {
		t.Errorf("Chunk 2: Zero-copy optimization failed (buffer copied)")
	}

	// Verify copy on the tail.
	if &dataRequests[2].buf[0] == &data[chunkSize*2] {
		t.Errorf("Tail: Expected buffer copy for small tail, but got zero-copy")
	}
}

type mockZeroCopySender struct {
	mu        sync.Mutex
	requests  []gRPCBidiWriteRequest
	errResult error
	wg        sync.WaitGroup // Waits for all async operations to complete.
}

func (m *mockZeroCopySender) connect(ctx context.Context, cs gRPCBufSenderChans, opts ...gax.CallOption) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		// Track active flush goroutines to prevent closing the channel prematurely.
		var completionWg sync.WaitGroup

		defer func() {
			completionWg.Wait()
			close(cs.completions)
		}()

		for req := range cs.requests {
			m.mu.Lock()
			m.requests = append(m.requests, req)
			m.mu.Unlock()

			if req.requestAck {
				select {
				case cs.requestAcks <- struct{}{}:
				case <-ctx.Done():
					return
				}
			}

			if req.flush {
				completionWg.Add(1)
				// Send completions asynchronously to avoid blocking the request loop.
				go func(offset int64) {
					defer completionWg.Done()
					select {
					case cs.completions <- gRPCBidiWriteCompletion{
						flushOffset: offset,
					}:
					case <-ctx.Done():
					}
				}(req.offset + int64(len(req.buf)))
			}
		}
	}()
}

func (m *mockZeroCopySender) err() error { return m.errResult }

// filterDataRequests returns only requests containing data, ignoring protocol overhead.
func filterDataRequests(reqs []gRPCBidiWriteRequest) []gRPCBidiWriteRequest {
	var dataReqs []gRPCBidiWriteRequest
	for _, r := range reqs {
		if len(r.buf) > 0 {
			dataReqs = append(dataReqs, r)
		}
	}
	return dataReqs
}
