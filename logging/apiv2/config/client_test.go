package config_test

import (
	gax "github.com/googleapis/gax-go"
	google_logging_v2 "github.com/googleapis/proto-client-go/logging/v2"
	"golang.org/x/net/context"
	"google.golang.org/cloud/logging/apiv2/config"
)

func ExampleNewClient() {
	ctx := context.Background()
	opts := []gax.ClientOption{ /* Optional client parameters. */ }
	c, err := config.NewClient(ctx, opts...)
	_, _ = c, err // Handle error.
}

func ExampleClient_ListSinks() {
	ctx := context.Background()
	c, err := config.NewClient(ctx)
	_ = err // Handle error.

	req := &google_logging_v2.ListSinksRequest{ /* Data... */ }
	it := c.ListSinks(ctx, req)
	var resp *google_logging_v2.LogSink
	for {
		resp, err = it.Next()
		if err != nil {
			break
		}
	}
	_ = resp
}

func ExampleClient_GetSink() {
	ctx := context.Background()
	c, err := config.NewClient(ctx)
	_ = err // Handle error.

	req := &google_logging_v2.GetSinkRequest{ /* Data... */ }
	var resp *google_logging_v2.LogSink
	resp, err = c.GetSink(ctx, req)
	_, _ = resp, err // Handle error.
}

func ExampleClient_CreateSink() {
	ctx := context.Background()
	c, err := config.NewClient(ctx)
	_ = err // Handle error.

	req := &google_logging_v2.CreateSinkRequest{ /* Data... */ }
	var resp *google_logging_v2.LogSink
	resp, err = c.CreateSink(ctx, req)
	_, _ = resp, err // Handle error.
}

func ExampleClient_UpdateSink() {
	ctx := context.Background()
	c, err := config.NewClient(ctx)
	_ = err // Handle error.

	req := &google_logging_v2.UpdateSinkRequest{ /* Data... */ }
	var resp *google_logging_v2.LogSink
	resp, err = c.UpdateSink(ctx, req)
	_, _ = resp, err // Handle error.
}

func ExampleClient_DeleteSink() {
	ctx := context.Background()
	c, err := config.NewClient(ctx)
	_ = err // Handle error.

	req := &google_logging_v2.DeleteSinkRequest{ /* Data... */ }
	err = c.DeleteSink(ctx, req)
	_ = err // Handle error.
}
