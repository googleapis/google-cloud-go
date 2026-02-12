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
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"path"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"

	gax "github.com/googleapis/gax-go/v2"
)

// parallelUploadConfig holds configuration for Parallel Composite Uploads.
// Setting this config and EnableParallelUpload flag on Writer enables PCU.
//
// **Note:** This feature is currently experimental and its API surface may change
// in future releases. It is not yet recommended for production use.
type parallelUploadConfig struct {
	// minSize is the minimum size of an object in bytes to use PCU.
	// If an object's size is less than this value, a simple upload is performed.
	// If this is not set, a default of 64 MiB will be used.
	// To enable PCU for all uploads regardless of size, set this to 0.
	minSize *int64

	// partSize is the size of each part to be uploaded in parallel.
	// Defaults to 16MiB. Must be a multiple of 256KiB.
	partSize int

	// numWorkers is the number of goroutines to use for uploading parts in parallel.
	// Defaults to a dynamic value based on the number of CPUs (min(4 + NumCPU/2, 16)).
	numWorkers int

	// bufferPoolSize is the number of PartSize buffers to pool.
	// Defaults to NumWorkers + 1.
	bufferPoolSize int

	// tmpObjectPrefix is the prefix for temporary object names.
	// Defaults to "gcs-go-sdk-pcu-tmp/".
	tmpObjectPrefix string

	// retryOptions defines the retry behavior for uploading parts.
	// Defaults to a sensible policy for part uploads (e.g., max 3 retries).
	retryOptions []RetryOption

	// cleanupStrategy dictates how temporary parts are cleaned up.
	// Defaults to CleanupAlways.
	cleanupStrategy partCleanupStrategy

	// namingStrategy provides a strategy for naming temporary part objects.
	// Defaults to a strategy that includes a random element to avoid hotspotting.
	namingStrategy partNamingStrategy

	// metadataDecorator allows adding custom metadata to temporary part objects.
	metadataDecorator partMetadataDecorator
}

// partCleanupStrategy defines when temporary objects are deleted.
type partCleanupStrategy int

const (
	// cleanupAlways clean up temporary parts on both success and failure.
	cleanupAlways partCleanupStrategy = iota
	// cleanupOnSuccess clean up temporary parts only on successful final composition.
	cleanupOnSuccess
	// cleanupNever means the application is responsible for cleaning up temporary parts.
	cleanupNever
)

func (s partCleanupStrategy) String() string {
	switch s {
	case cleanupAlways:
		return "always"
	case cleanupOnSuccess:
		return "on_success"
	case cleanupNever:
		return "never"
	default:
		return fmt.Sprintf("PartCleanupStrategy(%d)", s)
	}
}

// partNamingStrategy interface for generating temporary object names.
type partNamingStrategy interface {
	newPartName(bucket, prefix, finalName string, partNumber int) string
}

// defaultNamingStrategy provides a default implementation for naming temporary parts.
type defaultNamingStrategy struct{}

// newPartName creates a unique name for a temporary part to avoid hotspotting.
func (d *defaultNamingStrategy) newPartName(bucket, prefix, finalName string, partNumber int) string {
	rnd := generateRandomBytes(4)
	return path.Join(prefix, fmt.Sprintf("%x-%s-part-%d", rnd, finalName, partNumber))
}

// partMetadataDecorator interface for modifying temporary object metadata.
type partMetadataDecorator interface {
	Decorate(attrs *ObjectAttrs)
}

const (
	defaultPartSize           = 16 * 1024 * 1024 // 16 MiB
	defaultMinSize            = 64 * 1024 * 1024 // 64 MiB
	baseWorkers               = 4
	maxWorkers                = 16
	defaultTmpObjectPrefix    = "gcs-go-sdk-pcu-tmp/"
	maxComposeComponents      = 32
	defaultMaxRetries         = 3
	defaultBaseDelay          = 100 * time.Millisecond
	defaultMaxDelay           = 5 * time.Second
	pcuPartNumberMetadataKey  = "x-goog-meta-gcs-pcu-part-number"
	pcuFinalObjectMetadataKey = "x-goog-meta-gcs-pcu-final-object"
)

