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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestNewPartName(t *testing.T) {
	const (
		prefix     = "gcs-go-sdk-pu-tmp/"
		partNumber = 42
	)
	tests := []struct {
		name      string
		finalName string
	}{
		{
			name:      "simple_name",
			finalName: "my-object",
		},
		{
			name:      "dot_dot_segments",
			finalName: "tenant-a/../../../../other-prefix/target",
		},
		{
			name:      "max_length_object_name",
			finalName: strings.Repeat("a", 1024),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			partName := newPartName(prefix, tc.finalName, partNumber)
			wantPattern := fmt.Sprintf(`^gcs-go-sdk-pu-tmp/[0-9a-f]{8}-[0-9a-f]{%d}-part-%d$`, 2*partNameDigestBytes, partNumber)
			if !regexp.MustCompile(wantPattern).MatchString(partName) {
				t.Errorf("newPartName() = %q, want match %q", partName, wantPattern)
			}
		})
	}
}

func TestParallelUploadConfig_defaults(t *testing.T) {
	// Calculate the expected dynamic worker count based on the machine's CPU
	expectedWorkers := min(baseWorkers+(runtime.NumCPU()/2), maxWorkers)

	tests := []struct {
		name string
		in   *ParallelUploadConfig
		want *ParallelUploadConfig
	}{
		{
			name: "all defaults",
			in:   &ParallelUploadConfig{},
			want: &ParallelUploadConfig{
				PartSize:       defaultPartSize,
				MaxConcurrency: expectedWorkers,
			},
		},
		{
			name: "user-provided values are respected",
			in: &ParallelUploadConfig{
				PartSize:       10 * 1024 * 1024, // 10 MiB
				MaxConcurrency: 10,
			},
			want: &ParallelUploadConfig{
				PartSize:       10 * 1024 * 1024,
				MaxConcurrency: 10,
			},
		},
		{
			name: "PartSize below minimum is adjusted",
			in: &ParallelUploadConfig{
				PartSize: 1024 * 1024, // 1 MiB, below the 8 MiB minimum.
			},
			want: &ParallelUploadConfig{
				PartSize:       minPartSize,
				MaxConcurrency: expectedWorkers,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.in
			cfg.defaults()
			if diff := cmp.Diff(tt.want, cfg); diff != "" {
				t.Errorf("defaults() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewPCUSettings(t *testing.T) {
	MaxConcurrency := 8
	settings := newPCUSettings(MaxConcurrency)

	// Verify derived values.
	wantBufferPoolSize := MaxConcurrency + 1
	if settings.bufferPoolSize != wantBufferPoolSize {
		t.Errorf("bufferPoolSize = %d; want %d", settings.bufferPoolSize, wantBufferPoolSize)
	}
}

func TestPCUState_SetError(t *testing.T) {
	pCtx, cancel := context.WithCancel(context.Background())
	state := &pcuState{
		ctx:    pCtx,
		cancel: cancel,
	}

	err1 := fmt.Errorf("first error")
	state.setError(err1)

	// Verify firstErr is set and the error is added to the slice.
	if state.firstErr != err1 {
		t.Errorf("firstErr: got %v, want %v", state.firstErr, err1)
	}
	if len(state.errors) != 1 || state.errors[0] != err1 {
		t.Errorf("errors slice: got %v, want [%v]", state.errors, err1)
	}

	// Verify cancellation happens on the first error.
	select {
	case <-state.ctx.Done():
		if state.ctx.Err() != context.Canceled {
			t.Errorf("context error: got %v, want %v", state.ctx.Err(), context.Canceled)
		}
	default:
		t.Errorf("context not cancelled after first error")
	}

	// Verify context.Canceled is filtered out of the errors slice to avoid noise.
	state.setError(context.Canceled)
	if len(state.errors) != 1 {
		t.Errorf("errors slice after context.Canceled: got len %d, want 1", len(state.errors))
	}

	// Verify subsequent errors are collected but don't change firstErr.
	err2 := fmt.Errorf("second error")
	state.setError(err2)

	if state.firstErr != err1 {
		t.Errorf("firstErr after second error: got %v, want %v (should not change)", state.firstErr, err1)
	}
	if len(state.errors) != 2 || state.errors[1] != err2 {
		t.Errorf("errors slice after second error: got %v, want [%v, %v]", state.errors, err1, err2)
	}
}

func TestPCUState_ResultCollector(t *testing.T) {
	pCtx, cancel := context.WithCancel(context.Background())
	var progressSize int64
	w := &Writer{
		ProgressFunc: func(size int64) {
			progressSize = size
		},
	}
	state := &pcuState{
		ctx:      pCtx,
		w:        w,
		cancel:   cancel,
		resultCh: make(chan uploadResult, 4),
		partMap:  make(map[int]*ObjectHandle),
	}

	state.collectorWG.Add(1)
	go state.resultCollector()

	// Successful result with positive generation gets pinned.
	objHandle1 := &ObjectHandle{object: "part1", gen: defaultGen}
	state.resultCh <- uploadResult{partNumber: 1, handle: objHandle1, obj: &ObjectAttrs{Generation: 42}, err: nil, size: 50}

	// Zero generation and nil attrs stay unpinned (defaultGen).
	objHandle3 := &ObjectHandle{object: "part3", gen: defaultGen}
	state.resultCh <- uploadResult{partNumber: 3, handle: objHandle3, obj: &ObjectAttrs{Generation: 0}, err: nil, size: 10}
	objHandle4 := &ObjectHandle{object: "part4", gen: defaultGen}
	state.resultCh <- uploadResult{partNumber: 4, handle: objHandle4, obj: nil, err: nil, size: 10}

	// Error result.
	errResult := fmt.Errorf("upload failed")
	state.resultCh <- uploadResult{partNumber: 2, handle: nil, err: errResult}

	close(state.resultCh)
	state.collectorWG.Wait()

	if progressSize != 70 {
		t.Errorf("resultCollector: progressSize got %d, want 70", progressSize)
	}

	if handle, ok := state.partMap[1]; !ok || handle.object != objHandle1.object || handle.gen != 42 {
		t.Errorf("resultCollector: partMap[1] got (%v, %v), want (object=%q gen=42, true)", handle, ok, objHandle1.object)
	}
	if handle, ok := state.partMap[3]; !ok || handle.gen != defaultGen {
		t.Errorf("resultCollector: partMap[3] gen got %v (ok=%v), want %d", handle, ok, defaultGen)
	}
	if handle, ok := state.partMap[4]; !ok || handle.gen != defaultGen {
		t.Errorf("resultCollector: partMap[4] gen got %v (ok=%v), want %d", handle, ok, defaultGen)
	}
	if _, ok := state.partMap[2]; ok {
		t.Errorf("resultCollector: partMap[2] should not be present on error")
	}

	if state.firstErr == nil || state.firstErr.Error() != errResult.Error() {
		t.Errorf("resultCollector: firstErr got %v, want %v", state.firstErr, errResult)
	}

	// Check if context is cancelled.
	select {
	case <-state.ctx.Done():
		if state.ctx.Err() != context.Canceled {
			t.Errorf("resultCollector: context error got %v, want %v", state.ctx.Err(), context.Canceled)
		}
	default:
		t.Errorf("resultCollector: context should be cancelled on error")
	}
}

func TestPCUWorker_SuccessfulTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	buffer := make([]byte, 10)
	state := &pcuState{
		ctx:      ctx,
		cancel:   cancel,
		bufferCh: make(chan []byte, 1),
		uploadCh: make(chan uploadTask, 1),
		resultCh: make(chan uploadResult, 1),
		uploadPartFn: func(s *pcuState, task uploadTask) (*ObjectHandle, *ObjectAttrs, error) {
			return &ObjectHandle{object: "mockPart"}, &ObjectAttrs{Name: "mockPart"}, nil
		},
	}

	state.workerWG.Add(1)
	go state.worker()

	task := uploadTask{partNumber: 1, buffer: buffer, size: 10}
	state.uploadCh <- task

	// Wait for the worker to process the task and send the result.
	select {
	case result := <-state.resultCh:
		if result.err != nil {
			t.Errorf("worker unexpected error: %v", result.err)
		}
		if result.partNumber != 1 || result.handle == nil || result.handle.object != "mockPart" {
			t.Errorf("worker result mismatch: got %v", result)
		}
	// This safety timeout of 1 second prevents the test from hanging indefinitely
	// if the worker goroutine fails to respond.
	case <-time.After(1 * time.Second):
		t.Errorf("worker timeout waiting for result")
	}

	// Wait for the worker to return the buffer to the pool.
	select {
	case retBuffer := <-state.bufferCh:
		if len(retBuffer) != len(buffer) {
			t.Errorf("worker did not return the original buffer")
		}
	case <-time.After(1 * time.Second):
		t.Errorf("worker timeout waiting for buffer return")
	}

	close(state.uploadCh)
	state.workerWG.Wait()
}

func TestPCUWorker_FailedTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	buffer := make([]byte, 10)
	uploadErr := fmt.Errorf("upload failed")
	state := &pcuState{
		ctx:      ctx,
		cancel:   cancel,
		bufferCh: make(chan []byte, 1),
		uploadCh: make(chan uploadTask, 1),
		resultCh: make(chan uploadResult, 1),
		uploadPartFn: func(s *pcuState, task uploadTask) (*ObjectHandle, *ObjectAttrs, error) {
			return nil, nil, uploadErr
		},
	}

	state.workerWG.Add(1)
	go state.worker()

	task := uploadTask{partNumber: 1, buffer: buffer, size: 10}
	state.uploadCh <- task

	// Check for upload error.
	select {
	case result := <-state.resultCh:
		if result.err == nil || result.err.Error() != uploadErr.Error() {
			t.Errorf("worker error mismatch: got %v, want %v", result.err, uploadErr)
		}
		if result.partNumber != 1 {
			t.Errorf("worker partNumber mismatch: got %v", result.partNumber)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("worker timeout waiting for result")
	}

	// Check if buffer is returned.
	select {
	case <-state.bufferCh:
	case <-time.After(1 * time.Second):
		t.Errorf("worker timeout waiting for buffer return on error")
	}

	close(state.uploadCh)
	state.workerWG.Wait()
}

func TestPCUWorker_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	uploadStarted := make(chan struct{}) // To signal when upload starts.

	buffer := make([]byte, 10)
	state := &pcuState{
		ctx:      ctx,
		cancel:   cancel,
		bufferCh: make(chan []byte, 1),
		uploadCh: make(chan uploadTask, 1),
		resultCh: make(chan uploadResult, 1),
		uploadPartFn: func(s *pcuState, task uploadTask) (*ObjectHandle, *ObjectAttrs, error) {
			close(uploadStarted)
			<-s.ctx.Done()
			return nil, nil, s.ctx.Err()
		},
	}

	state.workerWG.Add(1)
	go state.worker()

	task := uploadTask{partNumber: 1, buffer: buffer, size: 10}
	state.uploadCh <- task

	<-uploadStarted // Wait until upload starts.

	cancel()

	// Worker safely returns early on cancellation to avoid deadlocks.
	done := make(chan struct{})
	go func() {
		state.workerWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success: worker exited.
	case <-time.After(2 * time.Second):
		t.Errorf("worker did not exit after context cancellation")
	}

	// Verify the buffer was correctly returned to the channel.
	select {
	case buf := <-state.bufferCh:
		if len(buf) != 10 {
			t.Errorf("got buffer size %d, want 10", len(buf))
		}
	case <-time.After(2 * time.Second):
		t.Errorf("worker failed to return buffer to pool after cancellation")
	}
}

func TestPCUWorker_UploadChannelClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	state := &pcuState{
		ctx:      ctx,
		cancel:   cancel,
		bufferCh: make(chan []byte, 1),
		uploadCh: make(chan uploadTask),
		resultCh: make(chan uploadResult, 1),
		uploadPartFn: func(s *pcuState, task uploadTask) (*ObjectHandle, *ObjectAttrs, error) {
			return nil, nil, nil
		},
	}

	state.workerWG.Add(1)
	go state.worker()

	// Close the upload channel to signal workers to stop.
	close(state.uploadCh)
	state.workerWG.Wait() // Should not block indefinitely.

	select {
	case res := <-state.resultCh:
		t.Errorf("Unexpected result on closed channel: %v", res)
	default:
	}
}

func TestPCUWorker_WriteContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	uploadStarted := make(chan struct{}) // To signal when upload starts.

	state := &pcuState{
		ctx:      ctx,
		cancel:   cancel,
		started:  true,
		bufferCh: make(chan []byte, 2),
		uploadCh: make(chan uploadTask, 2),
		resultCh: make(chan uploadResult, 2),
		w: &Writer{
			ctx: ctx,
			o: &ObjectHandle{
				bucket: "b",
				object: "test-object",
				c:      &Client{},
			},
		},
		config:   &ParallelUploadConfig{PartSize: 10, MaxConcurrency: 1},
		settings: &pcuSettings{bufferPoolSize: 2},
		partMap:  make(map[int]*ObjectHandle),
		doCleanupFn: func(s *pcuState) {
			// Mock cleanup to prevent nil pointer dereference.
		},
		uploadPartFn: func(s *pcuState, task uploadTask) (*ObjectHandle, *ObjectAttrs, error) {
			close(uploadStarted)
			<-s.ctx.Done() // Block until context is canceled.
			return nil, nil, s.ctx.Err()
		},
	}

	state.workerWG.Add(1)
	go state.worker()

	state.collectorWG.Add(1)
	go state.resultCollector()

	// Pre-fill buffer pool.
	for i := 0; i < 2; i++ {
		state.bufferCh <- make([]byte, 10)
	}

	// Trigger a flush by writing exact part size.
	n, err := state.write([]byte("0123456789"))
	if err != nil {
		t.Fatalf("state.write failed: %v", err)
	}
	if n != 10 {
		t.Errorf("n = %d; want 10", n)
	}

	<-uploadStarted // Wait until upload starts.

	cancel() // Cancel context mid-upload.

	// This Write should return an error because the context is canceled.
	// We might need to wait slightly for the cancellation to propagate to the worker and set firstErr.
	var writeErr error
	for i := 0; i < 50; i++ {
		_, writeErr = state.write([]byte("abcd"))
		if writeErr != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if writeErr == nil {
		t.Errorf("expected error on subsequent write after cancellation, got nil")
	}

	err = state.close()
	if err == nil {
		t.Errorf("expected error on close after cancellation, got nil")
	}

	// Ensure no goroutines leak.
	done := make(chan struct{})
	go func() {
		state.workerWG.Wait()
		state.collectorWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success: worker and collector exited.
	case <-time.After(2 * time.Second):
		t.Errorf("worker or collector did not exit after context cancellation")
	}
}

func TestNewPartName_Uniqueness(t *testing.T) {
	prefix := "gcs-go-sdk-pu-tmp/"
	finalName := "my-object"
	partNumber := 42

	name1 := newPartName(prefix, finalName, partNumber)
	name2 := newPartName(prefix, finalName, partNumber)

	if name1 == name2 {
		t.Errorf("NewPartName() returned the same name twice: %q", name1)
	}
}

func TestPCUState_Write(t *testing.T) {
	PartSize := 10 // bytes

	tests := []struct {
		name           string
		inputData      string
		expectDispatch int   // Tasks sent to uploadCh.
		expectBuffered int64 // Remaining bytes in state.
	}{
		{
			name:           "Buffer partial part",
			inputData:      "abc",
			expectDispatch: 0,
			expectBuffered: 3,
		},
		{
			name:           "Full part dispatch",
			inputData:      "0123456789",
			expectDispatch: 1,
			expectBuffered: 0,
		},
		{
			name:           "Multi-part overflow",
			inputData:      "0123456789extra", // 15 bytes.
			expectDispatch: 1,
			expectBuffered: 5,
		},
		{
			name:           "Exactly two parts",
			inputData:      "01234567890123456789",
			expectDispatch: 2,
			expectBuffered: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			state := &pcuState{
				ctx:      ctx,
				started:  true,
				bufferCh: make(chan []byte, 5),
				uploadCh: make(chan uploadTask, 5),
				config:   &ParallelUploadConfig{PartSize: PartSize},
			}

			// Pre-fill buffer pool.
			for i := 0; i < 5; i++ {
				state.bufferCh <- make([]byte, PartSize)
			}

			// Execute.
			n, err := state.write([]byte(tc.inputData))

			// Assertions.
			if err != nil {
				t.Fatalf("write failed: %v", err)
			}
			if n != len(tc.inputData) {
				t.Errorf("n = %d; want %d", n, len(tc.inputData))
			}
			if len(state.uploadCh) != tc.expectDispatch {
				t.Errorf("dispatched %d tasks; want %d", len(state.uploadCh), tc.expectDispatch)
			}
			if state.bytesBuffered != tc.expectBuffered {
				t.Errorf("bytesBuffered = %d; want %d", state.bytesBuffered, tc.expectBuffered)
			}
		})
	}
}

func TestPCUState_WriteStopsOnError(t *testing.T) {
	writeErr := errors.New("simulated write error")
	state := &pcuState{
		started:  true,
		firstErr: writeErr,
	}

	n, err := state.write([]byte("payload"))
	if n != 0 || err == nil {
		t.Fatalf("expected 0 bytes and error; got n=%v, err=%v", n, err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("expected error %v; got %v", writeErr, err)
	}
}

func TestPCUState_FlushCurrentBuffer(t *testing.T) {
	baseBuffer := make([]byte, 100)

	testCases := []struct {
		name              string
		setup             func() *pcuState // A function to create the specific state for the test case.
		expectTask        bool
		expectedPartNum   int
		expectedSize      int64
		expectErr         error
		checkBufferReturn bool
	}{
		{
			name: "should send task when buffer has data",
			setup: func() *pcuState {
				return &pcuState{
					ctx:           context.Background(),
					partNum:       0,
					bytesBuffered: 50,
					currentBuffer: baseBuffer,
					uploadCh:      make(chan uploadTask, 1),
					bufferCh:      make(chan []byte, 1),
					mu:            sync.Mutex{},
				}
			},
			expectTask:      true,
			expectedPartNum: 1,
			expectedSize:    50,
			expectErr:       nil,
		},
		{
			name: "should do nothing when buffer is empty",
			setup: func() *pcuState {
				return &pcuState{bytesBuffered: 0}
			},
			expectTask: false,
			expectErr:  nil,
		},
		{
			name: "should return error and buffer when context is canceled",
			setup: func() *pcuState {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return &pcuState{
					ctx:           ctx,
					bytesBuffered: 50,
					currentBuffer: baseBuffer,
					uploadCh:      make(chan uploadTask),
					bufferCh:      make(chan []byte, 1),
				}
			},
			expectTask:        false,
			expectErr:         context.Canceled,
			checkBufferReturn: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.setup()

			// Execute.
			err := s.flushCurrentBuffer()

			// Assertions.
			if !errors.Is(err, tc.expectErr) {
				t.Fatalf("Expected error '%v', but got '%v'", tc.expectErr, err)
			}

			if tc.expectTask {
				select {
				case task := <-s.uploadCh:
					if task.partNumber != tc.expectedPartNum {
						t.Errorf("Expected part number %d, got %d", tc.expectedPartNum, task.partNumber)
					}
					if task.size != tc.expectedSize {
						t.Errorf("Expected size %d, got %d", tc.expectedSize, task.size)
					}
					if s.currentBuffer != nil {
						t.Error("Expected currentBuffer to be nil after flush")
					}
				default:
					t.Fatal("Expected an uploadTask to be sent to uploadCh")
				}
			}

			if tc.checkBufferReturn {
				select {
				case <-s.bufferCh:
					// Success: buffer was correctly returned to the pool.
				default:
					t.Fatal("Expected buffer to be returned to bufferCh on cancellation")
				}
			}
		})
	}
}

func TestPCUState_GetSortedParts(t *testing.T) {
	// Create dummy handles for testing. We can use the object name to identify them.
	handle1 := &ObjectHandle{object: "part-1"}
	handle2 := &ObjectHandle{object: "part-2"}
	handle3 := &ObjectHandle{object: "part-3"}
	handle4 := &ObjectHandle{object: "part-4"}

	testCases := []struct {
		name          string
		partMap       map[int]*ObjectHandle
		expectedOrder []*ObjectHandle
	}{
		{
			name:          "Empty map should return empty slice",
			partMap:       map[int]*ObjectHandle{},
			expectedOrder: []*ObjectHandle{},
		},
		{
			name: "Map with one item",
			partMap: map[int]*ObjectHandle{
				10: handle1,
			},
			expectedOrder: []*ObjectHandle{handle1},
		},
		{
			name: "Map with items in random order",
			partMap: map[int]*ObjectHandle{
				4: handle4,
				1: handle1,
				3: handle3,
				2: handle2,
			},
			expectedOrder: []*ObjectHandle{handle1, handle2, handle3, handle4},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := &pcuState{
				partMap: tc.partMap,
			}

			// Execute.
			sortedParts := s.getSortedParts()

			// Assertions.
			if len(sortedParts) != len(tc.expectedOrder) {
				t.Fatalf("Expected slice of length %d, but got %d", len(tc.expectedOrder), len(sortedParts))
			}

			for i, expectedHandle := range tc.expectedOrder {
				// We compare the pointers directly since they are unique dummy handles.
				if sortedParts[i] != expectedHandle {
					t.Errorf("At index %d: expected handle for object %q, but got %q",
						i, expectedHandle.object, sortedParts[i].object)
				}
			}
		})
	}
}

