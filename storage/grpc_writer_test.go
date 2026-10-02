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
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	gax "github.com/googleapis/gax-go/v2"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	mu               sync.Mutex
	requests         []gRPCBidiWriteRequest
	errResult        error
	wg               sync.WaitGroup // Waits for all async operations to complete.
	failOnData       bool
	respondToAllData bool
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
			failOnData := m.failOnData
			respondToAllData := m.respondToAllData
			m.mu.Unlock()

			if req.requestAck {
				select {
				case cs.requestAcks <- struct{}{}:
				case <-ctx.Done():
					return
				}
			}

			if failOnData && (req.flush || len(req.buf) > 0) {
				return
			}

			if req.flush || respondToAllData {
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

func (m *mockSender) err() error             { return m.errResult }
func (m *mockSender) canResumeSession() bool { return false }

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
	canResume bool
}

func (i *instantFailSender) connect(ctx context.Context, cs gRPCBufSenderChans, opts ...gax.CallOption) {
	// Immediately close completions to simulate stream failure.
	// writeLoop will detect this instantly and return errResult.
	close(cs.completions)
}

func (i *instantFailSender) err() error {
	return i.errResult
}

func (i *instantFailSender) canResumeSession() bool {
	return i.canResume
}

// fakeClock is a manually advanced clock for chunkRetryBudget. Now may be
// called from the writer goroutine while the test goroutine calls Advance.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func testBudget(clk *fakeClock, deadline time.Duration) chunkRetryBudget {
	return chunkRetryBudget{deadline: deadline, now: clk.Now}
}

// progressRecorder captures w.progress callbacks so a test can wait for the
// writer goroutine to finish handleCompletion. A command sent after waitFor
// observes the post-completion state.
type progressRecorder struct {
	ch chan int64
}

func newProgressRecorder() *progressRecorder {
	return &progressRecorder{ch: make(chan int64, 64)}
}

func (p *progressRecorder) report(n int64) { p.ch <- n }

func (p *progressRecorder) waitFor(t *testing.T, offset int64) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case got := <-p.ch:
			if got >= offset {
				return
			}
		case <-timeout:
			t.Fatalf("timed out waiting for progress to reach %d", offset)
		}
	}
}

// attemptWriteLoop mirrors the closure OpenWriter hands to run(): it records
// the result in w.lastErr so a later chunkRetryDeadlineError wraps it.
func attemptWriteLoop(ctx context.Context, w *gRPCWriter) error {
	w.lastErr = w.writeLoop(ctx)
	return w.lastErr
}

func TestGRPCWriter_ChunkRetryDeadline_TimeoutEnforcedAcrossRetries(t *testing.T) {
	ctx := context.Background()
	clk := newFakeClock()
	deadline := 100 * time.Millisecond
	transientErr := errors.New("transient network error")
	sender := &instantFailSender{errResult: transientErr}
	w := &gRPCWriter{
		budget:        testBudget(clk, deadline),
		streamSender:  sender,
		settings:      &settings{},
		bufUnsentIdx:  100, // Makes isActive() == true.
		bufFlushedIdx: 0,
		buf:           make([]byte, 100),
		sendableUnits: 1,
		writeQuantum:  100,
		chunkSize:     100,
		writesChan:    make(chan gRPCWriterCommand, 1),
	}

	// Attempt 1 arms the stopwatch and fails.
	if err := attemptWriteLoop(ctx, w); !errors.Is(err, transientErr) {
		t.Fatalf("attempt 1: got %v, want %v", err, transientErr)
	}

	// Attempt 2, just inside the budget, must still be a plain transport error.
	clk.Advance(deadline - time.Millisecond)
	if err := attemptWriteLoop(ctx, w); isChunkRetryDeadlineError(err) {
		t.Fatalf("attempt 2: deadline reached too early: %v", err)
	}

	// Attempt 3, just past the budget, must fail with the deadline error.
	clk.Advance(2 * time.Millisecond)
	err := attemptWriteLoop(ctx, w)
	var deadlineErr *chunkRetryDeadlineError
	if !errors.As(err, &deadlineErr) {
		t.Fatalf("attempt 3: got %T %v, want *chunkRetryDeadlineError", err, err)
	}
	if deadlineErr.attempts != 3 {
		t.Errorf("deadlineErr.attempts = %d, want 3", deadlineErr.attempts)
	}
	if deadlineErr.deadline != deadline {
		t.Errorf("deadlineErr.deadline = %v, want %v", deadlineErr.deadline, deadline)
	}
	// The transport error is reachable through errors.Is and the message keeps
	// the "retry deadline" phrase that integration tests match.
	if !errors.Is(err, transientErr) {
		t.Errorf("errors.Is(err, transientErr) = false; err: %v", err)
	}
	if !strings.Contains(err.Error(), "retry deadline") {
		t.Errorf("err = %q, want it to mention \"retry deadline\"", err)
	}
}

func TestGRPCWriter_ChunkRetryDeadline_TimeoutResetOnProgress(t *testing.T) {
	ctx := context.Background()
	clk := newFakeClock()
	deadline := 200 * time.Millisecond
	transientErr := errors.New("transient network error")
	sender := &instantFailSender{errResult: transientErr}
	w := &gRPCWriter{
		budget:        testBudget(clk, deadline),
		streamSender:  sender,
		settings:      &settings{},
		bufBaseOffset: 0,
		bufUnsentIdx:  100,
		bufFlushedIdx: 0,
		buf:           make([]byte, 100),
		sendableUnits: 1,
		writeQuantum:  100,
		chunkSize:     100,
		writesChan:    make(chan gRPCWriterCommand, 1),
		setSize:       func(int64) {},
		progress:      func(int64) {},
	}

	// Attempt 1: start the clock.
	_ = attemptWriteLoop(ctx, w)

	// Consume more than half the deadline.
	clk.Advance(120 * time.Millisecond)

	// Attempt 2: clock should not be expired yet.
	if err := attemptWriteLoop(ctx, w); !errors.Is(err, transientErr) || isChunkRetryDeadlineError(err) {
		t.Fatalf("attempt 2: got %v, want %v", err, transientErr)
	}

	// Forward progress resets the stopwatch.
	w.handleCompletion(gRPCBidiWriteCompletion{flushOffset: 50})

	// Another 120ms: 240ms since the original start, but only 120ms since the
	// reset.
	clk.Advance(120 * time.Millisecond)

	// Attempt 3: must not be a deadline error.
	if err := attemptWriteLoop(ctx, w); !errors.Is(err, transientErr) || isChunkRetryDeadlineError(err) {
		t.Fatalf("attempt 3: timer was not reset by forward progress: %v", err)
	}

	// Attempt 4: the reset stopwatch has now expired (220ms > 200ms).
	clk.Advance(100 * time.Millisecond)
	if err := attemptWriteLoop(ctx, w); !isChunkRetryDeadlineError(err) {
		t.Fatalf("attempt 4: got %v, want chunkRetryDeadlineError", err)
	}
}

func TestGRPCWriter_ChunkRetryDeadline_TimeoutPausedOnIdle(t *testing.T) {
	ctx := context.Background()
	clk := newFakeClock()
	deadline := 100 * time.Millisecond
	sender := &instantFailSender{errResult: errors.New("transient network error")}
	w := &gRPCWriter{
		budget:        testBudget(clk, deadline),
		streamSender:  sender,
		settings:      &settings{},
		bufUnsentIdx:  0, // Makes isActive() == false.
		bufFlushedIdx: 0,
		buf:           make([]byte, 0, 100),
		sendableUnits: 1,
		writeQuantum:  100,
		chunkSize:     100,
		writesChan:    make(chan gRPCWriterCommand, 1),
	}

	// Because isActive() is false, writeLoop must leave the stopwatch stopped.
	_ = attemptWriteLoop(ctx, w)
	if !w.budget.expiresAt.IsZero() {
		t.Fatalf("expected stopwatch to be stopped when idle, got expiresAt=%v", w.budget.expiresAt)
	}

	// Way past the deadline, still idle: must not be a deadline error.
	clk.Advance(10 * deadline)
	if err := attemptWriteLoop(ctx, w); isChunkRetryDeadlineError(err) {
		t.Fatalf("expected no deadline error when idle, got: %v", err)
	}
}

// checkTimerCmd reports the budget's expiry from the writer goroutine.
type checkTimerCmd struct {
	timerCh chan time.Time
}

func (c *checkTimerCmd) handle(w *gRPCWriter, cs gRPCWriterCommandHandleChans) error {
	c.timerCh <- w.budget.expiresAt
	return nil
}

func TestGRPCWriter_ChunkRetryDeadline_TimerStartsOnlyWhenBufferFills(t *testing.T) {
	ctx := context.Background()
	clk := newFakeClock()
	deadline := 100 * time.Millisecond
	sender := &mockSender{errResult: errors.New("transient network error"), failOnData: true}

	w := &gRPCWriter{
		budget:        testBudget(clk, deadline),
		streamSender:  sender,
		settings:      &settings{},
		bufUnsentIdx:  0,
		bufFlushedIdx: 0,
		buf:           make([]byte, 0, 100),
		sendableUnits: 1,
		writeQuantum:  100,
		chunkSize:     100,
		writesChan:    make(chan gRPCWriterCommand, 3),
		setSize:       func(int64) {},
		progress:      func(int64) {},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- w.writeLoop(ctx)
	}()

	for i := 0; i < 4; i++ {
		done := make(chan struct{})
		w.writesChan <- &gRPCWriterCommandWrite{p: make([]byte, 20), done: done}
		<-done

		// The stopwatch must not be armed before the buffer fills.
		timerCh := make(chan time.Time)
		w.writesChan <- &checkTimerCmd{timerCh: timerCh}
		if expiresAt := <-timerCh; !expiresAt.IsZero() {
			t.Fatalf("expected stopwatch to be stopped before buffer fills, but expiresAt=%v after %d writes", expiresAt, i+1)
		}
	}

	done2 := make(chan struct{})
	w.writesChan <- &gRPCWriterCommandWrite{p: make([]byte, 20), done: done2} // Fills buffer!

	err := <-errCh

	if err == nil || !strings.Contains(err.Error(), "transient network error") {
		t.Fatalf("expected transient network error when buffer fills and triggers send, got: %v", err)
	}
	if want := clk.Now().Add(deadline); !w.budget.expiresAt.Equal(want) {
		t.Fatalf("expected stopwatch armed at %v when buffer fills and triggers send, got %v", want, w.budget.expiresAt)
	}
}

