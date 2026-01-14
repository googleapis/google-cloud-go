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
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	gax "github.com/googleapis/gax-go/v2"
)

func TestPartCleanupStrategy_String(t *testing.T) {
	tests := []struct {
		strategy partCleanupStrategy
		want     string
	}{
		{cleanupAlways, "always"},
		{cleanupOnSuccess, "on_success"},
		{cleanupNever, "never"},
		{partCleanupStrategy(99), "PartCleanupStrategy(99)"},
		{partCleanupStrategy(-1), "PartCleanupStrategy(-1)"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Strategy_%d", tt.strategy), func(t *testing.T) {
			if got := tt.strategy.String(); got != tt.want {
				t.Errorf("PartCleanupStrategy.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultNamingStrategy_NewPartName(t *testing.T) {
	strategy := &defaultNamingStrategy{}
	bucket := "my-bucket"
	prefix := "gcs-go-sdk-pcu-tmp/"
	finalName := "my-object"
	partNumber := 42

	partName := strategy.newPartName(bucket, prefix, finalName, partNumber)

	if !strings.HasPrefix(partName, prefix) {
		t.Errorf("NewPartName() should start with the prefix %q, but got %q", prefix, partName)
	}

	expectedFormat := prefix + "%x-" + finalName + "-part-%d"
	var randSuffix uint64
	var parsedPartNum int

	_, err := fmt.Sscanf(partName, expectedFormat, &randSuffix, &parsedPartNum)
	if err != nil {
		t.Errorf("NewPartName() returned a name with an unexpected format. Got %q, want format ~%q. Error: %v", partName, prefix+"<hex>-"+finalName+"-part-<int>", err)
		return // Return to avoid further checks if parsing failed.
	}

	if parsedPartNum != partNumber {
		t.Errorf("NewPartName() did not include the correct part number. Got %d, want %d", parsedPartNum, partNumber)
	}

	if randSuffix == 0 {
		t.Errorf("NewPartName() did not include a non-zero random hex part. Got %x", randSuffix)
	}
}

func TestParallelUploadConfig_defaults(t *testing.T) {

	// For the "all defaults" test case.
	expectedWorkers := min(baseWorkers+(runtime.NumCPU()/2), maxWorkers)
	defaultMinSizeVal := int64(defaultMinSize)
	userMinSizeVal := int64(0)

	tests := []struct {
		name string
		in   *parallelUploadConfig
		want *parallelUploadConfig
	}{
		{
			name: "all defaults",
			in:   &parallelUploadConfig{},
			want: &parallelUploadConfig{
				minSize:         &defaultMinSizeVal,
				partSize:        defaultPartSize,
				numWorkers:      expectedWorkers,
				bufferPoolSize:  expectedWorkers + 1,
				tmpObjectPrefix: defaultTmpObjectPrefix,
				retryOptions: []RetryOption{
					WithMaxAttempts(defaultMaxRetries),
					WithBackoff(gax.Backoff{
						Initial: defaultBaseDelay,
						Max:     defaultMaxDelay,
					}),
				},
				cleanupStrategy: cleanupAlways,
				namingStrategy:  &defaultNamingStrategy{},
			},
		},
		{
			name: "user-provided values are respected",
			in: &parallelUploadConfig{
				minSize:         &userMinSizeVal,
				partSize:        1024,
				numWorkers:      10,
				bufferPoolSize:  12,
				tmpObjectPrefix: "my-prefix/",
				retryOptions: []RetryOption{
					WithMaxAttempts(5),
					WithBackoff(gax.Backoff{
						Initial: 200 * time.Millisecond,
						Max:     10 * time.Second,
					}),
				},
				cleanupStrategy: cleanupOnSuccess,
				namingStrategy:  &testNamingStrategy{},
			},
			want: &parallelUploadConfig{
				minSize:         &userMinSizeVal,
				partSize:        1024,
				numWorkers:      10,
				bufferPoolSize:  12,
				tmpObjectPrefix: "my-prefix/",
				retryOptions: []RetryOption{
					WithMaxAttempts(5),
					WithBackoff(gax.Backoff{
						Initial: 200 * time.Millisecond,
						Max:     10 * time.Second,
					}),
				},
				cleanupStrategy: cleanupOnSuccess,
				namingStrategy:  &testNamingStrategy{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.in
			cfg.defaults()
			if diff := cmp.Diff(tt.want, cfg,
				cmp.AllowUnexported(parallelUploadConfig{}, defaultNamingStrategy{}, testNamingStrategy{}, withMaxAttempts{}, withBackoff{}),
				cmpopts.IgnoreUnexported(gax.Backoff{}, ObjectAttrs{})); diff != "" {
				t.Errorf("defaults() mismatch (-want +got):\n%s", diff)
			}
		})
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
	state := &pcuState{
		ctx:      pCtx,
		cancel:   cancel,
		resultCh: make(chan uploadResult, 2),
		partMap:  make(map[int]*ObjectHandle),
	}

	state.collectorWG.Add(1)
	go state.resultCollector()

	// Successful result.
	objHandle1 := &ObjectHandle{object: "part1"}
	state.resultCh <- uploadResult{partNumber: 1, handle: objHandle1, err: nil}

	// Error result.
	errResult := fmt.Errorf("upload failed")
	state.resultCh <- uploadResult{partNumber: 2, handle: nil, err: errResult}

	close(state.resultCh)
	state.collectorWG.Wait()

	if handle, ok := state.partMap[1]; !ok || handle.object != objHandle1.object {
		t.Errorf("resultCollector: partMap[1] got (%v, %v), want (%v, true)", handle, ok, objHandle1)
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

	// Check if context has been cancelled.
	select {
	case result := <-state.resultCh:
		if result.err != context.Canceled {
			t.Errorf("worker error on cancel: got %v, want %v", result.err, context.Canceled)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("worker timeout waiting for result after cancel")
	}

	// Wait for the worker to finish.
	state.workerWG.Wait()

	// Verify the buffer was correctly returned to the channel.
	select {
	case <-state.bufferCh:
	case <-time.After(1 * time.Second):
		t.Errorf("worker timeout waiting for buffer return after cancel")
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

func TestDefaultNamingStrategy_NewPartName_Uniqueness(t *testing.T) {
	strategy := &defaultNamingStrategy{}
	bucket := "my-bucket"
	prefix := "gcs-go-sdk-pcu-tmp/"
	finalName := "my-object"
	partNumber := 42

	name1 := strategy.newPartName(bucket, prefix, finalName, partNumber)
	name2 := strategy.newPartName(bucket, prefix, finalName, partNumber)

	if name1 == name2 {
		t.Errorf("NewPartName() returned the same name twice: %q", name1)
	}
}

func TestSetPartMetadata(t *testing.T) {
	testCases := []struct {
		name             string
		initialMetadata  map[string]string
		decorator        partMetadataDecorator
		task             uploadTask
		finalObjectName  string
		expectedMetadata map[string]string
	}{
		{
			name:            "Nil initial metadata, no decorator",
			initialMetadata: nil,
			decorator:       nil,
			task:            uploadTask{partNumber: 1},
			finalObjectName: "final-object",
			expectedMetadata: map[string]string{
				pcuPartNumberMetadataKey:  "1",
				pcuFinalObjectMetadataKey: "final-object",
			},
		},
		{
			name: "Existing metadata, no decorator",
			initialMetadata: map[string]string{
				"initial-key": "initial-value",
			},
			decorator:       nil,
			task:            uploadTask{partNumber: 2},
			finalObjectName: "final-object",
			expectedMetadata: map[string]string{
				"initial-key":             "initial-value",
				pcuPartNumberMetadataKey:  "2",
				pcuFinalObjectMetadataKey: "final-object",
			},
		},
		{
			name:            "Nil initial metadata, with decorator",
			initialMetadata: nil,
			decorator: &testMetadataDecorator{
				metadataToSet: map[string]string{"decorated-key": "decorated-value"},
			},
			task:            uploadTask{partNumber: 3},
			finalObjectName: "final-object",
			expectedMetadata: map[string]string{
				"decorated-key":           "decorated-value",
				pcuPartNumberMetadataKey:  "3",
				pcuFinalObjectMetadataKey: "final-object",
			},
		},
		{
			name: "Existing metadata, with decorator that overwrites",
			initialMetadata: map[string]string{
				"initial-key":            "initial-value",
				pcuPartNumberMetadataKey: "should-be-overwritten",
			},
			decorator: &testMetadataDecorator{
				metadataToSet: map[string]string{
					"decorated-key":           "decorated-value",
					pcuFinalObjectMetadataKey: "overwritten-by-decorator",
				},
			},
			task:            uploadTask{partNumber: 4},
			finalObjectName: "final-object-base",
			expectedMetadata: map[string]string{
				"initial-key":             "initial-value",
				"decorated-key":           "decorated-value",
				pcuPartNumberMetadataKey:  "4",                        // Overwrites initial.
				pcuFinalObjectMetadataKey: "overwritten-by-decorator", // Overwritten by decorator.
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup.
			sourceWriter := &Writer{
				ObjectAttrs: ObjectAttrs{
					Metadata: tc.initialMetadata,
				},
				o: &ObjectHandle{bucket: "my-bucket", object: tc.finalObjectName},
			}
			partWriter := &Writer{}
			state := &pcuState{
				w: sourceWriter,
				config: &parallelUploadConfig{
					metadataDecorator: tc.decorator,
				},
			}

			// Execute.
			setPartMetadata(partWriter, state, tc.task)

			// Verify.
			if !reflect.DeepEqual(partWriter.ObjectAttrs.Metadata, tc.expectedMetadata) {
				t.Errorf("Metadata mismatch:\ngot:  %v\nwant: %v", partWriter.ObjectAttrs.Metadata, tc.expectedMetadata)
			}
		})
	}
}

func TestPCUState_Write(t *testing.T) {
	partSize := 10 // bytes

	tests := []struct {
		name           string
		inputData      string
		expectDispatch int   // Tasks sent to uploadCh
		expectBuffered int64 // Remaining bytes in state
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
			inputData:      "0123456789extra", // 15 bytes
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
				config:   &parallelUploadConfig{partSize: partSize},
			}

			// Pre-fill buffer pool
			for i := 0; i < 5; i++ {
				state.bufferCh <- make([]byte, partSize)
			}

			// Execute
			n, err := state.write([]byte(tc.inputData))

			// Assertions
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
	state := &pcuState{
		started:  true,
		firstErr: fmt.Errorf("background failure"),
	}

	n, err := state.write([]byte("payload"))
	if n != 0 || err == nil {
		t.Errorf("expected 0 bytes and error; got n=%d, err=%v", n, err)
	}
}

func TestPCUState_FlushCurrentBuffer(t *testing.T) {
	baseBuffer := make([]byte, 100)

	testCases := []struct {
		name              string
		setup             func() *pcuState // A function to create the specific state for the test case
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
					uploadCh:      make(chan uploadTask), // Unbuffered
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

			// Execute
			err := s.flushCurrentBuffer()

			// Assertions
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
			name: "Map with items already in order",
			partMap: map[int]*ObjectHandle{
				1: handle1,
				2: handle2,
				3: handle3,
			},
			expectedOrder: []*ObjectHandle{handle1, handle2, handle3},
		},
		{
			name: "Map with items in reverse order",
			partMap: map[int]*ObjectHandle{
				3: handle3,
				2: handle2,
				1: handle1,
			},
			expectedOrder: []*ObjectHandle{handle1, handle2, handle3},
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

			// Execute
			sortedParts := s.getSortedParts()

			// Assertions
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
	tests := []struct {
		name     string
		numParts int
		// We'll simulate maxComposeComponents by checking how many calls occur.
		wantIntermediates int
	}{
		{
			name:              "Under limit (Single Level)",
			numParts:          10,
			wantIntermediates: 0, // 10 < 32, direct to final
		},
		{
			name:              "Exactly limit (Single Level)",
			numParts:          32,
			wantIntermediates: 0, // 32 = 32, direct to final
		},
		{
			name:              "Over limit (Multi Level)",
			numParts:          46,
			wantIntermediates: 2, // 2 intermediate groups (32+14) merged in final
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			intermediateCount := 0

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
					o: &ObjectHandle{c: dummyClient, object: "final-dest", bucket: "b"},
				},
				config: &parallelUploadConfig{
					namingStrategy: &defaultNamingStrategy{},
				},
			}

			state.composeFn = func(ctx context.Context, c *Composer) (*ObjectAttrs, error) {
				mu.Lock()
				defer mu.Unlock()

				// If the destination isn't the final object, it's an intermediate
				if c.dst.object != "final-dest" {
					intermediateCount++
				}

				// Return the name of the destination object as the "attribute"
				return &ObjectAttrs{Name: c.dst.object}, nil
			}

			// Execute
			err := state.composeParts()
			if err != nil {
				t.Fatalf("composeParts failed: %v", err)
			}

			// Assertions
			if intermediateCount != tc.wantIntermediates {
				t.Errorf("expected %d intermediate composes, got %d", tc.wantIntermediates, intermediateCount)
			}

			if state.w.obj.Name != "final-dest" {
				t.Errorf("final object name mismatch: got %s, want final-dest", state.w.obj.Name)
			}

			if len(state.intermediateMap) != intermediateCount {
				t.Errorf("intermediate map count mismatch: got %d, want %d", len(state.intermediateMap), intermediateCount)
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
				config: &parallelUploadConfig{
					namingStrategy: &defaultNamingStrategy{},
				},
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
		strategy          partCleanupStrategy
		firstErr          error
		partsToCreate     int
		interimsToCreate  int
		failDeletesFor    map[string]bool // Dummy object names that should fail deletion
		expectDeleteCalls int
		expectFailedCount int
	}{
		{
			name:              "CleanupNever should do nothing",
			strategy:          cleanupNever,
			partsToCreate:     2,
			expectDeleteCalls: 0,
		},
		{
			name:              "CleanupOnSuccess with error should do nothing",
			strategy:          cleanupOnSuccess,
			firstErr:          errors.New("upload failed"),
			partsToCreate:     2,
			expectDeleteCalls: 0,
		},
		{
			name:              "CleanupAlways should clean up all",
			strategy:          cleanupAlways,
			firstErr:          errors.New("upload failed"),
			partsToCreate:     2,
			interimsToCreate:  1,
			expectDeleteCalls: 3,
			expectFailedCount: 0,
		},
		{
			name:             "CleanupAlways with failing deletes should record failures",
			strategy:         cleanupAlways,
			partsToCreate:    2,
			interimsToCreate: 2,
			failDeletesFor: map[string]bool{
				"part-1":    true, // This part will fail
				"interim-0": true, // This interim object will fail
			},
			expectDeleteCalls: 4,
			expectFailedCount: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pCtx, cancel := context.WithCancel(context.Background())
			var mu sync.Mutex
			deleteCalls := 0

			// Prepare the pcuState for the test
			state := &pcuState{
				ctx:             pCtx,
				cancel:          cancel,
				config:          &parallelUploadConfig{cleanupStrategy: tc.strategy, numWorkers: 4},
				firstErr:        tc.firstErr,
				partMap:         make(map[int]*ObjectHandle),
				intermediateMap: make(map[string]*ObjectHandle),
			}

			state.deleteFn = func(ctx context.Context, h *ObjectHandle) error {
				mu.Lock()
				deleteCalls++
				mu.Unlock()

				// Simulate failure based on the object's dummy name
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

			// Execute
			state.doCleanup()

			// Assertions
			if deleteCalls != tc.expectDeleteCalls {
				t.Errorf("Expected %d delete calls, but got %d", tc.expectDeleteCalls, deleteCalls)
			}

			if len(state.failedDeletes) != tc.expectFailedCount {
				t.Errorf("Expected %d failed deletes, but got %d", tc.expectFailedCount, len(state.failedDeletes))
			}
		})
	}
}

func TestPCUState_Close(t *testing.T) {
	tests := []struct {
		name          string
		numParts      int
		mockErr       error
		expectCompose bool
		expectError   bool
	}{
		{
			name:          "Successful Upload",
			numParts:      2,
			mockErr:       nil,
			expectCompose: true,
			expectError:   false,
		},
		{
			name:          "Worker Error - Aborts Compose",
			numParts:      2,
			mockErr:       fmt.Errorf("upload failed"),
			expectCompose: false,
			expectError:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			composeCalled := false
			cleanupCalled := false
			ctx := context.Background()
			objName := "test-object"

			state := &pcuState{
				ctx:      ctx,
				started:  true,
				uploadCh: make(chan uploadTask, 10),
				resultCh: make(chan uploadResult, 10),
				partMap:  make(map[int]*ObjectHandle),
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
					return nil
				},
				doCleanupFn: func(s *pcuState) {
					cleanupCalled = true
				},
			}

			// Pre-populate handles if testing success/failure
			if tc.numParts > 0 {
				for i := 1; i <= tc.numParts; i++ {
					state.partMap[i] = &ObjectHandle{object: "tmp"}
				}
			}

			if tc.mockErr != nil {
				state.firstErr = tc.mockErr
			}

			// Execute
			err := state.close()

			// Assertions
			if (err != nil) != tc.expectError {
				t.Errorf("expectError %v, got err: %v", tc.expectError, err)
			}
			if composeCalled != tc.expectCompose {
				t.Errorf("expectCompose %v, but composeCalled was %v", tc.expectCompose, composeCalled)
			}
			if !cleanupCalled {
				t.Errorf("cleanup logic was not executed")
			}
		})
	}
}

// testNamingStrategy is a mock implementation of PartNamingStrategy for testing.
type testNamingStrategy struct{}

func (t *testNamingStrategy) newPartName(bucket, prefix, finalName string, partNumber int) string {
	return "test-part"
}

type testMetadataDecorator struct {
	metadataToSet map[string]string
}

func (m *testMetadataDecorator) Decorate(attrs *ObjectAttrs) {
	for k, v := range m.metadataToSet {
		attrs.Metadata[k] = v
	}
}