func TestPCUState_ComposeParts(t *testing.T) {
	csek := bytes.Repeat([]byte{0x24}, 32)
	const kmsKey = "projects/p/locations/us/keyRings/kr/cryptoKeys/k"

	tests := []struct {
		name     string
		numParts int
		// We'll simulate maxComposeComponents by checking how many calls occur.
		wantIntermediates int
		csek              []byte
		kmsKeyName        string
	}{
		{
			name:              "Under limit (Single Level)",
			numParts:          10,
			wantIntermediates: 0, // 10 < 32, direct to final.
		},
		{
			name:              "Exactly limit (Single Level)",
			numParts:          32,
			wantIntermediates: 0, // 32 = 32, direct to final.
		},
		{
			name:              "Over limit (Multi Level)",
			numParts:          46,
			wantIntermediates: 2, // 2 intermediate groups (32+14) merged in final.
		},
		{
			name:              "Over limit with CSEK",
			numParts:          46,
			wantIntermediates: 2,
			csek:              csek,
		},
		{
			name:              "Over limit with CMEK",
			numParts:          46,
			wantIntermediates: 2,
			kmsKeyName:        kmsKey,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			intermediateCount := 0
			var nextGen int64 = 100
			interGenByName := make(map[string]int64)

			partMap := make(map[int]*ObjectHandle)
			for i := 1; i <= tc.numParts; i++ {
				partMap[i] = &ObjectHandle{object: fmt.Sprintf("part-%03d", i)}
			}
			dummyClient := &Client{}
			state := &pcuState{
				ctx:             context.Background(),
				partMap:         partMap,
				intermediateMap: make(map[string]*ObjectHandle),
				w: &Writer{
					ObjectAttrs: ObjectAttrs{KMSKeyName: tc.kmsKeyName},
					o:           &ObjectHandle{c: dummyClient, object: "final-dest", bucket: "b", encryptionKey: tc.csek},
				},
				config: &ParallelUploadConfig{},
			}

			state.composeFn = func(ctx context.Context, c *Composer) (*ObjectAttrs, error) {
				mu.Lock()
				defer mu.Unlock()

				if !c.DeleteSourceObjects {
					t.Errorf("expected DeleteSourceObjects to be true, got false")
				}
				if !bytes.Equal(c.dst.encryptionKey, tc.csek) {
					t.Errorf("compose dst(%q) encryptionKey = %x, want %x", c.dst.object, c.dst.encryptionKey, tc.csek)
				}
				if c.KMSKeyName != tc.kmsKeyName {
					t.Errorf("compose dst(%q) KMSKeyName = %q, want %q", c.dst.object, c.KMSKeyName, tc.kmsKeyName)
				}

				nextGen++
				gen := nextGen
				// If the destination isn't the final object, it's an intermediate.
				if c.dst.object != "final-dest" {
					if c.dst.conds == nil || !c.dst.conds.DoesNotExist {
						t.Errorf("intermediate compose dst(%q) DoesNotExist = false, want true", c.dst.object)
					}
					intermediateCount++
					interGenByName[c.dst.object] = gen
				} else if tc.wantIntermediates > 0 {
					for _, src := range c.srcs {
						if src.conds != nil {
							t.Errorf("final compose source %q has unexpected conds %+v", src.object, src.conds)
						}
						wantGen, ok := interGenByName[src.object]
						if !ok {
							t.Errorf("final compose source %q was not created as an intermediate", src.object)
						} else if src.gen != wantGen {
							t.Errorf("final compose source %q gen = %d, want %d", src.object, src.gen, wantGen)
						}
					}
				}

				// Return the name and generation of the destination object as the "attribute".
				return &ObjectAttrs{Name: c.dst.object, Generation: gen}, nil
			}

			// Execute.
			err := state.composeParts()
			if err != nil {
				t.Fatalf("composeParts failed: %v", err)
			}

			// Assertions.
			if intermediateCount != tc.wantIntermediates {
				t.Errorf("expected %d intermediate composes, got %d", tc.wantIntermediates, intermediateCount)
			}

			if state.w.obj.Name != "final-dest" {
				t.Errorf("final object name mismatch: got %s, want final-dest", state.w.obj.Name)
			}

			if len(state.intermediateMap) != intermediateCount {
				t.Errorf("intermediate map count mismatch: got %d, want %d", len(state.intermediateMap), intermediateCount)
			}
			for name, h := range state.intermediateMap {
				if wantGen := interGenByName[name]; h.gen != wantGen {
					t.Errorf("intermediateMap[%q].gen = %d, want %d", name, h.gen, wantGen)
				}
			}
		})
	}
}

