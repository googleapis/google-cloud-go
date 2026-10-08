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
	"io"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	"github.com/googleapis/gax-go/v2/callctx"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// stallEmuStall is the server-side stall injected by the testbench. It must
	// match the stall-for-Ns instructions below. Each stall holds a testbench
	// gRPC worker thread (8 in total) for this long, so keep it short.
	stallEmuStall = 2 * time.Second
	// stallEmuRecoverBudget bounds cases that must recover from a stall. It is
	// shorter than stallEmuStall, so success proves the stall was detected and
	// retried rather than waited out. It also guarantees the upload finalizes
	// before the testbench's stalled handler wakes up and appends its stale
	// chunk to the upload.
	stallEmuRecoverBudget = 1800 * time.Millisecond
	// stallEmuHangBudget bounds cases that must not detect the stall.
	stallEmuHangBudget = 1 * time.Second

	stallEmuTimeout    = 250 * time.Millisecond
	stallEmuChunkSize  = 2 * 1024 * 1024
	stallEmuObjectSize = 5 * 1024 * 1024
)

// stallAfter returns a testbench instruction that stalls the upload for
// stallEmuStall once kib KiB of the object have been received.
func stallAfter(kib int) string {
	return fmt.Sprintf("stall-for-%ds-after-%dK", int(stallEmuStall/time.Second), kib)
}

func isDeadlineErr(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || status.Code(err) == codes.DeadlineExceeded
}

