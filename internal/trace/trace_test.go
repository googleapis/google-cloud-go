// Copyright 2018 Google LLC
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

package trace

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/googleapis/gax-go/v2/apierror"
	"go.opentelemetry.io/otel/attribute"
	otcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	ignoreEventFields = cmpopts.IgnoreFields(trace.Event{}, "Time")
	ignoreValueFields = cmpopts.IgnoreFields(attribute.Value{}, "vtype", "numeric", "stringly", "slice")
)

func TestStartSpan(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})

	ctx = StartSpan(ctx, "test-span")

	TracePrintf(ctx, annotationData(), "Add my annotations")

	err := &googleapi.Error{Code: http.StatusBadRequest, Message: "INVALID ARGUMENT"}
	EndSpan(ctx, err)
	spans := te.Spans()
	if len(spans) != 1 {
		t.Fatalf("got %d, want 1", len(spans))
	}
	if got, want := spans[0].Name, "test-span"; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	if want := otcodes.Error; spans[0].Status.Code != want {
		t.Errorf("got %v, want %v", spans[0].Status.Code, want)
	}
	if want := "INVALID ARGUMENT"; spans[0].Status.Description != want {
		t.Errorf("got %v, want %v", spans[0].Status.Description, want)
	}

	want := []attribute.KeyValue{
		attribute.Key("my_bool").Bool(true),
		attribute.Key("my_float").String("0.9"),
		attribute.Key("my_int").Int(123),
		attribute.Key("my_int64").Int64(int64(456)),
		attribute.Key("my_string").String("my string"),
	}
	got := spans[0].Events[0].Attributes
	// Sorting is required since the TracePrintf parameter is a map.
	sort.Slice(got, func(i, j int) bool {
		return got[i].Key < got[j].Key
	})
	if !cmp.Equal(got, want, ignoreEventFields, ignoreValueFields) {
		t.Errorf("got %v, want %v", got, want)
	}
	wantEvent := trace.Event{
		Name: "exception",
		Attributes: []attribute.KeyValue{
			// KeyValues are NOT sorted by key, but the sort is deterministic,
			// since this Event was created by Span.RecordError.
			attribute.Key("exception.type").String("*googleapi.Error"),
			attribute.Key("exception.message").String("googleapi: Error 400: INVALID ARGUMENT"),
		},
	}
	if !cmp.Equal(spans[0].Events[1], wantEvent, ignoreEventFields, ignoreValueFields) {
		t.Errorf("got %v, want %v", spans[0].Events[1], want)
	}
}

func TestToOpenTelemetryStatusDescription(t *testing.T) {
	for _, testcase := range []struct {
		input error
		want  string
	}{
		{
			errors.New("some random error"),
			"some random error",
		},
		{
			&googleapi.Error{Code: http.StatusConflict, Message: "some specific googleapi http error"},
			"some specific googleapi http error",
		},
		{
			status.Error(codes.DataLoss, "some specific grpc error"),
			"some specific grpc error",
		},
	} {
		// Wrap supported types in apierror.APIError as GAPIC clients
		// do, but fall back to the unwrapped error if not supported.
		// https://github.com/googleapis/gax-go/blob/v2.12.0/v2/invoke.go#L95
		var err error
		err, ok := apierror.FromError(testcase.input)
		if !ok {
			err = testcase.input
		}

		got := toOpenTelemetryStatusDescription(err)
		if got != testcase.want {
			t.Errorf("got %s, want %s", got, testcase.want)
		}
	}
}

func annotationData() map[string]interface{} {
	attrMap := make(map[string]interface{})
	attrMap["my_string"] = "my string"
	attrMap["my_bool"] = true
	attrMap["my_int"] = 123
	attrMap["my_int64"] = int64(456)
	attrMap["my_float"] = 0.9
	return attrMap
}