func TestPCUState_ComposePartsIntegrity(t *testing.T) {
	const manualHash uint32 = 0xDEADBEEF

	tests := []struct {
		name           string
		userSendCRC    bool
		userCRCValue   uint32
		expectSendCRC  bool
		expectCRCValue uint32
	}{
		{
			name:           "Default/Enabled (SDK calculates and verifies)",
			userSendCRC:    true,
			userCRCValue:   0,
			expectSendCRC:  true,
			expectCRCValue: 0,
		},
		{
			name:           "Explicit Bypass (No transport or server verification)",
			userSendCRC:    false,
			userCRCValue:   0,
			expectSendCRC:  false,
			expectCRCValue: 0,
		},
		{
			name:           "Manual Hash (Bypass transport, verify final object on GCS)",
			userSendCRC:    false,
			userCRCValue:   manualHash,
			expectSendCRC:  false,
			expectCRCValue: manualHash,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup dummy handles for parts.
			partMap := make(map[int]*ObjectHandle)
			for i := 1; i <= 3; i++ {
				partMap[i] = &ObjectHandle{object: fmt.Sprintf("part-%d", i)}
			}

			state := &pcuState{
				ctx:     context.Background(),
				partMap: partMap,
				w: &Writer{
					SendCRC32C: tc.userSendCRC,
					ObjectAttrs: ObjectAttrs{
						CRC32C: tc.userCRCValue,
					},
					o: &ObjectHandle{
						bucket: "test-bucket",
						object: "final-object",
						c:      &Client{},
					},
				},
				config: &ParallelUploadConfig{},
			}

			// Capture data from the final compose call.
			var capturedSendCRC bool
			var capturedCRCValue uint32

			state.composeFn = func(ctx context.Context, c *Composer) (*ObjectAttrs, error) {
				// We only verify the attributes on the final destination object.
				if c.dst.object == "final-object" {
					capturedSendCRC = c.SendCRC32C
					capturedCRCValue = c.ObjectAttrs.CRC32C
				}
				return &ObjectAttrs{Name: c.dst.object}, nil
			}

			// Execute.
			if err := state.composeParts(); err != nil {
				t.Fatalf("composeParts failed: %v", err)
			}

			// Assertions.
			if capturedSendCRC != tc.expectSendCRC {
				t.Errorf("SendCRC32C = %v; want %v", capturedSendCRC, tc.expectSendCRC)
			}
			if capturedCRCValue != tc.expectCRCValue {
				t.Errorf("CRC32C value = %v; want %v", capturedCRCValue, tc.expectCRCValue)
			}
		})
	}
}

