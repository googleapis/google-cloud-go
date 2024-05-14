// Copyright 2016 Google LLC
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

	"cloud.google.com/go/internal/testutil"
	pb "cloud.google.com/go/pubsub/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/pstest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// All returns the remaining subscriptions from this iterator.
func slurpSubs(it *SubscriptionIterator) ([]*Subscription, error) {
	var subs []*Subscription
	for {
		switch sub, err := it.Next(); err {
		case nil:
			subs = append(subs, sub)
		case iterator.Done:
			return subs, nil
		default:
			return nil, err
		}
	}
}

func TestSubscriptionID(t *testing.T) {
	const id = "id"
	c := &Client{projectID: "projid"}
	s := c.Subscription(id)
	if got, want := s.ID(), id; got != want {
		t.Errorf("Subscription.ID() = %q; want %q", got, want)
	}
}

func TestListProjectSubscriptions(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topic := mustCreateTopic(t, c, "t")
	var want []string
	for i := 1; i <= 2; i++ {
		id := fmt.Sprintf("s%d", i)
		want = append(want, id)
		_, err := c.CreateSubscription(ctx, id, SubscriptionConfig{Topic: topic})
		if err != nil {
			t.Fatal(err)
		}
	}
	subs, err := slurpSubs(c.Subscriptions(ctx))
	if err != nil {
		t.Fatal(err)
	}

	got := getSubIDs(subs)
	if !testutil.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// Call list again, but check the config this time.
	it := c.Subscriptions(ctx)
	i := 1
	for {
		sub, err := it.NextConfig()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Errorf("SubscriptionIterator.NextConfig() got err: %v", err)
		}
		if got := sub.Topic.ID(); got != topic.ID() {
			t.Errorf("subConfig.Topic mismatch, got: %v, want: %v", got, topic.ID())
		}

		want := fmt.Sprintf("s%d", i)
		if got := sub.ID(); got != want {
			t.Errorf("sub.ID() mismatch: got %s, want: %s", got, want)
		}
		want = fmt.Sprintf("projects/P/subscriptions/s%d", i)
		if got := sub.String(); got != want {
			t.Errorf("sub.String() mismatch: got %s, want: %s", got, want)
		}
		i++
	}
}

func getSubIDs(subs []*Subscription) []string {
	var names []string
	for _, sub := range subs {
		names = append(names, sub.ID())
	}
	return names
}

func TestListTopicSubscriptions(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topics := []*Topic{
		mustCreateTopic(t, c, "t0"),
		mustCreateTopic(t, c, "t1"),
	}
	wants := make([][]string, 2)
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("s%d", i)
		sub, err := c.CreateSubscription(ctx, id, SubscriptionConfig{Topic: topics[i%2]})
		if err != nil {
			t.Fatal(err)
		}
		wants[i%2] = append(wants[i%2], sub.ID())
	}

	for i, topic := range topics {
		subs, err := slurpSubs(topic.Subscriptions(ctx))
		if err != nil {
			t.Fatal(err)
		}
		got := getSubIDs(subs)
		if !testutil.Equal(got, wants[i]) {
			t.Errorf("#%d: got %v, want %v", i, got, wants[i])
		}
	}
}

const defaultRetentionDuration = 168 * time.Hour