func (c *parallelUploadConfig) defaults() {
	if c.minSize == nil {
		c.minSize = new(int64)
		*c.minSize = defaultMinSize
	}
	if c.partSize == 0 {
		c.partSize = defaultPartSize
	}
	// Use a heuristic for the number of workers: start with 4, add 1 for
	// every 2 CPUs, but don't exceed a cap of 16. This provides a
	// balance between parallelism and resource contention.
	if c.numWorkers == 0 {
		c.numWorkers = min(baseWorkers+(runtime.NumCPU()/2), maxWorkers)
	}
	if c.bufferPoolSize == 0 {
		c.bufferPoolSize = c.numWorkers + 1
	}
	if c.tmpObjectPrefix == "" {
		c.tmpObjectPrefix = defaultTmpObjectPrefix
	}
	if c.retryOptions == nil {
		c.retryOptions = []RetryOption{
			WithMaxAttempts(defaultMaxRetries),
			WithBackoff(gax.Backoff{
				Initial: defaultBaseDelay,
				Max:     defaultMaxDelay,
			}),
		}
	}
	if c.cleanupStrategy == 0 {
		c.cleanupStrategy = cleanupAlways
	}
	if c.namingStrategy == nil {
		c.namingStrategy = &defaultNamingStrategy{}
	}
}

type pcuState struct {
	ctx    context.Context
	cancel context.CancelFunc
	w      *Writer
	config *parallelUploadConfig

	mu sync.Mutex
	// Handles to the uploaded temporary parts, keyed by partNumber.
	partMap map[int]*ObjectHandle
	// Handles to intermediate composite objects, keyed by their object name.
	intermediateMap map[string]*ObjectHandle
	errOnce         sync.Once
	firstErr        error
	errors          []error
	partNum         int
	currentBuffer   []byte
	bytesBuffered   int64

	bufferCh    chan []byte
	uploadCh    chan uploadTask
	resultCh    chan uploadResult
	workerWG    sync.WaitGroup
	collectorWG sync.WaitGroup
	started     bool

	// Function to upload a part; can be overridden for testing.
	uploadPartFn func(s *pcuState, task uploadTask) (*ObjectHandle, *ObjectAttrs, error)
	// Function to delete an object; can be overridden for testing.
	deleteFn func(ctx context.Context, h *ObjectHandle) error
	// Function to perform cleanup; can be overridden for testing.
	doCleanupFn func(s *pcuState)
	// Function to compose parts; can be overridden for testing.
	composePartsFn func(s *pcuState) error
	// Function to run the compose operation; can be overridden for testing.
	composeFn func(ctx context.Context, composer *Composer) (*ObjectAttrs, error)
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

func (w *Writer) initPCU(ctx context.Context) error {
	// TODO: Check if PCU is enabled on the Writer.

	// TODO: Get the config from the Writer.
	cfg := &parallelUploadConfig{}
	cfg.defaults()

	// Ensure PartSize is a multiple of googleapi.MinUploadChunkSize.
	cfg.partSize = gRPCChunkSize(cfg.partSize)

	pCtx, cancel := context.WithCancel(ctx)

	state := &pcuState{
		ctx:             pCtx,
		cancel:          cancel,
		w:               w,
		config:          cfg,
		bufferCh:        make(chan []byte, cfg.bufferPoolSize),
		uploadCh:        make(chan uploadTask, cfg.numWorkers), // Buffered to prevent worker starvation
		resultCh:        make(chan uploadResult),
		partMap:         make(map[int]*ObjectHandle),
		intermediateMap: make(map[string]*ObjectHandle),
		uploadPartFn:    (*pcuState).uploadPart,
		deleteFn: func(ctx context.Context, h *ObjectHandle) error {
			return h.Delete(ctx)
		},
		doCleanupFn:    (*pcuState).doCleanup,
		composePartsFn: (*pcuState).composeParts,
		composeFn: func(ctx context.Context, c *Composer) (*ObjectAttrs, error) {
			return c.Run(ctx)
		},
	}
	// TODO: Assign the state to the Writer

	for i := 0; i < cfg.bufferPoolSize; i++ {
		state.bufferCh <- make([]byte, cfg.partSize)
	}

	state.workerWG.Add(cfg.numWorkers)
	for i := 0; i < cfg.numWorkers; i++ {
		go state.worker()
	}

	state.collectorWG.Add(1)
	go state.resultCollector()

	// Handle to get the first buffer.
	select {
	case <-state.ctx.Done():
		return state.ctx.Err()
	case state.currentBuffer = <-state.bufferCh:
		state.bytesBuffered = 0
	}
	state.started = true
	return nil
}

// worker processes upload tasks from upload channel, reporting results
// and returning buffers to the pool.
func (s *pcuState) worker() {
	defer s.workerWG.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case task, ok := <-s.uploadCh:
			if !ok {
				return
			}
			func(t uploadTask) {
				// Ensure the buffer is returned to the pool.
				defer func() { s.bufferCh <- t.buffer }()
				// This handles the case where cancellation happens before we begin upload.
				select {
				case <-s.ctx.Done():
					s.resultCh <- uploadResult{partNumber: t.partNumber, err: s.ctx.Err()}
					return
				default:
				}

				handle, attrs, err := s.uploadPartFn(s, t)

				// Always send a result to the collector.
				s.resultCh <- uploadResult{partNumber: t.partNumber, obj: attrs, handle: handle, err: err}
			}(task)
		}
	}
}