func TestPCUState_DoCleanup(t *testing.T) {
	testCases := []struct {
		name              string
		firstErr          error
		partsToCreate     int
		interimsToCreate  int
		failDeletesFor    map[string]bool
		notExistFor       map[string]bool
		expectDeleteCalls int
		// expectLogged is the set of objects expected to be reported as failed deletes.
		expectLogged []string
	}{
		{
			name:              "CleanupAlways should clean up all",
			firstErr:          errors.New("upload failed"),
			partsToCreate:     2,
			interimsToCreate:  1,
			expectDeleteCalls: 3,
		},
		{
			name:             "CleanupAlways with failing deletes",
			partsToCreate:    2,
			interimsToCreate: 2,
			failDeletesFor: map[string]bool{
				"part-1":    true,
				"interim-0": true,
			},
			expectDeleteCalls: 4,
			expectLogged:      []string{"part-1", "interim-0"},
		},
		{
			// Parts consumed by a successful compose with DeleteSourceObjects are
			// already gone; cleanup should not report them.
			name:             "Already deleted objects are not logged",
			partsToCreate:    3,
			interimsToCreate: 1,
			notExistFor: map[string]bool{
				"part-0": true,
				"part-1": true,
			},
			failDeletesFor: map[string]bool{
				"part-2": true,
			},
			expectDeleteCalls: 4,
			expectLogged:      []string{"part-2"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pCtx, cancel := context.WithCancel(context.Background())
			var mu sync.Mutex
			deleteCalls := 0

			// Prepare the pcuState for the test.
			state := &pcuState{
				ctx:             pCtx,
				cancel:          cancel,
				config:          &ParallelUploadConfig{MaxConcurrency: 4},
				firstErr:        tc.firstErr,
				partMap:         make(map[int]*ObjectHandle),
				intermediateMap: make(map[string]*ObjectHandle),
			}

			state.deleteFn = func(ctx context.Context, h *ObjectHandle) error {
				mu.Lock()
				deleteCalls++
				mu.Unlock()

				// Simulate failure based on the object's dummy name.
				if tc.notExistFor[h.object] {
					return fmt.Errorf("mock: %w", ErrObjectNotExist)
				}
				if shouldFail, ok := tc.failDeletesFor[h.object]; ok && shouldFail {
					return errors.New("mock delete error")
				}
				return nil
			}

			// Populate state with dummy object handles.
			// The pointers can be simple because our mock deleteFn doesn't use them.
			for i := 0; i < tc.partsToCreate; i++ {
				state.partMap[i] = &ObjectHandle{object: fmt.Sprintf("part-%d", i)}
			}
			for i := 0; i < tc.interimsToCreate; i++ {
				name := fmt.Sprintf("interim-%d", i)
				state.intermediateMap[name] = &ObjectHandle{object: name}
			}

			// Capture cleanup logs.
			var logBuf bytes.Buffer
			log.SetOutput(&logBuf)
			defer log.SetOutput(os.Stderr)

			// Execute.
			state.doCleanup()

			// Assertions.
			if deleteCalls != tc.expectDeleteCalls {
				t.Errorf("Expected %d delete calls, but got %d", tc.expectDeleteCalls, deleteCalls)
			}
			logs := logBuf.String()
			if got := strings.Count(logs, "failed to delete temporary part"); got != len(tc.expectLogged) {
				t.Errorf("got %d logged delete failures, want %d; logs:\n%s", got, len(tc.expectLogged), logs)
			}
			for _, name := range tc.expectLogged {
				if !strings.Contains(logs, fmt.Sprintf("%q", name)) {
					t.Errorf("expected delete failure for %q to be logged; logs:\n%s", name, logs)
				}
			}
		})
	}
}

