// Copyright 2022 Google LLC
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

package pubsub

import (
	"context"
	"fmt"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestPublishSpan(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	spanRecorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	topic := c.Topic("t")
	r := topic.Publish(ctx, &Message{
		Data: []byte("test"),
	})
	r.Get(ctx)
	defer topic.Stop()

	spans := spanRecorder.Ended()
	for i, span := range spans {

		// Check span
		// assert.True(t, span.SpanContext().IsValid())
		// assert.Equal(t, "pubsub.topic", span.Name())
		fmt.Printf("got span %d: %+v\n", i, span)
	}
}

func TestSubscribeSpan(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topic := c.Topic("t")
	r := topic.Publish(ctx, &Message{
		Data: []byte("test"),
	})
	r.Get(ctx)
	defer topic.Stop()

	spanRecorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	spans := spanRecorder.Ended()
	for i, span := range spans {

		// Check span
		// assert.True(t, span.SpanContext().IsValid())
		// assert.Equal(t, "pubsub.topic", span.Name())
		fmt.Printf("got span %d: %+v\n", i, span)
	}
}
