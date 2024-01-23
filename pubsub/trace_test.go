// Copyright 2023 Google LLC
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
	pb "cloud.google.com/go/pubsub/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/internal"
	"cloud.google.com/go/pubsub/pstest"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func TestTrace_MessageCarrier(t *testing.T) {
	ctx := context.Background()
	e := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(e))
	otel.SetTextMapPropagator(propagation.TraceContext{})
	defer tp.Shutdown(ctx)
	otel.SetTracerProvider(tp)

	ctx, _ = tp.Tracer("a").Start(ctx, "fake-span")
	msg := &Message{
		Data:        []byte("asdf"),
		OrderingKey: "asdf",
		Attributes:  map[string]string{},
	}
	otel.GetTextMapPropagator().Inject(ctx, newMessageCarrier(msg))

	if _, ok := msg.Attributes[googclientPrefix+"traceparent"]; !ok {
		t.Fatalf("expected traceparent in message attributes, found empty string")
	}

	newCtx := context.Background()
	propagation.TraceContext{}.Extract(newCtx, newMessageCarrier(msg))
	if _, ok := msg.Attributes[googclientPrefix+"traceparent"]; !ok {
		t.Fatalf("expected traceparent in message attributes, found empty string")
	}
}

func TestTrace_PublishSpan(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	e := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(e))
	otel.SetTextMapPropagator(propagation.TraceContext{})
	defer tp.Shutdown(ctx)
	otel.SetTracerProvider(tp)

	m := &Message{
		Data:        []byte("test"),
		OrderingKey: "my-key",
	}

	msgSize := proto.Size(&pb.PubsubMessage{
		Data:        m.Data,
		Attributes:  m.Attributes,
		OrderingKey: m.OrderingKey,
	})

	topicID := "t"
	topicName := fmt.Sprintf("projects/P/topics/%s", topicID)

	expectedSpans := tracetest.SpanStubs{
		tracetest.SpanStub{
			Name:     fmt.Sprintf("%s %s", topicName, publisherSpanName),
			SpanKind: trace.SpanKindProducer,
			Attributes: []attribute.KeyValue{
				semconv.MessagingDestinationName(topicName),
				// Hardcoded since the fake server always returns m0 first.
				semconv.MessagingMessageIDKey.String("m0"),
				semconv.MessagingMessagePayloadSizeBytesKey.Int(msgSize),
				attribute.String(orderingAttribute, m.OrderingKey),
				semconv.MessagingSystemKey.String("pubsub"),
			},
			ChildSpanCount: 2,
			InstrumentationLibrary: instrumentation.Scope{
				Name:    "cloud.google.com/go/pubsub",
				Version: internal.Version,
			},
		},
		tracetest.SpanStub{
			Name: publishFlowControlSpanName,
			InstrumentationLibrary: instrumentation.Scope{
				Name:    "cloud.google.com/go/pubsub",
				Version: internal.Version,
			},
		},
		tracetest.SpanStub{
			Name: publishBatcherSpanName,
			InstrumentationLibrary: instrumentation.Scope{
				Name:    "cloud.google.com/go/pubsub",
				Version: internal.Version,
			},
		},
		tracetest.SpanStub{
			Name: publishRPCSpanName,
			Attributes: []attribute.KeyValue{
				semconv.MessagingBatchMessageCount(1),
			},
			InstrumentationLibrary: instrumentation.Scope{
				Name:    "cloud.google.com/go/pubsub",
				Version: internal.Version,
			},
		},
	}

	topic, err := c.CreateTopic(ctx, topicID)
	if err != nil {
		t.Fatalf("failed to create topic: %v", err)
	}
	if m.OrderingKey != "" {
		topic.EnableMessageOrdering = true
	}
	r := topic.Publish(ctx, m)
	_, err = r.Get(ctx)
	if err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}
	defer topic.Stop()

	got := getSpans(e)
	opts := []cmp.Option{
		cmp.Comparer(spanStubComparer),
		cmpopts.SortSlices(sortSpanStub),
	}
	if diff := testutil.Diff(got, expectedSpans, opts...); diff != "" {
		t.Logf("got spans: %+v\n", got)
		t.Logf("expected spans: %+v\n", expectedSpans)
		t.Errorf("diff: -got, +want:\n%s\n", diff)
	}
}

