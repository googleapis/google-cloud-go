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
	"log"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	pb "cloud.google.com/go/pubsub/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/internal"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
)

func TestTrace_MessageCarrier(t *testing.T) {
	ctx := context.Background()
	msg := &Message{
		Data:        []byte("asdf"),
		OrderingKey: "asdf",
	}
	otel.GetTextMapPropagator().Extract(ctx, newMessageCarrier(msg))
}

func TestTrace_PublishSpan(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	e := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(e))
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
				semconv.MessagingDestinationKindTopic,
				semconv.MessagingDestinationKey.String(topicName),
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
				attribute.Int(numBatchedMessagesAttribute, 1),
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
		log.Printf("print spans: %+v\n", got)
		log.Printf("\n\nprint expected spans: %+v\n", expectedSpans)
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
				semconv.MessagingDestinationKindTopic,
				semconv.MessagingDestinationKey.String(topicName),
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