func TestPCUState_Close(t *testing.T) {
	tests := []struct {
		name string
		// workerErr is set as firstErr before close, simulating a failed part upload.
		workerErr error
		// composeErr is returned by composeParts.
		composeErr error
		// finalComposeSucceeded simulates the final compose succeeding before
		// composeParts returns (e.g. a CRC32C mismatch detected afterwards).
		finalComposeSucceeded bool
		expectCompose         bool
		expectError           bool
		expectCleanup         bool
	}{
		{
			name:                  "Successful Upload",
			finalComposeSucceeded: true,
			expectCompose:         true,
		},
		{
			name:          "Worker Error - Aborts Compose",
			workerErr:     fmt.Errorf("upload failed"),
			expectError:   true,
			expectCleanup: true,
		},
		{
			name:          "Compose Error",
			composeErr:    fmt.Errorf("compose failed"),
			expectCompose: true,
			expectError:   true,
			expectCleanup: true,
		},
		{
			// setError ignores context.Canceled, so firstErr stays nil; cleanup
			// must still run or the uploaded parts leak.
			name:          "Context Canceled During Compose",
			composeErr:    context.Canceled,
			expectCompose: true,
			expectError:   true,
			expectCleanup: true,
		},
		{
			name:          "Wrapped Context Canceled During Compose",
			composeErr:    fmt.Errorf("Post \"https://storage.googleapis.com\": %w", context.Canceled),
			expectCompose: true,
			expectError:   true,
			expectCleanup: true,
		},
		{
			// Sources were already deleted by the final compose.
			name:                  "Error After Final Compose Succeeded",
			composeErr:            fmt.Errorf("crc32c mismatch"),
			finalComposeSucceeded: true,
			expectCompose:         true,
			expectError:           true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			composeCalled := false
			cleanupCh := make(chan struct{}, 1)
			ctx := context.Background()
			objName := "test-object"

			state := &pcuState{
				ctx:      ctx,
				cancel:   func() {},
				started:  true,
				uploadCh: make(chan uploadTask, 10),
				resultCh: make(chan uploadResult, 10),
				partMap:  map[int]*ObjectHandle{1: {object: "tmp1"}, 2: {object: "tmp2"}},
				w: &Writer{
					ctx: ctx,
					ObjectAttrs: ObjectAttrs{
						Name: objName,
					},
					o: &ObjectHandle{
						bucket: "b",
						object: objName,
						c:      &Client{},
					},
				},
				composePartsFn: func(s *pcuState) error {
					composeCalled = true
					if tc.finalComposeSucceeded {
						s.mu.Lock()
						s.finalComposeSucceeded = true
						s.mu.Unlock()
					}
					return tc.composeErr
				},
				doCleanupFn: func(s *pcuState) {
					cleanupCh <- struct{}{}
				},
			}
			if tc.workerErr != nil {
				state.firstErr = tc.workerErr
			}

			err := state.close()

			if (err != nil) != tc.expectError {
				t.Errorf("expectError %v, got err: %v", tc.expectError, err)
			}
			if composeCalled != tc.expectCompose {
				t.Errorf("expectCompose %v, but composeCalled was %v", tc.expectCompose, composeCalled)
			}

			// Cleanup runs in the background.
			if tc.expectCleanup {
				select {
				case <-cleanupCh:
				case <-time.After(time.Second):
					t.Errorf("cleanup was not executed; temporary parts would leak")
				}
			} else {
				select {
				case <-cleanupCh:
					t.Errorf("cleanup executed unexpectedly")
				case <-time.After(50 * time.Millisecond):
				}
			}
		})
	}
}