func TestGRPCWriter_ChunkRetryDeadline_StaleTimerOnPartialBuffer(t *testing.T) {
	ctx := context.Background()
	clk := newFakeClock()
	deadline := 100 * time.Millisecond
	sender := &mockSender{errResult: errors.New("transient network error"), failOnData: false, respondToAllData: true}
	progress := newProgressRecorder()

	w := &gRPCWriter{
		budget:           testBudget(clk, deadline),
		streamSender:     sender,
		settings:         &settings{},
		bufUnsentIdx:     0,
		awaitingFirstAck: true,
		buf:              make([]byte, 0, 1000),
		sendableUnits:    1,
		writeQuantum:     100,
		chunkSize:        1000,
		writesChan:       make(chan gRPCWriterCommand, 3),
		setSize:          func(int64) {},
		progress:         progress.report,
		setObj:           func(*ObjectAttrs) {},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- w.writeLoop(ctx)
	}()

	// Write 150 bytes (writeQuantum is 100): 100 bytes are sent and acked, 50
	// remain unsent in buf.
	done := make(chan struct{})
	w.writesChan <- &gRPCWriterCommandWrite{p: make([]byte, 150), done: done}
	<-done
	progress.waitFor(t, 100)

	// With only 50 unsent bytes (< writeQuantum) the writer is idle, so the
	// stopwatch must be stopped.
	timerCh := make(chan time.Time)
	w.writesChan <- &checkTimerCmd{timerCh: timerCh}
	if expiresAt := <-timerCh; !expiresAt.IsZero() {
		t.Fatalf("expected stopwatch to be stopped when remaining unsent bytes < writeQuantum, got expiresAt=%v", expiresAt)
	}

	// Idle for longer than the deadline.
	clk.Advance(deadline + 50*time.Millisecond)

	// Now fail on data requests.
	sender.mu.Lock()
	sender.failOnData = true
	sender.mu.Unlock()

	// Write 50 more bytes to complete the next quantum (100 unsent bytes).
	done2 := make(chan struct{})
	w.writesChan <- &gRPCWriterCommandWrite{p: make([]byte, 50), done: done2}

	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "transient network error") {
		t.Fatalf("expected transient network error, got: %v", err)
	}

	// The retry must not fail instantly because of a stale stopwatch.
	if err := attemptWriteLoop(ctx, w); isChunkRetryDeadlineError(err) {
		t.Fatalf("retry failed instantly due to stale timer: %v", err)
	}
}

// TestGRPCWriter_ChunkRetryDeadline_OversizedWriteStaleTimerOnClose tests that an oversized write (len(p) > chunkSize)
// that leaves unsent leftover bytes (< writeQuantum) in w.buf properly stops the chunk retry stopwatch when idle,
// so that a subsequent Close() operation after an idle delay does not fail with a stale retry deadline error.

func TestGRPCWriter_ChunkRetryDeadline_OversizedWriteStaleTimerOnClose(t *testing.T) {
	ctx := context.Background()
	clk := newFakeClock()
	deadline := 100 * time.Millisecond
	sender := &mockSender{errResult: errors.New("transient network error"), failOnData: false, respondToAllData: true}
	progress := newProgressRecorder()

	// chunkSize = 1000, writeQuantum = 350 (350 is NOT a factor of 1000; 1000 / 350 = 2 with remainder 300)
	w := &gRPCWriter{
		budget:           testBudget(clk, deadline),
		streamSender:     sender,
		settings:         &settings{},
		bufUnsentIdx:     0,
		awaitingFirstAck: true,
		buf:              make([]byte, 0, 1000),
		sendableUnits:    3,
		writeQuantum:     350,
		chunkSize:        1000,
		lastSegmentStart: 700,
		writesChan:       make(chan gRPCWriterCommand, 3),
		setSize:          func(int64) {},
		progress:         progress.report,
		setObj:           func(*ObjectAttrs) {},
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- w.writeLoop(ctx)
	}()

	// 1. Write 1500 bytes (exceeds chunkSize of 1000: sends 350, 350, 300 tail chunk + 350 in buf; 150 unsent in buf).
	done := make(chan struct{})
	w.writesChan <- &gRPCWriterCommandWrite{p: make([]byte, 1500), done: done}
	<-done
	progress.waitFor(t, 1350)

	timerCh := make(chan time.Time)
	w.writesChan <- &checkTimerCmd{timerCh: timerCh}
	if t1 := <-timerCh; !t1.IsZero() {
		t.Fatalf("expected stopwatch to be stopped after sending large write with 150 unsent bytes < 350 quantum, got: %v", t1)
	}

	// 2. Idle past the deadline with 150 unsent bytes.
	clk.Advance(deadline + 50*time.Millisecond)

	// 3. Fail on data requests now.
	sender.mu.Lock()
	sender.failOnData = true
	sender.mu.Unlock()

	// 4. Send Close command (triggers tail send of remaining 150 bytes with finishWrite: true).
	w.writesChan <- &gRPCWriterCommandClose{err: nil}

	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "transient network error") {
		t.Fatalf("expected transient network error on Close, got: %v", err)
	}

	// 5. Retry writeLoop — must NOT fail with stale retry deadline error!
	err = attemptWriteLoop(ctx, w)
	if err == nil || !strings.Contains(err.Error(), "transient network error") || isChunkRetryDeadlineError(err) {
		t.Fatalf("expected retry to fail with transient network error, got: %v", err)
	}
}

func TestGRPCWriter_ChunkRetryDeadline_SingleShotCloseError(t *testing.T) {
	ctx := context.Background()
	clk := newFakeClock()
	deadline := 100 * time.Millisecond
	sender := &mockSender{errResult: errors.New("transient network error"), failOnData: true}

	w := &gRPCWriter{
		budget:           testBudget(clk, deadline),
		streamSender:     sender,
		settings:         &settings{},
		bufUnsentIdx:     0,
		awaitingFirstAck: true,
		buf:              make([]byte, 0, 1000),
		sendableUnits:    1,
		writeQuantum:     1000,
		chunkSize:        1000,
		writesChan:       make(chan gRPCWriterCommand, 3),
		setSize:          func(int64) {},
		progress:         func(int64) {},
		setObj:           func(*ObjectAttrs) {},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- w.writeLoop(ctx)
	}()

	// Write 50 bytes (fits in buffer, staged locally).
	done := make(chan struct{})
	w.writesChan <- &gRPCWriterCommandWrite{p: make([]byte, 50), done: done}
	<-done

	// Send Close command.
	w.writesChan <- &gRPCWriterCommandClose{err: nil}

	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "transient network error") {
		t.Fatalf("expected transient network error on Close, got: %v", err)
	}

	// Retry writeLoop while failOnData is true. It must return transient network error, NOT a stale retry deadline error.
	err = attemptWriteLoop(ctx, w)
	if err == nil || !strings.Contains(err.Error(), "transient network error") || isChunkRetryDeadlineError(err) {
		t.Fatalf("expected retry to fail with transient network error, got: %v", err)
	}
}

func TestGRPCWriter_ChunkRetryDeadline_PartialQuantumCloseStaleTimer(t *testing.T) {
	ctx := context.Background()
	clk := newFakeClock()
	deadline := 100 * time.Millisecond
	sender := &mockSender{errResult: errors.New("transient network error"), failOnData: false, respondToAllData: true}
	progress := newProgressRecorder()

	w := &gRPCWriter{
		budget:           testBudget(clk, deadline),
		streamSender:     sender,
		settings:         &settings{},
		bufUnsentIdx:     0,
		awaitingFirstAck: true,
		buf:              make([]byte, 0, 1000),
		sendableUnits:    10,
		writeQuantum:     100,
		chunkSize:        1000,
		writesChan:       make(chan gRPCWriterCommand, 3),
		setSize:          func(int64) {},
		progress:         progress.report,
		setObj:           func(*ObjectAttrs) {},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- w.writeLoop(ctx)
	}()

	// 1. Write 150 bytes: 100 sent/acked, 50 unsent in buf.
	done := make(chan struct{})
	w.writesChan <- &gRPCWriterCommandWrite{p: make([]byte, 150), done: done}
	<-done
	progress.waitFor(t, 100)

	// Idle past the deadline with 50 unsent bytes.
	clk.Advance(deadline + 20*time.Millisecond)

	// Fail on data requests now.
	sender.mu.Lock()
	sender.failOnData = true
	sender.mu.Unlock()

	// 2. Send Close command (triggers tail send of remaining 50 bytes with finishWrite: true).
	w.writesChan <- &gRPCWriterCommandClose{err: nil}

	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "transient network error") {
		t.Fatalf("expected transient network error on Close tail, got: %v", err)
	}

	// 3. Retry writeLoop while failOnData is true. Must return transient network error, NOT a stale retry deadline error.
	err = attemptWriteLoop(ctx, w)
	if err == nil || !strings.Contains(err.Error(), "transient network error") || isChunkRetryDeadlineError(err) {
		t.Fatalf("expected retry on Close tail to fail with transient network error, got: %v", err)
	}
}

func TestGRPCWriter_CanResumeSession(t *testing.T) {
	oneshot := &gRPCOneshotBidiWriteBufferSender{}
	if oneshot.canResumeSession() {
		t.Errorf("oneshot sender should never report canResumeSession() == true")
	}

	resumable := &gRPCResumableBidiWriteBufferSender{}
	if resumable.canResumeSession() {
		t.Errorf("resumable sender without upload ID should report canResumeSession() == false")
	}
	resumable.upid = "upload-123"
	if !resumable.canResumeSession() {
		t.Errorf("resumable sender with upload ID should report canResumeSession() == true")
	}

	appendSender := &gRPCAppendBidiWriteBufferSender{
		firstMessage: &storagepb.BidiWriteObjectRequest{
			FirstMessage: &storagepb.BidiWriteObjectRequest_WriteObjectSpec{
				WriteObjectSpec: &storagepb.WriteObjectSpec{},
			},
		},
	}
	if appendSender.canResumeSession() {
		t.Errorf("append sender before receiving WriteHandle should report canResumeSession() == false")
	}
	appendSender.maybeUpdateFirstMessage(&storagepb.BidiWriteObjectResponse{
		WriteHandle: &storagepb.BidiWriteHandle{Handle: []byte("handle-1")},
	})
	if !appendSender.canResumeSession() {
		t.Errorf("append sender with WriteHandle should report canResumeSession() == true")
	}

	takeover := &gRPCAppendTakeoverBidiWriteBufferSender{
		gRPCAppendBidiWriteBufferSender: gRPCAppendBidiWriteBufferSender{
			takeoverWriter: true,
		},
	}
	if !takeover.canResumeSession() {
		t.Errorf("append takeover sender should report canResumeSession() == true")
	}
}

