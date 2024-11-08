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

package storage

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/storage/internal"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/gax-go/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/option"
)

func TestTraceStorageTraceStartEndSpan(t *testing.T) {
	ctx := context.Background()
	e := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(e))
	defer tp.Shutdown(ctx)
	otel.SetTracerProvider(tp)

	spanName := "storage.TestTrace.TestStorageTraceStartEndSpan"
	spanStartOpts := []trace.SpanStartOption{
		trace.WithAttributes(
			attribute.String("foo", "bar"),
		),
	}
	addAttrs := attribute.String("fakeKey", "fakeVal")

	ctx, span := startSpan(ctx, spanName, spanStartOpts...)
	span.SetAttributes(addAttrs)
	endSpan(ctx, nil)

	spans := e.GetSpans()
	if len(spans) != 1 {
		t.Errorf("expected one span, got %d", len(spans))
	}

	// Test StartSpanOption and Cloud Trace Adoption common attributes are appended.
	wantAttributes := tracetest.SpanStub{
		Name: spanName,
		Attributes: []attribute.KeyValue{
			attribute.String("foo", "bar"),
			attribute.String("gcp.client.version", internal.Version),
			attribute.String("gcp.client.repo", gcpClientRepo),
			attribute.String("gcp.client.artifact", gcpClientArtifact),
		},
	}
	// Test startSpan returns the span and additional attributes can be set.
	wantAttributes.Attributes = append(wantAttributes.Attributes, addAttrs)
	opts := []cmp.Option{
		cmp.Comparer(spanAttributesComparer),
	}
	for _, span := range spans {
		if diff := testutil.Diff(span, wantAttributes, opts...); diff != "" {
			t.Errorf("diff: -got, +want:\n%s\n", diff)
		}
	}
}

func spanAttributesComparer(a, b tracetest.SpanStub) bool {
	if a.Name != b.Name {
		return false
	}
	if len(a.Attributes) != len(b.Attributes) {
		return false
	}
	return true
}

type emulatedTraceTest struct {
	*testing.T
	name            string
	resources       resources
	transportClient *Client
}

func TestTraceSpans(t *testing.T) {
	host := os.Getenv("STORAGE_EMULATOR_HOST")
	if host == "" {
		t.Skip("This test must use the testbench emulator; set STORAGE_EMULATOR_HOST to run.")
	}

	// Create non-wrapped client to use for setup steps.
	ctx := context.Background()
	client, err := NewClient(ctx)
	if err != nil {
		t.Fatalf("storage.NewClient: %v", err)
	}

	skippedTraceMethods := []string{"storage.Bucket.AddNotification", "storage.Bucket.Notifications", "storage.Bucket.DeleteNotification"}
	for spanName, fn := range traceMethods {
		transports := []string{"http", "grpc"}
		for _, transport := range transports {
			testName := fmt.Sprintf("TestTrace-%v-%v", transport, spanName)
			t.Run(testName, func(t *testing.T) {
				if transport == "grpc" && slices.Contains(skippedTraceMethods, spanName) {
					t.Skip("not supported")
				}
				// Setup: Create the trace subtest, transport client and test resources.
				subtest := &emulatedTraceTest{T: t, name: testName}
				subtest.initTransportClient(transport)
				resources := []string{"bucket", "object", "notification"}
				subtest.populateResources(ctx, client, resources)

				// TODO: Remove setting development env var upon launch.
				t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")
				// Configure the tracer provider and in-memory exporter.
				e, _ := enableOTelTraceInTests2()

				// Run the library method that has trace instrumentation.
				err = fn(ctx, subtest.transportClient, &subtest.resources)
				if err != nil {
					t.Errorf("%v error: %v", subtest.name, err)
				}

				// Verify trace spans.
				wantedSpan := createWantSpanStub(spanName)
				subtest.checkOTelTraceSpans(e, wantedSpan)
			})
		}
	}
}

// Creates the transport client used in emulated trace tests.
func (et *emulatedTraceTest) initTransportClient(transport string) {
	ctx := context.Background()
	// Create transportClient for http or grpc. To test veneer library
	// instrumentation, we disable transport layer traces for testing purposes.
	opts := []option.ClientOption{
		option.WithTelemetryDisabled(),
	}
	transportClient, err := NewClient(ctx, opts...)
	if err != nil {
		et.Fatalf("HTTP transportClient: %v", err)
	}

	if transport == "grpc" {
		transportClient, err = NewGRPCClient(ctx, opts...)
		if err != nil {
			et.Fatalf("GRPC transportClient: %v", err)
		}
	}
	// Reduce backoff to get faster test execution.
	transportClient.SetRetry(WithBackoff(gax.Backoff{Initial: 10 * time.Millisecond}))
	et.transportClient = transportClient
}

