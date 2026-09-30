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
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/storage/internal"
	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel/attribute"
	otcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func TestStorageTraceStartEndSpan(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})

	// TODO: Remove setting development env var upon launch.
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	spanName := "storage.TestTrace.TestStartEndSpan"
	ctx, span := startSpan(ctx, spanName)
	newAttrs := attribute.Int("fakeKey", 800)
	span.SetAttributes(newAttrs)
	endSpan(ctx, nil)

	spans := te.Spans()
	gotSpan := spans[0]
	if len(spans) != 1 {
		t.Errorf("expected one span, got %d", len(spans))
	}
	if got, want := gotSpan.Name, appendPackageName(spanName); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}

	wantSpan := createWantSpanStub(spanName, getCommonAttributes())
	wantSpan.Attributes = append(wantSpan.Attributes, newAttrs)
	opts := []cmp.Option{
		cmp.Comparer(spanAttributesComparer),
	}
	if diff := testutil.Diff(gotSpan, wantSpan, opts...); diff != "" {
		t.Errorf("diff: -got, +want:\n%s\n", diff)
	}
}
func TestStorageTraceStartSpanOption(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})

	// TODO: Remove setting development env var upon launch.
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	spanName := "storage.TestTrace.TestStartSpanOption"
	attrMap := make(map[string]interface{})
	attrMap["my_string"] = "my string"
	attrMap["my_bool"] = true
	attrMap["my_int"] = 123
	attrMap["my_int64"] = int64(456)
	attrMap["my_float"] = 0.9
	spanStartOpts := makeSpanStartOptAttrs(attrMap)

	ctx, _ = startSpan(ctx, spanName, spanStartOpts...)
	endSpan(ctx, nil)

	spans := te.Spans()
	gotSpan := spans[0]
	if len(spans) != 1 {
		t.Errorf("expected one span, got %d", len(spans))
	}
	if got, want := gotSpan.Name, appendPackageName(spanName); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}

	wantSpan := createWantSpanStub(spanName, getCommonAttributes())
	wantSpan.Attributes = append(wantSpan.Attributes, otAttrs(attrMap)...)
	opts := []cmp.Option{
		cmp.Comparer(spanAttributesComparer),
	}
	if diff := testutil.Diff(gotSpan, wantSpan, opts...); diff != "" {
		t.Errorf("diff: -got, +want:\n%s\n", diff)
	}
}

func TestStorageTraceEndSpanRecordError(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})

	// TODO: Remove setting development env var upon launch.
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	spanName := "storage.TestTrace.TestRecordError"
	ctx, _ = startSpan(ctx, spanName)
	err := &googleapi.Error{Code: http.StatusBadRequest, Message: "INVALID ARGUMENT"}
	endSpan(ctx, err)

	spans := te.Spans()
	gotSpan := spans[0]
	if len(spans) != 1 {
		t.Errorf("expected one span, got %d", len(spans))
	}
	if got, want := gotSpan.Name, appendPackageName(spanName); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	if want := otcodes.Error; gotSpan.Status.Code != want {
		t.Errorf("got %v, want %v", gotSpan.Status.Code, want)
	}
}

func createWantSpanStub(spanName string, attrs []attribute.KeyValue) tracetest.SpanStub {
	return tracetest.SpanStub{
		Name:       appendPackageName(spanName),
		Attributes: attrs,
		InstrumentationScope: instrumentation.Scope{
			Name:    "cloud.google.com/go/storage",
			Version: internal.Version,
		},
	}
}

func spanAttributesComparer(a, b tracetest.SpanStub) bool {
	if a.Name != b.Name {
		return false
	}
	if len(a.Attributes) != len(b.Attributes) {
		return false
	}
	if a.InstrumentationScope != b.InstrumentationScope {
		return false
	}
	return true
}