func TestGRPCWriter_SessionRecoveryRetries(t *testing.T) {
	transientErr := status.Error(codes.Unavailable, "transient unavailable")
	redirectErr := fmt.Errorf("%w%w", bidiWriteObjectRedirectionError{}, status.Error(codes.Aborted, "redirect"))

	tests := []struct {
		name         string
		policy       RetryPolicy
		idempotent   bool
		append       bool
		canResume    bool
		firstErr     error
		wantAttempts int
		wantErr      bool
	}{
		{
			name:         "RetryIdempotent_NoPreconditions_BeforeSession_DoesNotRetryTransient",
			policy:       RetryIdempotent,
			idempotent:   false,
			canResume:    false,
			firstErr:     transientErr,
			wantAttempts: 1,
			wantErr:      true,
		},
		{
			name:         "RetryIdempotent_NoPreconditions_AfterSession_RetriesTransient",
			policy:       RetryIdempotent,
			idempotent:   false,
			canResume:    true,
			firstErr:     transientErr,
			wantAttempts: 2,
			wantErr:      false,
		},
		{
			name:         "RetryIdempotent_WithPreconditions_BeforeSession_RetriesTransient",
			policy:       RetryIdempotent,
			idempotent:   true,
			canResume:    false,
			firstErr:     transientErr,
			wantAttempts: 2,
			wantErr:      false,
		},
		{
			name:         "RetryIdempotent_Append_BeforeSession_RetriesRedirect",
			policy:       RetryIdempotent,
			idempotent:   false,
			append:       true,
			canResume:    false,
			firstErr:     redirectErr,
			wantAttempts: 2,
			wantErr:      false,
		},
		{
			name:         "RetryAlways_NoPreconditions_BeforeSession_RetriesTransient",
			policy:       RetryAlways,
			idempotent:   false,
			canResume:    false,
			firstErr:     transientErr,
			wantAttempts: 2,
			wantErr:      false,
		},
		{
			name:         "RetryAlways_NoPreconditions_AfterSession_RetriesTransient",
			policy:       RetryAlways,
			idempotent:   false,
			canResume:    true,
			firstErr:     transientErr,
			wantAttempts: 2,
			wantErr:      false,
		},
		{
			name:         "RetryNever_AfterSession_DoesNotRetryTransient",
			policy:       RetryNever,
			idempotent:   false,
			canResume:    true,
			firstErr:     transientErr,
			wantAttempts: 1,
			wantErr:      true,
		},
		{
			name:         "RetryNever_Append_RetriesRedirect",
			policy:       RetryNever,
			idempotent:   false,
			append:       true,
			canResume:    false,
			firstErr:     redirectErr,
			wantAttempts: 2,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &instantFailSender{canResume: tt.canResume}
			w := &gRPCWriter{
				append:       tt.append,
				streamSender: sender,
				settings: &settings{
					idempotent: tt.idempotent,
					retry: &retryConfig{
						policy: tt.policy,
						backoff: &gax.Backoff{
							Initial:    time.Millisecond,
							Max:        5 * time.Millisecond,
							Multiplier: 1.1,
						},
					},
				},
			}

			calls := 0
			err := run(context.Background(), func(ctx context.Context) error {
				calls++
				if calls == 1 {
					return tt.firstErr
				}
				return nil
			}, w.writerRetryConfig(), w.settings.idempotent, withOperation("WriteObject"))

			if (err != nil) != tt.wantErr {
				t.Fatalf("run() error = %v, wantErr %v", err, tt.wantErr)
			}
			if calls != tt.wantAttempts {
				t.Fatalf("got %d attempts, want %d", calls, tt.wantAttempts)
			}
		})
	}
}

func TestGRPCWriter_RetryConfigDefaultBackoff(t *testing.T) {
	tests := []struct {
		name  string
		retry *retryConfig
		want  gax.Backoff
	}{
		{
			name:  "NoRetryConfig",
			retry: nil,
			want:  gax.Backoff{Initial: defaultWriteRetryInitialBackoff},
		},
		{
			name:  "RetryConfigWithoutBackoff",
			retry: &retryConfig{policy: RetryAlways},
			want:  gax.Backoff{Initial: defaultWriteRetryInitialBackoff},
		},
		{
			name: "UserBackoffPreserved",
			retry: &retryConfig{
				backoff: &gax.Backoff{Initial: 2 * time.Second, Max: 10 * time.Second, Multiplier: 3},
			},
			want: gax.Backoff{Initial: 2 * time.Second, Max: 10 * time.Second, Multiplier: 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &gRPCWriter{
				streamSender: &instantFailSender{},
				settings:     &settings{retry: tt.retry},
			}
			got := w.writerRetryConfig().backoff
			if got == nil {
				t.Fatal("writerRetryConfig().backoff = nil, want non-nil")
			}
			if got.Initial != tt.want.Initial || got.Max != tt.want.Max || got.Multiplier != tt.want.Multiplier {
				t.Errorf("writerRetryConfig().backoff = {Initial: %v, Max: %v, Multiplier: %v}, want {Initial: %v, Max: %v, Multiplier: %v}",
					got.Initial, got.Max, got.Multiplier, tt.want.Initial, tt.want.Max, tt.want.Multiplier)
			}
			if tt.retry != nil && tt.retry.backoff == got {
				t.Errorf("writerRetryConfig() aliased the caller's backoff")
			}
		})
	}

	if defaultRetry.backoff != nil {
		t.Errorf("writerRetryConfig() mutated defaultRetry.backoff to %+v", defaultRetry.backoff)
	}
}

func TestGRPCWriter_ChunkTransferTimeoutPlumbing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := &grpcStorageClient{settings: &settings{}}
	wantTimeout := 150 * time.Millisecond
	iw, err := c.OpenWriter(&openWriterParams{
		ctx:                  ctx,
		bucket:               "b",
		attrs:                &ObjectAttrs{Name: "o"},
		chunkTransferTimeout: wantTimeout,
		donec:                make(chan struct{}),
		setError:             func(error) {},
		progress:             func(int64) {},
		setObj:               func(*ObjectAttrs) {},
		setSize:              func(int64) {},
	})
	if err != nil {
		t.Fatalf("OpenWriter failed: %v", err)
	}
	gw, ok := iw.(*gRPCWriter)
	if !ok {
		t.Fatalf("expected *gRPCWriter, got %T", iw)
	}
	if gw.chunkTransferTimeout != wantTimeout {
		t.Errorf("gw.chunkTransferTimeout = %v, want %v", gw.chunkTransferTimeout, wantTimeout)
	}
	_ = gw.CloseWithError(context.Canceled)
}

func TestStallTimeoutError(t *testing.T) {
	tests := []struct {
		name    string
		err     *stallTimeoutError
		wantMsg string
	}{
		{
			name: "with stage",
			err: &stallTimeoutError{
				timeout: 250 * time.Millisecond,
				stage:   "BidiWriteObject",
			},
			wantMsg: "storage: chunk transfer timeout (BidiWriteObject exceeded 250ms)",
		},
		{
			name: "without stage",
			err: &stallTimeoutError{
				timeout: 250 * time.Millisecond,
			},
			wantMsg: "storage: chunk transfer timeout (exceeded 250ms)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.wantMsg {
				t.Errorf("err.Error() = %q, want %q", got, tt.wantMsg)
			}
			if !errors.Is(tt.err, errStallTimeout) {
				t.Errorf("expected errors.Is(err, errStallTimeout) to be true")
			}
			if errors.Is(tt.err, context.Canceled) {
				t.Errorf("stallTimeoutError must not match context.Canceled")
			}
			if !tt.err.Temporary() {
				t.Errorf("expected Temporary() to be true")
			}
			if !tt.err.Timeout() {
				t.Errorf("expected Timeout() to be true")
			}
			if !ShouldRetry(tt.err) {
				t.Errorf("expected ShouldRetry(stallTimeoutError) to be true")
			}
			if got := checkCanceled(tt.err); !errors.Is(got, errStallTimeout) {
				t.Errorf("checkCanceled(stallTimeoutError) = %v, want errStallTimeout", got)
			}
		})
	}

	if !ShouldRetry(errStallTimeout) {
		t.Errorf("expected ShouldRetry(errStallTimeout) to be true")
	}
}

type reconnectStatusSender struct {
	mu            sync.Mutex
	persistedSize int64
	errResult     error
}

func (s *reconnectStatusSender) connect(ctx context.Context, cs gRPCBufSenderChans, opts ...gax.CallOption) {
	s.mu.Lock()
	persisted := s.persistedSize
	s.mu.Unlock()
	cs.completions <- gRPCBidiWriteCompletion{flushOffset: persisted}
	go func() {
		select {
		case <-cs.requests:
		case <-ctx.Done():
		}
		close(cs.completions)
	}()
}

func (s *reconnectStatusSender) err() error {
	return s.errResult
}

func (s *reconnectStatusSender) canResumeSession() bool { return false }

func TestGRPCWriter_ChunkRetryDeadline_NoResetOnUnchangedOffsetAfterPartialShift(t *testing.T) {
	ctx := context.Background()
	clk := newFakeClock()
	deadline := 200 * time.Millisecond
	transientErr := errors.New("transient network error")
	sender := &reconnectStatusSender{
		persistedSize: 50,
		errResult:     transientErr,
	}

	w := &gRPCWriter{
		budget:           testBudget(clk, deadline),
		streamSender:     sender,
		settings:         &settings{},
		bufBaseOffset:    0,
		bufUnsentIdx:     100,
		awaitingFirstAck: true,
		buf:              make([]byte, 100),
		sendableUnits:    2,
		writeQuantum:     50,
		chunkSize:        100,
		writesChan:       make(chan gRPCWriterCommand, 1),
		setSize:          func(int64) {},
		progress:         func(int64) {},
		setObj:           func(*ObjectAttrs) {},
	}

	// Attempt 1: QueryWriteStatus reports 50 bytes persisted (strict progress
	// from 0 -> 50). writeLoop drains the completion, restarts the stopwatch,
	// shifts w.buf by 50 bytes (bufBaseOffset = 50, bufFlushedIdx = 0), and
	// then returns the transport error.
	if err := attemptWriteLoop(ctx, w); !errors.Is(err, transientErr) {
		t.Fatalf("attempt 1: got %v, want %v", err, transientErr)
	}
	firstExpiry := w.budget.expiresAt
	if want := clk.Now().Add(deadline); !firstExpiry.Equal(want) {
		t.Fatalf("expected stopwatch restarted at %v after partial progress, got %v", want, firstExpiry)
	}
	if w.bufBaseOffset != 50 || w.bufFlushedIdx != 0 {
		t.Fatalf("expected bufBaseOffset=50 and bufFlushedIdx=0 after shift, got base=%d flushed=%d", w.bufBaseOffset, w.bufFlushedIdx)
	}
	if w.awaitingFirstAck {
		t.Fatalf("expected awaitingFirstAck to be cleared by the first completion")
	}
	if !w.budget.consumeProgress() {
		t.Fatalf("expected consumeProgress() == true after strict forward progress to offset 50")
	}

	// Consume more than half of the retry deadline.
	clk.Advance(120 * time.Millisecond)

	// Attempt 2: reconnect reports the SAME persistedSize = 50. Must not
	// restart the stopwatch or reset attempts.
	if err := attemptWriteLoop(ctx, w); !errors.Is(err, transientErr) {
		t.Fatalf("attempt 2: got %v, want %v", err, transientErr)
	}
	if !w.budget.expiresAt.Equal(firstExpiry) {
		t.Fatalf("expected expiresAt to remain %v on unchanged offset, got %v", firstExpiry, w.budget.expiresAt)
	}
	if w.budget.attempts != 1 {
		t.Fatalf("expected budget.attempts to be 1 (not reset to 0), got %d", w.budget.attempts)
	}
	if w.budget.consumeProgress() {
		t.Fatalf("expected consumeProgress() == false when offset remained unchanged at 50")
	}

	// Past the deadline (120ms + 100ms > 200ms).
	clk.Advance(100 * time.Millisecond)

	// Attempt 3: must fail with the deadline error because no progress was
	// made since offset 50.
	err := attemptWriteLoop(ctx, w)
	var deadlineErr *chunkRetryDeadlineError
	if !errors.As(err, &deadlineErr) {
		t.Fatalf("attempt 3: got %v, want *chunkRetryDeadlineError", err)
	}
	if deadlineErr.attempts != 2 {
		t.Errorf("deadlineErr.attempts = %d, want 2", deadlineErr.attempts)
	}
}

