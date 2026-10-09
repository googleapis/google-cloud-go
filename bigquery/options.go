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

package bigquery

import (
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
)

// type for collecting custom ClientOption values.
type customClientConfig struct {
	jobCreationMode JobCreationMode

	// maxRetries controls the maximum number of retry attempts for retryable errors.
	// When set to 0 (default), retries continue indefinitely until context cancellation.
	// When set to a positive value N, after N consecutive retryable failures,
	// the operation returns a RetryExhaustedError containing all N errors for debugging.
	maxRetries int
}

type customClientOption interface {
	option.ClientOption
	ApplyCustomClientOpt(*customClientConfig)
}

func newCustomClientConfig(opts ...option.ClientOption) *customClientConfig {
	conf := &customClientConfig{}
	for _, opt := range opts {
		if cOpt, ok := opt.(customClientOption); ok {
			cOpt.ApplyCustomClientOpt(conf)
		}
	}
	return conf
}

// JobCreationMode controls how job creation is handled.  Some queries may
// be run without creating a job to expedite fetching results.
type JobCreationMode string

var (
	// JobCreationModeUnspecified is the default (unspecified) option.
	JobCreationModeUnspecified JobCreationMode = "JOB_CREATION_MODE_UNSPECIFIED"
	// JobCreationModeRequired indicates job creation is required.
	JobCreationModeRequired JobCreationMode = "JOB_CREATION_REQUIRED"
	// JobCreationModeOptional indicates job creation is optional, and returning
	// results immediately is prioritized.  The conditions under which BigQuery
	// can choose to avoid job creation are internal and subject to change.
	JobCreationModeOptional JobCreationMode = "JOB_CREATION_OPTIONAL"
)

// WithDefaultJobCreationMode is a ClientOption that governs the job creation
// mode used when executing queries that can be accelerated via the jobs.Query
// API.  Users may experience performance improvements by leveraging the
// JobCreationModeOptional mode.
func WithDefaultJobCreationMode(mode JobCreationMode) option.ClientOption {
	return &applierJobCreationMode{mode: mode}
}

// applier for propagating the custom client option to the config object
type applierJobCreationMode struct {
	internaloption.EmbeddableAdapter
	mode JobCreationMode
}

func (s *applierJobCreationMode) ApplyCustomClientOpt(c *customClientConfig) {
	c.jobCreationMode = s.mode
}

// WithMaxRetries is a ClientOption that configures the maximum number of retry
// attempts for retryable errors during BigQuery operations.
//
// When maxRetries is 0 or less (the default), retries continue indefinitely with
// exponential backoff until the context is cancelled or the operation succeeds.
//
// When maxRetries is a positive value N, after N consecutive retryable failures,
// the operation returns a `RetryExhaustedError` that contains all N errors for
// debugging purposes.
//
// This is useful for:
//   - Preventing indefinite retries in automated pipelines
//   - Debugging persistent failures by examining all error messages
//   - Implementing fail-fast behavior for known transient issues
//
// Example usage:
//
//	client, err := bigquery.NewClient(ctx, projectID,
//	    bigquery.WithMaxRetries(5),
//	)
func WithMaxRetries(maxRetries int) option.ClientOption {
	return &applierMaxRetries{maxRetries: maxRetries}
}

type applierMaxRetries struct {
	internaloption.EmbeddableAdapter
	maxRetries int
}

func (s *applierMaxRetries) ApplyCustomClientOpt(c *customClientConfig) {
	c.maxRetries = s.maxRetries
}
