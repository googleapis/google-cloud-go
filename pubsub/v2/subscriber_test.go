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
	"sync/atomic"
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
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
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

func TestPerStreamFlowControl(t *testing.T) {
	// This test verifies that flow control can be applied per-stream (per-goroutine)
	// instead of per-subscriber.

	// Use a fake server. The fake doesn't currently implement server-side flow
	// control, so it will deliver messages as soon as client requests them.
	// This is perfect for testing both forms of flow control.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topicName := fmt.Sprintf("projects/%s/topics/t", projName)
	subName := fmt.Sprintf("projects/%s/subscriptions/s", subID)
	mustCreateTopic(t, client, topicName)
	sub := mustCreateSubConfig(t, client, &pb.Subscription{
		Name:  subName,
		Topic: topicName,
	})

	// Publish enough messages to saturate flow control.
	// We'll set MaxOutstandingMessages=5.
	// If per-stream is FALSE (legacy), total allowed = 5.
	// If per-stream is TRUE, total allowed = 5 * NumGoroutines.
	const maxOutstanding = 5
	const numGoroutines = 2
	const totalMessages = 20

	for i := 0; i < totalMessages; i++ {
		srv.Publish(topicName, []byte(fmt.Sprintf("msg-%d", i)), nil)
	}

	testCases := []struct {
		desc                   string
		enablePerStream        bool
		wantMaxActiveCallbacks int
	}{
		{
			desc:                   "Legacy Flow Control (Shared)",
			enablePerStream:        false,
			wantMaxActiveCallbacks: maxOutstanding,
		},
		{
			desc:                   "Per-Stream Flow Control",
			enablePerStream:        true,
			wantMaxActiveCallbacks: maxOutstanding * numGoroutines,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// Republish messages for each test case since the previous one consumed them.
			for i := 0; i < totalMessages; i++ {
				srv.Publish(topicName, []byte(fmt.Sprintf("%s-msg-%d", tc.desc, i)), nil)
			}

			sub.ReceiveSettings = ReceiveSettings{
				MaxOutstandingMessages:     maxOutstanding,
				NumGoroutines:              numGoroutines,
				EnablePerStreamFlowControl: tc.enablePerStream,
			}

			// We need a way to count active callbacks and block them
			// to simulate saturation.
			var activeCallbacks int32
			var maxSeen int32

			// channel to hold callbacks
			holdCh := make(chan struct{})
			// channel to signal that we've reached a stable state (saturation or drain)

			// We will use a context with timeout for the Receive call to avoid hanging forever.
			recvCtx, recvCancel := context.WithTimeout(ctx, 5*time.Second)
			defer recvCancel()

			errCh := make(chan error, 1)
			go func() {
				errCh <- sub.Receive(recvCtx, func(ctx context.Context, m *Message) {
					current := atomic.AddInt32(&activeCallbacks, 1)

					// Update max active callbacks seen
					for {
						seen := atomic.LoadInt32(&maxSeen)
						if current <= seen {
							break
						}
						// Try to swap maxSeen with current
						if atomic.CompareAndSwapInt32(&maxSeen, seen, current) {
							break
						}
					}

					// Hold the callback until told to proceed or context matches
					select {
					case <-holdCh:
					case <-ctx.Done():
					}

					atomic.AddInt32(&activeCallbacks, -1)
					m.Ack()
				})
			}()

			// Give it some time to saturate
			time.Sleep(500 * time.Millisecond)

			// Check the max saturated value
			actualMax := atomic.LoadInt32(&maxSeen)

			// Release callbacks
			close(holdCh)

			// Wait for Receive to finish
			select {
			case err := <-errCh:
				if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
					t.Errorf("Receive returned unexpected error: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("Receive timed out waiting to return")
			}

			if actualMax == 0 {
				t.Error("Did not receive any messages")
			}

			// In the legacy case, we expect exactly maxOutstanding.
			// In the per-stream case, we expect up to maxOutstanding * numGoroutines.

			// We relax the check slightly for Legacy to allow for transients, but it should definitely be less than per-stream potential.
			// Legacy limit: 5. Per-Stream limit: 10.

			if tc.enablePerStream {
				// We expect to breach the single-stream limit
				if actualMax <= maxOutstanding {
					t.Errorf("EnablePerStreamFlowControl=true: got max %d, want > %d (approx %d)", actualMax, maxOutstanding, maxOutstanding*numGoroutines)
				}
			} else {
				// We expect to stay within the shared limit
				// Allowing small buffer (+2) for test flakiness/race but strictly it should be <= maxOutstanding
				if actualMax > maxOutstanding+2 {
					t.Errorf("EnablePerStreamFlowControl=false: got max %d, want around %d", actualMax, maxOutstanding)
				}
			}
		})
	}
}