func TestGRPCWriter_ChunkRetryDeadline_NoResetOnZeroOffsetReconnect(t *testing.T) {
	ctx := context.Background()
	clk := newFakeClock()
	deadline := 200 * time.Millisecond
	transientErr := errors.New("transient network error")
	sender := &reconnectStatusSender{
		persistedSize: 0,
		errResult:     transientErr,
	}

	w := &gRPCWriter{
		budget:           testBudget(clk, deadline),
		streamSender:     sender,
		settings:         &settings{},
		bufBaseOffset:    0,
		bufUnsentIdx:     100,
		awaitingFirstAck: true,
		buf:              make([]byte, 100),
		sendableUnits:    1,
		writeQuantum:     100,
		chunkSize:        100,
		writesChan:       make(chan gRPCWriterCommand, 1),
		setSize:          func(int64) {},
		progress:         func(int64) {},
		setObj:           func(*ObjectAttrs) {},
	}

	// Attempt 1 arms the stopwatch and receives flushOffset == 0. That clears
	// awaitingFirstAck but is not strict progress past 0, so neither the
	// stopwatch nor attempts are reset.
	if err := attemptWriteLoop(ctx, w); !errors.Is(err, transientErr) {
		t.Fatalf("attempt 1: got %v, want %v", err, transientErr)
	}
	firstExpiry := w.budget.expiresAt
	if want := clk.Now().Add(deadline); !firstExpiry.Equal(want) {
		t.Fatalf("expected stopwatch armed at %v, got %v", want, firstExpiry)
	}
	if w.awaitingFirstAck {
		t.Fatalf("expected awaitingFirstAck to be cleared by the 0-offset completion")
	}
	if w.budget.attempts != 1 {
		t.Fatalf("expected budget.attempts == 1 after 0-offset completion, got %d", w.budget.attempts)
	}
	if w.budget.consumeProgress() {
		t.Fatalf("expected consumeProgress() == false after 0-offset completion")
	}

	clk.Advance(120 * time.Millisecond)

	// Attempt 2 also receives flushOffset == 0.
	if err := attemptWriteLoop(ctx, w); !errors.Is(err, transientErr) {
		t.Fatalf("attempt 2: got %v, want %v", err, transientErr)
	}
	if !w.budget.expiresAt.Equal(firstExpiry) {
		t.Fatalf("expected expiresAt to remain %v, got %v", firstExpiry, w.budget.expiresAt)
	}
	if w.budget.attempts != 2 {
		t.Fatalf("expected budget.attempts == 2, got %d", w.budget.attempts)
	}
	if w.budget.consumeProgress() {
		t.Fatalf("expected consumeProgress() == false on attempt 2 with 0-offset completion")
	}

	clk.Advance(100 * time.Millisecond)

	// Attempt 3 must hit the retry deadline.
	if err := attemptWriteLoop(ctx, w); !isChunkRetryDeadlineError(err) {
		t.Fatalf("attempt 3: got %v, want chunkRetryDeadlineError", err)
	}
}

func TestGRPCWriter_PerChunkMaxAttemptsResetOnStrictProgress(t *testing.T) {
	ctx := context.Background()
	sender := &reconnectStatusSender{
		persistedSize: 0,
		errResult:     io.ErrUnexpectedEOF,
	}

	w := &gRPCWriter{
		budget:           testBudget(newFakeClock(), 5*time.Second),
		streamSender:     sender,
		settings:         &settings{},
		bufBaseOffset:    0,
		bufUnsentIdx:     100,
		awaitingFirstAck: true,
		buf:              make([]byte, 100),
		sendableUnits:    2,
		writeQuantum:     50,
		chunkSize:        100,
		writesChan:       make(chan gRPCWriterCommand, 1),
		setSize:          func(int64) {},
		progress:         func(int64) {},
		setObj:           func(*ObjectAttrs) {},
	}

	retry := &retryConfig{
		policy:      RetryAlways,
		maxAttempts: intPointer(2),
		backoff:     &gax.Backoff{Initial: time.Millisecond},
	}

	callCount := 0
	err := run(ctx, func(ctx context.Context) error {
		callCount++
		if callCount == 2 {
			// Simulate server having persisted the first 50-byte quantum on attempt 2,
			// while the second 50-byte quantum still fails on attempt 2 and attempt 3.
			sender.mu.Lock()
			sender.persistedSize = 50
			sender.mu.Unlock()
		}
		return attemptWriteLoop(ctx, w)
	}, retry, true, withProgressReset(w.budget.consumeProgress))

	if err == nil || !strings.Contains(err.Error(), "retry failed after 2 attempts") {
		t.Fatalf("expected retry failed after 2 attempts error, got: %v", err)
	}
	// Call 1: offset 0 fails (attempt 1 of first quantum).
	// Call 2: offset advances 0 -> 50 (resets attempts to 1), then fails at offset 50 (attempt 1 of second quantum).
	// Call 3: offset stays 50 (no progress, attempts=2 >= maxAttempts=2), terminates.
	if callCount != 3 {
		t.Fatalf("expected 3 writeLoop invocations with per-chunk maxAttempts=2, got %d", callCount)
	}
	// Call 2 confirmed the first 50 bytes and the next attempt shifted them out
	// of w.buf. Call 3's completion at the same offset is not new progress.
	if w.bufBaseOffset != 50 {
		t.Errorf("bufBaseOffset = %d, want 50", w.bufBaseOffset)
	}
	if w.bufFlushedIdx != 0 {
		t.Errorf("bufFlushedIdx = %d, want 0", w.bufFlushedIdx)
	}
}

// TestGRPCWriter_ChunkRetryDeadlineError_IsTerminal checks that the writer's
// retry predicate never retries a chunkRetryDeadlineError, even though the
// transport error it wraps is retryable and regardless of any user ErrorFunc.
func TestGRPCWriter_ChunkRetryDeadlineError_IsTerminal(t *testing.T) {
	// The context deadline bounds the test if run() keeps retrying.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	clk := newFakeClock()
	deadline := 100 * time.Millisecond
	transientErr := status.Error(codes.Unavailable, "transient")
	sender := &instantFailSender{errResult: transientErr, canResume: true}
	retryEverything := func(error, *RetryContext) bool { return true }

	w := &gRPCWriter{
		budget:       testBudget(clk, deadline),
		streamSender: sender,
		settings: &settings{
			idempotent: true,
			retry: &retryConfig{
				policy:      RetryAlways,
				backoff:     &gax.Backoff{Initial: time.Millisecond, Max: time.Millisecond},
				shouldRetry: retryEverything,
			},
		},
		bufUnsentIdx:  100,
		bufFlushedIdx: 0,
		buf:           make([]byte, 100),
		sendableUnits: 1,
		writeQuantum:  100,
		chunkSize:     100,
		writesChan:    make(chan gRPCWriterCommand, 1),
	}

	calls := 0
	err := run(ctx, func(ctx context.Context) error {
		calls++
		if calls == 2 {
			// Expire the budget armed by call 1 before call 2 checks it.
			clk.Advance(deadline + time.Millisecond)
		}
		return attemptWriteLoop(ctx, w)
	}, w.writerRetryConfig(), w.settings.idempotent, withProgressReset(w.budget.consumeProgress))

	if calls != 2 {
		t.Fatalf("run() made %d calls, want 2 (deadline error must not be retried)", calls)
	}
	var deadlineErr *chunkRetryDeadlineError
	if !errors.As(err, &deadlineErr) {
		t.Fatalf("got %T %v, want *chunkRetryDeadlineError", err, err)
	}
	if !errors.Is(err, transientErr) {
		t.Errorf("errors.Is(err, transientErr) = false; the deadline error should wrap the last transport error")
	}
	if got := status.Code(err); got != codes.Unavailable {
		t.Errorf("status.Code(err) = %v, want %v (wrapped status must stay visible)", got, codes.Unavailable)
	}

	// The guard must hold for every writerRetryConfig shape, including the
	// append + RetryNever path that otherwise retries redirections.
	t.Run("shouldRetry refuses deadline errors under every policy", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			policy  RetryPolicy
			append  bool
			wrapped error
		}{
			{"RetryAlways wrapping Unavailable", RetryAlways, false, transientErr},
			{"RetryIdempotent wrapping Unavailable", RetryIdempotent, false, transientErr},
			{"RetryNever append wrapping redirection", RetryNever, true, bidiWriteObjectRedirectionError{}},
			{"RetryAlways append wrapping redirection", RetryAlways, true, bidiWriteObjectRedirectionError{}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				w := &gRPCWriter{
					append:       tc.append,
					streamSender: &instantFailSender{canResume: true},
					settings: &settings{
						idempotent: true,
						retry:      &retryConfig{policy: tc.policy, shouldRetry: retryEverything},
					},
				}
				cfg := w.writerRetryConfig()
				if cfg.shouldRetry == nil {
					t.Fatal("writerRetryConfig() returned no shouldRetry predicate")
				}
				err := &chunkRetryDeadlineError{deadline: deadline, attempts: 3, err: tc.wrapped}
				if cfg.shouldRetry(err, &RetryContext{}) {
					t.Errorf("shouldRetry(%v) = true, want false", err)
				}
				// Sanity check: the same predicate does retry the bare wrapped error.
				if !cfg.shouldRetry(tc.wrapped, &RetryContext{}) {
					t.Errorf("shouldRetry(%v) = false, want true; the guard should only affect deadline errors", tc.wrapped)
				}
			})
		}
	})

	// writerRetryConfig retries a bare stall even when the user's ErrorFunc
	// rejects it. A stall wrapped in a deadline error matches
	// errors.Is(err, errStallTimeout) too, and must still be terminal.
	t.Run("deadline error wrapping a stall is not force-retried", func(t *testing.T) {
		rejectAll := func(error, *RetryContext) bool { return false }
		stallErr := &stallTimeoutError{timeout: time.Millisecond, stage: "BidiWriteObject"}
		for _, tc := range []struct {
			name   string
			policy RetryPolicy
			append bool
		}{
			{"RetryAlways", RetryAlways, false},
			{"RetryIdempotent", RetryIdempotent, false},
			{"RetryAlways append", RetryAlways, true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				w := &gRPCWriter{
					append:       tc.append,
					streamSender: &instantFailSender{canResume: true},
					settings: &settings{
						idempotent: true,
						retry:      &retryConfig{policy: tc.policy, shouldRetry: rejectAll},
					},
				}
				cfg := w.writerRetryConfig()
				err := &chunkRetryDeadlineError{deadline: deadline, attempts: 3, err: stallErr}
				if cfg.shouldRetry(err, &RetryContext{}) {
					t.Errorf("shouldRetry(%v) = true, want false", err)
				}
				// Sanity check: the bare stall is still force-retried.
				if !cfg.shouldRetry(stallErr, &RetryContext{}) {
					t.Errorf("shouldRetry(%v) = false, want true", stallErr)
				}
			})
		}
	})
}