func TestTrace_PublishSpanError(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	e := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(e))
	defer tp.Shutdown(ctx)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetTracerProvider(tp)

	m := &Message{
		Data:        []byte("test"),
		OrderingKey: "m",
	}

	msgSize := proto.Size(&pb.PubsubMessage{
		Data:        m.Data,
		Attributes:  m.Attributes,
		OrderingKey: m.OrderingKey,
	})

	topicID := "t"
	topicName := fmt.Sprintf("projects/P/topics/%s", topicID)

	topic, err := c.CreateTopic(ctx, topicID)
	if err != nil {
		t.Fatalf("failed to create topic: %v", err)
	}

	// Publishing a message with an ordering key without enabling ordering topic ordering
	// should fail.
	t.Run("no ordering key", func(t *testing.T) {
		r := topic.Publish(ctx, m)
		_, err = r.Get(ctx)
		if err == nil {
			t.Fatal("expected err, got nil")
		}

		want := getPublishSpanStubsWithError(topicName, m, msgSize, errTopicOrderingNotEnabled)

		got := getSpans(e)
		opts := []cmp.Option{
			cmp.Comparer(spanStubComparer),
			cmpopts.SortSlices(sortSpanStub),
		}
		if diff := testutil.Diff(got, want, opts...); diff != "" {
			t.Errorf("diff: -got, +want:\n%s\n", diff)
		}
		e.Reset()
		topic.ResumePublish(m.OrderingKey)
	})

	t.Run("stopped topic", func(t *testing.T) {
		// Publishing a message with a stopped publisher should fail too
		topic.ResumePublish(m.OrderingKey)
		topic.EnableMessageOrdering = true
		topic.Stop()
		r := topic.Publish(ctx, m)
		_, err = r.Get(ctx)
		if err == nil {
			t.Fatal("expected err, got nil")
		}

		got := getSpans(e)
		want := getPublishSpanStubsWithError(topicName, m, msgSize, ErrTopicStopped)
		opts := []cmp.Option{
			cmp.Comparer(spanStubComparer),
			cmpopts.SortSlices(sortSpanStub),
		}
		if diff := testutil.Diff(got, want, opts...); diff != "" {
			t.Errorf("diff: -got, +want:\n%s\n", diff)
		}
		e.Reset()
		topic.ResumePublish(m.OrderingKey)
	})

	t.Run("flow control error", func(t *testing.T) {
		// Use a different topic here than above since
		// we need to adjust the flow control settings,
		// which are immutable after publish.
		topicID := "t2"

		topic, err := c.CreateTopic(ctx, topicID)
		if err != nil {
			t.Fatalf("failed to create topic: %v", err)
		}
		topic.EnableMessageOrdering = true
		topic.PublishSettings.FlowControlSettings = FlowControlSettings{
			LimitExceededBehavior: FlowControlSignalError,
			MaxOutstandingBytes:   1,
		}

		r := topic.Publish(ctx, m)
		_, err = r.Get(ctx)
		if err == nil {
			t.Fatal("expected err, got nil")
		}

		got := getSpans(e)
		want := getFlowControlSpanStubs(ErrFlowControllerMaxOutstandingBytes)
		opts := []cmp.Option{
			cmp.Comparer(spanStubComparer),
			cmpopts.SortSlices(sortSpanStub),
		}
		if diff := testutil.Diff(got, want, opts...); diff != "" {
			t.Errorf("diff: -got, +want:\n%s\n", diff)
		}
	})
}

