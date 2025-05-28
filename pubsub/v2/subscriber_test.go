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

package pubsub

import (
	"context"
	"errors"
	"fmt"
	"log"
	"testing"
	"time"

	pb "cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/v2/pstest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestReceive(t *testing.T) {
	testReceive(t, false)
	testReceive(t, false)
	testReceive(t, true)
}

func testReceive(t *testing.T, exactlyOnceDelivery bool) {
	t.Run(fmt.Sprintf("exactlyOnceDelivery:%t", exactlyOnceDelivery), func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		client, srv := newFake(t)
		defer client.Close()
		defer srv.Close()

		topicName := "projects/p/topics/t"
		mustCreateTopic(t, client, topicName)
		sub := mustCreateSubConfig(t, client, &pb.Subscription{
			Name:                      "projects/p/subscriptions/s",
			Topic:                     topicName,
			EnableExactlyOnceDelivery: exactlyOnceDelivery,
		})
		for i := 0; i < 256; i++ {
			srv.Publish(topicName, []byte{byte(i)}, nil)
		}
		msgs, err := pullN(ctx, sub, 256, 0, func(_ context.Context, m *Message) {
			if exactlyOnceDelivery {
				ar := m.AckWithResult()
				// Don't use the above ctx here since that will get cancelled.
				ackStatus, err := ar.Get(context.Background())
				if err != nil {
					t.Fatalf("pullN err for message(%s): %v", m.ID, err)
				}
				if ackStatus != AcknowledgeStatusSuccess {
					t.Fatalf("pullN got non-success AckStatus: %v", ackStatus)
				}
			} else {
				m.Ack()
			}
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Pull: %v", err)
		}
		var seen [256]bool
		for _, m := range msgs {
			seen[m.Data[0]] = true
		}
		for i, saw := range seen {
			if !saw {
				t.Errorf("eod=%t: did not see message #%d", exactlyOnceDelivery, i)
			}
		}
	})
}