func TestChunkRetryBudget(t *testing.T) {
	clk := newFakeClock()
	b := testBudget(clk, 100*time.Millisecond)

	if b.expired() {
		t.Fatal("a stopped budget must not be expired")
	}
	b.start()
	armedAt := b.expiresAt
	if want := clk.Now().Add(100 * time.Millisecond); !armedAt.Equal(want) {
		t.Fatalf("start(): expiresAt = %v, want %v", armedAt, want)
	}

	// start is idempotent while running.
	clk.Advance(50 * time.Millisecond)
	b.start()
	if !b.expiresAt.Equal(armedAt) {
		t.Fatalf("start() on a running budget moved expiresAt from %v to %v", armedAt, b.expiresAt)
	}
	if b.expired() {
		t.Fatal("budget expired at 50ms of 100ms")
	}

	// Exactly at the deadline is not yet expired; one tick later is.
	clk.Advance(50 * time.Millisecond)
	if b.expired() {
		t.Fatal("budget expired exactly at the deadline; want strictly after")
	}
	clk.Advance(time.Nanosecond)
	if !b.expired() {
		t.Fatal("budget not expired after the deadline")
	}

	// stop clears the stopwatch; an expired-then-stopped budget is not expired.
	b.stop()
	if b.expired() || !b.expiresAt.IsZero() {
		t.Fatalf("stop(): expired=%v expiresAt=%v, want false/zero", b.expired(), b.expiresAt)
	}

	// recordProgress resets attempts, flags progress once, and restarts the
	// stopwatch only if the writer is still active.
	b.attempts = 7
	b.recordProgress(false)
	if b.attempts != 0 || !b.expiresAt.IsZero() {
		t.Fatalf("recordProgress(false): attempts=%d expiresAt=%v, want 0/zero", b.attempts, b.expiresAt)
	}
	if !b.consumeProgress() || b.consumeProgress() {
		t.Fatal("consumeProgress() should report true exactly once per recordProgress")
	}
	b.recordProgress(true)
	if want := clk.Now().Add(100 * time.Millisecond); !b.expiresAt.Equal(want) {
		t.Fatalf("recordProgress(true): expiresAt = %v, want %v", b.expiresAt, want)
	}

	// A zero deadline disables the budget entirely.
	disabled := testBudget(clk, 0)
	disabled.start()
	disabled.recordProgress(true)
	if !disabled.expiresAt.IsZero() || disabled.expired() {
		t.Fatalf("disabled budget: expiresAt=%v expired=%v, want zero/false", disabled.expiresAt, disabled.expired())
	}

	// A nil clock falls back to time.Now.
	wallClock := chunkRetryBudget{deadline: time.Hour}
	wallClock.start()
	if wallClock.expiresAt.IsZero() || wallClock.expired() {
		t.Fatalf("nil clock: expiresAt=%v expired=%v", wallClock.expiresAt, wallClock.expired())
	}
}

// TestGRPCWriter_AwaitingFirstAck checks that the first completion at offset
// 0 is processed but not counted as progress, and that the same offset is
// then dropped as a duplicate.
func TestGRPCWriter_AwaitingFirstAck(t *testing.T) {
	var progressCalls []int64
	w := &gRPCWriter{
		budget:           testBudget(newFakeClock(), time.Second),
		awaitingFirstAck: true,
		buf:              make([]byte, 0, 100),
		writeQuantum:     100,
		chunkSize:        100,
		setSize:          func(int64) {},
		progress:         func(n int64) { progressCalls = append(progressCalls, n) },
		setObj:           func(*ObjectAttrs) {},
	}

	w.handleCompletion(gRPCBidiWriteCompletion{flushOffset: 0})
	if w.awaitingFirstAck {
		t.Fatal("awaitingFirstAck still set after the first completion")
	}
	if len(progressCalls) != 1 || progressCalls[0] != 0 {
		t.Fatalf("progress calls after first 0-offset ack = %v, want [0]", progressCalls)
	}
	if w.budget.consumeProgress() {
		t.Fatal("a 0-offset first ack must not count as forward progress")
	}

	// The same offset again is a duplicate and must be dropped.
	w.handleCompletion(gRPCBidiWriteCompletion{flushOffset: 0})
	if len(progressCalls) != 1 {
		t.Fatalf("duplicate 0-offset ack was processed; progress calls = %v", progressCalls)
	}

	// Strict progress past the confirmed offset is still recorded.
	w.buf = w.buf[:50]
	w.bufUnsentIdx = 50
	w.handleCompletion(gRPCBidiWriteCompletion{flushOffset: 50})
	if !w.budget.consumeProgress() {
		t.Fatal("expected progress after the confirmed offset advanced 0 -> 50")
	}
	if w.bufBaseOffset != 50 || len(w.buf) != 0 {
		t.Fatalf("buffer not cleared after full ack: base=%d len=%d", w.bufBaseOffset, len(w.buf))
	}
}

// TestGRPCWriter_ZeroByteFlushWaitsForFirstAck checks that a Flush issued
// before any data has been acked blocks until the server's first completion.
func TestGRPCWriter_ZeroByteFlushWaitsForFirstAck(t *testing.T) {
	requests := make(chan gRPCBidiWriteRequest, 1)
	completions := make(chan gRPCBidiWriteCompletion, 1)
	cs := gRPCWriterCommandHandleChans{requests: requests, requestAcks: make(chan struct{}), completions: completions}
	w := &gRPCWriter{
		budget:           testBudget(newFakeClock(), time.Second),
		awaitingFirstAck: true,
		buf:              make([]byte, 0, 100),
		writeQuantum:     100,
		chunkSize:        100,
		setSize:          func(int64) {},
		progress:         func(int64) {},
		setObj:           func(*ObjectAttrs) {},
		streamSender:     &instantFailSender{errResult: errors.New("stream closed")},
	}
	flush := &gRPCWriterCommandFlush{done: make(chan int64, 1)}

	errCh := make(chan error, 1)
	go func() { errCh <- flush.handle(w, cs) }()

	req := <-requests
	if !req.flush || len(req.buf) != 0 || req.offset != 0 {
		t.Fatalf("flush request = %+v, want empty flush at offset 0", req)
	}
	select {
	case err := <-errCh:
		t.Fatalf("Flush returned %v before the server acked anything", err)
	case <-time.After(50 * time.Millisecond):
	}

	completions <- gRPCBidiWriteCompletion{flushOffset: 0}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Flush returned %v after the 0-offset ack", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Flush did not return after the 0-offset ack")
	}
	if got := <-flush.done; got != 0 {
		t.Fatalf("Flush reported offset %d, want 0", got)
	}
	if w.awaitingFirstAck {
		t.Fatal("awaitingFirstAck still set after the ack")
	}
}

// time.Timer.Stop cannot cancel a callback that has already started, so a
// callback blocked on the watchdog's mutex can run after the timer was
// re-armed. It must not trip a stall for the new arming. The race is not
// reproducible with real timers, so the test calls fire with a stale
// generation directly.
func TestWriteStallWatchdog_StaleCallbackIgnored(t *testing.T) {
	stalls := 0
	w := newWriteStallWatchdog(time.Hour, "", func(string) { stalls++ })
	defer w.stop()
	currentGen := func() uint64 {
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.gen
	}

	w.resume()
	stale := currentGen()
	w.reset()
	w.fire(stale)
	if w.isStalled() {
		t.Fatal("callback armed before reset() tripped a stall")
	}

	stale = currentGen()
	w.pause()
	w.resume()
	w.fire(stale)
	if w.isStalled() {
		t.Fatal("callback armed before pause() and resume() tripped a stall")
	}

	w.fire(currentGen())
	if !w.isStalled() || stalls != 1 {
		t.Fatalf("current callback: isStalled() = %v, stalls = %d; want true, 1", w.isStalled(), stalls)
	}
}

type hangingConnectSender struct {
	errResult error
}

func (s *hangingConnectSender) connect(ctx context.Context, cs gRPCBufSenderChans, opts ...gax.CallOption) {
	<-ctx.Done()
	s.errResult = ctx.Err()
	close(cs.completions)
}

func (s *hangingConnectSender) err() error {
	return s.errResult
}

func (s *hangingConnectSender) canResumeSession() bool { return false }

// hangingStartSender hangs in connect like a resumable sender whose
// StartResumableWrite call never returns.
type hangingStartSender struct {
	hangingConnectSender
}

func (s *hangingStartSender) connectStage() string { return "StartResumableWrite" }

func TestGRPCResumableBidiWriteBufferSender_ConnectStage(t *testing.T) {
	start := &gRPCResumableBidiWriteBufferSender{startWriteRequest: &storagepb.StartResumableWriteRequest{}}
	if got := start.connectStage(); got != "StartResumableWrite" {
		t.Errorf("new session: connectStage() = %q, want StartResumableWrite", got)
	}
	resume := &gRPCResumableBidiWriteBufferSender{upid: "upload-id"}
	if got := resume.connectStage(); got != "QueryWriteStatus" {
		t.Errorf("resumed session: connectStage() = %q, want QueryWriteStatus", got)
	}
}

type hangingDataSender struct {
	errResult error
}

func (s *hangingDataSender) connect(ctx context.Context, cs gRPCBufSenderChans, opts ...gax.CallOption) {
	go func() {
		defer close(cs.completions)
		for {
			select {
			case <-ctx.Done():
				s.errResult = ctx.Err()
				return
			case r, ok := <-cs.requests:
				if !ok {
					return
				}
				if r.requestAck {
					select {
					case cs.requestAcks <- struct{}{}:
					case <-ctx.Done():
						s.errResult = ctx.Err()
						return
					}
				}
				// Intentionally do not send completions to simulate a stall on data.
			}
		}
	}()
}

func (s *hangingDataSender) err() error {
	return s.errResult
}

func (s *hangingDataSender) canResumeSession() bool { return false }

func TestGRPCWriter_ChunkTransferTimeout_Stage1_Stall(t *testing.T) {
	ctx := context.Background()
	timeout := 50 * time.Millisecond
	sender := &hangingConnectSender{}

	w := &gRPCWriter{
		chunkTransferTimeout: timeout,
		streamSender:         sender,
		settings:             &settings{},
		writesChan:           make(chan gRPCWriterCommand, 1),
	}

	start := time.Now()
	err := w.writeLoop(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected stall error, got nil")
	}
	if !errors.Is(err, errStallTimeout) {
		t.Fatalf("expected errStallTimeout, got: %v", err)
	}
	if !strings.Contains(err.Error(), "BidiWriteObject exceeded 50ms") {
		t.Fatalf("unexpected error message: %v", err)
	}
	if elapsed < timeout {
		t.Fatalf("expected writeLoop to wait at least %v, but returned in %v", timeout, elapsed)
	}
}

