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
	t.Run("all defaults", func(t *testing.T) {
		cfg := &ParallelUploadConfig{}
		cfg.defaults()

		if *cfg.MinSize != 64*1024*1024 {
			t.Errorf("MinSize default is incorrect. got %d, want %d", *cfg.MinSize, 128*1024*1024)
		}
		if cfg.PartSize != defaultPartSize {
			t.Errorf("PartSize default is incorrect. got %d, want %d", cfg.PartSize, defaultPartSize)
		}
		expectedWorkers := min(baseWorkers+(runtime.NumCPU()/2), maxWorkers)
		if cfg.NumWorkers != expectedWorkers {
			t.Errorf("NumWorkers default is incorrect. got %d, want %d", cfg.NumWorkers, expectedWorkers)
		}
		if cfg.BufferPoolSize != expectedWorkers+1 {
			t.Errorf("BufferPoolSize default is incorrect. got %d, want %d", cfg.BufferPoolSize, expectedWorkers+1)
		}
		if cfg.TmpObjectPrefix != defaultTmpObjectPrefix {
			t.Errorf("TmpObjectPrefix default is incorrect. got %q, want %q", cfg.TmpObjectPrefix, defaultTmpObjectPrefix)
		}
		if cfg.RetryPolicy.MaxRetries != 3 {
			t.Errorf("RetryPolicy.MaxRetries default is incorrect. got %d, want %d", cfg.RetryPolicy.MaxRetries, 3)
		}
		if cfg.RetryPolicy.BaseDelay != 100*time.Millisecond {
			t.Errorf("RetryPolicy.BaseDelay default is incorrect. got %v, want %v", cfg.RetryPolicy.BaseDelay, 100*time.Millisecond)
		}
		if cfg.RetryPolicy.MaxDelay != 5*time.Second {
			t.Errorf("RetryPolicy.MaxDelay default is incorrect. got %v, want %v", cfg.RetryPolicy.MaxDelay, 5*time.Second)
		}
		if cfg.CleanupStrategy != CleanupAlways {
			t.Errorf("CleanupStrategy default is incorrect. got %v, want %v", cfg.CleanupStrategy, CleanupAlways)
		}
		if _, ok := cfg.NamingStrategy.(*DefaultNamingStrategy); !ok {
			t.Errorf("NamingStrategy default is incorrect. got %T, want %T", cfg.NamingStrategy, &DefaultNamingStrategy{})
		}
	})

	t.Run("no overrides", func(t *testing.T) {
		minSize := int64(0)
		cfg := &ParallelUploadConfig{
			MinSize:         &minSize,
			PartSize:        1024,
			NumWorkers:      10,
			BufferPoolSize:  12,
			TmpObjectPrefix: "my-prefix/",
			RetryPolicy: &PartRetryPolicy{
				MaxRetries: 5,
				BaseDelay:  200 * time.Millisecond,
				MaxDelay:   10 * time.Second,
			},
			CleanupStrategy: CleanupOnSuccess,
			NamingStrategy:  &testNamingStrategy{},
		}

		// Create a copy to compare against after calling defaults()
		originalCfg := *cfg
		originalRetryPolicy := *cfg.RetryPolicy

		cfg.defaults()

		if *cfg.MinSize != *originalCfg.MinSize {
			t.Errorf("MinSize should not be overridden. got %d, want %d", *cfg.MinSize, *originalCfg.MinSize)
		}
		if cfg.PartSize != originalCfg.PartSize {
			t.Errorf("PartSize should not be overridden. got %d, want %d", cfg.PartSize, originalCfg.PartSize)
		}
		if cfg.NumWorkers != originalCfg.NumWorkers {
			t.Errorf("NumWorkers should not be overridden. got %d, want %d", cfg.NumWorkers, originalCfg.NumWorkers)
		}
		if cfg.BufferPoolSize != originalCfg.BufferPoolSize {
			t.Errorf("BufferPoolSize should not be overridden. got %d, want %d", cfg.BufferPoolSize, originalCfg.BufferPoolSize)
		}
		if cfg.TmpObjectPrefix != originalCfg.TmpObjectPrefix {
			t.Errorf("TmpObjectPrefix should not be overridden. got %q, want %q", cfg.TmpObjectPrefix, originalCfg.TmpObjectPrefix)
		}
		if cfg.RetryPolicy.MaxRetries != originalRetryPolicy.MaxRetries {
			t.Errorf("RetryPolicy should not be overridden. got %v, want %v", cfg.RetryPolicy, originalRetryPolicy)
		}
		if cfg.CleanupStrategy != originalCfg.CleanupStrategy {
			t.Errorf("CleanupStrategy should not be overridden. got %v, want %v", cfg.CleanupStrategy, originalCfg.CleanupStrategy)
		}
		if _, ok := cfg.NamingStrategy.(*testNamingStrategy); !ok {
			t.Errorf("NamingStrategy should not be overridden. got %T, want %T", cfg.NamingStrategy, &testNamingStrategy{})
		}
	})
}

// testNamingStrategy is a mock implementation of PartNamingStrategy for testing.
type testNamingStrategy struct{}

func (t *testNamingStrategy) NewPartName(bucket, prefix, finalName string, partNumber int) string {
	return "test-part"
}