type pcuRecordedPartWrite struct {
	name         string
	kmsKey       string
	csekKeyBytes []byte
	assignedGen  int64
}

type pcuFakeGRPCServer struct {
	storagepb.UnimplementedStorageServer

	mu               sync.Mutex
	nextGen          int64
	partWrites       []pcuRecordedPartWrite
	composeReqs      []*storagepb.ComposeObjectRequest
	deleteReqs       []*storagepb.DeleteObjectRequest
	wantDeletes           int
	deletesDone           chan struct{}
	failFirstIntermediate bool
	failFinalCompose      bool
}

func (s *pcuFakeGRPCServer) BidiWriteObject(stream storagepb.Storage_BidiWriteObjectServer) error {
	var spec *storagepb.WriteObjectSpec
	var csek *storagepb.CommonObjectRequestParams
	var totalSize int64

	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if ws := req.GetWriteObjectSpec(); ws != nil {
			spec = ws
			csek = req.GetCommonObjectRequestParams()
		}
		totalSize += int64(len(req.GetChecksummedData().GetContent()))

		if req.GetFinishWrite() {
			if spec == nil {
				return status.Error(codes.InvalidArgument, "missing WriteObjectSpec")
			}
			s.mu.Lock()
			s.nextGen++
			gen := s.nextGen
			rec := pcuRecordedPartWrite{
				name:         spec.GetResource().GetName(),
				kmsKey:       spec.GetResource().GetKmsKey(),
				csekKeyBytes: append([]byte(nil), csek.GetEncryptionKeyBytes()...),
				assignedGen:  gen,
			}
			s.partWrites = append(s.partWrites, rec)
			s.mu.Unlock()

			return stream.Send(&storagepb.BidiWriteObjectResponse{
				WriteStatus: &storagepb.BidiWriteObjectResponse_Resource{
					Resource: &storagepb.Object{
						Bucket:     spec.GetResource().GetBucket(),
						Name:       spec.GetResource().GetName(),
						Size:       totalSize,
						Generation: gen,
						KmsKey:     spec.GetResource().GetKmsKey(),
					},
				},
			})
		}
	}
}

