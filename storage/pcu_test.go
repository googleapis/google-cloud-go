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
	"fmt"
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

// testNamingStrategy is a mock implementation of PartNamingStrategy for testing.
type testNamingStrategy struct{}

func (t *testNamingStrategy) NewPartName(bucket, prefix, finalName string, partNumber int) string {
	return "test-part"
}
