// Copyright 2016 Google Inc. All Rights Reserved.
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

// AUTO-GENERATED CODE. DO NOT EDIT.

package logging

import (
	"fmt"
	"math"
	"runtime"
	"time"

	gax "github.com/googleapis/gax-go"
	"golang.org/x/net/context"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/api/transport"
	loggingpb "google.golang.org/genproto/googleapis/logging/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

var (
	metricsParentPathTemplate = gax.MustCompilePathTemplate("projects/{project}")
	metricsMetricPathTemplate = gax.MustCompilePathTemplate("projects/{project}/metrics/{metric}")
)

// MetricsCallOptions contains the retry settings for each method of MetricsClient.
type MetricsCallOptions struct {
	ListLogMetrics  []gax.CallOption
	GetLogMetric    []gax.CallOption
	CreateLogMetric []gax.CallOption
	UpdateLogMetric []gax.CallOption
	DeleteLogMetric []gax.CallOption
}

func defaultMetricsClientOptions() []option.ClientOption {
	return []option.ClientOption{
		option.WithEndpoint("logging.googleapis.com:443"),
		option.WithScopes(
			"https://www.googleapis.com/auth/cloud-platform",
			"https://www.googleapis.com/auth/cloud-platform.read-only",
			"https://www.googleapis.com/auth/logging.admin",
			"https://www.googleapis.com/auth/logging.read",
			"https://www.googleapis.com/auth/logging.write",
		),
	}
}

func defaultMetricsCallOptions() *MetricsCallOptions {
	retry := map[[2]string][]gax.CallOption{
		{"default", "idempotent"}: {
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.DeadlineExceeded,
					codes.Unavailable,
				}, gax.Backoff{
					Initial:    100 * time.Millisecond,
					Max:        1000 * time.Millisecond,
					Multiplier: 1.2,
				})
			}),
		},
	}
	return &MetricsCallOptions{
		ListLogMetrics:  retry[[2]string{"default", "idempotent"}],
		GetLogMetric:    retry[[2]string{"default", "idempotent"}],
		CreateLogMetric: retry[[2]string{"default", "non_idempotent"}],
		UpdateLogMetric: retry[[2]string{"default", "non_idempotent"}],
		DeleteLogMetric: retry[[2]string{"default", "idempotent"}],
	}
}

// MetricsClient is a client for interacting with Stackdriver Logging API.
type MetricsClient struct {
	// The connection to the service.
	conn *grpc.ClientConn

	// The gRPC API client.
	metricsClient loggingpb.MetricsServiceV2Client

	// The call options for this service.
	CallOptions *MetricsCallOptions

	// The metadata to be sent with each request.
	metadata map[string][]string
}

// NewMetricsClient creates a new metrics service v2 client.
//
// Service for configuring logs-based metrics.
func NewMetricsClient(ctx context.Context, opts ...option.ClientOption) (*MetricsClient, error) {
	conn, err := transport.DialGRPC(ctx, append(defaultMetricsClientOptions(), opts...)...)
	if err != nil {
		return nil, err
	}
	c := &MetricsClient{
		conn:        conn,
		CallOptions: defaultMetricsCallOptions(),

		metricsClient: loggingpb.NewMetricsServiceV2Client(conn),
	}
	c.SetGoogleClientInfo("gax", gax.Version)
	return c, nil
}

// Connection returns the client's connection to the API service.
func (c *MetricsClient) Connection() *grpc.ClientConn {
	return c.conn
}

// Close closes the connection to the API service. The user should invoke this when
// the client is no longer required.
func (c *MetricsClient) Close() error {
	return c.conn.Close()
}

// SetGoogleClientInfo sets the name and version of the application in
// the `x-goog-api-client` header passed on each request. Intended for
// use by Google-written clients.
func (c *MetricsClient) SetGoogleClientInfo(name, version string) {
	c.metadata = map[string][]string{
		"x-goog-api-client": {fmt.Sprintf("%s/%s %s gax/%s go/%s", name, version, gapicNameVersion, gax.Version, runtime.Version())},
	}
}

// MetricsParentPath returns the path for the parent resource.
func MetricsParentPath(project string) string {
	path, err := metricsParentPathTemplate.Render(map[string]string{
		"project": project,
	})
	if err != nil {
		panic(err)
	}
	return path
}

