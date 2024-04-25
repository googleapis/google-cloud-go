// Copyright 2024 Google LLC
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

package transfermanager

import (
	"runtime"
	"time"
)

// A TransferManagerOption is an option for a transfermanager Downloader or Uploader.
type TransferManagerOption interface {
	apply(*transferManagerConfig)
}

// WithWorkers returns a TransferManagerOption that specifies the maximum number
// of concurrent goroutines that will be used to download or upload objects.
// Defaults to runtime.NumCPU()/2.
func WithWorkers(numWorkers int) TransferManagerOption {
	return &withWorkers{numWorkers: numWorkers}
}

type withWorkers struct {
	numWorkers int
}

func (ww withWorkers) apply(tm *transferManagerConfig) {
	tm.numWorkers = ww.numWorkers
}

// WithPartSize returns a TransferManagerOption that specifies the size of the
// shards to transfer; that is, if the object is larger than this size, it will
// be uploaded or downloaded in concurrent pieces.
// The default is 32 MiB for downloads.
// NOTE: Sharding is not yet implemented.
func WithPartSize(partSize int) TransferManagerOption {
	return &withPartSize{partSize: partSize}
}

type withPartSize struct {
	partSize int
}

func (wps withPartSize) apply(tm *transferManagerConfig) {
	tm.partSize = wps.partSize
}

// WithPerOpTimeout returns a TransferManagerOption that sets a timeout on each
// operation that is performed to download or upload an object. The timeout is
// set when the operation begins processing, not when it is added.
// By default, no timeout is set.
func WithPerOpTimeout(timeout time.Duration) TransferManagerOption {
	return &withPerOpTimeout{timeout: timeout}
}

type withPerOpTimeout struct {
	timeout time.Duration
}

func (wpt withPerOpTimeout) apply(tm *transferManagerConfig) {
	tm.perOperationTimeout = wpt.timeout
}

type transferManagerConfig struct {
	// Workers in thread pool; default numCPU/2 based on previous benchmarks?
	numWorkers int
	// Size of shards to transfer; Python found 32 MiB to be good default for
	// JSON downloads but gRPC may benefit from larger.
	partSize int
	// Timeout for a single operation (including all retries). Zero value means
	// no timeout.
	perOperationTimeout time.Duration
}

func defaultTransferManagerConfig() *transferManagerConfig {
	return &transferManagerConfig{
		numWorkers:          runtime.NumCPU() / 2,
		partSize:            32 * 1024 * 1024, // 32 MiB
		perOperationTimeout: 0,                // no timeout
	}
}

// initTransferManagerConfig initializes a config with the defaults and applies
// the options passed in.
func initTransferManagerConfig(opts ...TransferManagerOption) *transferManagerConfig {
	config := defaultTransferManagerConfig()
	for _, o := range opts {
		o.apply(config)
	}
	return config
}