// makeSpanStartOptAttrs makes a SpanStartOption and converts a generic map to OpenTelemetry attributes.
func makeSpanStartOptAttrs(attrMap map[string]interface{}) []trace.SpanStartOption {
	attrs := otAttrs(attrMap)
	return []trace.SpanStartOption{
		trace.WithAttributes(attrs...),
	}
}

// otAttrs converts a generic map to OpenTelemetry attributes.
func otAttrs(attrMap map[string]interface{}) []attribute.KeyValue {
	var attrs []attribute.KeyValue
	for k, v := range attrMap {
		var a attribute.KeyValue
		switch v := v.(type) {
		case string:
			a = attribute.Key(k).String(v)
		case bool:
			a = attribute.Key(k).Bool(v)
		case int:
			a = attribute.Key(k).Int(v)
		case int64:
			a = attribute.Key(k).Int64(v)
		default:
			a = attribute.Key(k).String(fmt.Sprintf("%#v", v))
		}
		attrs = append(attrs, a)
	}
	return attrs
}

func TestStartSpanWithBucket(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})

	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	fetcher := &mockMetadataFetcher{
		fetchFunc: func(ctx context.Context, bucket string) (resource string, location string, err error) {
			return "projects/p1/buckets/" + bucket, "us-west1", nil
		},
	}

	tests := []struct {
		name         string
		bucket       string
		setupCache   func(*bucketMetadataCache)
		wantResource string
		wantLocation string
		verifyCache  bool
	}{
		{
			name:   "Cache Miss (Placeholder)",
			bucket: "bucket-miss",
			setupCache: func(c *bucketMetadataCache) {
				// empty cache
			},
			wantResource: "projects/_/buckets/bucket-miss",
			wantLocation: "global",
			verifyCache:  true,
		},
		{
			name:   "Cache Hit (Resolved)",
			bucket: "bucket-hit",
			setupCache: func(c *bucketMetadataCache) {
				c.put("bucket-hit", bucketMetadata{resource: "projects/p1/buckets/bucket-hit", location: "us-west1"})
			},
			wantResource: "projects/p1/buckets/bucket-hit",
			wantLocation: "us-west1",
			verifyCache:  false,
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := newBucketMetadataCache(10, fetcher)
			tc.setupCache(cache)
			doneChan := make(chan struct{}, 1)
			if tc.verifyCache {
				cache.fetchDone = doneChan
			}
			client := &Client{bucketMetadataCache: cache}

			ctx1, _ := startSpanWithBucket(ctx, client, tc.bucket, "TestSpan")
			endSpan(ctx1, nil)

			spans := te.Spans()
			if len(spans) != i+1 {
				t.Fatalf("expected %d spans, got %d", i+1, len(spans))
			}
			gotSpan := spans[i]

			verifySpanAttributes(t, gotSpan, tc.wantResource, tc.wantLocation)

			if tc.verifyCache {
				// Wait for background fetch to complete and populate cache.
				select {
				case <-doneChan:
				case <-time.After(fetchBackgroundTimeout):
					t.Fatalf("timeout waiting for fetchBackground completion")
				}
				_, found := cache.get(tc.bucket)
				if !found {
					t.Fatalf("expected entry to be populated in cache")
				}
			}
		})
	}
}

func verifySpanAttributes(t *testing.T, span tracetest.SpanStub, wantResource, wantLocation string) {
	t.Helper()
	var gotResource, gotLocation string
	for _, attr := range span.Attributes {
		if attr.Key == "gcp.resource.destination.id" {
			gotResource = attr.Value.AsString()
		}
		if attr.Key == "gcp.resource.destination.location" {
			gotLocation = attr.Value.AsString()
		}
	}

	if gotResource != wantResource {
		t.Errorf("got resource %q, want %q", gotResource, wantResource)
	}

	if gotLocation != wantLocation {
		t.Errorf("got location %q, want %q", gotLocation, wantLocation)
	}
}

