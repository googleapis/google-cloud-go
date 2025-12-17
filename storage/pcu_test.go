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
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	gax "github.com/googleapis/gax-go/v2"
)

func TestPartCleanupStrategy_String(t *testing.T) {
	tests := []struct {
		strategy PartCleanupStrategy
		want     string
	}{
		{CleanupAlways, "always"},
		{CleanupOnSuccess, "on_success"},
		{CleanupNever, "never"},
		{PartCleanupStrategy(99), "PartCleanupStrategy(99)"},
		{PartCleanupStrategy(-1), "PartCleanupStrategy(-1)"},
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
	strategy := &DefaultNamingStrategy{}
	bucket := "my-bucket"
	prefix := "gcs-go-sdk-pcu-tmp/"
	finalName := "my-object"
	partNumber := 42

	partName := strategy.NewPartName(bucket, prefix, finalName, partNumber)

	if !strings.HasPrefix(partName, prefix) {
		t.Errorf("NewPartName() should start with the prefix %q, but got %q", prefix, partName)
	}

	expectedFormat := prefix + "%x-" + finalName + "-part-%d"
	var randSuffix uint64
	var parsedPartNum int

	_, err := fmt.Sscanf(partName, expectedFormat, &randSuffix, &parsedPartNum)
	if err != nil {
		t.Errorf("NewPartName() returned a name with an unexpected format. Got %q, want format ~%q. Error: %v", partName, prefix+"<hex>-"+finalName+"-part-<int>", err)
		return // Return to avoid further checks if parsing failed
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
		in   *ParallelUploadConfig
		want *ParallelUploadConfig
	}{
		{
			name: "all defaults",
			in:   &ParallelUploadConfig{},
			want: &ParallelUploadConfig{
				MinSize:         &defaultMinSizeVal,
				PartSize:        defaultPartSize,
				NumWorkers:      expectedWorkers,
				BufferPoolSize:  expectedWorkers + 1,
				TmpObjectPrefix: defaultTmpObjectPrefix,
				RetryOptions: []RetryOption{
					WithMaxAttempts(defaultMaxRetries),
					WithBackoff(gax.Backoff{
						Initial: defaultBaseDelay,
						Max:     defaultMaxDelay,
					}),
				},
				CleanupStrategy: CleanupAlways,
				NamingStrategy:  &DefaultNamingStrategy{},
			},
		},
		{
			name: "user-provided values are respected",
			in: &ParallelUploadConfig{
				MinSize:         &userMinSizeVal,
				PartSize:        int64(1024),
				NumWorkers:      10,
				BufferPoolSize:  12,
				TmpObjectPrefix: "my-prefix/",
				RetryOptions: []RetryOption{
					WithMaxAttempts(5),
					WithBackoff(gax.Backoff{
						Initial: 200 * time.Millisecond,
						Max:     10 * time.Second,
					}),
				},
				CleanupStrategy: CleanupOnSuccess,
				NamingStrategy:  &testNamingStrategy{},
			},
			want: &ParallelUploadConfig{
				MinSize:         &userMinSizeVal,
				PartSize:        int64(1024),
				NumWorkers:      10,
				BufferPoolSize:  12,
				TmpObjectPrefix: "my-prefix/",
				RetryOptions: []RetryOption{
					WithMaxAttempts(5),
					WithBackoff(gax.Backoff{
						Initial: 200 * time.Millisecond,
						Max:     10 * time.Second,
					}),
				},
				CleanupStrategy: CleanupOnSuccess,
				NamingStrategy:  &testNamingStrategy{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.in
			cfg.defaults()
			if diff := cmp.Diff(tt.want, cfg,
				cmp.AllowUnexported(DefaultNamingStrategy{}, testNamingStrategy{}, withMaxAttempts{}, withBackoff{}),
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

	if state.firstErr != err1 {
		t.Errorf("setError() first call: got %v, want %v", state.firstErr, err1)
	}

	select {
	case <-state.ctx.Done():
		// Correctly cancelled
		if state.ctx.Err() != context.Canceled {
			t.Errorf("setError() first call: context error = %v, want %v", state.ctx.Err(), context.Canceled)
		}
	default:
		t.Errorf("setError() first call: context not cancelled")
	}

	err2 := fmt.Errorf("second error")
	state.setError(err2)

	if state.firstErr != err1 {
		t.Errorf("setError() second call: got %v, want %v (should not change)", state.firstErr, err1)
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

	// Successful result
	objHandle1 := &ObjectHandle{object: "part1"}
	state.resultCh <- uploadResult{partNumber: 1, handle: objHandle1, err: nil}

	// Error result
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

	// Check if context is cancelled
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
	state.bufferCh <- buffer // Pre-fill buffer

	state.workerWG.Add(1)
	go state.worker()

	task := uploadTask{partNumber: 1, buffer: buffer, size: 10}
	state.uploadCh <- task

	select {
	case result := <-state.resultCh:
		if result.err != nil {
			t.Errorf("worker unexpected error: %v", result.err)
		}
		if result.partNumber != 1 || result.handle == nil || result.handle.object != "mockPart" {
			t.Errorf("worker result mismatch: got %v", result)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("worker timeout waiting for result")
	}

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
	state.bufferCh <- buffer

	state.workerWG.Add(1)
	go state.worker()

	task := uploadTask{partNumber: 1, buffer: buffer, size: 10}
	state.uploadCh <- task

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
	uploadStarted := make(chan struct{}) // To signal when upload starts

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

func TestPCUWorker_ChannelClose(t *testing.T) {
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

	close(state.uploadCh)
	state.workerWG.Wait() // Should not block indefinitely

	select {
	case res := <-state.resultCh:
		t.Errorf("Unexpected result on closed channel: %v", res)
	default:
	}
}

func TestDefaultNamingStrategy_NewPartName_Uniqueness(t *testing.T) {
	strategy := &DefaultNamingStrategy{}
	bucket := "my-bucket"
	prefix := "gcs-go-sdk-pcu-tmp/"
	finalName := "my-object"
	partNumber := 42

	name1 := strategy.NewPartName(bucket, prefix, finalName, partNumber)
	name2 := strategy.NewPartName(bucket, prefix, finalName, partNumber)

	if name1 == name2 {
		t.Errorf("NewPartName() returned the same name twice: %q", name1)
	}
}

func TestSetPartMetadata(t *testing.T) {
	testCases := []struct {
		name             string
		initialMetadata  map[string]string
		decorator        PartMetadataDecorator
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
				xGoogMetaGcsPCUPartNumber:  "1",
				xGoogMetaGcsPCUFinalObject: "final-object",
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
				"initial-key":              "initial-value",
				xGoogMetaGcsPCUPartNumber:  "2",
				xGoogMetaGcsPCUFinalObject: "final-object",
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
				"decorated-key":            "decorated-value",
				xGoogMetaGcsPCUPartNumber:  "3",
				xGoogMetaGcsPCUFinalObject: "final-object",
			},
		},
		{
			name: "Existing metadata, with decorator that overwrites",
			initialMetadata: map[string]string{
				"initial-key":             "initial-value",
				xGoogMetaGcsPCUPartNumber: "should-be-overwritten",
			},
			decorator: &testMetadataDecorator{
				metadataToSet: map[string]string{
					"decorated-key":            "decorated-value",
					xGoogMetaGcsPCUFinalObject: "overwritten-by-decorator",
				},
			},
			task:            uploadTask{partNumber: 4},
			finalObjectName: "final-object-base",
			expectedMetadata: map[string]string{
				"initial-key":              "initial-value",
				"decorated-key":            "decorated-value",
				xGoogMetaGcsPCUPartNumber:  "4",                        // Overwrites initial
				xGoogMetaGcsPCUFinalObject: "overwritten-by-decorator", // Overwritten by decorator
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			sourceWriter := &Writer{
				ObjectAttrs: ObjectAttrs{
					Metadata: tc.initialMetadata,
				},
			}
			partWriter := &Writer{
				o: &ObjectHandle{object: tc.finalObjectName},
			}
			state := &pcuState{
				w: sourceWriter,
				config: &ParallelUploadConfig{
					MetadataDecorator: tc.decorator,
				},
			}

			// Execute
			setPartMetadata(partWriter, state, tc.task)

			// Verify
			if !reflect.DeepEqual(partWriter.ObjectAttrs.Metadata, tc.expectedMetadata) {
				t.Errorf("Metadata mismatch:\ngot:  %v\nwant: %v", partWriter.ObjectAttrs.Metadata, tc.expectedMetadata)
			}
		})
	}
}

// testNamingStrategy is a mock implementation of PartNamingStrategy for testing.
type testNamingStrategy struct{}

func (t *testNamingStrategy) NewPartName(bucket, prefix, finalName string, partNumber int) string {
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
