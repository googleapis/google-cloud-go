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
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/protobuf/proto"
)

func TestGetObjectChecksums(t *testing.T) {
	tests := []struct {
		name                string
		fullObjectChecksum  func() *uint32
		finishWrite         bool
		sendCRC32C          bool
		disableAutoChecksum bool
		attrs               *ObjectAttrs
		append              bool
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
			fullObjectChecksum:  func() *uint32 { return proto.Uint32(456) },
			finishWrite:         true,
			sendCRC32C:          false,
			disableAutoChecksum: false,
			attrs:               &ObjectAttrs{},
			want: &storagepb.ObjectChecksums{
				Crc32C: proto.Uint32(456),
			},
		},
		{
			name:                "CRC32C enabled, but callback returns nil (missing initial checksum)",
			fullObjectChecksum:  func() *uint32 { return nil },
			finishWrite:         true,
			sendCRC32C:          false,
			disableAutoChecksum: false,
			attrs:               &ObjectAttrs{},
			want:                nil,
		},
		{
			name:                "Append operation without final user-provided CRC32C (callback returns nil)",
			fullObjectChecksum:  func() *uint32 { return nil },
			finishWrite:         true,
			append:              true,
			sendCRC32C:          false,
			disableAutoChecksum: false,
			attrs:               &ObjectAttrs{},
			want:                nil,
		},
		{
			name:                "Append operation with final CRC32C and initial CRC32C",
			fullObjectChecksum:  func() *uint32 { return proto.Uint32(123) },
			finishWrite:         true,
			append:              true,
			sendCRC32C:          false,
			disableAutoChecksum: false,
			attrs:               &ObjectAttrs{CRC32C: 456},
			want: &storagepb.ObjectChecksums{
				Crc32C: proto.Uint32(123),
			},
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
				append:              tt.append,
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
	mu         sync.Mutex
	requests   []gRPCBidiWriteRequest
	errResult  error
	wg         sync.WaitGroup // Waits for all async operations to complete.
	failOnData bool
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

			if m.failOnData && (req.flush || len(req.buf) > 0) {
				return
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

// Test the logic correctly handles the combination of io.EOF
// from Recv (recvErr) and a generic error from Send (sendErr).
func TestGRPCWriterErrorHandling(t *testing.T) {
	// As this is deeply embedded in the unexported types, we verify the logic
	// by simulating the exact error assignment sequence.
	tests := []struct {
		name      string
		recvErr   error
		sendErr   error
		wantError error
	}{
		{
			name:      "recvErr is io.EOF, sendErr is nil",
			recvErr:   io.EOF,
			sendErr:   nil,
			wantError: nil,
		},
		{
			name:      "recvErr is io.EOF, sendErr is an error",
			recvErr:   io.EOF,
			sendErr:   errors.New("send error"),
			wantError: errors.New("send error"), // Send error takes precedence.
		},
		{
			name:      "recvErr is an error, sendErr is nil",
			recvErr:   errors.New("recv error"),
			sendErr:   nil,
			wantError: errors.New("recv error"), // Recv error takes precedence.
		},
		{
			name:      "recvErr is an error, sendErr is an error",
			recvErr:   errors.New("recv error"),
			sendErr:   errors.New("send error"),
			wantError: errors.New("recv error"), // Recv error takes precedence.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var streamErr error

			streamErr = pickStreamError(tt.recvErr, tt.sendErr)

			if tt.wantError == nil {
				if streamErr != nil {
					t.Errorf("got error %v, want nil", streamErr)
				}
			} else {
				if streamErr == nil || streamErr.Error() != tt.wantError.Error() {
					t.Errorf("got error %v, want %v", streamErr, tt.wantError)
				}
			}
		})
	}
}

// TestGRPCWriter_Deadlock simulates a deadlock scenario if Recv and Send channels
// were not isolated in gRPCOneshotBidiWriteBufferSender.
func TestGRPCWriter_Deadlock(t *testing.T) {
	// A timeout means a deadlock likely occurred.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sendDone := make(chan struct{})
	recvDone := make(chan struct{})

	requests := make(chan gRPCBidiWriteRequest)
	completions := make(chan gRPCBidiWriteCompletion)

	var sendErr error

	go func() {
		sendErr = func() error {
			for {
				select {
				case <-recvDone:
					return nil
				case r, ok := <-requests:
					if !ok {
						return nil
					}
					if r.requestAck {
						continue
					}
					// mimic send logic
					if r.finishWrite {
						return nil
					}
				}
			}
		}()
		close(sendDone)
	}()

	go func() {
		// Mimic recv loop that immediately exits.
		// If recvDone isn't checked by the sender loop, sending
		// requests could block forever if the consumer closes early.
		close(recvDone)
	}()

	// sendDone should be closed immediately.
	select {
	case <-sendDone:
		// Success, no deadlock.
	case <-ctx.Done():
		t.Fatal("deadlock detected: send loop did not exit after recvDone was closed")
	}

	if sendErr != nil {
		t.Errorf("expected no error, got %v", sendErr)
	}
	close(completions)
}

type instantFailSender struct {
	errResult error
}

func (i *instantFailSender) connect(ctx context.Context, cs gRPCBufSenderChans, opts ...gax.CallOption) {
	// Immediately close completions to simulate stream failure.
	// writeLoop will detect this instantly and return errResult.
	close(cs.completions)
}

func (i *instantFailSender) err() error {
	return i.errResult
}

func TestGRPCWriter_ChunkRetryDeadline_TimeoutEnforcedAcrossRetries(t *testing.T) {
	ctx := context.Background()
	deadline := 100 * time.Millisecond
	sender := &instantFailSender{errResult: errors.New("transient network error")}
	w := &gRPCWriter{
		chunkRetryDeadline: deadline,
		streamSender:       sender,
		settings:           &settings{},
		bufUnsentIdx:       100, // Makes isActive() == true.
		bufFlushedIdx:      0,
		buf:                make([]byte, 100),
		sendableUnits:      1,
		writeQuantum:       100,
		chunkSize:          100,
		writesChan:         make(chan gRPCWriterCommand, 1),
	}

	var err error
	for i := 0; i < 20; i++ {
		err = w.writeLoop(ctx)
		if err != nil && strings.Contains(err.Error(), "retry deadline") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err == nil || !strings.Contains(err.Error(), "retry deadline") {
		t.Fatalf("expected retry deadline error, got: %v", err)
	}
	if w.attempts < 2 {
		t.Errorf("expected multiple attempts before deadline was reached, got %d", w.attempts)
	}
}

func TestGRPCWriter_ChunkRetryDeadline_TimeoutResetOnProgress(t *testing.T) {
	ctx := context.Background()
	deadline := 200 * time.Millisecond
	sender := &instantFailSender{errResult: errors.New("transient network error")}
	w := &gRPCWriter{
		chunkRetryDeadline: deadline,
		streamSender:       sender,
		settings:           &settings{},
		bufBaseOffset:      0,
		bufUnsentIdx:       100,
		bufFlushedIdx:      0,
		buf:                make([]byte, 100),
		sendableUnits:      1,
		writeQuantum:       100,
		chunkSize:          100,
		writesChan:         make(chan gRPCWriterCommand, 1),
		setSize:            func(int64) {},
		progress:           func(int64) {},
	}

	// Attempt 1: Start the clock.
	_ = w.writeLoop(ctx)

	// Sleep to consume more than half the deadline.
	time.Sleep(120 * time.Millisecond)

	// Attempt 2: Clock should not be expired yet.
	err := w.writeLoop(ctx)
	if err != nil && strings.Contains(err.Error(), "retry deadline") {
		t.Fatalf("deadline reached too early: %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "transient network error") {
		t.Fatalf("expected transient network error, got: %v", err)
	}

	// Simulate forward progress by invoking handleCompletion.
	w.handleCompletion(gRPCBidiWriteCompletion{flushOffset: 50})

	// Sleep to consume another portion of the original deadline.
	// If the timer wasn't reset, the next writeLoop would fail since
	// 120ms + 120ms = 240ms > 200ms.
	time.Sleep(120 * time.Millisecond)

	// Attempt 3: Clock was reset, so this should NOT fail with deadline exceeded.
	err = w.writeLoop(ctx)
	if err != nil && strings.Contains(err.Error(), "retry deadline") {
		t.Fatalf("timer was not reset by forward progress, got deadline error: %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "transient network error") {
		t.Fatalf("expected transient network error, got: %v", err)
	}

	// Attempt 4: Wait for the reset timer to actually expire.
	time.Sleep(100 * time.Millisecond)
	err = w.writeLoop(ctx)
	if err == nil || !strings.Contains(err.Error(), "retry deadline") {
		t.Fatalf("expected retry deadline error after reset timer expired, got: %v", err)
	}
}

func TestGRPCWriter_ChunkRetryDeadline_TimeoutPausedOnIdle(t *testing.T) {
	ctx := context.Background()
	deadline := 100 * time.Millisecond
	sender := &instantFailSender{errResult: errors.New("transient network error")}
	w := &gRPCWriter{
		chunkRetryDeadline: deadline,
		streamSender:       sender,
		settings:           &settings{},
		bufUnsentIdx:       0, // Makes isActive() == false.
		bufFlushedIdx:      0,
		buf:                make([]byte, 100),
		sendableUnits:      1,
		writeQuantum:       100,
		chunkSize:          100,
		writesChan:         make(chan gRPCWriterCommand, 1),
	}

	// Call writeLoop. Because isActive() is false, it should set abandonRetriesTime to zero.
	_ = w.writeLoop(ctx)

	if !w.abandonRetriesTime.IsZero() {
		t.Fatalf("expected timer to be zeroed when idle, but got: %v", w.abandonRetriesTime)
	}

	// Wait way past the deadline.
	time.Sleep(150 * time.Millisecond)

	// Next call should still not fail with deadline exceeded.
	err := w.writeLoop(ctx)
	if err != nil && strings.Contains(err.Error(), "retry deadline") {
		t.Fatalf("expected no deadline error when idle, got: %v", err)
	}
}

type checkTimerCmd struct {
	timerCh chan time.Time
}

func (c *checkTimerCmd) handle(w *gRPCWriter, cs gRPCWriterCommandHandleChans) error {
	c.timerCh <- w.abandonRetriesTime
	return nil
}

func TestGRPCWriter_ChunkRetryDeadline_TimerStartsOnlyWhenBufferFills(t *testing.T) {
	ctx := context.Background()
	deadline := 100 * time.Millisecond
	sender := &mockSender{errResult: errors.New("transient network error"), failOnData: true}

	w := &gRPCWriter{
		chunkRetryDeadline: deadline,
		streamSender:       sender,
		settings:           &settings{},
		bufUnsentIdx:       0,
		bufFlushedIdx:      0,
		buf:                make([]byte, 0, 100),
		sendableUnits:      1,
		writeQuantum:       100,
		chunkSize:          100,
		writesChan:         make(chan gRPCWriterCommand, 3),
		setSize:            func(int64) {},
		progress:           func(int64) {},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- w.writeLoop(ctx)
	}()

	for i := 0; i < 4; i++ {
		done := make(chan struct{})
		w.writesChan <- &gRPCWriterCommandWrite{p: make([]byte, 20), done: done}
		<-done

		// Assert the timer is not started yet because the chunk size hasn't been reached.
		timerCh := make(chan time.Time)
		w.writesChan <- &checkTimerCmd{timerCh: timerCh}
		if abandonRetriesTime := <-timerCh; !abandonRetriesTime.IsZero() {
			t.Fatalf("expected timer to NOT be started before buffer fills, but it was %v after %d writes", abandonRetriesTime, i+1)
		}
	}

	done2 := make(chan struct{})
	w.writesChan <- &gRPCWriterCommandWrite{p: make([]byte, 20), done: done2} // Fills buffer!

	err := <-errCh

	if err == nil || !strings.Contains(err.Error(), "transient network error") {
		t.Fatalf("expected transient network error when buffer fills and triggers send, got: %v", err)
	}
	if w.abandonRetriesTime.IsZero() {
		t.Fatalf("expected timer to be started when buffer fills and triggers send, but it was zero")
	}
}
