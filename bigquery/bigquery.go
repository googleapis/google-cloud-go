// Copyright 2015 Google LLC
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
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"cloud.google.com/go/bigquery/internal"
	"cloud.google.com/go/internal/detect"
	"cloud.google.com/go/internal/trace"
	"cloud.google.com/go/internal/version"
	bq "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/option"
)

const (
	// Scope is the Oauth2 scope for the service.
	// For relevant BigQuery scopes, see:
	// https://developers.google.com/identity/protocols/googlescopes#bigqueryv2
	Scope           = "https://www.googleapis.com/auth/bigquery"
	userAgentPrefix = "gcloud-golang-bigquery"
)

var xGoogHeader = fmt.Sprintf("gl-go/%s gccl/%s", version.Go(), internal.Version)

func setClientHeader(headers http.Header) {
	headers.Set("x-goog-api-client", xGoogHeader)
}

// Client may be used to perform BigQuery operations.
type Client struct {
	// Location, if set, will be used as the default location for all subsequent
	// dataset creation and job operations. A location specified directly in one of
	// those operations will override this value.
	Location string

	projectID string
	bqs       *bq.Service
	rc        *readClient
	retry     *retryConfig
}

// DetectProjectID is a sentinel value that instructs NewClient to detect the
// project ID. It is given in place of the projectID argument. NewClient will
// use the project ID from the given credentials or the default credentials
// (https://developers.google.com/accounts/docs/application-default-credentials)
// if no credentials were provided. When providing credentials, not all
// options will allow NewClient to extract the project ID. Specifically a JWT
// does not have the project ID encoded.
const DetectProjectID = "*detect-project-id*"

// NewClient constructs a new Client which can perform BigQuery operations.
// Operations performed via the client are billed to the specified GCP project.
//
// If the project ID is set to DetectProjectID, NewClient will attempt to detect
// the project ID from credentials.
func NewClient(ctx context.Context, projectID string, opts ...option.ClientOption) (*Client, error) {
	o := []option.ClientOption{
		option.WithScopes(Scope),
		option.WithUserAgent(fmt.Sprintf("%s/%s", userAgentPrefix, internal.Version)),
	}
	o = append(o, opts...)
	bqs, err := bq.NewService(ctx, o...)
	if err != nil {
		return nil, fmt.Errorf("bigquery: constructing client: %w", err)
	}

	// Handle project autodetection.
	projectID, err = detect.ProjectID(ctx, projectID, "", opts...)
	if err != nil {
		return nil, err
	}

	c := &Client{
		projectID: projectID,
		bqs:       bqs,
		retry:     defaultRetryConfig(),
	}
	return c, nil
}

// EnableStorageReadClient sets up Storage API connection to be used when fetching
// large datasets from tables, jobs or queries.
// Calling this method twice will return an error.
func (c *Client) EnableStorageReadClient(ctx context.Context, opts ...option.ClientOption) error {
	if c.isStorageReadAvailable() {
		return fmt.Errorf("failed: storage read client already set up")
	}
	rc, err := newReadClient(ctx, c.projectID, opts...)
	if err != nil {
		return err
	}
	c.rc = rc
	return nil
}

func (c *Client) isStorageReadAvailable() bool {
	return c.rc != nil
}

// Project returns the project ID or number for this instance of the client, which may have
// either been explicitly specified or autodetected.
func (c *Client) Project() string {
	return c.projectID
}

// Close closes any resources held by the client.
// Close should be called when the client is no longer needed.
// It need not be called at program exit.
func (c *Client) Close() error {
	if c.isStorageReadAvailable() {
		err := c.rc.close()
		if err != nil {
			return err
		}
	}
	return nil
}

// Calls the Jobs.Insert RPC and returns a Job.
func (c *Client) insertJob(ctx context.Context, job *bq.Job, media io.Reader) (*Job, error) {
	call := c.bqs.Jobs.Insert(c.projectID, job).Context(ctx)
	setClientHeader(call.Header())
	if media != nil {
		call.Media(media)
	}
	var res *bq.Job
	var err error
	invoke := func() error {
		sCtx := trace.StartSpan(ctx, "bigquery.jobs.insert")
		res, err = call.Do()
		trace.EndSpan(sCtx, err)
		return err
	}
	// A job with a client-generated ID can be retried; the presence of the
	// ID makes the insert operation idempotent.
	// We don't retry if there is media, because it is an io.Reader. We'd
	// have to read the contents and keep it in memory, and that could be expensive.
	// TODO(jba): Look into retrying if media != nil.
	if job.JobReference != nil && media == nil {
		// We deviate from default retries due to BigQuery wanting to retry structured internal job errors.
		err = runWithRetryExplicit(ctx, c.retry, invoke, jobRetryReasons)
	} else {
		err = invoke()
	}
	if err != nil {
		return nil, err
	}
	return bqToJob(res, c)
}

// runQuery invokes the optimized query path.
// Due to differences in options it supports, it cannot be used for all existing
// jobs.insert requests that are query jobs.
func (c *Client) runQuery(ctx context.Context, queryRequest *bq.QueryRequest) (*bq.QueryResponse, error) {
	call := c.bqs.Jobs.Query(c.projectID, queryRequest).Context(ctx)
	setClientHeader(call.Header())

	var res *bq.QueryResponse
	var err error
	invoke := func() error {
		sCtx := trace.StartSpan(ctx, "bigquery.jobs.query")
		res, err = call.Do()
		trace.EndSpan(sCtx, err)
		return err
	}

	// We control request ID, so we can always runWithRetry.
	err = runWithRetryExplicit(ctx, c.retry, invoke, jobRetryReasons)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// SetRetry configures the client with custom retry behavior as specified by the
// options that are passed to it. All operations using this client will use the
// customized retry configuration.
func (c *Client) SetRetry(opts ...RetryOption) {
	var retry *retryConfig
	if c.retry != nil {
		// merge the options with the existing retry
		retry = c.retry
	} else {
		retry = defaultRetryConfig()
	}
	for _, opt := range opts {
		opt.apply(retry)
	}
	c.retry = retry
}

// Convert a number of milliseconds since the Unix epoch to a time.Time.
// Treat an input of zero specially: convert it to the zero time,
// rather than the start of the epoch.
func unixMillisToTime(m int64) time.Time {
	if m == 0 {
		return time.Time{}
	}
	return time.Unix(0, m*1e6)
}