// TestGRPCWriterStallEmulated verifies gRPC write stall detection end to end
// against the testbench. Each case asserts the outcome, the exact number of
// BidiWriteObject streams opened (1 + retries), that the injected stall was
// consumed, and the object contents on success.
func TestGRPCWriterStallEmulated(t *testing.T) {
	checkEmulatorEnvironment(t)

	rng := rand.New(rand.NewSource(1))
	data := make([]byte, 8*1024*1024)
	rng.Read(data)

	setupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	setup, err := NewGRPCClient(setupCtx)
	if err != nil {
		t.Fatalf("NewGRPCClient: %v", err)
	}
	defer setup.Close()
	bucket := fmt.Sprintf("grpc-stall-bucket-%d", time.Now().UnixNano())
	if err := setup.Bucket(bucket).Create(setupCtx, "project", nil); err != nil {
		t.Fatalf("creating bucket: %v", err)
	}

	writeAll := func(n int) func(*Writer) error {
		return func(w *Writer) error {
			_, err := w.Write(data[:n])
			return err
		}
	}
	// flushTo flushes w and checks that GCS reports want bytes persisted.
	flushTo := func(w *Writer, want int64) error {
		got, err := w.Flush()
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("Flush() = %d, want %d", got, want)
		}
		return nil
	}

	tests := []struct {
		name         string
		instructions []string
		budget       time.Duration
		configure    func(*Writer)
		conds        *Conditions
		retry        []RetryOption
		write        func(*Writer) error
		size         int
		// wantErr is nil when the upload must succeed.
		wantErr func(error) bool
		// wantStreams is the exact number of BidiWriteObject streams, if set.
		wantStreams int32
		// minStreams is a lower bound on BidiWriteObject streams, for cases
		// where the retry count depends on timing.
		minStreams int32
		// skipConsumedCheck skips asserting that all instructions were used.
		skipConsumedCheck bool
	}{
		{
			name:         "StallInFirstChunk_Recovers",
			instructions: []string{stallAfter(1024)},
			budget:       stallEmuRecoverBudget,
			wantStreams:  2,
		},
		{
			name:         "StallInSecondChunk_Recovers",
			instructions: []string{stallAfter(3072)},
			budget:       stallEmuRecoverBudget,
			wantStreams:  2,
		},
		{
			// With a 4 MiB ChunkSize (two 2 MiB BidiWriteObjectRequest messages per
			// chunk), stalling at 3072 KiB means the first 2 MiB message is already
			// persisted on the server without a flush ack when the second message
			// stalls. On atomic retry (without QueryWriteStatus), the client replays
			// from offset 0 and the server ignores the already-persisted 2 MiB prefix.
			name:         "StallMidMultiMessageChunk_Recovers",
			instructions: []string{stallAfter(3072)},
			budget:       stallEmuRecoverBudget,
			configure: func(w *Writer) {
				w.ChunkSize = 2 * stallEmuChunkSize
			},
			wantStreams: 2,
		},
		{
			// The 1 MiB tail is sent by Close as the final request.
			name:         "StallInFinalRequest_Recovers",
			instructions: []string{stallAfter(4608)},
			budget:       stallEmuRecoverBudget,
			wantStreams:  2,
		},
		{
			name:         "ConsecutiveStalls_Recover",
			instructions: []string{stallAfter(1024), stallAfter(1024)},
			budget:       stallEmuRecoverBudget,
			wantStreams:  3,
		},
		{
			name:         "CustomErrorFuncRejectingStall_Recovers",
			instructions: []string{stallAfter(1024)},
			budget:       stallEmuRecoverBudget,
			retry:        []RetryOption{WithErrorFunc(func(error) bool { return false })},
			wantStreams:  2,
		},
		{
			// The stall hits the first message, before the server returns a
			// WriteHandle. Without a handle or precondition, recreating the
			// object is not idempotent, so the session gate must not retry.
			name:         "AppendableStallBeforeHandle_NotRetried",
			instructions: []string{stallAfter(1024)},
			budget:       stallEmuRecoverBudget,
			configure: func(w *Writer) {
				w.Append = true
				w.FinalizeOnClose = true
			},
			wantErr:     func(err error) bool { return errors.Is(err, errStallTimeout) },
			wantStreams: 1,
		},
		{
			// With DoesNotExist the create is idempotent, so the stall before
			// the WriteHandle is retried. The first attempt's message already
			// created the object, so the retry fails the precondition rather
			// than writing a second object.
			name:         "AppendableStallBeforeHandle_DoesNotExist_RetryFailsPrecondition",
			instructions: []string{stallAfter(1024)},
			budget:       stallEmuRecoverBudget,
			conds:        &Conditions{DoesNotExist: true},
			configure: func(w *Writer) {
				w.Append = true
				w.FinalizeOnClose = true
			},
			wantErr:     func(err error) bool { return status.Code(err) == codes.FailedPrecondition },
			wantStreams: 2,
		},
		{
			// The first chunk's response carries the WriteHandle, so a stall
			// in the second chunk resumes the session.
			name:         "AppendableStallAfterHandle_Recovers",
			instructions: []string{stallAfter(3072)},
			budget:       stallEmuRecoverBudget,
			configure: func(w *Writer) {
				w.Append = true
				w.FinalizeOnClose = true
			},
			wantStreams: 2,
		},
		{
			// Close without finalizing: the final ack carries no object, so
			// the watchdog must pause once every byte is acknowledged.
			name:   "AppendableUnfinalizedClose_NoFalseStall",
			budget: 10 * time.Second,
			configure: func(w *Writer) {
				w.Append = true
				w.FinalizeOnClose = false
			},
			wantStreams: 1,
		},
		{
			name:         "AppendableUnfinalizedStallAfterHandle_Recovers",
			instructions: []string{stallAfter(3072)},
			budget:       stallEmuRecoverBudget,
			configure: func(w *Writer) {
				w.Append = true
				w.FinalizeOnClose = false
			},
			wantStreams: 2,
		},
		{
			name:         "TimeoutDisabled_StallNotDetected",
			instructions: []string{stallAfter(1024)},
			budget:       stallEmuHangBudget,
			configure:    func(w *Writer) { w.ChunkTransferTimeout = 0 },
			wantErr:      isDeadlineErr,
			wantStreams:  1,
		},
		{
			// ChunkSize 0 selects a oneshot upload, which the watchdog skips.
			name:         "Oneshot_NotArmed",
			instructions: []string{stallAfter(1024)},
			budget:       stallEmuHangBudget,
			configure:    func(w *Writer) { w.ChunkSize = 0 },
			wantErr:      isDeadlineErr,
			wantStreams:  1,
		},
		{
			name:         "RetryNever_NotRetried",
			instructions: []string{stallAfter(1024)},
			budget:       stallEmuRecoverBudget,
			retry:        []RetryOption{WithPolicy(RetryNever)},
			wantErr:      func(err error) bool { return errors.Is(err, errStallTimeout) },
			wantStreams:  1,
		},
		{
			// The first write exceeds ChunkSize, so the stream opens, the first
			// 4 MiB is flushed, and the next 2 MiB is sent as a non-flush
			// quantum. The application then idles for longer than the timeout.
			// GCS has nothing to acknowledge, so this must not be a stall.
			name:   "SlowProducer_NoFalseStall",
			budget: 10 * time.Second,
			configure: func(w *Writer) {
				w.ChunkSize = 2 * stallEmuChunkSize
			},
			write: func(w *Writer) error {
				if _, err := w.Write(data[:3*stallEmuChunkSize]); err != nil {
					return err
				}
				time.Sleep(3 * stallEmuTimeout)
				_, err := w.Write(data[3*stallEmuChunkSize : 7*1024*1024])
				return err
			},
			size:        7 * 1024 * 1024,
			wantStreams: 1,
		},
		{
			name:   "AppendableFlushThenIdle_NoFalseStall",
			budget: 10 * time.Second,
			configure: func(w *Writer) {
				w.Append = true
				w.FinalizeOnClose = true
			},
			write: func(w *Writer) error {
				if _, err := w.Write(data[:1024*1024]); err != nil {
					return err
				}
				if _, err := w.Flush(); err != nil {
					return err
				}
				time.Sleep(3 * stallEmuTimeout)
				_, err := w.Write(data[1024*1024 : stallEmuObjectSize])
				return err
			},
			wantStreams: 1,
		},
		{
			// The stall in the second chunk is recovered inside Write, so the
			// following Flush must report every byte written.
			name:         "AppendableStallBeforeFlush_FlushReturnsOffset",
			instructions: []string{stallAfter(3072)},
			budget:       stallEmuRecoverBudget,
			configure: func(w *Writer) {
				w.Append = true
				w.FinalizeOnClose = true
			},
			write: func(w *Writer) error {
				if _, err := w.Write(data[:4*1024*1024]); err != nil {
					return err
				}
				if err := flushTo(w, 4*1024*1024); err != nil {
					return err
				}
				_, err := w.Write(data[4*1024*1024 : stallEmuObjectSize])
				return err
			},
			wantStreams: 2,
		},
		{
			// The first Flush obtains the WriteHandle. The second Flush's
			// request stalls, so the Flush itself is retried and must report
			// every byte written.
			name:         "AppendableFlushStalls_RetriedFlushReturnsOffset",
			instructions: []string{stallAfter(1536)},
			budget:       stallEmuRecoverBudget,
			configure: func(w *Writer) {
				w.Append = true
				w.FinalizeOnClose = true
			},
			write: func(w *Writer) error {
				if _, err := w.Write(data[:1024*1024]); err != nil {
					return err
				}
				if err := flushTo(w, 1024*1024); err != nil {
					return err
				}
				if _, err := w.Write(data[1024*1024 : 2*1024*1024]); err != nil {
					return err
				}
				if err := flushTo(w, 2*1024*1024); err != nil {
					return err
				}
				_, err := w.Write(data[2*1024*1024 : stallEmuObjectSize])
				return err
			},
			wantStreams: 2,
		},
		{
			// Every attempt stalls at the same offset, so ChunkRetryDeadline
			// must end the retries.
			name: "RepeatedStalls_ExhaustChunkRetryDeadline",
			instructions: []string{
				stallAfter(1024), stallAfter(1024), stallAfter(1024),
				stallAfter(1024), stallAfter(1024), stallAfter(1024),
			},
			budget:    stallEmuRecoverBudget,
			configure: func(w *Writer) { w.ChunkRetryDeadline = 600 * time.Millisecond },
			wantErr: func(err error) bool {
				var deadlineErr *chunkRetryDeadlineError
				return errors.As(err, &deadlineErr) && errors.Is(err, errStallTimeout)
			},
			minStreams:        2,
			skipConsumedCheck: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var streams atomic.Int32
			streamInterceptor := grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
				if method == "/google.storage.v2.Storage/BidiWriteObject" {
					streams.Add(1)
				}
				return streamer(ctx, desc, cc, method, opts...)
			})
			unaryInterceptor := grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
				if method == "/google.storage.v2.Storage/QueryWriteStatus" {
					t.Errorf("unexpected QueryWriteStatus call during resumable upload retry")
				}
				return invoker(ctx, method, req, reply, cc, opts...)
			})
			client, err := NewGRPCClient(context.Background(), option.WithGRPCDialOption(streamInterceptor), option.WithGRPCDialOption(unaryInterceptor))
			if err != nil {
				t.Fatalf("NewGRPCClient: %v", err)
			}
			defer client.Close()

			ctx := context.Background()
			var testID string
			if len(tc.instructions) > 0 {
				testID = createRetryTest(t, client.tc, map[string][]string{"storage.objects.insert": tc.instructions})
				ctx = callctx.SetHeaders(ctx, "x-retry-test-id", testID)
			}
			ctx, cancel := context.WithTimeout(ctx, tc.budget)
			defer cancel()

			name := fmt.Sprintf("%s-%d", tc.name, time.Now().UnixNano())
			obj := client.Bucket(bucket).Object(name)
			if tc.conds != nil {
				obj = obj.If(*tc.conds)
			}
			if len(tc.retry) > 0 {
				obj = obj.Retryer(tc.retry...)
			}
			w := obj.NewWriter(ctx)
			w.ChunkSize = stallEmuChunkSize
			w.ChunkTransferTimeout = stallEmuTimeout
			if tc.configure != nil {
				tc.configure(w)
			}
			size := tc.size
			if size == 0 {
				size = stallEmuObjectSize
			}
			write := tc.write
			if write == nil {
				write = writeAll(size)
			}

			start := time.Now()
			err = errors.Join(write(w), w.Close())
			elapsed := time.Since(start)

			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("upload failed after %v: %v", elapsed, err)
				}
			} else if !tc.wantErr(err) {
				t.Fatalf("upload returned %v after %v; unexpected error", err, elapsed)
			}
			if errors.Is(err, errStallTimeout) && isDeadlineErr(err) {
				t.Errorf("error matches both stall and deadline: %v", err)
			}
			nStreams := streams.Load()
			if tc.wantStreams > 0 && nStreams != tc.wantStreams {
				t.Errorf("BidiWriteObject streams = %d, want %d", nStreams, tc.wantStreams)
			}
			if nStreams < tc.minStreams {
				t.Errorf("BidiWriteObject streams = %d, want at least %d", nStreams, tc.minStreams)
			}
			if testID != "" && !tc.skipConsumedCheck {
				checkRetryTestCompleted(t, testID)
			}

			if tc.wantErr != nil {
				return
			}
			readCtx, readCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer readCancel()
			r, err := client.Bucket(bucket).Object(name).NewReader(readCtx)
			if err != nil {
				t.Fatalf("NewReader: %v", err)
			}
			defer r.Close()
			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("reading object: %v", err)
			}
			if !bytes.Equal(got, data[:size]) {
				t.Errorf("object contents differ: got %d bytes, want %d", len(got), size)
			}
		})
	}
}
