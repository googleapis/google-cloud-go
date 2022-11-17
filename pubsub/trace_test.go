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
	"time"

	"cloud.google.com/go/internal/testutil"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	pb "google.golang.org/genproto/googleapis/pubsub/v1"
	"google.golang.org/protobuf/proto"
)

func TestPublishSpan(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	spanRecorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	topic, err := c.CreateTopic(ctx, "t")
	if err != nil {
		t.Fatalf("failed to create topic: %v", err)
	}
	m := &Message{
		Data: []byte("test"),
	}
	r := topic.Publish(ctx, m)
	id, err := r.Get(ctx)
	if err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}
	defer topic.Stop()

	spans := spanRecorder.Ended()

	msgSize := proto.Size(&pb.PubsubMessage{
		Data:        m.Data,
		Attributes:  m.Attributes,
		OrderingKey: m.OrderingKey,
	})

	want := []struct {
		spanName   string
		attributes []attribute.KeyValue
	}{
		{
			spanName:   publishFlowControlSpanName,
			attributes: []attribute.KeyValue{},
		},
		{
			spanName:   publishSchedulerSpanName,
			attributes: []attribute.KeyValue{},
		},
		{
			spanName: publishRPCSpanName,
			attributes: []attribute.KeyValue{
				attribute.Int(numBatchedMessagesAttribute, 1),
			},
		},
		{
			spanName: "projects/P/topics/t send",
			attributes: []attribute.KeyValue{
				semconv.MessagingSystemKey.String("pubsub"),
				semconv.MessagingDestinationKey.String(topic.name),
				semconv.MessagingDestinationKindTopic,
				semconv.MessagingMessageIDKey.String(id),
				semconv.MessagingMessagePayloadSizeBytesKey.Int(msgSize),
				attribute.String(orderingAttribute, m.OrderingKey),
			},
		},
	}
	for i, span := range spans {
		if !span.SpanContext().IsValid() {
			t.Fatalf("span(%d) is invalid: %v", i, span)
		}
		if span.Name() != want[i].spanName {
			t.Errorf("span(%d) got name: %s, want: %s", i, span.Name(), want[i].spanName)
		}
		gotLength := len(span.Attributes())
		wantLength := len(want[i].attributes)
		if gotLength != wantLength {
			t.Fatalf("got mismatched attribute lengths for span(%d), got: %d, want: %d", i, gotLength, wantLength)
		}
		for j, kv := range span.Attributes() {
			// got := kv.Value
			if diff := testutil.Diff(kv.Key, want[i].attributes[j].Key); diff != "" {
				t.Errorf("span(%d): +got,-want: %s", i, diff)
			}
		}
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
	_, err := r.Get(ctx)
	if err != nil {
		t.Fatalf("Failed to publish message: %v", err)
	}
	defer topic.Stop()

	spanRecorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)

	sub, err := c.CreateSubscription(ctx, "sub", SubscriptionConfig{
		Topic: topic,
	})
	if err != nil {
		t.Fatalf("Failed to create subscription: %v", err)
	}
	ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	sub.Receive(ctx2, func(ctx context.Context, m *Message) {
		m.Ack()
	})
	spans := spanRecorder.Ended()

	want := []struct {
		spanName   string
		attributes []attribute.KeyValue
	}{
		{
			spanName:   publishFlowControlSpanName,
			attributes: []attribute.KeyValue{},
		},
		{
			spanName:   publishSchedulerSpanName,
			attributes: []attribute.KeyValue{},
		},
		{
			spanName: publishRPCSpanName,
			attributes: []attribute.KeyValue{
				attribute.Int(numBatchedMessagesAttribute, 1),
			},
		},
		{
			spanName: "projects/P/topics/t process",
			attributes: []attribute.KeyValue{
				semconv.MessagingSystemKey.String("pubsub"),
				semconv.MessagingDestinationKey.String(topic.name),
				semconv.MessagingDestinationKindTopic,
			},
		},
	}
	for i, span := range spans {
		if !span.SpanContext().IsValid() {
			t.Fatalf("span(%d) is invalid: %v", i, span)
		}
		if span.Name() != want[i].spanName {
			t.Errorf("span(%d) got name: %s, want: %s", i, span.Name(), want[i].spanName)
		}
		gotLength := len(span.Attributes())
		wantLength := len(want[i].attributes)
		if gotLength != wantLength {
			t.Fatalf("got mismatched attribute lengths for span(%d), got: %d, want: %d", i, gotLength, wantLength)
		}
		for j, kv := range span.Attributes() {
			// got := kv.Value
			if diff := testutil.Diff(kv.Key, want[i].attributes[j].Key); diff != "" {
				t.Errorf("span(%d): +got,-want: %s", i, diff)
			}
		}
	}
	for i, span := range spans {
		// TODO(hongalex): test subscribe path spans
		fmt.Printf("got span(%d): %+v\n", i, span)
	}
}
