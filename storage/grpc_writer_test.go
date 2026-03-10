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
			name:        "objectAttrs is nil",
			finishWrite: true,
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
			name:        "sendCRC32C is true",
			finishWrite: true,
			sendCRC32C:  true,
			attrs:       &ObjectAttrs{CRC32C: 123},
			want: &storagepb.ObjectChecksums{
				Crc32C: proto.Uint32(123),
			},
		},
		{
			name:        "MD5 is provided",
			finishWrite: true,
			attrs:       &ObjectAttrs{MD5: []byte{1, 5, 0}},
			want: &storagepb.ObjectChecksums{
				Md5Hash: []byte{1, 5, 0},
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
func TestGRPCWriter_MemoryAllocationPaths(t *testing.T) {
	tests := []struct {
		name         string
		chunkSize    int
		dataSize     int
		forceOneShot bool
		wantZeroCopy bool
	}{
		{
			name:         "OneShot_ZeroCopy_1MB",
			chunkSize:    0,
			dataSize:     1 * 1024 * 1024, // 1 MiB
			forceOneShot: true,
			wantZeroCopy: true,
		},
		{
			name:         "OneShot_ZeroCopy_10MB",
			chunkSize:    0,
			dataSize:     10 * 1024 * 1024, // 10 MiB
			forceOneShot: true,
			wantZeroCopy: true,
		},
		{
			name:         "Resumable_Buffering",
			chunkSize:    2 * 1024 * 1024, // 2 MiB
			dataSize:     1 * 1024 * 1024, // 1 MiB
			forceOneShot: false,
			wantZeroCopy: false,
		},
		{
			name:         "Resumable_ZeroCopy",
			chunkSize:    1 * 1024 * 1024, // 1 MiB
			dataSize:     2 * 1024 * 1024, // 2 MiB
			forceOneShot: false,
			wantZeroCopy: true,
		},
		{
			name:         "Resumable_Hybrid",
			chunkSize:    2 * 1024 * 1024, // 2 MiB
			dataSize:     3 * 1024 * 1024, // 3 MiB
			forceOneShot: false,
			wantZeroCopy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, tt.dataSize)
			data[0] = 1
			data[tt.dataSize-1] = 2
			chunkSize := gRPCChunkSize(tt.chunkSize)
			mockSender := &mockSender{}
			w := &gRPCWriter{
				buf:           nil, // Allocated lazily on first buffered write.
				chunkSize:     chunkSize,
				forceOneShot:  tt.forceOneShot,
				writeQuantum:  maxPerMessageWriteSize,
				preRunCtx:     context.Background(),
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

			if _, err := w.Write(data); err != nil {
				t.Fatalf("Write failed: %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("Close failed: %v", err)
			}
			mockSender.wg.Wait()

			mockSender.mu.Lock()
			defer mockSender.mu.Unlock()

			reqs := filterDataRequests(mockSender.requests)
			if len(reqs) == 0 {
				t.Fatalf("Expected at least 1 data request, got 0")
			}

			// Verify memory address logic:
			// The last byte of the last request buffer should match the last byte of the input data for zero-copy.
			// For buffering/copying, the pointers must differ.
			idx := len(reqs) - 1
			bufIdx := len(reqs[idx].buf) - 1
			isZeroCopy := &reqs[idx].buf[bufIdx] == &data[tt.dataSize-1]
			if isZeroCopy != tt.wantZeroCopy {
				if tt.wantZeroCopy && tt.forceOneShot {
					t.Errorf("One-shot upload bypassed zero-copy path; data was unexpectedly copied")
				} else if !tt.wantZeroCopy && !tt.forceOneShot {
					t.Errorf("Resumable upload bypassed buffering path; data was unexpectedly zero-copied")
				} else if tt.wantZeroCopy && !tt.forceOneShot {
					t.Errorf("Resumable upload bypassed zero-copy path; data was unexpectedly copied")
				}
			}
		})
	}
}

type mockSender struct {
	mu        sync.Mutex
	requests  []gRPCBidiWriteRequest
	errResult error
	wg        sync.WaitGroup // Waits for all async operations to complete.
}

func (m *mockSender) connect(ctx context.Context, cs gRPCBufSenderChans, opts ...gax.CallOption) {
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

func (m *mockSender) err() error { return m.errResult }

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