func TestGRPCWriter_ChunkTransferTimeout_DataStall(t *testing.T) {
	ctx := context.Background()
	timeout := 50 * time.Millisecond
	sender := &hangingDataSender{}

	w := &gRPCWriter{
		chunkTransferTimeout: timeout,
		streamSender:         sender,
		settings:             &settings{},
		chunkSize:            100,
		writeQuantum:         100,
		buf:                  make([]byte, 100),
		bufBaseOffset:        0,
		bufUnsentIdx:         0,
		awaitingFirstAck:     true,
		sendableUnits:        1,
		writesChan:           make(chan gRPCWriterCommand, 1),
	}

	start := time.Now()
	err := w.writeLoop(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected stall error, got nil")
	}
	if !errors.Is(err, errStallTimeout) {
		t.Fatalf("expected errStallTimeout, got: %v", err)
	}
	if elapsed < timeout {
		t.Fatalf("expected writeLoop to wait at least %v, but returned in %v", timeout, elapsed)
	}
}

func TestGRPCWriter_ChunkTransferTimeout_Telemetry(t *testing.T) {
	ctx := context.Background()
	timeout := 40 * time.Millisecond
	sender := &hangingStartSender{}

	mr := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	defer provider.Shutdown(ctx)

	cfg := storageConfig{
		enableOtelMetrics:      true,
		enableOtelDebugMetrics: true,
		meterProvider:          provider,
	}

	cm, _, err := initMetrics(ctx, "project-id", &cfg)
	if err != nil {
		t.Fatalf("initMetrics: %v", err)
	}

	state := &metricsState{metrics: cm}
	target := "storage.googleapis.com"
	state.target.Store(&target)
	ctxWithMetrics := contextWithMetricsState(ctx, state)

	w := &gRPCWriter{
		preRunCtx:            ctxWithMetrics,
		c:                    &grpcStorageClient{metrics: cm},
		chunkTransferTimeout: timeout,
		streamSender:         sender,
		settings:             &settings{},
		writesChan:           make(chan gRPCWriterCommand, 1),
	}

	err = w.writeLoop(ctx)
	var stallErr *stallTimeoutError
	if !errors.As(err, &stallErr) {
		t.Fatalf("expected stallTimeoutError, got: %v", err)
	}
	if stallErr.stage != "StartResumableWrite" {
		t.Errorf("stallTimeoutError.stage = %q, want StartResumableWrite", stallErr.stage)
	}

	var rm metricdata.ResourceMetrics
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "gcp.storage.client.stall.duration" {
				found = true
				hist := m.Data.(metricdata.Histogram[float64])
				if len(hist.DataPoints) == 0 {
					t.Fatalf("expected data points, got 0")
				}
				dp := hist.DataPoints[0]
				if dp.Sum != timeout.Seconds() {
					t.Errorf("expected sum %v, got %v", timeout.Seconds(), dp.Sum)
				}
				if getHistAttr(dp, "rpc.method") != "StartResumableWrite" {
					t.Errorf("expected rpc.method StartResumableWrite, got %v", getHistAttr(dp, "rpc.method"))
				}
				if getHistAttr(dp, "rpc.system.name") != "grpc" {
					t.Errorf("expected rpc.system.name grpc, got %v", getHistAttr(dp, "rpc.system.name"))
				}
				if getHistAttr(dp, "server.address") != "storage.googleapis.com" {
					t.Errorf("expected server.address storage.googleapis.com, got %v", getHistAttr(dp, "server.address"))
				}
			}
		}
	}
	if !found {
		t.Errorf("metric gcp.storage.client.stall.duration not found")
	}
}

// slowAckSender acknowledges flushes in order, each ackDelay after the
// previous one, like a slow but live server.
type slowAckSender struct {
	ackDelay  time.Duration
	errResult error
}

func (s *slowAckSender) connect(ctx context.Context, cs gRPCBufSenderChans, opts ...gax.CallOption) {
	flushed := make(chan int64, 64)
	go func() {
		defer close(flushed)
		for {
			select {
			case <-ctx.Done():
				return
			case r, ok := <-cs.requests:
				if !ok {
					return
				}
				if r.requestAck {
					cs.requestAcks <- struct{}{}
				}
				if r.flush {
					flushed <- r.offset + int64(len(r.buf))
				}
			}
		}
	}()
	go func() {
		defer close(cs.completions)
		for off := range flushed {
			select {
			case <-time.After(s.ackDelay):
			case <-ctx.Done():
				s.errResult = ctx.Err()
				return
			}
			select {
			case cs.completions <- gRPCBidiWriteCompletion{flushOffset: off}:
			case <-ctx.Done():
				s.errResult = ctx.Err()
				return
			}
		}
	}()
}

func (s *slowAckSender) err() error             { return s.errResult }
func (s *slowAckSender) canResumeSession() bool { return false }

// A Write spanning several chunks keeps the watchdog armed until its last
// flush is acknowledged. The acks together take longer than the timeout, so
// the upload succeeds only if each ack restarts the timer.
func TestGRPCWriter_ChunkTransferTimeout_ProgressAdvancesTimer(t *testing.T) {
	ctx := context.Background()
	timeout := 100 * time.Millisecond
	const chunkSize, chunks = 50, 5
	sender := &slowAckSender{ackDelay: 40 * time.Millisecond}
	allAcked := make(chan struct{})

	w := &gRPCWriter{
		chunkTransferTimeout: timeout,
		streamSender:         sender,
		settings:             &settings{},
		chunkSize:            chunkSize,
		writeQuantum:         chunkSize,
		awaitingFirstAck:     true,
		sendableUnits:        2,
		writesChan:           make(chan gRPCWriterCommand, 1),
		setSize:              func(int64) {},
		progress: func(n int64) {
			if n == chunkSize*chunks {
				close(allAcked)
			}
		},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- w.writeLoop(ctx)
	}()

	start := time.Now()
	w.writesChan <- &gRPCWriterCommandWrite{p: make([]byte, chunkSize*chunks), done: make(chan struct{})}
	select {
	case <-allAcked:
	case err := <-errCh:
		t.Fatalf("writeLoop returned %v before every chunk was acknowledged", err)
	}
	if elapsed := time.Since(start); elapsed <= timeout {
		t.Fatalf("acks took %v; they must span more than the %v timeout", elapsed, timeout)
	}

	w.writesChan <- &gRPCWriterCommandClose{}
	if err := <-errCh; err != nil {
		t.Fatalf("expected nil error on progress, got: %v", err)
	}
}

func TestGRPCWriter_ChunkTransferTimeout_PausedOnIdle(t *testing.T) {
	ctx := context.Background()
	timeout := 50 * time.Millisecond
	sender := &mockSender{respondToAllData: true}
	acked := make(chan struct{})

	w := &gRPCWriter{
		chunkTransferTimeout: timeout,
		streamSender:         sender,
		settings:             &settings{},
		chunkSize:            100,
		writeQuantum:         50,
		buf:                  nil,
		bufBaseOffset:        0,
		bufUnsentIdx:         0,
		awaitingFirstAck:     true,
		sendableUnits:        2,
		writesChan:           make(chan gRPCWriterCommand, 1),
		setSize:              func(int64) {},
		progress: func(n int64) {
			if n == 50 {
				close(acked)
			}
		},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- w.writeLoop(ctx)
	}()

	// 1. Write 50 bytes and wait for it to be acked.
	done := make(chan struct{})
	w.writesChan <- &gRPCWriterCommandWrite{p: make([]byte, 50), done: done}
	<-done
	<-acked

	// Writer is now idle waiting for caller's next Write().
	// Sleep 80ms (> 50ms timeout). Watchdog must be paused so no stall occurs.
	time.Sleep(80 * time.Millisecond)

	// 2. Write another 50 bytes. Watchdog resumes and acks.
	done2 := make(chan struct{})
	w.writesChan <- &gRPCWriterCommandWrite{p: make([]byte, 50), done: done2}
	<-done2

	w.writesChan <- &gRPCWriterCommandClose{err: nil}
	err := <-errCh
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
}

// flushOnlyAckSender acknowledges persisted bytes only in response to flush
// requests, as GCS does.
type flushOnlyAckSender struct {
	mu        sync.Mutex
	errResult error
}

func (s *flushOnlyAckSender) connect(ctx context.Context, cs gRPCBufSenderChans, opts ...gax.CallOption) {
	s.mu.Lock()
	s.errResult = nil
	s.mu.Unlock()
	go func() {
		defer close(cs.completions)
		setErr := func() {
			s.mu.Lock()
			s.errResult = ctx.Err()
			s.mu.Unlock()
		}
		for {
			select {
			case <-ctx.Done():
				setErr()
				return
			case r, ok := <-cs.requests:
				if !ok {
					return
				}
				if r.requestAck {
					select {
					case cs.requestAcks <- struct{}{}:
					case <-ctx.Done():
						setErr()
						return
					}
					continue
				}
				if !r.flush {
					continue
				}
				select {
				case cs.completions <- gRPCBidiWriteCompletion{flushOffset: r.offset + int64(len(r.buf))}:
				case <-ctx.Done():
					setErr()
					return
				}
			}
		}
	}()
}

func (s *flushOnlyAckSender) err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.errResult
}

func (s *flushOnlyAckSender) canResumeSession() bool { return false }

// unresponsiveSender accepts requests but never acknowledges data, and keeps
// the stream open until its context is cancelled.
type unresponsiveSender struct {
	mu        sync.Mutex
	errResult error
}

func (s *unresponsiveSender) connect(ctx context.Context, cs gRPCBufSenderChans, opts ...gax.CallOption) {
	go func() {
		defer close(cs.completions)
		setErr := func() {
			s.mu.Lock()
			s.errResult = ctx.Err()
			s.mu.Unlock()
		}
		requests := cs.requests
		for {
			select {
			case <-ctx.Done():
				setErr()
				return
			case r, ok := <-requests:
				if !ok {
					// Half-closed by the client; keep the stream open.
					requests = nil
					continue
				}
				if r.requestAck {
					select {
					case cs.requestAcks <- struct{}{}:
					case <-ctx.Done():
						setErr()
						return
					}
				}
			}
		}
	}()
}

func (s *unresponsiveSender) err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.errResult
}

func (s *unresponsiveSender) canResumeSession() bool { return false }