// TODO: add retry logic.
func (s *pcuState) uploadPart(task uploadTask) (*ObjectHandle, *ObjectAttrs, error) {
	partName := s.config.namingStrategy.newPartName(s.w.o.bucket, s.config.tmpObjectPrefix, s.w.o.object, task.partNumber)
	partHandle := s.w.o.c.Bucket(s.w.o.bucket).Object(partName)

	pw := partHandle.NewWriter(s.ctx)
	pw.ObjectAttrs.Name = partName
	pw.ObjectAttrs.Size = task.size
	pw.DisableAutoChecksum = s.w.DisableAutoChecksum
	pw.ChunkSize = 0 // Force single-shot upload for parts.
	// Clear fields not applicable to parts or that are set by compose.
	pw.ObjectAttrs.MD5 = nil
	setPartMetadata(pw, s, task)

	if _, err := pw.Write(task.buffer[:task.size]); err != nil {
		_ = pw.CloseWithError(err)
		return nil, nil, fmt.Errorf("failed to write part %d: %w", task.partNumber, err)
	}

	if err := pw.Close(); err != nil {
		return nil, nil, fmt.Errorf("failed to close part %d: %w", task.partNumber, err)
	}

	return partHandle, pw.Attrs(), nil
}

func setPartMetadata(pw *Writer, s *pcuState, task uploadTask) {
	partNumberStr := strconv.Itoa(task.partNumber)
	var md map[string]string
	if s.w.ObjectAttrs.Metadata != nil {
		md = maps.Clone(s.w.ObjectAttrs.Metadata)
	} else {
		md = make(map[string]string)
	}
	pw.ObjectAttrs.Metadata = md
	pw.ObjectAttrs.Metadata[pcuPartNumberMetadataKey] = partNumberStr
	pw.ObjectAttrs.Metadata[pcuFinalObjectMetadataKey] = s.w.o.object
	if s.config.metadataDecorator != nil {
		s.config.metadataDecorator.Decorate(&pw.ObjectAttrs)
	}
}

func (s *pcuState) resultCollector() {
	defer s.collectorWG.Done()
	for result := range s.resultCh {
		if result.err != nil {
			s.setError(result.err)
		} else if result.handle != nil {
			s.mu.Lock()
			s.partMap[result.partNumber] = result.handle
			s.mu.Unlock()
		}
	}
}

func (s *pcuState) setError(err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errors = append(s.errors, err)

	s.errOnce.Do(func() {
		s.firstErr = err
		s.cancel() // Cancel context on first error.
	})
}

func (s *pcuState) write(p []byte) (int, error) {
	if !s.started {
		return 0, fmt.Errorf("pcuState not started")
	}
	s.mu.Lock()
	err := s.firstErr
	s.mu.Unlock()
	if err != nil {
		return 0, err
	}

	total := len(p)
	for len(p) > 0 {
		// Acquire a buffer from the pool if we don't have one.
		if s.currentBuffer == nil {
			// Fail-fast check before taking a new buffer.
			s.mu.Lock()
			err = s.firstErr
			s.mu.Unlock()
			if err != nil {
				return total - len(p), err
			}

			select {
			case <-s.ctx.Done():
				return total - len(p), s.ctx.Err()
			case s.currentBuffer = <-s.bufferCh:
				s.bytesBuffered = 0
			}
		}

		n := copy(s.currentBuffer[s.bytesBuffered:], p)
		s.bytesBuffered += int64(n)
		p = p[n:]

		// If the buffer is full, dispatch it to a worker.
		if s.bytesBuffered == int64(s.config.partSize) {
			if err := s.flushCurrentBuffer(); err != nil {
				return total - len(p), err
			}
		}
	}
	return total, nil
}

func (s *pcuState) flushCurrentBuffer() error {
	if s.bytesBuffered == 0 {
		return nil
	}

	// Capture state for the task while under lock, then release immediately.
	s.mu.Lock()
	if s.firstErr != nil {
		s.mu.Unlock()
		return s.firstErr
	}
	s.partNum++
	task := uploadTask{
		partNumber: s.partNum,
		buffer:     s.currentBuffer,
		size:       s.bytesBuffered,
	}
	s.mu.Unlock()

	// Clear current state so the next Write call picks up a fresh buffer.
	s.currentBuffer = nil
	s.bytesBuffered = 0

	// Dispatch the task. Using a select ensures we don't hang indefinitely
	// if the context is cancelled while the upload queue is full.
	select {
	case <-s.ctx.Done():
		// Return buffer to pool if we couldn't dispatch.
		s.bufferCh <- task.buffer
		return s.ctx.Err()
	case s.uploadCh <- task:
		return nil
	}
}