// Creates test resources.
func (et *emulatedTraceTest) populateResources(ctx context.Context, c *Client, resources []string) {
	for _, resource := range resources {
		switch resource {
		case "bucket":
			bkt := c.Bucket(bucketIDs.New())
			if err := bkt.Create(ctx, projectID, &BucketAttrs{}); err != nil {
				et.Fatalf("creating bucket: %v", err)
			}
			attrs, err := bkt.Attrs(ctx)
			if err != nil {
				et.Fatalf("getting bucket attrs: %v", err)
			}
			et.resources.bucket = attrs
		case "object":
			// Assumes bucket has been populated first.
			obj := c.Bucket(et.resources.bucket.Name).Object(objectIDs.New())
			w := obj.NewWriter(ctx)
			if _, err := w.Write(randomBytesToWrite); err != nil {
				et.Fatalf("writing object: %v", err)
			}
			if err := w.Close(); err != nil {
				et.Fatalf("closing object: %v", err)
			}
			attrs, err := obj.Attrs(ctx)
			if err != nil {
				et.Fatalf("getting object attrs: %v", err)
			}
			et.resources.object = attrs
		case "notification":
			// Assumes bucket has been populated first.
			n, err := c.Bucket(et.resources.bucket.Name).AddNotification(ctx, &Notification{
				TopicProjectID: projectID,
				TopicID:        notificationIDs.New(),
				PayloadFormat:  JSONPayload,
			})
			if err != nil {
				et.Fatalf("adding notification: %v", err)
			}
			et.resources.notification = n
		}
	}
}

func createWantSpanStub(spanName string) tracetest.SpanStub {
	return tracetest.SpanStub{
		Name: spanName,
		Attributes: []attribute.KeyValue{
			attribute.String("gcp.client.version", internal.Version),
			attribute.String("gcp.client.repo", gcpClientRepo),
			attribute.String("gcp.client.artifact", gcpClientArtifact),
		},
	}
}

func enableOTelTraceInTests2() (*tracetest.InMemoryExporter, context.Context) {
	span_context := context.Background()
	e := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(e))
	otel.SetTracerProvider(tp)

	return e, span_context
}

// Tests trace span instrumentation on the veneer library.
func (et *emulatedTraceTest) checkOTelTraceSpans(e *tracetest.InMemoryExporter, wantSpan tracetest.SpanStub) {
	spans := e.GetSpans()
	if len(spans) == 0 {
		et.Errorf("Wanted trace spans, got none")
	}
	opts := []cmp.Option{
		cmp.Comparer(spanAttributesComparer),
	}
	for _, span := range spans {
		if diff := testutil.Diff(span, wantSpan, opts...); diff != "" {
			et.Errorf("diff: -got, +want:\n%s\n", diff)
		}
	}
}

type traceFunc func(ctx context.Context, c *Client, fs *resources) error

var traceMethods = map[string]traceFunc{
	// Bucket module
	"storage.Bucket.Attrs": func(ctx context.Context, c *Client, fs *resources) error {
		_, err := c.Bucket(fs.bucket.Name).Attrs(ctx)
		return err
	},
	"storage.Bucket.Delete": func(ctx context.Context, c *Client, fs *resources) error {
		c.Bucket(fs.bucket.Name).Delete(ctx)
		return nil
	},
	"storage.Bucket.Create": func(ctx context.Context, c *Client, fs *resources) error {
		b := bucketIDs.New()
		return c.Bucket(b).Create(ctx, projectID, nil)
	},
	"storage.Bucket.Update": func(ctx context.Context, c *Client, fs *resources) error {
		uattrs := BucketAttrsToUpdate{StorageClass: "ARCHIVE"}
		bkt := c.Bucket(fs.bucket.Name)
		_, err := bkt.Update(ctx, uattrs)
		return err
	},
	// Notifications module
	"storage.Bucket.DeleteNotification": func(ctx context.Context, c *Client, fs *resources) error {
		return c.Bucket(fs.bucket.Name).DeleteNotification(ctx, fs.notification.ID)
	},
	"storage.Bucket.AddNotification": func(ctx context.Context, c *Client, fs *resources) error {
		notification := Notification{
			TopicID:        "my-topic",
			TopicProjectID: projectID,
			PayloadFormat:  "json",
		}
		_, err := c.Bucket(fs.bucket.Name).AddNotification(ctx, &notification)
		return err
	},
	"storage.Bucket.Notifications": func(ctx context.Context, c *Client, fs *resources) error {
		_, err := c.Bucket(fs.bucket.Name).Notifications(ctx)
		return err
	},
	// Storage module
	"storage.Object.Attrs": func(ctx context.Context, c *Client, fs *resources) error {
		_, err := c.Bucket(fs.bucket.Name).Object(fs.object.Name).Attrs(ctx)
		return err
	},
	// ACL module
	"storage.ACL.List": func(ctx context.Context, c *Client, fs *resources) error {
		_, err := c.Bucket(fs.bucket.Name).ACL().List(ctx)
		return err
	},
	"storage.ACL.Set": func(ctx context.Context, c *Client, fs *resources) error {
		return c.Bucket(fs.bucket.Name).ACL().Set(ctx, AllAuthenticatedUsers, RoleOwner)
	},
}