func TestTrace_SubscribeSpans(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	// For subscribe spans, we'll publish before setting the tracer provider
	// so we don't trace the publish spans. Context propagation will be tested
	// at a later time.
	m := &Message{
		Data:        []byte("test"),
		OrderingKey: "my-key",
	}

	msgSize := proto.Size(&pb.PubsubMessage{
		Data:        m.Data,
		Attributes:  m.Attributes,
		OrderingKey: m.OrderingKey,
	})

	topicID := "t"
	// topicName := fmt.Sprintf("projects/P/topics/%s", topicID)

	topic, err := c.CreateTopic(ctx, topicID)
	if err != nil {
		t.Fatalf("failed to create topic: %v", err)
	}

	subID := "s"
	subName := fmt.Sprintf("projects/P/subscriptions/%s", subID)
	enableEOS := false

	sub, err := c.CreateSubscription(ctx, subID, SubscriptionConfig{
		Topic:                     topic,
		EnableExactlyOnceDelivery: enableEOS,
	})
	if err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}
	if m.OrderingKey != "" {
		topic.EnableMessageOrdering = true
	}

	// Call publish before enabling tracer provider to only test subscribe spans.
	r := topic.Publish(ctx, m)
	_, err = r.Get(ctx)
	if err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}

	e := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(e))
	defer tp.Shutdown(ctx)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetTracerProvider(tp)

	ctx, cancel := context.WithCancel(ctx)

	sub.Receive(ctx, func(ctx context.Context, m *Message) {
		m.Ack()
		cancel()
	})

	expectedSpans := tracetest.SpanStubs{
		tracetest.SpanStub{
			Name: fmt.Sprintf("%s %s", subName, subscribeProcessSpanName),
			Attributes: []attribute.KeyValue{
				attribute.String(resultAttribute, "ack"),
			},
			Events: []sdktrace.Event{},
			InstrumentationLibrary: instrumentation.Scope{
				Name:    "cloud.google.com/go/pubsub",
				Version: internal.Version,
			},
		},
		tracetest.SpanStub{
			Name:     fmt.Sprintf("%s %s", subName, subscribeSpanName),
			SpanKind: trace.SpanKindConsumer,
			Attributes: []attribute.KeyValue{
				semconv.MessagingDestinationName(topicName),
				// Hardcoded since the fake server always returns m0 first.
				semconv.MessagingMessageIDKey.String("m0"),
				semconv.MessagingMessagePayloadSizeBytesKey.Int(msgSize),
				semconv.MessagingOperationReceive,
				// The fake server uses message ID as ackID, this is not the case with live service.
				attribute.String(ackIDAttribute, "m0"),
				attribute.Bool(eosAttribute, enableEOS),
				attribute.String(resultAttribute, "ack"),
				attribute.String(orderingAttribute, m.OrderingKey),
				semconv.MessagingSystemKey.String("pubsub"),
				semconv.MessagingBatchMessageCount(1),
			},
			InstrumentationLibrary: instrumentation.Scope{
				Name:    "cloud.google.com/go/pubsub",
				Version: internal.Version,
			},
		},
		tracetest.SpanStub{
			Name: subscribeSchedulerSpanName,
			InstrumentationLibrary: instrumentation.Scope{
				Name:    "cloud.google.com/go/pubsub",
				Version: internal.Version,
			},
		},
		tracetest.SpanStub{
			Name: subscriberFlowControlSpanName,
			InstrumentationLibrary: instrumentation.Scope{
				Name:    "cloud.google.com/go/pubsub",
				Version: internal.Version,
			},
		},
		tracetest.SpanStub{
			Name: ackSpanName,
			InstrumentationLibrary: instrumentation.Scope{
				Name:    "cloud.google.com/go/pubsub",
				Version: internal.Version,
			},
		},
		tracetest.SpanStub{
			Name: modAckSpanName,
			InstrumentationLibrary: instrumentation.Scope{
				Name:    "cloud.google.com/go/pubsub",
				Version: internal.Version,
			},
			Attributes: []attribute.KeyValue{
				attribute.Bool(receiptModackAttribute, true),
				attribute.Int(ackDeadlineSecAttribute, 10),
				semconv.MessagingBatchMessageCount(1),
			},
		},
	}

	got := getSpans(e)
	opts := []cmp.Option{
		cmp.Comparer(spanStubComparer),
		cmpopts.SortSlices(sortSpanStub),
	}
	if diff := testutil.Diff(got, expectedSpans, opts...); diff != "" {
		t.Logf("got spans: %+v\n", got)
		t.Logf("expected spans: %+v\n", expectedSpans)
		t.Errorf("diff: -got, +want:\n%s\n", diff)
	}
}

