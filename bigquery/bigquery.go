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

// TODO(mcgreevy): support dry-run mode when creating jobs.

import (
	"fmt"
	"net/http"
	"reflect"

	"golang.org/x/net/context"
	bq "google.golang.org/api/bigquery/v2"
)

// A Source is a source of data for the Copy function.
type Source interface {
	implementsSource()
}

// A Destination is a destination of data for the Copy function.
type Destination interface {
	implementsDestination()
}

// An Option is an optional argument to Copy.
type Option interface {
	implementsOption()
}

const Scope = "https://www.googleapis.com/auth/bigquery"

// Client may be used to perform BigQuery operations.
type Client struct {
	service   *bq.Service
	projectID string
}

// NewClient constructs a new Client which can perform BigQuery operations.
// Operations performed via the client are billed to the specified GCP project.
// The supplied http.Client is used for making requests to the BigQuery server and must be capable of
// authenticating requests with Scope.
func NewClient(client *http.Client, projectID string) (*Client, error) {
	service, err := bq.New(client)
	if err != nil {
		return nil, fmt.Errorf("constructing bigquery client: %v", err)
	}

	c := &Client{
		service:   service,
		projectID: projectID,
	}
	return c, nil
}

type dstSrc struct {
	dst, src reflect.Type
}

func newDstSrc(dst Destination, src Source) dstSrc {
	return dstSrc{
		dst: reflect.TypeOf(dst),
		src: reflect.TypeOf(src),
	}
}

type operation func(jobInserter, Destination, Source, ...Option) (*Job, error)

// TODO(mcgreevy): support more operations.
var ops = map[dstSrc]operation{
	newDstSrc((*Table)(nil), (*GCSReference)(nil)): load,
	newDstSrc((*GCSReference)(nil), (*Table)(nil)): extract,
	newDstSrc((*Table)(nil), (*Table)(nil)):        cp,
}

// Copy starts a BigQuery operation to copy data from a Source to a Destination.
func (c *Client) Copy(ctx context.Context, dst Destination, src Source, options ...Option) (*Job, error) {
	// TODO(mcgreevy): use ctx
	op, ok := ops[newDstSrc(dst, src)]
	if !ok {
		return nil, fmt.Errorf("no operation matches dst/src pair")
	}
	return op(c, dst, src, options...)
}

type jobInserter interface {
	insertJob(job *bq.Job) (*Job, error)
}

func (c *Client) insertJob(job *bq.Job) (*Job, error) {
	res, err := c.service.Jobs.Insert(c.projectID, job).Do()
	if err != nil {
		return nil, err
	}
	return &Job{client: c, jobID: res.JobReference.JobId}, nil
}