// Note: be sure to close client and server!
func newFake(t *testing.T) (*Client, *pstest.Server) {
	ctx := context.Background()
	srv := pstest.NewServer()
	client, err := NewClient(ctx, projName,
		option.WithEndpoint(srv.Addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithInsecure()),
		option.WithTelemetryDisabled(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return client, srv
}

// Check if incoming ReceivedMessages are properly converted to Message structs
// that expose the DeliveryAttempt field when dead lettering is enabled/disabled.
func TestDeadLettering_toMessage(t *testing.T) {
	// If dead lettering is disabled, DeliveryAttempt should default to 0.
	receivedMsg := &pb.ReceivedMessage{
		AckId: "1234",
		Message: &pb.PubsubMessage{
			Data:        []byte("some message"),
			MessageId:   "id-1234",
			PublishTime: timestamppb.Now(),
		},
	}
	got, err := toMessage(receivedMsg, time.Time{}, nil)
	if err != nil {
		t.Errorf("toMessage failed: %v", err)
	}
	if got.DeliveryAttempt != nil {
		t.Errorf("toMessage with dead-lettering disabled failed\ngot: %d, want nil", *got.DeliveryAttempt)
	}

	// If dead lettering is enabled, toMessage should properly pass through the DeliveryAttempt field.
	receivedMsg.DeliveryAttempt = 10
	got, err = toMessage(receivedMsg, time.Time{}, nil)
	if err != nil {
		t.Errorf("toMessage failed: %v", err)
	}
	if *got.DeliveryAttempt != int(receivedMsg.DeliveryAttempt) {
		t.Errorf("toMessage with dead-lettered enabled failed\ngot: %d, want %d", *got.DeliveryAttempt, receivedMsg.DeliveryAttempt)
	}
}

func TestExactlyOnceDelivery_AckSuccess(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topicName := fmt.Sprintf("projects/%s/topics/t", projName)
	subName := fmt.Sprintf("projects/%s/subscriptions/s", subID)
	publisher := mustCreateTopic(t, client, topicName)
	s := mustCreateSubConfig(t, client, &pb.Subscription{
		Name:  subName,
		Topic: topicName,
	})
	s.ReceiveSettings.NumGoroutines = 1
	r := publisher.Publish(ctx, &Message{
		Data: []byte("exactly-once-message"),
	})
	if _, err := r.Get(ctx); err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}

	err := s.Receive(ctx, func(ctx context.Context, msg *Message) {
		ar := msg.AckWithResult()
		s, err := ar.Get(ctx)
		if s != AcknowledgeStatusSuccess {
			t.Errorf("AckResult AckStatus got %v, want %v", s, AcknowledgeStatusSuccess)
		}
		if err != nil {
			t.Errorf("AckResult error got %v", err)
		}
		cancel()
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("s.Receive err: %v", err)
	}
}

func TestExactlyOnceDelivery_AckFailureErrorPermissionDenied(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	srv := pstest.NewServer(pstest.WithErrorInjection("Acknowledge", codes.PermissionDenied, "insufficient permission"))
	client, err := NewClient(ctx, projName,
		option.WithEndpoint(srv.Addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	defer srv.Close()

	topicName := fmt.Sprintf("projects/%s/topics/t", projName)
	subName := fmt.Sprintf("projects/%s/subscriptions/s", subID)
	publisher := mustCreateTopic(t, client, topicName)
	s := mustCreateSubConfig(t, client, &pb.Subscription{
		Name:                      subName,
		Topic:                     topicName,
		EnableExactlyOnceDelivery: true,
	})
	s.ReceiveSettings.NumGoroutines = 1
	r := publisher.Publish(ctx, &Message{
		Data: []byte("exactly-once-message"),
	})
	if _, err := r.Get(ctx); err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}
	err = s.Receive(ctx, func(ctx context.Context, msg *Message) {
		ar := msg.AckWithResult()
		s, err := ar.Get(ctx)
		if s != AcknowledgeStatusPermissionDenied {
			t.Errorf("AckResult AckStatus got %v, want %v", s, AcknowledgeStatusPermissionDenied)
		}
		wantErr := status.Errorf(codes.PermissionDenied, "insufficient permission")
		if !errors.Is(err, wantErr) {
			t.Errorf("AckResult error\ngot  %v\nwant %s", err, wantErr)
		}
		cancel()
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("s.Receive err: %v", err)
	}
}

func TestExactlyOnceDelivery_AckRetryDeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := pstest.NewServer(pstest.WithErrorInjection("Acknowledge", codes.Internal, "internal error"))
	client, err := NewClient(ctx, projName,
		option.WithEndpoint(srv.Addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	defer srv.Close()

	topicName := fmt.Sprintf("projects/%s/topics/t", projName)
	subName := fmt.Sprintf("projects/%s/subscriptions/s", subID)
	topic := mustCreateTopic(t, client, topicName)
	s := mustCreateSubConfig(t, client, &pb.Subscription{
		Name:                      subName,
		Topic:                     topicName,
		EnableExactlyOnceDelivery: true,
	})
	r := topic.Publish(ctx, &Message{
		Data: []byte("exactly-once-message"),
	})
	if _, err := r.Get(ctx); err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}

	s.ReceiveSettings = ReceiveSettings{
		NumGoroutines: 1,
	}
	// Override the default timeout here so this test doesn't take 10 minutes.
	exactlyOnceDeliveryRetryDeadline = 10 * time.Second
	err = s.Receive(ctx, func(ctx context.Context, msg *Message) {
		log.Printf("received message: %v\n", msg)
		ar := msg.AckWithResult()
		s, err := ar.Get(ctx)
		if s != AcknowledgeStatusOther {
			t.Errorf("AckResult AckStatus got %v, want %v", s, AcknowledgeStatusOther)
		}
		wantErr := context.DeadlineExceeded
		if !errors.Is(err, wantErr) {
			t.Errorf("AckResult error\ngot  %v\nwant %s", err, wantErr)
		}
		cancel()
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("s.Receive err: %v", err)
	}
}

func TestExactlyOnceDelivery_NackSuccess(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topicName := fmt.Sprintf("projects/%s/topics/t", projName)
	subName := fmt.Sprintf("projects/%s/subscriptions/s", subID)
	publisher := mustCreateTopic(t, client, topicName)
	s := mustCreateSubConfig(t, client, &pb.Subscription{
		Name:                      subName,
		Topic:                     topicName,
		EnableExactlyOnceDelivery: true,
	})
	r := publisher.Publish(ctx, &Message{
		Data: []byte("exactly-once-message"),
	})
	if _, err := r.Get(ctx); err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}

	s.ReceiveSettings = ReceiveSettings{
		NumGoroutines: 1,
	}
	err := s.Receive(ctx, func(ctx context.Context, msg *Message) {
		ar := msg.NackWithResult()
		s, err := ar.Get(context.Background())
		if s != AcknowledgeStatusSuccess {
			t.Errorf("AckResult AckStatus got %v, want %v", s, AcknowledgeStatusSuccess)
		}
		if err != nil {
			t.Errorf("AckResult error got %v", err)
		}
		cancel()
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("s.Receive err: %v", err)
	}
}

func TestExactlyOnceDelivery_ReceiptModackError(t *testing.T) {
	ctx := context.Background()
	srv := pstest.NewServer(pstest.WithErrorInjection("ModifyAckDeadline", codes.Internal, "internal error"))
	client, err := NewClient(ctx, projName,
		option.WithEndpoint(srv.Addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	defer srv.Close()

	topicName := fmt.Sprintf("projects/%s/topics/t", projName)
	subName := fmt.Sprintf("projects/%s/subscriptions/s", subID)
	publisher := mustCreateTopic(t, client, topicName)
	s := mustCreateSubConfig(t, client, &pb.Subscription{
		Name:                      subName,
		Topic:                     topicName,
		EnableExactlyOnceDelivery: true,
	})
	r := publisher.Publish(ctx, &Message{
		Data: []byte("exactly-once-message"),
	})
	if _, err := r.Get(ctx); err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	s.Receive(ctx, func(ctx context.Context, msg *Message) {
		t.Fatal("expected message to not have been delivered when exactly once enabled")
	})
}

func TestSubscribeMessageExpirationFlowControl(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topicName := fmt.Sprintf("projects/%s/topics/t", projName)
	subName := fmt.Sprintf("projects/%s/subscriptions/s", subID)
	publisher := mustCreateTopic(t, client, topicName)
	s := mustCreateSubConfig(t, client, &pb.Subscription{
		Name:  subName,
		Topic: topicName,
	})

	s.ReceiveSettings.NumGoroutines = 1
	s.ReceiveSettings.MaxOutstandingMessages = 1
	s.ReceiveSettings.MaxExtension = 10 * time.Second
	s.ReceiveSettings.MaxDurationPerAckExtension = 10 * time.Second
	r := publisher.Publish(ctx, &Message{
		Data: []byte("redelivered-message"),
	})
	if _, err := r.Get(ctx); err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}

	deliveryCount := 0
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := s.Receive(ctx, func(ctx context.Context, msg *Message) {
		// Only acknowledge the message on the 2nd invocation of the callback (2nd delivery).
		if deliveryCount == 1 {
			msg.Ack()
		}
		// Otherwise, do nothing and let the message expire.
		deliveryCount++
		if deliveryCount == 2 {
			cancel()
		}
	})
	if deliveryCount != 2 {
		t.Fatalf("expected 2 iterations of the callback, got %d", deliveryCount)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("s.Receive err: %v", err)
	}
}

func mustCreateSubConfig(t *testing.T, c *Client, pbs *pb.Subscription) *Subscriber {
	ctx := context.Background()
	if _, err := c.SubscriptionAdminClient.CreateSubscription(ctx, pbs); err != nil {
		t.Fatal(err)
	}
	return c.Subscriber(pbs.Name)
}