func TestTrace_WithTracingDisabled(t *testing.T) {
	ctx := context.Background()
	srv := pstest.NewServer()
	c, err := NewClientWithConfig(ctx, projName,
		&ClientConfig{DisableOpenTelemetryTracing: true},
		option.WithEndpoint(srv.Addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithInsecure()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	defer srv.Close()

	e := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(e))
	defer tp.Shutdown(ctx)
	otel.SetTracerProvider(tp)

	m := &Message{
		Data: []byte("test"),
	}

	topicID := "t"

	topic, err := c.CreateTopic(ctx, topicID)
	if err != nil {
		t.Fatalf("failed to create topic: %v", err)
	}
	r := topic.Publish(ctx, m)
	_, err = r.Get(ctx)
	if err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}
	defer topic.Stop()

	got := getSpans(e)
	if len(got) != 0 {
		t.Fatalf("expected no spans to be exported when tracing is disabled")
	}
}

func spanStubComparer(a, b tracetest.SpanStub) bool {
	if a.Name != b.Name {
		return false
	}
	if a.ChildSpanCount != b.ChildSpanCount {
		return false
	}
	as := attribute.NewSet(a.Attributes...)
	bs := attribute.NewSet(b.Attributes...)
	if !as.Equals(&bs) {
		return false
	}
	if a.InstrumentationLibrary != b.InstrumentationLibrary {
		return false
	}
	if a.Status != b.Status {
		return false
	}
	return true

}

func sortSpanStub(a, b tracetest.SpanStub) bool {
	return a.Name < b.Name
}

func getSpans(e *tracetest.InMemoryExporter) tracetest.SpanStubs {
	// Wait a fixed amount for spans to be fully exported.
	time.Sleep(100 * time.Millisecond)

	return e.GetSpans()
}

func getPublishSpanStubsWithError(topicName string, m *Message, msgSize int, err error) tracetest.SpanStubs {
	return tracetest.SpanStubs{
		tracetest.SpanStub{
			Name:     fmt.Sprintf("%s %s", topicName, publisherSpanName),
			SpanKind: trace.SpanKindProducer,
			Attributes: []attribute.KeyValue{
				semconv.MessagingDestinationName(topicName),
				semconv.MessagingMessageIDKey.String(""),
				semconv.MessagingMessagePayloadSizeBytesKey.Int(msgSize),
				attribute.String(orderingAttribute, m.OrderingKey),
				semconv.MessagingSystemKey.String("pubsub"),
			},
			InstrumentationLibrary: instrumentation.Scope{
				Name:    "cloud.google.com/go/pubsub",
				Version: internal.Version,
			},
			Status: sdktrace.Status{
				Code:        codes.Error,
				Description: err.Error(),
			},
		},
	}
}

func getFlowControlSpanStubs(err error) tracetest.SpanStubs {
	return tracetest.SpanStubs{
		tracetest.SpanStub{
			Name: publishFlowControlSpanName,
			InstrumentationLibrary: instrumentation.Scope{
				Name:    "cloud.google.com/go/pubsub",
				Version: internal.Version,
			},
			Status: sdktrace.Status{
				Code:        codes.Error,
				Description: err.Error(),
			},
		},
	}
}
