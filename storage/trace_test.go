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

package storage

import (
	"context"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/storage/internal"
	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/option"
)

type emulatedTraceTest struct {
	*testing.T
	resources resources
}

func TestTraceStorageTraceStartEndSpan(t *testing.T) {
	ctx := context.Background()
	e := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(e))
	defer tp.Shutdown(ctx)
	otel.SetTracerProvider(tp)

	// TODO: Remove setting development env var upon launch.
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	spanName := "storage.TestTrace.TestStorageTraceStartEndSpan"
	addAttrs := attribute.String("foo", "bar")
	spanStartOpts := []trace.SpanStartOption{
		trace.WithAttributes(addAttrs),
	}
	newAttrs := attribute.Int("fakeKey", 800)

	ctx, span := startSpan(ctx, spanName, spanStartOpts...)
	span.SetAttributes(newAttrs)
	endSpan(ctx, nil)

	spans := e.GetSpans()
	if len(spans) != 1 {
		t.Errorf("expected one span, got %d", len(spans))
	}

	// Test StartSpanOption and Cloud Trace adoption common attributes are appended.
	// Test startSpan returns the span and additional attributes can be set.
	wantSpan := createWantSpanStub(spanName)
	wantSpan.Attributes = append(wantSpan.Attributes, addAttrs)
	wantSpan.Attributes = append(wantSpan.Attributes, newAttrs)
	opts := []cmp.Option{
		cmp.Comparer(spanAttributesComparer),
	}
	for _, span := range spans {
		if diff := testutil.Diff(span, wantSpan, opts...); diff != "" {
			t.Errorf("diff: -got, +want:\n%s\n", diff)
		}
	}
	e.Reset()
}

func TestTraceSpansMultiEmulated(t *testing.T) {
	checkEmulatorEnvironment(t)
	ctx := skipJSONReads(context.Background(), "no reads in test")
	// To only test storage library layer instrumentation,
	// we disable transport layer traces for testing purposes.
	opts := []option.ClientOption{
		option.WithTelemetryDisabled(),
	}
	multiTransportTest(ctx, t, func(t *testing.T, ctx context.Context, bucket string, prefix string, client *Client) {
		for _, c := range []struct {
			name      string
			resources []string
			call      func(ctx context.Context, c *Client, fs *resources) error
		}{
			{
				name:      "Bucket.Attrs",
				resources: []string{"bucket"},
				call: func(ctx context.Context, c *Client, fs *resources) error {
					_, err := c.Bucket(fs.bucket.Name).Attrs(ctx)
					return err
				},
			},
			{
				name:      "Bucket.Create",
				resources: []string{},
				call: func(ctx context.Context, c *Client, fs *resources) error {
					b := bucketIDs.New()
					return c.Bucket(b).Create(ctx, projectID, nil)
				},
			},
			{
				name:      "Bucket.Delete",
				resources: []string{"bucket"},
				call: func(ctx context.Context, c *Client, fs *resources) error {
					c.Bucket(fs.bucket.Name).Delete(ctx)
					return nil
				},
			},
			{
				name:      "Bucket.Update",
				resources: []string{"bucket"},
				call: func(ctx context.Context, c *Client, fs *resources) error {
					uattrs := BucketAttrsToUpdate{StorageClass: "ARCHIVE"}
					bkt := c.Bucket(fs.bucket.Name)
					_, err := bkt.Update(ctx, uattrs)
					return err
				},
			},
			{
				name:      "Object.Attrs",
				resources: []string{"bucket", "object"},
				call: func(ctx context.Context, c *Client, fs *resources) error {
					_, err := c.Bucket(fs.bucket.Name).Object(fs.object.Name).Attrs(ctx)
					return err
				},
			},
			{
				name:      "ACL.List",
				resources: []string{"bucket"},
				call: func(ctx context.Context, c *Client, fs *resources) error {
					_, err := c.Bucket(fs.bucket.Name).ACL().List(ctx)
					return err
				},
			},
			{
				name:      "ACL.Set",
				resources: []string{"bucket"},
				call: func(ctx context.Context, c *Client, fs *resources) error {
					return c.Bucket(fs.bucket.Name).ACL().Set(ctx, AllAuthenticatedUsers, RoleOwner)
				},
			},
		} {
			t.Run(c.name, func(t *testing.T) {
				// Create the test resources.
				subtest := &emulatedTraceTest{}
				subtest.populateResources(ctx, veneerClient, c.resources)

				// TODO: Remove setting development env var upon launch.
				t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

				// Configure the tracer provider and in-memory exporter.
				e := tracetest.NewInMemoryExporter()
				tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(e))
				defer tp.Shutdown(ctx)
				otel.SetTracerProvider(tp)

				// Run the library method to test trace instrumentation.
				err := c.call(ctx, client, &subtest.resources)
				if err != nil {
					t.Errorf("%v error: %v", c.name, err)
				}

				// Verify trace spans.
				wantSpan := createWantSpanStub(c.name)
				checkOTelTraceSpans(t, e, wantSpan)
				e.Reset()
			})
		}
	}, opts...)
}

func createWantSpanStub(spanName string) tracetest.SpanStub {
	return tracetest.SpanStub{
		Name: appendPackageName(spanName),
		Attributes: []attribute.KeyValue{
			attribute.String("gcp.client.version", internal.Version),
			attribute.String("gcp.client.repo", gcpClientRepo),
			attribute.String("gcp.client.artifact", gcpClientArtifact),
		},
		InstrumentationLibrary: instrumentation.Scope{
			Name:    "cloud.google.com/go/storage",
			Version: internal.Version,
		},
	}
}

func checkOTelTraceSpans(t *testing.T, e *tracetest.InMemoryExporter, wantSpan tracetest.SpanStub) {
	spans := e.GetSpans()
	if len(spans) == 0 {
		t.Errorf("Wanted trace spans, got none")
	}
	opts := []cmp.Option{
		cmp.Comparer(spanAttributesComparer),
	}
	for _, span := range spans {
		if diff := testutil.Diff(span, wantSpan, opts...); diff != "" {
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
	if a.InstrumentationLibrary != b.InstrumentationLibrary {
		return false
	}
	return true
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
