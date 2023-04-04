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
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/googleapi"
)

var tracer trace.Tracer

func init() {
	tracer = otel.Tracer("cloud.google.com/go")
}

// StartSpan adds a span to the trace with the given name.
func StartSpan(ctx context.Context, name string) context.Context {
	ctx, _ = tracer.Start(ctx, name)
	return ctx
}

// EndSpan ends a span with the given error.
func EndSpan(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(toStatus(err))
	}
	span.End()
}

// toStatus interrogates an error and converts it to an appropriate
// OpenCensus status.
func toStatus(err error) (codes.Code, string) {
	var err2 *googleapi.Error
	if ok := errors.As(err, &err2); ok {
		return codes.Error, err2.Message
	} else {
		return codes.Error, err.Error()
	}
}

// TODO: (odeke-em): perhaps just pass around spans due to the cost
// incurred from using trace.FromContext(ctx) yet we could avoid
// throwing away the work done by ctx, span := trace.StartSpan.
func TracePrintf(ctx context.Context, attrMap map[string]interface{}, format string, args ...interface{}) {
	var attrs []attribute.KeyValue
	for k, v := range attrMap {
		var a attribute.KeyValue
		switch v := v.(type) {
		case string:
			a = attribute.String(k, v)
		case bool:
			a = attribute.Bool(k, v)
		case int:
			a = attribute.Int(k, v)
		case int64:
			a = attribute.Int64(k, v)
		default:
			a = attribute.String(k, fmt.Sprintf("%#v", v))
		}
		attrs = append(attrs, a)
	}
	trace.SpanFromContext(ctx).AddEvent(format, trace.WithAttributes(attrs...))
}