func (s *pcuState) close() error {
	if !s.started {
		return nil
	}

	// Flush the final partial buffer if it exists.
	if err := s.flushCurrentBuffer(); err != nil {
		s.setError(err)
	}

	// Wait for workers, then close resultCh.
	// This prevents "send on closed channel" panics.
	close(s.uploadCh)
	s.workerWG.Wait()
	close(s.resultCh)
	s.collectorWG.Wait()

	// Cleanup is always attempted.
	defer s.doCleanupFn(s)

	s.mu.Lock()
	err := s.firstErr
	s.mu.Unlock()

	if err != nil {
		return err
	}

	// If no parts were actually uploaded (e.g. empty file),
	// fall back to a standard empty object creation.
	if len(s.partMap) == 0 {
		ow := s.w.o.NewWriter(s.w.ctx)
		if ow == nil {
			return fmt.Errorf("failed to create writer for empty object")
		}
		ow.ObjectAttrs = s.w.ObjectAttrs
		return ow.Close()
	}

	// Perform the recursive composition of parts.
	if err := s.composePartsFn(s); err != nil {
		s.setError(err)
		return err
	}
	return nil
}

// getSortedParts returns the uploaded parts sorted by part number.
func (s *pcuState) getSortedParts() []*ObjectHandle {
	keys := make([]int, 0, len(s.partMap))
	for k := range s.partMap {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	parts := make([]*ObjectHandle, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, s.partMap[k])
	}
	return parts
}

// composeParts performs the multi-level compose operation to create the final object.
func (s *pcuState) composeParts() error {
	finalComps := s.getSortedParts()
	level := 0

	for len(finalComps) > maxComposeComponents {
		level++
		numIntermediates := (len(finalComps) + maxComposeComponents - 1) / maxComposeComponents
		nextLevel := make([]*ObjectHandle, numIntermediates)

		var wg sync.WaitGroup
		// Use a thread-safe way to capture the first error at this level.
		var levelErr error
		var errOnce sync.Once

		for i := 0; i < numIntermediates; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				start := idx * maxComposeComponents
				end := min(start+maxComposeComponents, len(finalComps))

				// Level-based naming with hash to prevent exceeding 1024 bytes.
				h := hex.EncodeToString(generateRandomBytes(4))
				compName := path.Join(s.config.tmpObjectPrefix, fmt.Sprintf("int-%s-lv%d-%d", h, level, idx))

				interHandle := s.w.o.c.Bucket(s.w.o.bucket).Object(compName)
				composer := interHandle.ComposerFrom(finalComps[start:end]...)

				attrs, err := s.composeFn(s.ctx, composer)
				if err != nil {
					errOnce.Do(func() { levelErr = err })
					return
				}

				s.mu.Lock()
				s.intermediateMap[attrs.Name] = interHandle
				s.mu.Unlock()
				nextLevel[idx] = interHandle
			}(i)
		}
		wg.Wait()
		if levelErr != nil {
			return levelErr
		}
		finalComps = nextLevel
	}

	// Final Compose
	composer := s.w.o.ComposerFrom(finalComps...)
	composer.ObjectAttrs = s.w.ObjectAttrs
	composer.KMSKeyName = s.w.ObjectAttrs.KMSKeyName
	composer.SendCRC32C = s.w.SendCRC32C

	attrs, err := s.composeFn(s.ctx, composer)
	if err != nil {
		return err
	}
	s.w.obj = attrs
	return nil
}

func (s *pcuState) doCleanup() {
	skip := s.config.cleanupStrategy == cleanupNever ||
		(s.config.cleanupStrategy == cleanupOnSuccess && s.firstErr != nil)

	if skip || (len(s.partMap) == 0 && len(s.intermediateMap) == 0) {
		return
	}

	var wg sync.WaitGroup

	// Semaphore to avoid spawning too many goroutines for deletion.
	sem := make(chan struct{}, s.config.numWorkers)

	runDelete := func(h *ObjectHandle) {
		defer wg.Done()
		sem <- struct{}{}
		defer func() { <-sem }()

		// Use WithoutCancel to ensure cleanup isn't killed by parent context cancellation
		// Ignore cleanup errors here since its best effort and will rely on bucket
		// lifecycle policies if cleanup fails.
		_ = s.deleteFn(context.WithoutCancel(s.ctx), h)
	}

	for _, h := range s.partMap {
		wg.Add(1)
		go runDelete(h)
	}
	for _, h := range s.intermediateMap {
		wg.Add(1)
		go runDelete(h)
	}
	wg.Wait()
}

// Generates size random bytes.
func generateRandomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}