func TestEndSpanEviction(t *testing.T) {
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	bucketName := "evict-bucket"
	tests := []struct {
		name      string
		spanName  string
		err       error
		wantEvict bool
	}{
		{
			name:      "Evict on ErrBucketNotExist",
			spanName:  "Bucket.Attrs",
			err:       ErrBucketNotExist,
			wantEvict: true,
		},
		{
			name:      "Evict on googleapi.Error 404",
			spanName:  "Bucket.Attrs",
			err:       &googleapi.Error{Code: http.StatusNotFound},
			wantEvict: true,
		},
		{
			name:      "No Evict on 500",
			spanName:  "Bucket.Attrs",
			err:       &googleapi.Error{Code: http.StatusInternalServerError},
			wantEvict: false,
		},
		{
			name:      "No Evict on Object 404",
			spanName:  "Object.Attrs",
			err:       ErrObjectNotExist,
			wantEvict: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fetcher := &mockMetadataFetcher{}
			cache := newBucketMetadataCache(10, fetcher)
			client := &Client{bucketMetadataCache: cache}

			// Populate cache.
			cache.put(bucketName, bucketMetadata{resource: "res", location: "loc"})

			ctx, _ := startSpanWithBucket(context.Background(), client, bucketName, tc.spanName)
			endSpan(ctx, tc.err)

			_, found := cache.get(bucketName)
			if tc.wantEvict && found {
				t.Errorf("expected bucket to be evicted")
			}
			if !tc.wantEvict && !found {
				t.Errorf("expected bucket to remain in cache")
			}
		})
	}
}

// errRoundTripper fails every request so that operation spans are created and
// ended without any network access.
type errRoundTripper struct{}

func (errRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("fake transport error")
}

// newTraceTestClient returns an HTTP client whose requests always fail and
// which never retries.
func newTraceTestClient(t *testing.T) *Client {
	t.Helper()
	c, err := NewClient(context.Background(), option.WithHTTPClient(&http.Client{Transport: errRoundTripper{}}))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.SetRetry(WithPolicy(RetryNever))
	t.Cleanup(func() { c.Close() })
	return c
}

// storageURIFromSpan returns the gcp.storage.uri attribute of the span named
// spanName, and whether that attribute was present.
func storageURIFromSpan(t *testing.T, spans tracetest.SpanStubs, spanName string) (string, bool) {
	t.Helper()
	fullName := appendPackageName(spanName)
	for _, s := range spans {
		if s.Name != fullName {
			continue
		}
		for _, a := range s.Attributes {
			if string(a.Key) == storageURIAttrKey {
				return a.Value.AsString(), true
			}
		}
		return "", false
	}
	t.Fatalf("span %q not found", fullName)
	return "", false
}