// A non-flush quantum is never acknowledged by GCS. If the application is slow
// to fill the rest of the chunk, nothing is outstanding on the server and the
// watchdog must not fire.
func TestGRPCWriter_ChunkTransferTimeout_SlowProducerMidChunk(t *testing.T) {
	for _, appendable := range []bool{false, true} {
		t.Run(fmt.Sprintf("append=%v", appendable), func(t *testing.T) {
			timeout := 50 * time.Millisecond
			w := &gRPCWriter{
				chunkTransferTimeout: timeout,
				streamSender:         &flushOnlyAckSender{},
				settings:             &settings{},
				append:               appendable,
				chunkSize:            100,
				writeQuantum:         50,
				sendableUnits:        2,
				awaitingFirstAck:     true,
				writesChan:           make(chan gRPCWriterCommand, 1),
				setSize:              func(int64) {},
				progress:             func(int64) {},
				setObj:               func(*ObjectAttrs) {},
			}
			errCh := make(chan error, 1)
			go func() { errCh <- w.writeLoop(context.Background()) }()

			// Sends one non-flush quantum.
			done := make(chan struct{})
			w.writesChan <- &gRPCWriterCommandWrite{p: make([]byte, 50), done: done}
			<-done
			time.Sleep(4 * timeout)

			// Completes the chunk, which sends a flush.
			done2 := make(chan struct{})
			w.writesChan <- &gRPCWriterCommandWrite{p: make([]byte, 50), done: done2}
			select {
			case <-done2:
			case err := <-errCh:
				t.Fatalf("writeLoop exited while waiting on the application: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("write did not complete")
			}

			w.writesChan <- &gRPCWriterCommandClose{}
			if err := <-errCh; err != nil {
				t.Fatalf("writeLoop: %v", err)
			}
		})
	}
}

func TestGRPCWriter_ChunkTransferTimeout_FinalRequest(t *testing.T) {
	timeout := 30 * time.Millisecond
	tests := []struct {
		name             string
		forceOneShot     bool
		append           bool
		bufLen           int
		awaitingFirstAck bool
		wantStall        bool
	}{
		{
			name:             "Oneshot_NotArmed",
			forceOneShot:     true,
			bufLen:           10,
			awaitingFirstAck: true,
			wantStall:        false,
		},
		{
			name:             "Resumable_Armed",
			bufLen:           10,
			awaitingFirstAck: true,
			wantStall:        true,
		},
		{
			name:             "Resumable_NoNewBytes_Armed",
			bufLen:           0,
			awaitingFirstAck: false,
			wantStall:        true,
		},
		{
			// gatherFirstBuffer sets forceOneShot for appendable objects closed
			// before the buffer fills, but they still use the appendable sender.
			name:             "AppendableClosedBeforeChunkFills_Armed",
			forceOneShot:     true,
			append:           true,
			bufLen:           10,
			awaitingFirstAck: true,
			wantStall:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*timeout)
			defer cancel()
			w := &gRPCWriter{
				chunkTransferTimeout: timeout,
				streamSender:         &unresponsiveSender{},
				settings:             &settings{},
				forceOneShot:         tt.forceOneShot,
				append:               tt.append,
				chunkSize:            100,
				writeQuantum:         50,
				sendableUnits:        2,
				buf:                  make([]byte, tt.bufLen, 100),
				awaitingFirstAck:     tt.awaitingFirstAck,
				writesChan:           make(chan gRPCWriterCommand, 1),
				currentCommand:       &gRPCWriterCommandClose{},
				setSize:              func(int64) {},
				progress:             func(int64) {},
				setObj:               func(*ObjectAttrs) {},
			}

			err := w.writeLoop(ctx)
			if got := errors.Is(err, errStallTimeout); got != tt.wantStall {
				t.Fatalf("writeLoop() = %v, want stall: %v", err, tt.wantStall)
			}
			if !tt.wantStall && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("writeLoop() = %v, want context.DeadlineExceeded", err)
			}
		})
	}
}

// delayedCloseSender acks every request like mockSender but delays closing the
// completions channel, modelling a slow stream teardown after the final ack.
type delayedCloseSender struct {
	mockSender
	closeDelay time.Duration
	// cancelled reports whether the attempt context was cancelled before the
	// completions channel was closed.
	cancelled bool
}

func (s *delayedCloseSender) connect(ctx context.Context, cs gRPCBufSenderChans, opts ...gax.CallOption) {
	inner := make(chan gRPCBidiWriteCompletion, cap(cs.completions))
	s.mockSender.connect(ctx, gRPCBufSenderChans{cs.requests, cs.requestAcks, inner}, opts...)
	go func() {
		for c := range inner {
			cs.completions <- c
		}
		time.Sleep(s.closeDelay) // teardown slower than the watchdog
		s.cancelled = ctx.Err() != nil
		close(cs.completions)
	}()
}

func TestGRPCWriter_ChunkTransferTimeout_SlowTeardownIsNotAStall(t *testing.T) {
	ctx := context.Background()
	timeout := 50 * time.Millisecond
	sender := &delayedCloseSender{
		mockSender: mockSender{respondToAllData: true},
		closeDelay: 4 * timeout,
	}

	// Appendable and not finalized on close, so the final ack carries no
	// resource.
	w := &gRPCWriter{
		append:               true,
		finalizeOnClose:      false,
		chunkTransferTimeout: timeout,
		streamSender:         sender,
		settings:             &settings{},
		chunkSize:            100,
		writeQuantum:         50,
		awaitingFirstAck:     true,
		sendableUnits:        2,
		writesChan:           make(chan gRPCWriterCommand, 1),
		setSize:              func(int64) {},
		progress:             func(int64) {},
		setObj:               func(*ObjectAttrs) {},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- w.writeLoop(ctx)
	}()

	done := make(chan struct{})
	w.writesChan <- &gRPCWriterCommandWrite{p: make([]byte, 50), done: done}
	<-done
	w.writesChan <- &gRPCWriterCommandClose{}
	if err := <-errCh; err != nil {
		t.Fatalf("clean close with all bytes acked returned %v; want nil", err)
	}
	if sender.cancelled {
		t.Error("watchdog cancelled the attempt during stream teardown")
	}
}

func TestGRPCWriter_ChunkTransferTimeout_RetryPolicy(t *testing.T) {
	stallErr := &stallTimeoutError{timeout: time.Millisecond, stage: "BidiWriteObject"}
	transientErr := status.Error(codes.Unavailable, "transient unavailable")
	rejectAll := func(error, *RetryContext) bool { return false }

	tests := []struct {
		name         string
		policy       RetryPolicy
		shouldRetry  func(error, *RetryContext) bool
		idempotent   bool
		canResume    bool
		firstErr     error
		wantAttempts int
	}{
		{
			name:         "CustomErrorFuncRejectsStall_Retried",
			policy:       RetryAlways,
			shouldRetry:  rejectAll,
			canResume:    true,
			firstErr:     stallErr,
			wantAttempts: 2,
		},
		{
			name:         "CustomErrorFuncRejectsOtherError_NotRetried",
			policy:       RetryAlways,
			shouldRetry:  rejectAll,
			canResume:    true,
			firstErr:     transientErr,
			wantAttempts: 1,
		},
		{
			name:         "RetryIdempotent_WithPreconditions_CustomErrorFunc_Retried",
			policy:       RetryIdempotent,
			shouldRetry:  rejectAll,
			idempotent:   true,
			firstErr:     stallErr,
			wantAttempts: 2,
		},
		{
			name:         "RetryIdempotent_NoPreconditions_BeforeSession_NotRetried",
			policy:       RetryIdempotent,
			firstErr:     stallErr,
			wantAttempts: 1,
		},
		{
			name:         "RetryIdempotent_NoPreconditions_AfterSession_Retried",
			policy:       RetryIdempotent,
			canResume:    true,
			firstErr:     stallErr,
			wantAttempts: 2,
		},
		{
			name:         "RetryNever_NotRetried",
			policy:       RetryNever,
			canResume:    true,
			firstErr:     stallErr,
			wantAttempts: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &gRPCWriter{
				streamSender: &instantFailSender{canResume: tt.canResume},
				settings: &settings{
					idempotent: tt.idempotent,
					retry: &retryConfig{
						policy:      tt.policy,
						shouldRetry: tt.shouldRetry,
						backoff: &gax.Backoff{
							Initial:    time.Millisecond,
							Max:        5 * time.Millisecond,
							Multiplier: 1.1,
						},
					},
				},
			}

			calls := 0
			err := run(context.Background(), func(ctx context.Context) error {
				calls++
				if calls == 1 {
					return tt.firstErr
				}
				return nil
			}, w.writerRetryConfig(), w.settings.idempotent, withOperation("WriteObject"))

			if wantErr := tt.wantAttempts == 1; (err != nil) != wantErr {
				t.Fatalf("run() error = %v, wantErr %v", err, wantErr)
			}
			if calls != tt.wantAttempts {
				t.Fatalf("got %d attempts, want %d", calls, tt.wantAttempts)
			}
		})
	}
}

type scriptedStallSender struct {
	mu           sync.Mutex
	attempts     int
	stallAttempt map[int]bool
	sendDelay    time.Duration
	errResult    error
}

func (s *scriptedStallSender) connect(ctx context.Context, cs gRPCBufSenderChans, opts ...gax.CallOption) {
	s.mu.Lock()
	s.attempts++
	attempt := s.attempts
	shouldStall := s.stallAttempt[attempt]
	delay := s.sendDelay
	s.errResult = nil
	s.mu.Unlock()

	go func() {
		defer close(cs.completions)
		for {
			select {
			case <-ctx.Done():
				s.mu.Lock()
				s.errResult = ctx.Err()
				s.mu.Unlock()
				return
			case r, ok := <-cs.requests:
				if !ok {
					return
				}
				if r.requestAck {
					select {
					case cs.requestAcks <- struct{}{}:
					case <-ctx.Done():
						s.mu.Lock()
						s.errResult = ctx.Err()
						s.mu.Unlock()
						return
					}
					continue
				}
				if shouldStall {
					<-ctx.Done()
					s.mu.Lock()
					s.errResult = ctx.Err()
					s.mu.Unlock()
					return
				}
				if delay > 0 {
					select {
					case <-time.After(delay):
					case <-ctx.Done():
						s.mu.Lock()
						s.errResult = ctx.Err()
						s.mu.Unlock()
						return
					}
				}
				select {
				case cs.completions <- gRPCBidiWriteCompletion{
					flushOffset: r.offset + int64(len(r.buf)),
				}:
				case <-ctx.Done():
					s.mu.Lock()
					s.errResult = ctx.Err()
					s.mu.Unlock()
					return
				}
			}
		}
	}()
}

func (s *scriptedStallSender) err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.errResult
}

func (s *scriptedStallSender) canResumeSession() bool { return false }

type funcSender struct {
	mu        sync.Mutex
	connectFn func(ctx context.Context, cs gRPCBufSenderChans) error
	errResult error
}

func (f *funcSender) connect(ctx context.Context, cs gRPCBufSenderChans, opts ...gax.CallOption) {
	go func() {
		defer close(cs.completions)
		err := f.connectFn(ctx, cs)
		f.mu.Lock()
		f.errResult = err
		f.mu.Unlock()
	}()
}

func (f *funcSender) err() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.errResult
}