func TestSubscriptionConfig(t *testing.T) {
	ctx := context.Background()
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topic := mustCreateTopic(t, client, "t")
	sub, err := client.CreateSubscription(ctx, "s", SubscriptionConfig{
		Topic:            topic,
		ExpirationPolicy: 30 * time.Hour,
		PushConfig: PushConfig{
			Endpoint: "https://example.com/push",
			AuthenticationMethod: &OIDCToken{
				ServiceAccountEmail: "foo@example.com",
				Audience:            "client-12345",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := sub.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := SubscriptionConfig{
		Topic:               topic,
		AckDeadline:         10 * time.Second,
		RetainAckedMessages: false,
		RetentionDuration:   defaultRetentionDuration,
		ExpirationPolicy:    30 * time.Hour,
		PushConfig: PushConfig{
			Endpoint: "https://example.com/push",
			AuthenticationMethod: &OIDCToken{
				ServiceAccountEmail: "foo@example.com",
				Audience:            "client-12345",
			},
			Wrapper: &PubsubWrapper{},
		},
		EnableExactlyOnceDelivery: false,
		State:                     SubscriptionStateActive,
	}
	opt := cmpopts.IgnoreUnexported(SubscriptionConfig{})
	if diff := testutil.Diff(cfg, want, opt); diff != "" {
		t.Fatalf("compare subscription config mismatch, -got, +want\n%s", diff)
	}

	got, err := sub.Update(ctx, SubscriptionConfigToUpdate{
		AckDeadline:         20 * time.Second,
		RetainAckedMessages: true,
		Labels:              map[string]string{"label": "value"},
		ExpirationPolicy:    72 * time.Hour,
		PushConfig: &PushConfig{
			Endpoint: "https://example2.com/push",
			AuthenticationMethod: &OIDCToken{
				ServiceAccountEmail: "bar@example.com",
				Audience:            "client-98765",
			},
			Wrapper: &NoWrapper{
				WriteMetadata: true,
			},
		},
		EnableExactlyOnceDelivery: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	want = SubscriptionConfig{
		Topic:               topic,
		AckDeadline:         20 * time.Second,
		RetainAckedMessages: true,
		RetentionDuration:   defaultRetentionDuration,
		Labels:              map[string]string{"label": "value"},
		ExpirationPolicy:    72 * time.Hour,
		PushConfig: PushConfig{
			Endpoint: "https://example2.com/push",
			AuthenticationMethod: &OIDCToken{
				ServiceAccountEmail: "bar@example.com",
				Audience:            "client-98765",
			},
			Wrapper: &NoWrapper{
				WriteMetadata: true,
			},
		},
		EnableExactlyOnceDelivery: true,
		State:                     SubscriptionStateActive,
	}
	if diff := testutil.Diff(got, want, opt); diff != "" {
		t.Fatalf("compare subscription config mismatch, -got, +want\n%s", diff)
	}

	got, err = sub.Update(ctx, SubscriptionConfigToUpdate{
		RetentionDuration: 2 * time.Hour,
		Labels:            map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	want.RetentionDuration = 2 * time.Hour
	want.Labels = nil
	if diff := testutil.Diff(got, want, opt); diff != "" {
		t.Fatalf("compare subscription config mismatch, -got, +want\n%s", diff)
	}

	_, err = sub.Update(ctx, SubscriptionConfigToUpdate{})
	if err == nil {
		t.Fatal("got nil, want error")
	}

	// Check ExpirationPolicy when set to never expire.
	got, err = sub.Update(ctx, SubscriptionConfigToUpdate{
		ExpirationPolicy: time.Duration(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	want.ExpirationPolicy = time.Duration(0)
	if diff := testutil.Diff(got, want, opt); diff != "" {
		t.Fatalf("compare subscription config mismatch, -got, +want\n%s", diff)
	}
}

func TestReceive(t *testing.T) {
	testReceive(t, true, false)
	testReceive(t, false, false)
	testReceive(t, false, true)
}

func testReceive(t *testing.T, synchronous, exactlyOnceDelivery bool) {
	t.Run(fmt.Sprintf("synchronous:%t,exactlyOnceDelivery:%t", synchronous, exactlyOnceDelivery), func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		client, srv := newFake(t)
		defer client.Close()
		defer srv.Close()

		topic := mustCreateTopic(t, client, "t")
		sub, err := client.CreateSubscription(ctx, "s", SubscriptionConfig{
			Topic:                     topic,
			EnableExactlyOnceDelivery: exactlyOnceDelivery,
		})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 256; i++ {
			srv.Publish(topic.name, []byte{byte(i)}, nil)
		}
		sub.ReceiveSettings.Synchronous = synchronous
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
				t.Errorf("sync=%t, eod=%t: did not see message #%d", synchronous, exactlyOnceDelivery, i)
			}
		}
	})
}

func (t1 *Topic) Equal(t2 *Topic) bool {
	if t1 == nil && t2 == nil {
		return true
	}
	if t1 == nil || t2 == nil {
		return false
	}
	return t1.c == t2.c && t1.name == t2.name
}

// Note: be sure to close client and server!
func newFake(t *testing.T) (*Client, *pstest.Server) {
	ctx := context.Background()
	srv := pstest.NewServer()
	client, err := NewClient(ctx, projName,
		option.WithEndpoint(srv.Addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithInsecure()))
	if err != nil {
		t.Fatal(err)
	}
	return client, srv
}

func TestPushConfigAuthenticationMethod_toProto(t *testing.T) {
	in := &PushConfig{
		Endpoint: "https://example.com/push",
		AuthenticationMethod: &OIDCToken{
			ServiceAccountEmail: "foo@example.com",
			Audience:            "client-12345",
		},
	}
	got := in.toProto()
	want := &pb.PushConfig{
		PushEndpoint: "https://example.com/push",
		AuthenticationMethod: &pb.PushConfig_OidcToken_{
			OidcToken: &pb.PushConfig_OidcToken{
				ServiceAccountEmail: "foo@example.com",
				Audience:            "client-12345",
			},
		},
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("Roundtrip to Proto failed\ngot: - want: +\n%s", diff)
	}
}

func TestDeadLettering_toProto(t *testing.T) {
	in := &DeadLetterPolicy{
		MaxDeliveryAttempts: 10,
		DeadLetterTopic:     "projects/p/topics/t",
	}
	got := in.toProto()
	want := &pb.DeadLetterPolicy{
		DeadLetterTopic:     "projects/p/topics/t",
		MaxDeliveryAttempts: 10,
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("Roundtrip to Proto failed\ngot: - want: +\n%s", diff)
	}
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

func TestRetryPolicy_toProto(t *testing.T) {
	in := &RetryPolicy{
		MinimumBackoff: 20 * time.Second,
		MaximumBackoff: 300 * time.Second,
	}
	got := in.toProto()
	want := &pb.RetryPolicy{
		MinimumBackoff: durationpb.New(20 * time.Second),
		MaximumBackoff: durationpb.New(300 * time.Second),
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("Roundtrip to Proto failed\ngot: - want: +\n%s", diff)
	}
}

func TestOrdering_CreateSubscription(t *testing.T) {
	ctx := context.Background()
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topic := mustCreateTopic(t, client, "t")
	subConfig := SubscriptionConfig{
		Topic:                 topic,
		EnableMessageOrdering: true,
	}
	orderSub, err := client.CreateSubscription(ctx, "s", subConfig)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := orderSub.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.EnableMessageOrdering {
		t.Fatalf("Expected EnableMessageOrdering to be true in %s", orderSub.String())
	}

	// Test cancellation works as intended with ordering enabled.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	orderSub.Receive(ctx, func(ctx context.Context, msg *Message) {
		msg.Ack()
	})
}

func TestBigQuerySubscription(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topic := mustCreateTopic(t, client, "t")
	bqTable := "some-project:some-dataset.some-table"
	bqConfig := BigQueryConfig{
		Table: bqTable,
	}

	subConfig := SubscriptionConfig{
		Topic:          topic,
		BigQueryConfig: bqConfig,
	}
	bqSub, err := client.CreateSubscription(ctx, "s", subConfig)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := bqSub.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}

	want := bqConfig
	want.State = BigQueryConfigActive
	if diff := testutil.Diff(cfg.BigQueryConfig, want); diff != "" {
		t.Fatalf("CreateBQSubscription mismatch: \n%s", diff)
	}
}

func TestCloudStorageSubscription(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topic := mustCreateTopic(t, client, "t")
	bucket := "fake-bucket"
	csCfg := CloudStorageConfig{
		Bucket:         bucket,
		FilenamePrefix: "some-prefix",
		FilenameSuffix: "some-suffix",
		OutputFormat: &CloudStorageOutputFormatAvroConfig{
			WriteMetadata: true,
		},
		MaxDuration: 10 * time.Minute,
		MaxBytes:    10e5,
	}

	subConfig := SubscriptionConfig{
		Topic:              topic,
		CloudStorageConfig: csCfg,
	}
	csSub, err := client.CreateSubscription(ctx, "s", subConfig)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := csSub.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}

	want := csCfg
	want.State = CloudStorageConfigActive
	if diff := testutil.Diff(cfg.CloudStorageConfig, want); diff != "" {
		t.Fatalf("create cloud storage subscription mismatch: \n%s", diff)
	}

	csCfg.OutputFormat = &CloudStorageOutputFormatTextConfig{}
	cfg, err = csSub.Update(ctx, SubscriptionConfigToUpdate{
		CloudStorageConfig: &csCfg,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.CloudStorageConfig
	want = csCfg
	want.State = CloudStorageConfigActive
	if diff := testutil.Diff(got, want); diff != "" {
		t.Fatalf("update cloud storage subscription mismatch: \n%s", diff)
	}

	// Test resetting to a pull based subscription.
	cfg, err = csSub.Update(ctx, SubscriptionConfigToUpdate{
		CloudStorageConfig: &CloudStorageConfig{},
	})
	if err != nil {
		t.Fatal(err)
	}
	got = cfg.CloudStorageConfig
	want = CloudStorageConfig{}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Fatalf("remove cloud storage subscription mismatch: \n%s", diff)
	}
}

func TestExactlyOnceDelivery_AckSuccess(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topic := mustCreateTopic(t, client, "t")
	subConfig := SubscriptionConfig{
		Topic:                     topic,
		EnableExactlyOnceDelivery: true,
	}
	s, err := client.CreateSubscription(ctx, "s", subConfig)
	if err != nil {
		t.Fatalf("create sub err: %v", err)
	}
	s.ReceiveSettings.NumGoroutines = 1
	r := topic.Publish(ctx, &Message{
		Data: []byte("exactly-once-message"),
	})
	if _, err := r.Get(ctx); err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}

	err = s.Receive(ctx, func(ctx context.Context, msg *Message) {
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

	topic := mustCreateTopic(t, client, "t")
	subConfig := SubscriptionConfig{
		Topic:                     topic,
		EnableExactlyOnceDelivery: true,
	}
	s, err := client.CreateSubscription(ctx, "s", subConfig)
	if err != nil {
		t.Fatalf("create sub err: %v", err)
	}
	s.ReceiveSettings.NumGoroutines = 1
	r := topic.Publish(ctx, &Message{
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

	topic := mustCreateTopic(t, client, "t")
	subConfig := SubscriptionConfig{
		Topic:                     topic,
		EnableExactlyOnceDelivery: true,
	}
	s, err := client.CreateSubscription(ctx, "s", subConfig)
	if err != nil {
		t.Fatalf("create sub err: %v", err)
	}
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

	topic := mustCreateTopic(t, client, "t")
	subConfig := SubscriptionConfig{
		Topic:                     topic,
		EnableExactlyOnceDelivery: true,
	}
	s, err := client.CreateSubscription(ctx, "s", subConfig)
	if err != nil {
		t.Fatalf("create sub err: %v", err)
	}
	r := topic.Publish(ctx, &Message{
		Data: []byte("exactly-once-message"),
	})
	if _, err := r.Get(ctx); err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}

	s.ReceiveSettings = ReceiveSettings{
		NumGoroutines: 1,
	}
	err = s.Receive(ctx, func(ctx context.Context, msg *Message) {
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

	topic := mustCreateTopic(t, client, "t")
	subConfig := SubscriptionConfig{
		Topic:                     topic,
		EnableExactlyOnceDelivery: true,
	}
	s, err := client.CreateSubscription(ctx, "s", subConfig)
	if err != nil {
		t.Fatalf("create sub err: %v", err)
	}
	r := topic.Publish(ctx, &Message{
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

	topic := mustCreateTopic(t, client, "t")
	subConfig := SubscriptionConfig{
		Topic: topic,
	}
	s, err := client.CreateSubscription(ctx, "s", subConfig)
	if err != nil {
		t.Fatalf("create sub err: %v", err)
	}

	s.ReceiveSettings.NumGoroutines = 1
	s.ReceiveSettings.MaxOutstandingMessages = 1
	s.ReceiveSettings.MaxExtension = 10 * time.Second
	s.ReceiveSettings.MaxExtensionPeriod = 10 * time.Second
	r := topic.Publish(ctx, &Message{
		Data: []byte("redelivered-message"),
	})
	if _, err := r.Get(ctx); err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}

	deliveryCount := 0
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = s.Receive(ctx, func(ctx context.Context, msg *Message) {
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
