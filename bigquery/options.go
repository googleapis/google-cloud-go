// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bigquery

import (
	"runtime"

	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
)

// newReaderClientSettings builds a client config based on package-specific custom ClientOptions.
func newReaderClientSettings(opts ...option.ClientOption) readClientSettings {
	settings := defaultReadClientSettings()
	for _, opt := range opts {
		if rOpt, ok := opt.(readerClientOption); ok {
			rOpt.ApplyReaderOpt(&settings)
		}
	}

	if settings.maxStreamCount < 0 {
		settings.maxStreamCount = 0
	}
	if settings.maxWorkerCount < 0 {
		settings.maxWorkerCount = 1
	}
	return settings
}

type readClientSettings struct {
	maxStreamCount int
	maxWorkerCount int
	opt            gax.CallOption
}

func defaultReadClientSettings() readClientSettings {
	maxWorkerCount := runtime.GOMAXPROCS(0)
	return readClientSettings{
		// with zero, the server will provide a value of streams so as to produce reasonable throughput
		maxStreamCount: 0,
		maxWorkerCount: maxWorkerCount,
	}
}

// WithStorageReaderOptions is an EXPERIMENTAL ClientOption for controlling
// the usage of the BigQuery Storage Read API when enabled via the
// EnableStorageReadClient method.
//
// This ClientOption is EXPERIMENTAL and subject to change.
func WithStorageReaderOptions(opts ...ReaderOption) option.ClientOption {
	return &readerOptions{opts: opts}
}

// readerClientOption allows us to extend ClientOptions for client-specific needs.
type readerClientOption interface {
	option.ClientOption
	ApplyReaderOpt(*readClientSettings)
}

type readerOptions struct {
	internaloption.EmbeddableAdapter
	opts []ReaderOption
}

func (s *readerOptions) ApplyReaderOpt(c *readClientSettings) {
	for _, opt := range s.opts {
		opt(c)
	}
}

// ReaderOption are variadic options used to configure a Storage Read Client.
type ReaderOption func(*readClientSettings)

// WithMaxStreamCount set the max initial number of streams. If unset or zero, the server will
// provide a value of streams so as to produce reasonable throughput. Must be
// non-negative. The number of streams may be lower than the requested number,
// depending on the amount parallelism that is reasonable for the table.
// There is a default system max limit of 1,000.
func WithMaxStreamCount(n int) ReaderOption {
	return func(rc *readClientSettings) {
		rc.maxStreamCount = n
	}
}

// WithMaxWorkerCount set the number of goroutines to process ReadStreams.
// This is typically a target parallelism of the client to ensure
// good CPU utilization.
func WithMaxWorkerCount(n int) ReaderOption {
	return func(rc *readClientSettings) {
		rc.maxWorkerCount = n
	}
}