func (f *funcSender) canResumeSession() bool { return false }

func TestGRPCWriter_TwoLayerStallAndRetryDeadline(t *testing.T) {
	t.Run("StallDetectedAndRecovered", func(t *testing.T) {
		ctx := context.Background()
		sender := &scriptedStallSender{
			stallAttempt: map[int]bool{1: true},
		}
		w := &gRPCWriter{
			preRunCtx:            ctx,
			chunkTransferTimeout: 40 * time.Millisecond,
			budget:               chunkRetryBudget{deadline: 500 * time.Millisecond},
			streamSender:         sender,
			settings: &settings{
				retry:      &retryConfig{backoff: &gax.Backoff{Initial: 5 * time.Millisecond}},
				idempotent: true,
			},
			chunkSize:        100,
			writeQuantum:     100,
			buf:              make([]byte, 100),
			bufBaseOffset:    0,
			bufUnsentIdx:     0,
			awaitingFirstAck: true,
			sendableUnits:    1,
			writesChan:       make(chan gRPCWriterCommand, 2),
			setSize:          func(int64) {},
			progress:         func(int64) {},
			setObj:           func(*ObjectAttrs) {},
		}
		w.writesChan <- &gRPCWriterCommandClose{err: nil}

		err := run(w.preRunCtx, func(ctx context.Context) error {
			w.lastErr = w.writeLoop(ctx)
			return w.lastErr
		}, w.writerRetryConfig(), w.settings.idempotent, withOperation("WriteObject"))
		if err != nil {
			t.Fatalf("expected stall recovery to succeed, got: %v", err)
		}
		sender.mu.Lock()
		gotAttempts := sender.attempts
		sender.mu.Unlock()
		if gotAttempts != 2 {
			t.Fatalf("expected 2 attempts (1 stall + 1 recovery), got %d", gotAttempts)
		}
	})

	t.Run("RepeatedStallsExhaustRetryDeadline", func(t *testing.T) {
		ctx := context.Background()
		sender := &scriptedStallSender{
			stallAttempt: map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true},
		}
		w := &gRPCWriter{
			preRunCtx:            ctx,
			chunkTransferTimeout: 40 * time.Millisecond,
			budget:               chunkRetryBudget{deadline: 110 * time.Millisecond},
			streamSender:         sender,
			settings: &settings{
				retry:      &retryConfig{backoff: &gax.Backoff{Initial: 5 * time.Millisecond}},
				idempotent: true,
			},
			chunkSize:        100,
			writeQuantum:     100,
			buf:              make([]byte, 100),
			bufBaseOffset:    0,
			bufUnsentIdx:     0,
			awaitingFirstAck: true,
			sendableUnits:    1,
			writesChan:       make(chan gRPCWriterCommand, 2),
			setSize:          func(int64) {},
			progress:         func(int64) {},
			setObj:           func(*ObjectAttrs) {},
		}

		err := run(w.preRunCtx, func(ctx context.Context) error {
			w.lastErr = w.writeLoop(ctx)
			return w.lastErr
		}, w.writerRetryConfig(), w.settings.idempotent, withOperation("WriteObject"))
		if err == nil {
			t.Fatal("expected retry deadline error, got nil")
		}
		if !strings.Contains(err.Error(), "retry deadline") {
			t.Fatalf("expected retry deadline error, got: %v", err)
		}
		if !strings.Contains(err.Error(), errStallTimeout.Error()) {
			t.Fatalf("expected retry deadline error to mention last stall error %q, got: %v", errStallTimeout.Error(), err)
		}
	})

	t.Run("ProgressResetsBothTimers", func(t *testing.T) {
		ctx := context.Background()
		// ChunkRetryDeadline is 120ms, ChunkTransferTimeout is 50ms.
		// Attempt 1 (Chunk 1): stalls (50ms) -> fails with errStallTimeout.
		// Attempt 2 (Chunk 1): succeeds and ACKs offset 100 -> resets ChunkRetryDeadline!
		// Attempt 2 then receives Chunk 2 (offset 100..200), which we stall on Attempt 2 by failing after offset 100.
		// Instead, we can drive two writes sequentially where Attempt 1 stalls (50ms), Attempt 2 succeeds on Chunk 1 (100 bytes),
		// and then stalls on Chunk 2 (another 50ms, total 100ms + backoff > 120ms if not reset), and Attempt 3 succeeds on Chunk 2.
		var mu sync.Mutex
		attempt := 0
		chunk1Acked := make(chan struct{})
		var chunk1Once sync.Once

		customSender := &funcSender{
			connectFn: func(ctx context.Context, cs gRPCBufSenderChans) error {
				mu.Lock()
				attempt++
				curAttempt := attempt
				mu.Unlock()

				for {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case r, ok := <-cs.requests:
						if !ok {
							return nil
						}
						if r.requestAck {
							select {
							case cs.requestAcks <- struct{}{}:
							case <-ctx.Done():
								return ctx.Err()
							}
							continue
						}
						endOffset := r.offset + int64(len(r.buf))
						// Attempt 1 stalls on Chunk 1 (endOffset == 100).
						if curAttempt == 1 && endOffset == 100 {
							<-ctx.Done()
							return ctx.Err()
						}
						// Attempt 2 ACKs Chunk 1 (endOffset == 100), then stalls on Chunk 2 (endOffset == 200).
						if curAttempt == 2 && endOffset == 200 {
							<-ctx.Done()
							return ctx.Err()
						}
						select {
						case cs.completions <- gRPCBidiWriteCompletion{flushOffset: endOffset}:
							if endOffset == 100 {
								chunk1Once.Do(func() { close(chunk1Acked) })
							}
						case <-ctx.Done():
							return ctx.Err()
						}
					}
				}
			},
		}

		w := &gRPCWriter{
			preRunCtx:            ctx,
			chunkTransferTimeout: 50 * time.Millisecond,
			budget:               chunkRetryBudget{deadline: 90 * time.Millisecond},
			streamSender:         customSender,
			settings: &settings{
				retry:      &retryConfig{backoff: &gax.Backoff{Initial: 5 * time.Millisecond}},
				idempotent: true,
			},
			chunkSize:        100,
			writeQuantum:     100,
			buf:              make([]byte, 0, 100),
			bufBaseOffset:    0,
			bufUnsentIdx:     0,
			awaitingFirstAck: true,
			sendableUnits:    2,
			writesChan:       make(chan gRPCWriterCommand, 4),
			setSize:          func(int64) {},
			progress:         func(int64) {},
			setObj:           func(*ObjectAttrs) {},
		}

		go func() {
			done1 := make(chan struct{})
			w.writesChan <- &gRPCWriterCommandWrite{p: make([]byte, 100), done: done1}
			<-done1
			<-chunk1Acked

			done2 := make(chan struct{})
			w.writesChan <- &gRPCWriterCommandWrite{p: make([]byte, 100), done: done2}
			<-done2

			w.writesChan <- &gRPCWriterCommandClose{err: nil}
		}()

		err := run(w.preRunCtx, func(ctx context.Context) error {
			w.lastErr = w.writeLoop(ctx)
			return w.lastErr
		}, w.writerRetryConfig(), w.settings.idempotent, withOperation("WriteObject"))
		if err != nil {
			t.Fatalf("expected both chunks to succeed after progress reset timers, got: %v", err)
		}
		mu.Lock()
		gotAttempts := attempt
		mu.Unlock()
		if gotAttempts != 3 {
			t.Fatalf("expected 3 attempts across 2 chunks, got %d", gotAttempts)
		}
	})

	t.Run("StallTimeoutLongerThanRetryDeadline", func(t *testing.T) {
		ctx := context.Background()
		sender := &scriptedStallSender{
			stallAttempt: map[int]bool{1: true, 2: true},
		}
		w := &gRPCWriter{
			preRunCtx:            ctx,
			chunkTransferTimeout: 80 * time.Millisecond,
			budget:               chunkRetryBudget{deadline: 30 * time.Millisecond},
			streamSender:         sender,
			settings: &settings{
				retry:      &retryConfig{backoff: &gax.Backoff{Initial: 5 * time.Millisecond}},
				idempotent: true,
			},
			chunkSize:        100,
			writeQuantum:     100,
			buf:              make([]byte, 100),
			bufBaseOffset:    0,
			bufUnsentIdx:     0,
			awaitingFirstAck: true,
			sendableUnits:    1,
			writesChan:       make(chan gRPCWriterCommand, 2),
			setSize:          func(int64) {},
			progress:         func(int64) {},
			setObj:           func(*ObjectAttrs) {},
		}

		err := run(w.preRunCtx, func(ctx context.Context) error {
			w.lastErr = w.writeLoop(ctx)
			return w.lastErr
		}, w.writerRetryConfig(), w.settings.idempotent, withOperation("WriteObject"))
		if err == nil || !strings.Contains(err.Error(), "retry deadline") {
			t.Fatalf("expected retry deadline error on attempt 2 after stall, got: %v", err)
		}
		sender.mu.Lock()
		gotAttempts := sender.attempts
		sender.mu.Unlock()
		if gotAttempts != 1 {
			t.Fatalf("expected only 1 stream connect before retry deadline aborted attempt 2, got %d", gotAttempts)
		}
	})

	t.Run("ZeroTimeoutDisablesStallDetection", func(t *testing.T) {
		ctx := context.Background()
		sender := &scriptedStallSender{
			sendDelay: 60 * time.Millisecond,
		}
		w := &gRPCWriter{
			preRunCtx:            ctx,
			chunkTransferTimeout: 0, // Disabled
			budget:               chunkRetryBudget{deadline: 500 * time.Millisecond},
			streamSender:         sender,
			settings:             &settings{idempotent: true},
			chunkSize:            100,
			writeQuantum:         100,
			buf:                  make([]byte, 100),
			bufBaseOffset:        0,
			bufUnsentIdx:         0,
			awaitingFirstAck:     true,
			sendableUnits:        1,
			writesChan:           make(chan gRPCWriterCommand, 2),
			setSize:              func(int64) {},
			progress:             func(int64) {},
			setObj:               func(*ObjectAttrs) {},
		}
		w.writesChan <- &gRPCWriterCommandClose{err: nil}

		if err := w.writeLoop(ctx); err != nil {
			t.Fatalf("expected success when chunkTransferTimeout is 0, got: %v", err)
		}
	})

	t.Run("UserContextCancelPrecedence", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		sender := &hangingConnectSender{}
		w := &gRPCWriter{
			preRunCtx:            ctx,
			chunkTransferTimeout: 500 * time.Millisecond,
			streamSender:         sender,
			settings:             &settings{},
			writesChan:           make(chan gRPCWriterCommand, 1),
		}

		go func() {
			time.Sleep(30 * time.Millisecond)
			cancel()
		}()

		err := w.writeLoop(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled when user cancels parent context, got: %v", err)
		}
		if errors.Is(err, errStallTimeout) {
			t.Fatalf("expected user cancellation not to be masked as errStallTimeout, got: %v", err)
		}
	})
}
