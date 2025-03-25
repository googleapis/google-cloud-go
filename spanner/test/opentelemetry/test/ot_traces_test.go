//go:build go1.20
// +build go1.20

/*
Copyright 2024 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package test

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"cloud.google.com/go/spanner"
	stestutil "cloud.google.com/go/spanner/internal/testutil"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestSpannerTracesWithOpenTelemetry(t *testing.T) {
	ctx := context.Background()
	te := newOpenTelemetryTestExporter(false, true)

	t.Cleanup(func() {
		te.Unregister(ctx)
	})

	minOpened := uint64(1)
	server, client, teardown := setupMockedTestServerWithConfig(t, spanner.ClientConfig{
		SessionPoolConfig: spanner.SessionPoolConfig{
			MinOpened: minOpened,
		},
	})
	defer teardown()

	waitFor(t, func() error {
		if isMultiplexEnabled {
			if uint64(server.TestSpanner.TotalSessionsCreated()) == minOpened+1 {
				return nil
			}
		}
		if uint64(server.TestSpanner.TotalSessionsCreated()) == minOpened {
			return nil
		}
		return errors.New("not yet initialized")
	})

	iter := client.Single().Query(context.Background(), spanner.NewStatement(stestutil.SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	iter.Do(func(r *spanner.Row) error {
		return nil
	})
	spans := te.spans()
	if len(spans) == 0 {
		t.Fatal("No spans were exported")
	}
	spanName := "cloud.google.com/go/spanner.Query"
	if !findSpan(spans, spanName) {
		t.Errorf("Expected span %s not found", spanName)
	}
}

func TestSpanAnnotatedWithXGoogSpannerRequestID_unary(t *testing.T) {
	testSpanAnnotatedWithXGoogSpannerRequestID(t, "cloud.google.com/go/spanner.BatchCreateSessions")
}

func TestSpanAnnotatedWithXGoogSpannerRequestID_streaming(t *testing.T) {
	testSpanAnnotatedWithXGoogSpannerRequestID(t, "cloud.google.com/go/spanner.RowIterator")
}

func testSpanAnnotatedWithXGoogSpannerRequestID(t *testing.T, targetSpanName string) {
	ctx := context.Background()
	te := newOpenTelemetryTestExporter(false, true)

	t.Cleanup(func() {
		te.Unregister(ctx)
	})

	minOpened := uint64(1)
	server, client, teardown := setupMockedTestServerWithConfig(t, spanner.ClientConfig{
		SessionPoolConfig: spanner.SessionPoolConfig{
			MinOpened: minOpened,
		},
	})
	defer teardown()

	waitFor(t, func() error {
		if isMultiplexEnabled {
			if uint64(server.TestSpanner.TotalSessionsCreated()) == minOpened+1 {
				return nil
			}
		}
		if uint64(server.TestSpanner.TotalSessionsCreated()) == minOpened {
			return nil
		}
		return errors.New("not yet initialized")
	})

	iter := client.Single().Query(context.Background(), spanner.NewStatement(stestutil.SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	iter.Do(func(r *spanner.Row) error {
		return nil
	})
	spans := te.spans()
	if len(spans) == 0 {
		t.Fatal("No spans were exported")
	}

	// Find the unary span "/BatchCreateSession".
	var targetSpan tracetest.SpanStub
	found := false
	for _, span := range spans {
		if span.Name == targetSpanName {
			targetSpan = span
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("Could not find target span with name: %q", targetSpanName)
	}

	var headerKey attribute.Key = "x_goog_spanner_request_id"
	ourHeaderFound := false
	foundValue := ""
	for _, attrKV := range targetSpan.Attributes {
		if attrKV.Key == headerKey {
			foundValue = attrKV.Value.AsString()
			ourHeaderFound = foundValue != ""
			break
		}
	}

	if !ourHeaderFound {
		t.Fatalf("did not find our header: %q", headerKey)
	}

	// It must match the desired format of x-goog-spanner-request-id.
	var reg = regexp.MustCompile("\\d+\\.([a-z0-9]{16})(\\.\\d+){4}")
	if !reg.MatchString(foundValue) {
		t.Fatalf("Regex=%q did not match %q", reg, foundValue)
	}
}

func findSpan(spans tracetest.SpanStubs, spanName string) bool {
	for _, span := range spans {
		if span.Name == spanName {
			return true
		}
	}
	return false
}
