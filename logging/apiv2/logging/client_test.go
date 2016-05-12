package logging_test

import (
	gax "github.com/googleapis/gax-go"
	google_api "github.com/googleapis/proto-client-go/api"
	google_logging_v2 "github.com/googleapis/proto-client-go/logging/v2"
	"golang.org/x/net/context"
	"google.golang.org/cloud/logging/apiv2/logging"
)

func ExampleNewClient() {
	ctx := context.Background()
	opts := []gax.ClientOption{ /* Optional client parameters. */ }
	c, err := logging.NewClient(ctx, opts...)
	_, _ = c, err // Handle error.
}

func ExampleClient_DeleteLog() {
	ctx := context.Background()
	c, err := logging.NewClient(ctx)
	_ = err // Handle error.

	req := &google_logging_v2.DeleteLogRequest{ /* Data... */ }
	err = c.DeleteLog(ctx, req)
	_ = err // Handle error.
}

func ExampleClient_WriteLogEntries() {
	ctx := context.Background()
	c, err := logging.NewClient(ctx)
	_ = err // Handle error.

	req := &google_logging_v2.WriteLogEntriesRequest{ /* Data... */ }
	var resp *google_logging_v2.WriteLogEntriesResponse
	resp, err = c.WriteLogEntries(ctx, req)
	_, _ = resp, err // Handle error.
}

func ExampleClient_ListLogEntries() {
	ctx := context.Background()
	c, err := logging.NewClient(ctx)
	_ = err // Handle error.

	req := &google_logging_v2.ListLogEntriesRequest{ /* Data... */ }
	it := c.ListLogEntries(ctx, req)
	var resp *google_logging_v2.LogEntry
	for {
		resp, err = it.Next()
		if err != nil {
			break
		}
	}
	_ = resp
}

func ExampleClient_ListMonitoredResourceDescriptors() {
	ctx := context.Background()
	c, err := logging.NewClient(ctx)
	_ = err // Handle error.

	req := &google_logging_v2.ListMonitoredResourceDescriptorsRequest{ /* Data... */ }
	it := c.ListMonitoredResourceDescriptors(ctx, req)
	var resp *google_api.MonitoredResourceDescriptor
	for {
		resp, err = it.Next()
		if err != nil {
			break
		}
	}
	_ = resp
}