// MetricsMetricPath returns the path for the metric resource.
func MetricsMetricPath(project, metric string) string {
	path, err := metricsMetricPathTemplate.Render(map[string]string{
		"project": project,
		"metric":  metric,
	})
	if err != nil {
		panic(err)
	}
	return path
}

// ListLogMetrics lists logs-based metrics.
func (c *MetricsClient) ListLogMetrics(ctx context.Context, req *loggingpb.ListLogMetricsRequest) *LogMetricIterator {
	md, _ := metadata.FromContext(ctx)
	ctx = metadata.NewContext(ctx, metadata.Join(md, c.metadata))
	it := &LogMetricIterator{}

	fetch := func(pageSize int, pageToken string) (string, error) {
		var resp *loggingpb.ListLogMetricsResponse
		req.PageToken = pageToken
		if pageSize > math.MaxInt32 {
			req.PageSize = math.MaxInt32
		} else {
			req.PageSize = int32(pageSize)
		}
		err := gax.Invoke(ctx, func(ctx context.Context) error {
			var err error
			resp, err = c.metricsClient.ListLogMetrics(ctx, req)
			return err
		}, c.CallOptions.ListLogMetrics...)
		if err != nil {
			return "", err
		}
		it.items = append(it.items, resp.Metrics...)
		return resp.NextPageToken, nil
	}
	bufLen := func() int { return len(it.items) }
	takeBuf := func() interface{} {
		b := it.items
		it.items = nil
		return b
	}

	it.pageInfo, it.nextFunc = iterator.NewPageInfo(fetch, bufLen, takeBuf)
	return it
}

// GetLogMetric gets a logs-based metric.
func (c *MetricsClient) GetLogMetric(ctx context.Context, req *loggingpb.GetLogMetricRequest) (*loggingpb.LogMetric, error) {
	md, _ := metadata.FromContext(ctx)
	ctx = metadata.NewContext(ctx, metadata.Join(md, c.metadata))
	var resp *loggingpb.LogMetric
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		resp, err = c.metricsClient.GetLogMetric(ctx, req)
		return err
	}, c.CallOptions.GetLogMetric...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// CreateLogMetric creates a logs-based metric.
func (c *MetricsClient) CreateLogMetric(ctx context.Context, req *loggingpb.CreateLogMetricRequest) (*loggingpb.LogMetric, error) {
	md, _ := metadata.FromContext(ctx)
	ctx = metadata.NewContext(ctx, metadata.Join(md, c.metadata))
	var resp *loggingpb.LogMetric
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		resp, err = c.metricsClient.CreateLogMetric(ctx, req)
		return err
	}, c.CallOptions.CreateLogMetric...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// UpdateLogMetric creates or updates a logs-based metric.
func (c *MetricsClient) UpdateLogMetric(ctx context.Context, req *loggingpb.UpdateLogMetricRequest) (*loggingpb.LogMetric, error) {
	md, _ := metadata.FromContext(ctx)
	ctx = metadata.NewContext(ctx, metadata.Join(md, c.metadata))
	var resp *loggingpb.LogMetric
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		resp, err = c.metricsClient.UpdateLogMetric(ctx, req)
		return err
	}, c.CallOptions.UpdateLogMetric...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// DeleteLogMetric deletes a logs-based metric.
func (c *MetricsClient) DeleteLogMetric(ctx context.Context, req *loggingpb.DeleteLogMetricRequest) error {
	md, _ := metadata.FromContext(ctx)
	ctx = metadata.NewContext(ctx, metadata.Join(md, c.metadata))
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		_, err = c.metricsClient.DeleteLogMetric(ctx, req)
		return err
	}, c.CallOptions.DeleteLogMetric...)
	return err
}

// LogMetricIterator manages a stream of *loggingpb.LogMetric.
type LogMetricIterator struct {
	items    []*loggingpb.LogMetric
	pageInfo *iterator.PageInfo
	nextFunc func() error
}

// PageInfo supports pagination. See the google.golang.org/api/iterator package for details.
func (it *LogMetricIterator) PageInfo() *iterator.PageInfo {
	return it.pageInfo
}

// Next returns the next result. Its second return value is iterator.Done if there are no more
// results. Once Next returns Done, all subsequent calls will return Done.
func (it *LogMetricIterator) Next() (*loggingpb.LogMetric, error) {
	if err := it.nextFunc(); err != nil {
		return nil, err
	}
	item := it.items[0]
	it.items = it.items[1:]
	return item, nil
}
