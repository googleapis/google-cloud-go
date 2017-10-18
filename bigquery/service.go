// Copyright 2015 Google Inc. All Rights Reserved.
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
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/internal"
	"cloud.google.com/go/internal/version"
	gax "github.com/googleapis/gax-go"

	"golang.org/x/net/context"
	bq "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/googleapi"
)

// service provides an internal abstraction to isolate the generated
// BigQuery API; most of this package uses this interface instead.
// The single implementation, *bigqueryService, contains all the knowledge
// of the generated BigQuery API.
type service interface {
	// Table data
	readTabledata(ctx context.Context, conf *readTableConf, pageToken string) (*readDataResult, error)

	// Misc

	// Waits for a query to complete.
	waitForQuery(ctx context.Context, projectID, jobID string) (Schema, error)
}

var xGoogHeader = fmt.Sprintf("gl-go/%s gccl/%s", version.Go(), version.Repo)

func setClientHeader(headers http.Header) {
	headers.Set("x-goog-api-client", xGoogHeader)
}

type bigqueryService struct {
	s *bq.Service
}

func newBigqueryService(client *http.Client, endpoint string) (*bigqueryService, error) {
	s, err := bq.New(client)
	if err != nil {
		return nil, fmt.Errorf("constructing bigquery client: %v", err)
	}
	s.BasePath = endpoint

	return &bigqueryService{s: s}, nil
}

// getPages calls the supplied getPage function repeatedly until there are no pages left to get.
// token is the token of the initial page to start from.  Use an empty string to start from the beginning.
func getPages(token string, getPage func(token string) (nextToken string, err error)) error {
	for {
		var err error
		token, err = getPage(token)
		if err != nil {
			return err
		}
		if token == "" {
			return nil
		}
	}
}

type pagingConf struct {
	recordsPerRequest    int64
	setRecordsPerRequest bool

	startIndex uint64
}

type readTableConf struct {
	projectID, datasetID, tableID string
	paging                        pagingConf
	schema                        Schema // lazily initialized when the first page of data is fetched.
}

func (conf *readTableConf) fetch(ctx context.Context, s service, token string) (*readDataResult, error) {
	return s.readTabledata(ctx, conf, token)
}

func (conf *readTableConf) setPaging(pc *pagingConf) { conf.paging = *pc }

type readDataResult struct {
	pageToken string
	rows      [][]Value
	totalRows uint64
	schema    Schema
}

func (s *bigqueryService) readTabledata(ctx context.Context, conf *readTableConf, pageToken string) (*readDataResult, error) {
	// Prepare request to fetch one page of table data.
	req := s.s.Tabledata.List(conf.projectID, conf.datasetID, conf.tableID)
	setClientHeader(req.Header())
	if pageToken != "" {
		req.PageToken(pageToken)
	} else {
		req.StartIndex(conf.paging.startIndex)
	}

	if conf.paging.setRecordsPerRequest {
		req.MaxResults(conf.paging.recordsPerRequest)
	}

	// Fetch the table schema in the background, if necessary.
	errc := make(chan error, 1)
	if conf.schema != nil {
		errc <- nil
	} else {
		go func() {
			var t *bq.Table
			err := runWithRetry(ctx, func() (err error) {
				t, err = s.s.Tables.Get(conf.projectID, conf.datasetID, conf.tableID).
					Fields("schema").
					Context(ctx).
					Do()
				return err
			})
			if err == nil && t.Schema != nil {
				conf.schema = convertTableSchema(t.Schema)
			}
			errc <- err
		}()
	}
	var res *bq.TableDataList
	err := runWithRetry(ctx, func() (err error) {
		res, err = req.Context(ctx).Do()
		return err
	})
	if err != nil {
		return nil, err
	}
	err = <-errc
	if err != nil {
		return nil, err
	}
	result := &readDataResult{
		pageToken: res.PageToken,
		totalRows: uint64(res.TotalRows),
		schema:    conf.schema,
	}
	result.rows, err = convertRows(res.Rows, conf.schema)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *bigqueryService) waitForQuery(ctx context.Context, projectID, jobID string) (Schema, error) {
	// Use GetQueryResults only to wait for completion, not to read results.
	req := s.s.Jobs.GetQueryResults(projectID, jobID).Context(ctx).MaxResults(0)
	setClientHeader(req.Header())
	backoff := gax.Backoff{
		Initial:    1 * time.Second,
		Multiplier: 2,
		Max:        60 * time.Second,
	}
	var res *bq.GetQueryResultsResponse
	err := internal.Retry(ctx, backoff, func() (stop bool, err error) {
		res, err = req.Do()
		if err != nil {
			return !retryableError(err), err
		}
		if !res.JobComplete { // GetQueryResults may return early without error; retry.
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return convertTableSchema(res.Schema), nil
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

// runWithRetry calls the function until it returns nil or a non-retryable error, or
// the context is done.
// See the similar function in ../storage/invoke.go. The main difference is the
// reason for retrying.
func runWithRetry(ctx context.Context, call func() error) error {
	// These parameters match the suggestions in https://cloud.google.com/bigquery/sla.
	backoff := gax.Backoff{
		Initial:    1 * time.Second,
		Max:        32 * time.Second,
		Multiplier: 2,
	}
	return internal.Retry(ctx, backoff, func() (stop bool, err error) {
		err = call()
		if err == nil {
			return true, nil
		}
		return !retryableError(err), err
	})
}

// This is the correct definition of retryable according to the BigQuery team.
func retryableError(err error) bool {
	e, ok := err.(*googleapi.Error)
	if !ok {
		return false
	}
	var reason string
	if len(e.Errors) > 0 {
		reason = e.Errors[0].Reason
	}
	return reason == "backendError" || reason == "rateLimitExceeded"
}