func TestStorageURIAttribute(t *testing.T) {
	const (
		bucket    = "my-bucket"
		object    = "dir/my object.txt"
		dstBucket = "dst-bucket"
		dstObject = "dst/object.txt"
	)
	wantObjURI := "gs://" + bucket + "/" + object
	wantDstURI := "gs://" + dstBucket + "/" + dstObject

	tests := []struct {
		name     string
		spanName string
		op       func(ctx context.Context, c *Client)
		wantURI  string // empty means the attribute must be absent
	}{
		{
			name:     "Object.Attrs",
			spanName: "Object.Attrs",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).Object(object).Attrs(ctx) },
			wantURI:  wantObjURI,
		},
		{
			name:     "Object.Update",
			spanName: "Object.Update",
			op: func(ctx context.Context, c *Client) {
				c.Bucket(bucket).Object(object).Update(ctx, ObjectAttrsToUpdate{ContentType: "text/plain"})
			},
			wantURI: wantObjURI,
		},
		{
			name:     "Object.Delete",
			spanName: "Object.Delete",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).Object(object).Delete(ctx) },
			wantURI:  wantObjURI,
		},
		{
			name:     "Object.Reader",
			spanName: "Object.Reader",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).Object(object).NewReader(ctx) },
			wantURI:  wantObjURI,
		},
		{
			name:     "Object.Writer",
			spanName: "Object.Writer",
			op: func(ctx context.Context, c *Client) {
				w := c.Bucket(bucket).Object(object).NewWriter(ctx)
				w.Write([]byte("data"))
				w.Close()
			},
			wantURI: wantObjURI,
		},
		{
			name:     "Object.MultiRangeDownloader",
			spanName: "Object.MultiRangeDownloader",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).Object(object).NewMultiRangeDownloader(ctx) },
			wantURI:  wantObjURI,
		},
		{
			name:     "Copier.Run uses destination",
			spanName: "Copier.Run",
			op: func(ctx context.Context, c *Client) {
				dst := c.Bucket(dstBucket).Object(dstObject)
				dst.CopierFrom(c.Bucket(bucket).Object(object)).Run(ctx)
			},
			wantURI: wantDstURI,
		},
		{
			name:     "Composer.Run uses destination",
			spanName: "Composer.Run",
			op: func(ctx context.Context, c *Client) {
				dst := c.Bucket(dstBucket).Object(dstObject)
				dst.ComposerFrom(c.Bucket(dstBucket).Object("src1")).Run(ctx)
			},
			wantURI: wantDstURI,
		},
		{
			name:     "object ACL.List",
			spanName: "ACL.List",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).Object(object).ACL().List(ctx) },
			wantURI:  wantObjURI,
		},
		{
			name:     "object ACL.Set",
			spanName: "ACL.Set",
			op: func(ctx context.Context, c *Client) {
				c.Bucket(bucket).Object(object).ACL().Set(ctx, AllUsers, RoleReader)
			},
			wantURI: wantObjURI,
		},
		{
			name:     "object ACL.Delete",
			spanName: "ACL.Delete",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).Object(object).ACL().Delete(ctx, AllUsers) },
			wantURI:  wantObjURI,
		},
		{
			name:     "bucket ACL.List has no URI",
			spanName: "ACL.List",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).ACL().List(ctx) },
		},
		{
			name:     "default object ACL.List has no URI",
			spanName: "ACL.List",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).DefaultObjectACL().List(ctx) },
		},
		{
			name:     "Bucket.Attrs has no URI",
			spanName: "Bucket.Attrs",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).Attrs(ctx) },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(storageOtelTracingDevVar, "true")
			// Disable the bucket metadata cache so no background fetch runs.
			t.Setenv(storageBucketMetadataDisabledVar, "true")
			ctx := context.Background()
			te := testutil.NewOpenTelemetryTestExporter()
			t.Cleanup(func() { te.Unregister(ctx) })

			tc.op(ctx, newTraceTestClient(t))

			got, ok := storageURIFromSpan(t, te.Spans(), tc.spanName)
			if tc.wantURI == "" {
				if ok {
					t.Errorf("%s = %q, want attribute absent", storageURIAttrKey, got)
				}
				return
			}
			if !ok {
				t.Fatalf("%s not found on span %q", storageURIAttrKey, tc.spanName)
			}
			if got != tc.wantURI {
				t.Errorf("%s = %q, want %q", storageURIAttrKey, got, tc.wantURI)
			}
		})
	}
}

func TestStorageURIAttributeDevTracingDisabled(t *testing.T) {
	t.Setenv(storageOtelTracingDevVar, "false")
	t.Setenv(storageBucketMetadataDisabledVar, "true")
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() { te.Unregister(ctx) })

	newTraceTestClient(t).Bucket("b").Object("o").Attrs(ctx)

	for _, s := range te.Spans() {
		for _, a := range s.Attributes {
			if string(a.Key) == storageURIAttrKey {
				t.Errorf("span %q has %s=%q, want it absent when dev tracing is disabled", s.Name, storageURIAttrKey, a.Value.AsString())
			}
		}
	}
}
