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
	"math/rand"
	"path"
	"runtime"
	"sync"
	"time"

	gax "github.com/googleapis/gax-go/v2"
)

// ParallelUploadConfig holds configuration for Parallel Composite Uploads.
// Setting this config and EnableParallelUpload flag on Writer enables PCU.
//
// **Note:** This feature is currently experimental and its API surface may change
// in future releases. It is not yet recommended for production use.
type ParallelUploadConfig struct {
	// MinSize is the minimum size of an object in bytes to use PCU.
	// If an object's size is less than this value, a simple upload is performed.
	// If this is not set, a default of 64 MiB will be used.
	// To enable PCU for all uploads regardless of size, set this to 0.
	MinSize *int64

	// PartSize is the size of each part to be uploaded in parallel.
	// Defaults to 16MiB. Must be a multiple of 256KiB.
	PartSize int64

	// NumWorkers is the number of goroutines to use for uploading parts in parallel.
	// Defaults to a dynamic value based on the number of CPUs (min(4 + NumCPU/2, 16)).
	NumWorkers int

	// BufferPoolSize is the number of PartSize buffers to pool.
	// Defaults to NumWorkers + 1.
	BufferPoolSize int

	// TmpObjectPrefix is the prefix for temporary object names.
	// Defaults to "gcs-go-sdk-pcu-tmp/".
	TmpObjectPrefix string

	// RetryOptions defines the retry behavior for uploading parts.
	// Defaults to a sensible policy for part uploads (e.g., max 3 retries).
	RetryOptions []RetryOption

	// CleanupStrategy dictates how temporary parts are cleaned up.
	// Defaults to CleanupAlways.
	CleanupStrategy PartCleanupStrategy

	// NamingStrategy provides a strategy for naming temporary part objects.
	// Defaults to a strategy that includes a random element to avoid hotspotting.
	NamingStrategy PartNamingStrategy

	// MetadataDecorator allows adding custom metadata to temporary part objects.
	MetadataDecorator PartMetadataDecorator
}

// PartCleanupStrategy defines when temporary objects are deleted.
type PartCleanupStrategy int

const (
	// CleanupAlways clean up temporary parts on both success and failure.
	CleanupAlways PartCleanupStrategy = iota
	// CleanupOnSuccess clean up temporary parts only on successful final composition.
	CleanupOnSuccess
	// CleanupNever means the application is responsible for cleaning up temporary parts.
	CleanupNever
)

func (s PartCleanupStrategy) String() string {
	switch s {
	case CleanupAlways:
		return "always"
	case CleanupOnSuccess:
		return "on_success"
	case CleanupNever:
		return "never"
	default:
		return fmt.Sprintf("PartCleanupStrategy(%d)", s)
	}
}

// PartNamingStrategy interface for generating temporary object names.
type PartNamingStrategy interface {
	NewPartName(bucket, prefix, finalName string, partNumber int) string
}

// DefaultNamingStrategy provides a default implementation for naming temporary parts.
type DefaultNamingStrategy struct{}

// NewPartName creates a unique name for a temporary part to avoid hotspotting.
func (d *DefaultNamingStrategy) NewPartName(bucket, prefix, finalName string, partNumber int) string {
	rnd := rand.Uint64()
	return path.Join(prefix, fmt.Sprintf("%x-%s-part-%d", rnd, finalName, partNumber))
}

// PartMetadataDecorator interface for modifying temporary object metadata.
type PartMetadataDecorator interface {
	Decorate(attrs *ObjectAttrs)
}

const (
	defaultPartSize        = 16 * 1024 * 1024 // 16 MiB
	defaultMinSize         = 64 * 1024 * 1024 // 64 MiB
	baseWorkers            = 4
	maxWorkers             = 16
	defaultTmpObjectPrefix = "gcs-go-sdk-pcu-tmp/"
	maxComposeComponents   = 32
	defaultMaxRetries      = 3
	defaultBaseDelay       = 100 * time.Millisecond
	defaultMaxDelay        = 5 * time.Second
)

func (c *ParallelUploadConfig) defaults() {
	if c.MinSize == nil {
		c.MinSize = new(int64)
		*c.MinSize = defaultMinSize
	}
	if c.PartSize == 0 {
		c.PartSize = defaultPartSize
	}
	// Use a heuristic for the number of workers: start with 4, add 1 for
	// every 2 CPUs, but don't exceed a cap of 16. This provides a
	// balance between parallelism and resource contention.
	if c.NumWorkers == 0 {
		c.NumWorkers = min(baseWorkers+(runtime.NumCPU()/2), maxWorkers)
	}
	if c.BufferPoolSize == 0 {
		c.BufferPoolSize = c.NumWorkers + 1
	}
	if c.TmpObjectPrefix == "" {
		c.TmpObjectPrefix = defaultTmpObjectPrefix
	}
	if c.RetryOptions == nil {
		c.RetryOptions = []RetryOption{
			WithMaxAttempts(defaultMaxRetries),
			WithBackoff(gax.Backoff{
				Initial: defaultBaseDelay,
				Max:     defaultMaxDelay,
			}),
		}
	}
	if c.CleanupStrategy == 0 {
		c.CleanupStrategy = CleanupAlways
	}
	if c.NamingStrategy == nil {
		c.NamingStrategy = &DefaultNamingStrategy{}
	}
}

type pcuState struct {
	ctx    context.Context
	cancel context.CancelFunc
	w      *Writer
	config *ParallelUploadConfig

	mu sync.Mutex
	// Handles to the uploaded temporary parts, keyed by partNumber.
	partMap map[int]*ObjectHandle
	// Handles to intermediate composite objects, keyed by their object name.
	intermediateMap map[string]*ObjectHandle
	failedDeletes   []*ObjectHandle
	errOnce         sync.Once
	firstErr        error
	partNum         int
	currentBuffer   []byte
	bytesBuffered   int64

	bufferCh    chan []byte
	uploadCh    chan uploadTask
	resultCh    chan uploadResult
	workerWG    sync.WaitGroup
	collectorWG sync.WaitGroup
	started     bool
}

type uploadTask struct {
	partNumber int
	buffer     []byte
	size       int64
}

type uploadResult struct {
	partNumber int
	obj        *ObjectAttrs
	handle     *ObjectHandle
	err        error
}