func (s *pcuFakeGRPCServer) ComposeObject(ctx context.Context, req *storagepb.ComposeObjectRequest) (*storagepb.Object, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.composeReqs = append(s.composeReqs, req)
	if s.failFirstIntermediate && strings.HasPrefix(req.GetDestination().GetName(), tmpObjectPrefix) {
		s.failFirstIntermediate = false
		return nil, status.Error(codes.Unavailable, "simulated transient intermediate compose failure")
	}
	if s.failFinalCompose && !strings.HasPrefix(req.GetDestination().GetName(), tmpObjectPrefix) {
		return nil, status.Error(codes.Aborted, "simulated final compose failure")
	}

	s.nextGen++
	gen := s.nextGen
	return &storagepb.Object{
		Bucket:     req.GetDestination().GetBucket(),
		Name:       req.GetDestination().GetName(),
		Generation: gen,
		KmsKey:     req.GetKmsKey(),
	}, nil
}

func (s *pcuFakeGRPCServer) DeleteObject(ctx context.Context, req *storagepb.DeleteObjectRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteReqs = append(s.deleteReqs, req)
	if s.deletesDone != nil && len(s.deleteReqs) == s.wantDeletes {
		close(s.deletesDone)
	}
	return &emptypb.Empty{}, nil
}

