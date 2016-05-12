package metrics_test

import (
	gax "github.com/googleapis/gax-go"
	google_logging_v2 "github.com/googleapis/proto-client-go/logging/v2"
	"golang.org/x/net/context"
	"google.golang.org/cloud/logging/apiv2/metrics"
)

func ExampleNewClient() {
	ctx := context.Background()
	opts := []gax.ClientOption{ /* Optional client parameters. */ }
	c, err := metrics.NewClient(ctx, opts...)
	_, _ = c, err // Handle error.
}

func ExampleClient_ListLogMetrics() {
	ctx := context.Background()
	c, err := metrics.NewClient(ctx)
	_ = err // Handle error.

	req := &google_logging_v2.ListLogMetricsRequest{ /* Data... */ }
	it := c.ListLogMetrics(ctx, req)
	var resp *google_logging_v2.LogMetric
	for {
		resp, err = it.Next()
		if err != nil {
			break
		}
	}
	_ = resp
}

func ExampleClient_GetLogMetric() {
	ctx := context.Background()
	c, err := metrics.NewClient(ctx)
	_ = err // Handle error.

	req := &google_logging_v2.GetLogMetricRequest{ /* Data... */ }
	var resp *google_logging_v2.LogMetric
	resp, err = c.GetLogMetric(ctx, req)
	_, _ = resp, err // Handle error.
}

func ExampleClient_CreateLogMetric() {
	ctx := context.Background()
	c, err := metrics.NewClient(ctx)
	_ = err // Handle error.

	req := &google_logging_v2.CreateLogMetricRequest{ /* Data... */ }
	var resp *google_logging_v2.LogMetric
	resp, err = c.CreateLogMetric(ctx, req)
	_, _ = resp, err // Handle error.
}

func ExampleClient_UpdateLogMetric() {
	ctx := context.Background()
	c, err := metrics.NewClient(ctx)
	_ = err // Handle error.

	req := &google_logging_v2.UpdateLogMetricRequest{ /* Data... */ }
	var resp *google_logging_v2.LogMetric
	resp, err = c.UpdateLogMetric(ctx, req)
	_, _ = resp, err // Handle error.
}

func ExampleClient_DeleteLogMetric() {
	ctx := context.Background()
	c, err := metrics.NewClient(ctx)
	_ = err // Handle error.

	req := &google_logging_v2.DeleteLogMetricRequest{ /* Data... */ }
	err = c.DeleteLogMetric(ctx, req)
	_ = err // Handle error.
}