func TestPCU_PartEncryptionAndGenerationPinning(t *testing.T) {
	csek := bytes.Repeat([]byte{0x42}, 32)
	const kmsKey = "projects/p/locations/us/keyRings/kr/cryptoKeys/k"

	tests := []struct {
		name       string
		csek       []byte
		kmsKeyName string
	}{
		{
			name: "no_key",
		},
		{
			name: "csek",
			csek: csek,
		},
		{
			name:       "cmek",
			kmsKeyName: kmsKey,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := &pcuFakeGRPCServer{nextGen: 1000}
			client := newBufconnGRPCClient(t, srv)

			obj := client.Bucket("my-bucket").Object("tenant-a/../../dest-obj")
			if len(tc.csek) > 0 {
				obj = obj.Key(tc.csek)
			}
			w := obj.NewWriter(t.Context())
			w.EnableParallelUpload = true
			w.ParallelUploadConfig = ParallelUploadConfig{PartSize: minPartSize, MaxConcurrency: 2}
			w.KMSKeyName = tc.kmsKeyName

			// Write 2 parts (8 MiB + 1 MiB) so both part upload and compose run.
			payload := make([]byte, minPartSize+(1<<20))
			if _, err := w.Write(payload); err != nil {
				t.Fatalf("Writer.Write() failed: %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("Writer.Close() failed: %v", err)
			}

			srv.mu.Lock()
			defer srv.mu.Unlock()

			if len(srv.partWrites) != 2 {
				t.Fatalf("partWrites count = %d, want 2", len(srv.partWrites))
			}
			wantGenByName := make(map[string]int64, len(srv.partWrites))
			for i, pw := range srv.partWrites {
				if !bytes.Equal(pw.csekKeyBytes, tc.csek) {
					t.Errorf("partWrites[%d] CSEK = %x, want %x", i, pw.csekKeyBytes, tc.csek)
				}
				if pw.kmsKey != tc.kmsKeyName {
					t.Errorf("partWrites[%d].kmsKey = %q, want %q", i, pw.kmsKey, tc.kmsKeyName)
				}
				wantGenByName[pw.name] = pw.assignedGen
			}

			if len(srv.composeReqs) != 1 {
				t.Fatalf("composeReqs count = %d, want 1", len(srv.composeReqs))
			}
			comp := srv.composeReqs[0]
			if comp.GetKmsKey() != tc.kmsKeyName {
				t.Errorf("ComposeObjectRequest.KmsKey = %q, want %q", comp.GetKmsKey(), tc.kmsKeyName)
			}
			if got := comp.GetCommonObjectRequestParams().GetEncryptionKeyBytes(); !bytes.Equal(got, tc.csek) {
				t.Errorf("ComposeObjectRequest CSEK = %x, want %x", got, tc.csek)
			}
			for _, src := range comp.GetSourceObjects() {
				wantGen, ok := wantGenByName[src.GetName()]
				if !ok {
					t.Errorf("compose source %q was never uploaded as a part", src.GetName())
				} else if src.GetGeneration() != wantGen {
					t.Errorf("compose source %q generation = %d, want %d", src.GetName(), src.GetGeneration(), wantGen)
				}
			}
		})
	}
}

func TestPCU_MultiLevelComposeEncryptionAndCleanupGenerations(t *testing.T) {
	csek := bytes.Repeat([]byte{0x24}, 32)
	const wantDeletes = 35 // 33 parts + 2 intermediates

	srv := &pcuFakeGRPCServer{
		nextGen:               5000,
		wantDeletes:           wantDeletes,
		deletesDone:           make(chan struct{}),
		failFirstIntermediate: true,
		failFinalCompose:      true,
	}
	client := newBufconnGRPCClient(t, srv)

	obj := client.Bucket("my-bucket").Object("multi-level-dest").Key(csek)
	w := obj.NewWriter(t.Context())
	w.EnableParallelUpload = true
	w.ParallelUploadConfig = ParallelUploadConfig{PartSize: minPartSize, MaxConcurrency: 4}

	// Write 33 parts (32*minPartSize + 1024 bytes) to trigger intermediate composition.
	chunk := make([]byte, minPartSize)
	for i := 0; i < 32; i++ {
		if _, err := w.Write(chunk); err != nil {
			t.Fatalf("Writer.Write(part %d) failed: %v", i+1, err)
		}
	}
	if _, err := w.Write(make([]byte, 1024)); err != nil {
		t.Fatalf("Writer.Write(tail) failed: %v", err)
	}
	if err := w.Close(); err == nil {
		t.Fatalf("Writer.Close() = nil, want simulated final compose error")
	}

	select {
	case <-srv.deletesDone:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for cleanup deletes")
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()

	if len(srv.composeReqs) != 4 { // 1 retried + 2 succeeded intermediates + 1 failed final
		t.Fatalf("composeReqs count = %d, want 4", len(srv.composeReqs))
	}
	for i, inter := range srv.composeReqs[:3] {
		if got := inter.GetCommonObjectRequestParams().GetEncryptionKeyBytes(); !bytes.Equal(got, csek) {
			t.Errorf("intermediate compose[%d] CSEK = %x, want %x", i, got, csek)
		}
		if inter.IfGenerationMatch == nil || inter.GetIfGenerationMatch() != 0 {
			t.Errorf("intermediate compose[%d] IfGenerationMatch = %v, want 0", i, inter.IfGenerationMatch)
		}
	}
	for i, src := range srv.composeReqs[3].GetSourceObjects() {
		if src.GetGeneration() == 0 {
			t.Errorf("final compose SourceObjects[%d] (%q) Generation = 0, want pinned intermediate generation", i, src.GetName())
		}
	}
	if len(srv.deleteReqs) != wantDeletes {
		t.Fatalf("deleteReqs count = %d, want %d", len(srv.deleteReqs), wantDeletes)
	}
	for i, del := range srv.deleteReqs {
		if del.GetGeneration() == 0 {
			t.Errorf("deleteReqs[%d] (%q) Generation = 0, want pinned generation", i, del.GetObject())
		}
	}
}
