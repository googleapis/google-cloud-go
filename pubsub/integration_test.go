// Copyright 2014 Google LLC
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
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/iam"
	"cloud.google.com/go/internal"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"cloud.google.com/go/internal/version"
	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	pb "cloud.google.com/go/pubsub/apiv1/pubsubpb"
	testutil2 "cloud.google.com/go/pubsub/internal/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	gax "github.com/googleapis/gax-go/v2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

var (
	topicIDs  = uid.NewSpace("topic", nil)
	subIDs    = uid.NewSpace("sub", nil)
	schemaIDs = uid.NewSpace("schema", nil)
)

// messageData is used to hold the contents of a message so that it can be compared against the contents
// of another message without regard to irrelevant fields.
type messageData struct {
	ID         string
	Data       string
	Attributes map[string]string
}

func extractMessageData(m *Message) messageData {
	return messageData{
		ID:         m.ID,
		Data:       string(m.Data),
		Attributes: m.Attributes,
	}
}

func withGRPCHeadersAssertion(t *testing.T, opts ...option.ClientOption) []option.ClientOption {
	grpcHeadersEnforcer := &testutil.HeadersEnforcer{
		OnFailure: t.Errorf,
		Checkers: []*testutil.HeaderChecker{
			testutil.XGoogClientHeaderChecker,
		},
	}
	return append(grpcHeadersEnforcer.CallOptions(), opts...)
}

func integrationTestClient(ctx context.Context, t *testing.T, opts ...option.ClientOption) *Client {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	projID := testutil.ProjID()
	if projID == "" {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	ts := testutil.TokenSource(ctx, ScopePubSub, ScopeCloudPlatform)
	if ts == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	opts = append(withGRPCHeadersAssertion(t, option.WithTokenSource(ts)), opts...)
	client, err := NewClient(ctx, projID, opts...)
	if err != nil {
		t.Fatalf("Creating client error: %v", err)
	}
	return client
}

func integrationTestSchemaClient(ctx context.Context, t *testing.T, opts ...option.ClientOption) *SchemaClient {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	projID := testutil.ProjID()
	if projID == "" {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	ts := testutil.TokenSource(ctx, ScopePubSub, ScopeCloudPlatform)
	if ts == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	opts = append(withGRPCHeadersAssertion(t, option.WithTokenSource(ts)), opts...)
	sc, err := NewSchemaClient(ctx, projID, opts...)
	if err != nil {
		t.Fatalf("Creating client error: %v", err)
	}
	return sc
}

func TestIntegration_Admin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Errorf("CreateTopic error: %v", err)
	}
	defer topic.Stop()
	exists, err := topic.Exists(ctx)
	if err != nil {
		t.Fatalf("TopicExists error: %v", err)
	}
	if !exists {
		t.Errorf("topic %v should exist, but it doesn't", topic)
	}

	var sub *Subscription
	if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), SubscriptionConfig{Topic: topic}); err != nil {
		t.Errorf("CreateSub error: %v", err)
	}
	exists, err = sub.Exists(ctx)
	if err != nil {
		t.Fatalf("SubExists error: %v", err)
	}
	if !exists {
		t.Errorf("subscription %s should exist, but it doesn't", sub.ID())
	}

	if msg, ok := testIAM(ctx, topic.IAM(), "pubsub.topics.get"); !ok {
		t.Errorf("topic IAM: %s", msg)
	}
	if msg, ok := testIAM(ctx, sub.IAM(), "pubsub.subscriptions.get"); !ok {
		t.Errorf("sub IAM: %s", msg)
	}

	snap, err := sub.CreateSnapshot(ctx, "")
	if err != nil {
		t.Fatalf("CreateSnapshot error: %v", err)
	}

	labels := map[string]string{"foo": "bar"}
	sc, err := snap.SetLabels(ctx, labels)
	if err != nil {
		t.Fatalf("Snapshot.SetLabels error: %v", err)
	}
	if diff := testutil.Diff(sc.Labels, labels); diff != "" {
		t.Fatalf("\ngot: - want: +\n%s", diff)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	err = internal.Retry(timeoutCtx, gax.Backoff{}, func() (bool, error) {
		snapIt := client.Snapshots(timeoutCtx)
		for {
			s, err := snapIt.Next()
			if err == nil && s.name == snap.name {
				return true, nil
			}
			if errors.Is(err, iterator.Done) {
				return false, fmt.Errorf("cannot find snapshot: %q", snap.name)
			}
			if err != nil {
				return false, err
			}
		}
	})
	if err != nil {
		t.Error(err)
	}

	err = internal.Retry(timeoutCtx, gax.Backoff{}, func() (bool, error) {
		err := sub.SeekToSnapshot(timeoutCtx, snap.Snapshot)
		return err == nil, err
	})
	if err != nil {
		t.Error(err)
	}

	err = internal.Retry(timeoutCtx, gax.Backoff{}, func() (bool, error) {
		err := sub.SeekToTime(timeoutCtx, time.Now())
		return err == nil, err
	})
	if err != nil {
		t.Error(err)
	}

	err = internal.Retry(timeoutCtx, gax.Backoff{}, func() (bool, error) {
		snapHandle := client.Snapshot(snap.ID())
		err := snapHandle.Delete(timeoutCtx)
		return err == nil, err
	})
	if err != nil {
		t.Error(err)
	}

	if err := sub.Delete(ctx); err != nil {
		t.Errorf("DeleteSub error: %v", err)
	}

	if err := topic.Delete(ctx); err != nil {
		t.Errorf("DeleteTopic error: %v", err)
	}
}

func TestIntegration_PublishReceive(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t)

	for _, sync := range []bool{false, true} {
		for _, maxMsgs := range []int{0, 3, -1} { // MaxOutstandingMessages = default, 3, unlimited
			testPublishAndReceive(t, client, maxMsgs, sync, false, 10, 0)
		}

		// Tests for large messages (larger than the 4MB gRPC limit).
		testPublishAndReceive(t, client, 0, sync, false, 1, 5*1024*1024)
	}
}

// withGoogleClientInfo sets the name and version of the application in
// the `x-goog-api-client` header passed on each request and returns the
// updated context.
func withGoogleClientInfo(ctx context.Context) context.Context {
	ctxMD, _ := metadata.FromOutgoingContext(ctx)
	kv := []string{
		"gl-go",
		version.Go(),
		"gax",
		gax.Version,
		"grpc",
		grpc.Version,
	}

	allMDs := append([]metadata.MD{ctxMD}, metadata.Pairs("x-goog-api-client", gax.XGoogHeader(kv...)))
	return metadata.NewOutgoingContext(ctx, metadata.Join(allMDs...))
}

func testPublishAndReceive(t *testing.T, client *Client, maxMsgs int, synchronous, exactlyOnceDelivery bool, numMsgs, extraBytes int) {
	t.Run(fmt.Sprintf("maxMsgs:%d,synchronous:%t,exactlyOnceDelivery:%t,numMsgs:%d", maxMsgs, synchronous, exactlyOnceDelivery, numMsgs), func(t *testing.T) {
		t.Parallel()
		testutil.Retry(t, 3, 10*time.Second, func(r *testutil.R) {
			ctx := context.Background()
			topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
			if err != nil {
				r.Errorf("CreateTopic error: %v", err)
			}
			defer topic.Delete(ctx)
			defer topic.Stop()
			exists, err := topic.Exists(ctx)
			if err != nil {
				r.Errorf("TopicExists error: %v", err)
			}
			if !exists {
				r.Errorf("topic %v should exist, but it doesn't", topic)
			}

			sub, err := createSubWithRetry(ctx, t, client, subIDs.New(), SubscriptionConfig{
				Topic:                     topic,
				EnableExactlyOnceDelivery: exactlyOnceDelivery,
			})
			if err != nil {
				r.Errorf("CreateSub error: %v", err)
			}
			defer sub.Delete(ctx)
			exists, err = sub.Exists(ctx)
			if err != nil {
				r.Errorf("SubExists error: %v", err)
			}
			if !exists {
				r.Errorf("subscription %s should exist, but it doesn't", sub.ID())
			}
			var msgs []*Message
			for i := 0; i < numMsgs; i++ {
				text := fmt.Sprintf("a message with an index %d - %s", i, strings.Repeat(".", extraBytes))
				attrs := make(map[string]string)
				attrs["foo"] = "bar"
				msgs = append(msgs, &Message{
					Data:       []byte(text),
					Attributes: attrs,
				})
			}

			// Publish some messages.
			type pubResult struct {
				m *Message
				r *PublishResult
			}
			var rs []pubResult
			for _, m := range msgs {
				r := topic.Publish(ctx, m)
				rs = append(rs, pubResult{m, r})
			}
			want := make(map[string]messageData)
			for _, res := range rs {
				id, err := res.r.Get(ctx)
				if err != nil {
					r.Errorf("r.Get: %v", err)
				}
				md := extractMessageData(res.m)
				md.ID = id
				want[md.ID] = md
			}

			sub.ReceiveSettings.MaxOutstandingMessages = maxMsgs
			sub.ReceiveSettings.Synchronous = synchronous

			// Use a timeout to ensure that Pull does not block indefinitely if there are
			// unexpectedly few messages available.
			now := time.Now()
			timeout := 3 * time.Minute
			timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			gotMsgs, err := pullN(timeoutCtx, sub, len(want), 0, func(ctx context.Context, m *Message) {
				m.Ack()
			})
			if err != nil {
				if c := status.Convert(err); c.Code() == codes.Canceled {
					if time.Since(now) >= timeout {
						r.Errorf("pullN took longer than %v", timeout)
					}
				} else {
					r.Errorf("Pull: %v", err)
				}
			}
			got := make(map[string]messageData)
			for _, m := range gotMsgs {
				md := extractMessageData(m)
				got[md.ID] = md
			}
			if !testutil.Equal(got, want) {
				r.Errorf("MaxOutstandingMessages=%d, Synchronous=%t, messages got: %+v, messages want: %+v",
					maxMsgs, synchronous, got, want)
			}
		})
	})
}

// IAM tests.
// NOTE: for these to succeed, the test runner identity must have the Pub/Sub Admin or Owner roles.
// To set, visit https://console.developers.google.com, select "IAM & Admin" from the top-left
// menu, choose the account, click the Roles dropdown, and select "Pub/Sub > Pub/Sub Admin".
// TODO(jba): move this to a testing package within cloud.google.com/iam, so we can re-use it.
func testIAM(ctx context.Context, h *iam.Handle, permission string) (msg string, ok bool) {
	// Manually adding withGoogleClientInfo here because this code only takes
	// a handle with a grpc.ClientConn that has the "x-goog-api-client" header enforcer,
	// but unfortunately not the underlying infrastructure that takes pre-set headers.
	ctx = withGoogleClientInfo(ctx)

	// Attempting to add an non-existent identity  (e.g. "alice@example.com") causes the service
	// to return an internal error, so use a real identity.
	const member = "domain:google.com"

	var policy *iam.Policy
	var err error

	if policy, err = h.Policy(ctx); err != nil {
		return fmt.Sprintf("Policy: %v", err), false
	}
	// The resource is new, so the policy should be empty.
	if got := policy.Roles(); len(got) > 0 {
		return fmt.Sprintf("initially: got roles %v, want none", got), false
	}
	// Add a member, set the policy, then check that the member is present.
	policy.Add(member, iam.Viewer)
	if err := h.SetPolicy(ctx, policy); err != nil {
		return fmt.Sprintf("SetPolicy: %v", err), false
	}
	if policy, err = h.Policy(ctx); err != nil {
		return fmt.Sprintf("Policy: %v", err), false
	}
	if got, want := policy.Members(iam.Viewer), []string{member}; !testutil.Equal(got, want) {
		return fmt.Sprintf("after Add: got %v, want %v", got, want), false
	}
	// Now remove that member, set the policy, and check that it's empty again.
	policy.Remove(member, iam.Viewer)
	if err := h.SetPolicy(ctx, policy); err != nil {
		return fmt.Sprintf("SetPolicy: %v", err), false
	}
	if policy, err = h.Policy(ctx); err != nil {
		return fmt.Sprintf("Policy: %v", err), false
	}
	if got := policy.Roles(); len(got) > 0 {
		return fmt.Sprintf("after Remove: got roles %v, want none", got), false
	}
	// Call TestPermissions.
	// Because this user is an admin, it has all the permissions on the
	// resource type. Note: the service fails if we ask for inapplicable
	// permissions (e.g. a subscription permission on a topic, or a topic
	// create permission on a topic rather than its parent).
	wantPerms := []string{permission}
	gotPerms, err := h.TestPermissions(ctx, wantPerms)
	if err != nil {
		return fmt.Sprintf("TestPermissions: %v", err), false
	}
	if !testutil.Equal(gotPerms, wantPerms) {
		return fmt.Sprintf("TestPermissions: got %v, want %v", gotPerms, wantPerms), false
	}
	return "", true
}

func TestIntegration_LargePublishSize(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	// Calculate the largest possible message length that is still valid.
	// First, calculate the max length of the encoded message accounting for the topic name.
	length := MaxPublishRequestBytes - calcFieldSizeString(topic.String())
	// Next, account for the overhead from encoding an individual PubsubMessage,
	// and the inner PubsubMessage.Data field.
	pbMsgOverhead := 1 + protowire.SizeVarint(uint64(length))
	dataOverhead := 1 + protowire.SizeVarint(uint64(length-pbMsgOverhead))
	maxLengthSingleMessage := length - pbMsgOverhead - dataOverhead

	publishReq := &pb.PublishRequest{
		Topic: topic.String(),
		Messages: []*pb.PubsubMessage{
			{
				Data: bytes.Repeat([]byte{'A'}, maxLengthSingleMessage),
			},
		},
	}

	if got := proto.Size(publishReq); got != MaxPublishRequestBytes {
		t.Fatalf("Created request size of %d bytes,\nwant %f bytes", got, MaxPublishRequestBytes)
	}

	// Publishing the max length message by itself should succeed.
	msg := &Message{
		Data: bytes.Repeat([]byte{'A'}, maxLengthSingleMessage),
	}
	topic.PublishSettings.FlowControlSettings.LimitExceededBehavior = FlowControlSignalError
	r := topic.Publish(ctx, msg)
	if _, err := r.Get(ctx); err != nil {
		t.Fatalf("Failed to publish max length message: %v", err)
	}

	// Publish a small message first and make sure the max length message
	// is added to its own bundle.
	smallMsg := &Message{
		Data: []byte{'A'},
	}
	topic.Publish(ctx, smallMsg)
	r = topic.Publish(ctx, msg)
	if _, err := r.Get(ctx); err != nil {
		t.Fatalf("Failed to publish max length message after a small message: %v", err)
	}

	// Increase the data byte string by 1 byte, which should cause the request to fail,
	// specifically due to exceeding the bundle byte limit.
	msg.Data = append(msg.Data, 'A')
	r = topic.Publish(ctx, msg)
	if _, err := r.Get(ctx); err != ErrOversizedMessage {
		t.Fatalf("Should throw item size too large error, got %v", err)
	}
}

func TestIntegration_CancelReceive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Errorf("failed to create topic: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	var sub *Subscription
	if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), SubscriptionConfig{Topic: topic}); err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}
	defer sub.Delete(ctx)

	ctx, cancel := context.WithCancel(context.Background())
	sub.ReceiveSettings.MaxOutstandingMessages = -1
	sub.ReceiveSettings.MaxOutstandingBytes = -1
	sub.ReceiveSettings.NumGoroutines = 1

	doneReceiving := make(chan struct{})

	// Publish the messages.
	go func() {
		for {
			select {
			case <-doneReceiving:
				return
			default:
				topic.Publish(ctx, &Message{Data: []byte("some msg")})
				time.Sleep(time.Second)
			}
		}
	}()

	go func() {
		err = sub.Receive(ctx, func(_ context.Context, msg *Message) {
			cancel()
			time.AfterFunc(5*time.Second, msg.Ack)
		})
		close(doneReceiving)
	}()

	select {
	case <-time.After(60 * time.Second):
		t.Fatalf("Waited 60 seconds for Receive to finish, should have finished sooner")
	case <-doneReceiving:
	}
}

func TestIntegration_CreateSubscription_NeverExpire(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	cfg := SubscriptionConfig{
		Topic:            topic,
		ExpirationPolicy: time.Duration(0),
	}
	var sub *Subscription
	if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), cfg); err != nil {
		t.Fatalf("CreateSub error: %v", err)
	}
	defer sub.Delete(ctx)

	got, err := sub.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Duration(0)
	if got.ExpirationPolicy != want {
		t.Fatalf("config.ExpirationPolicy mismatch, got: %v, want: %v\n", got.ExpirationPolicy, want)
	}
}

// findServiceAccountEmail tries to find the service account using testutil
// JWTConfig as well as the ADC credentials. It will only invoke t.Skip if
// it successfully retrieves credentials but finds a blank JWTConfig JSON blob.
// For all other errors, it will invoke t.Fatal.
func findServiceAccountEmail(ctx context.Context, t *testing.T) string {
	jwtConf, err := testutil.JWTConfig()
	if err == nil && jwtConf != nil {
		return jwtConf.Email
	}
	creds := testutil.Credentials(ctx, ScopePubSub, ScopeCloudPlatform)
	if creds == nil {
		t.Fatal("Failed to retrieve credentials")
	}
	if len(creds.JSON) == 0 {
		t.Skip("No JWTConfig JSON was present so can't get serviceAccountEmail")
	}
	jwtConf, err = google.JWTConfigFromJSON(creds.JSON)
	if err != nil {
		if strings.Contains(err.Error(), "authorized_user") {
			t.Skip("Found ADC user so can't get serviceAccountEmail")
		}
		t.Fatalf("Failed to parse Google JWTConfig from JSON: %v", err)
	}
	return jwtConf.Email
}

func TestIntegration_UpdateSubscription(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	client := integrationTestClient(ctx, t)
	defer client.Close()

	serviceAccountEmail := findServiceAccountEmail(ctx, t)

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	var sub *Subscription
	projID := testutil.ProjID()
	sCfg := SubscriptionConfig{
		Topic: topic,
		PushConfig: PushConfig{
			Endpoint: "https://" + projID + ".appspot.com/_ah/push-handlers/push",
			AuthenticationMethod: &OIDCToken{
				Audience:            "client-12345",
				ServiceAccountEmail: serviceAccountEmail,
			},
		},
	}
	if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), sCfg); err != nil {
		t.Fatalf("CreateSub error: %v", err)
	}
	defer sub.Delete(ctx)

	got, err := sub.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := SubscriptionConfig{
		Topic:               topic,
		AckDeadline:         10 * time.Second,
		RetainAckedMessages: false,
		RetentionDuration:   defaultRetentionDuration,
		ExpirationPolicy:    defaultExpirationPolicy,
		PushConfig: PushConfig{
			Endpoint: "https://" + projID + ".appspot.com/_ah/push-handlers/push",
			AuthenticationMethod: &OIDCToken{
				Audience:            "client-12345",
				ServiceAccountEmail: serviceAccountEmail,
			},
		},
		State: SubscriptionStateActive,
	}
	opt := cmpopts.IgnoreUnexported(SubscriptionConfig{})
	if diff := testutil.Diff(got, want, opt); diff != "" {
		t.Fatalf("\ngot: - want: +\n%s", diff)
	}
	// Add a PushConfig and change other fields.
	pc := PushConfig{
		Endpoint:   "https://" + projID + ".appspot.com/_ah/push-handlers/push",
		Attributes: map[string]string{"x-goog-version": "v1"},
		AuthenticationMethod: &OIDCToken{
			Audience:            "client-updated-54321",
			ServiceAccountEmail: serviceAccountEmail,
		},
	}
	got, err = sub.Update(ctx, SubscriptionConfigToUpdate{
		PushConfig:          &pc,
		AckDeadline:         2 * time.Minute,
		RetainAckedMessages: true,
		RetentionDuration:   2 * time.Hour,
		Labels:              map[string]string{"label": "value"},
		ExpirationPolicy:    25 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	want = SubscriptionConfig{
		Topic:               topic,
		PushConfig:          pc,
		AckDeadline:         2 * time.Minute,
		RetainAckedMessages: true,
		RetentionDuration:   2 * time.Hour,
		Labels:              map[string]string{"label": "value"},
		ExpirationPolicy:    25 * time.Hour,
		State:               SubscriptionStateActive,
	}

	if !testutil.Equal(got, want, opt) {
		t.Fatalf("\ngot  %+v\nwant %+v", got, want)
	}

	// Update ExpirationPolicy to never expire.
	got, err = sub.Update(ctx, SubscriptionConfigToUpdate{
		ExpirationPolicy: time.Duration(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	want.ExpirationPolicy = time.Duration(0)

	if !testutil.Equal(got, want, opt) {
		t.Fatalf("\ngot  %+v\nwant %+v", got, want)
	}

	// Remove the PushConfig, turning the subscription back into pull mode.
	// Change AckDeadline, remove labels.
	pc = PushConfig{}
	got, err = sub.Update(ctx, SubscriptionConfigToUpdate{
		PushConfig:  &pc,
		AckDeadline: 30 * time.Second,
		Labels:      map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	want.PushConfig = pc
	want.AckDeadline = 30 * time.Second
	want.Labels = nil
	// service issue: PushConfig attributes are not removed.
	// TODO(jba): remove when issue resolved.
	want.PushConfig.Attributes = map[string]string{"x-goog-version": "v1"}
	if !testutil.Equal(got, want, opt) {
		t.Fatalf("\ngot  %+v\nwant %+v", got, want)
	}
	// If nothing changes, our client returns an error.
	_, err = sub.Update(ctx, SubscriptionConfigToUpdate{})
	if err == nil {
		t.Fatal("got nil, wanted error")
	}
}

// publishSync is a utility function for publishing a message and
// blocking until the message has been confirmed.
func publishSync(ctx context.Context, t *testing.T, topic *Topic, msg *Message) {
	res := topic.Publish(ctx, msg)
	_, err := res.Get(ctx)
	if err != nil {
		t.Fatalf("publishSync err: %v", err)
	}
}

func TestIntegration_UpdateSubscription_ExpirationPolicy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	var sub *Subscription
	if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), SubscriptionConfig{Topic: topic}); err != nil {
		t.Fatalf("CreateSub error: %v", err)
	}
	defer sub.Delete(ctx)

	// Set ExpirationPolicy within the valid range.
	got, err := sub.Update(ctx, SubscriptionConfigToUpdate{
		RetentionDuration: 2 * time.Hour,
		ExpirationPolicy:  25 * time.Hour,
		AckDeadline:       2 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := 25 * time.Hour
	if got.ExpirationPolicy != want {
		t.Fatalf("config.ExpirationPolicy mismatch; got: %v, want: %v", got.ExpirationPolicy, want)
	}

	// ExpirationPolicy to never expire.
	got, err = sub.Update(ctx, SubscriptionConfigToUpdate{
		ExpirationPolicy: time.Duration(0),
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v\n", err)
	}
	want = time.Duration(0)
	if diff := testutil.Diff(got.ExpirationPolicy, want); diff != "" {
		t.Fatalf("\ngot: - want: +\n%s", diff)
	}

	// ExpirationPolicy when nil is passed in, should not cause any updates.
	got, err = sub.Update(ctx, SubscriptionConfigToUpdate{
		ExpirationPolicy: nil,
	})
	if err == nil || err.Error() != "pubsub: UpdateSubscription call with nothing to update" {
		t.Fatalf("Expected no attributes to be updated, error: %v", err)
	}

	// ExpirationPolicy of nil, with the previous value having been a non-zero value.
	_, err = sub.Update(ctx, SubscriptionConfigToUpdate{
		ExpirationPolicy: 26 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Now examine what setting it to nil produces.
	_, err = sub.Update(ctx, SubscriptionConfigToUpdate{
		ExpirationPolicy: nil,
	})
	if err == nil || err.Error() != "pubsub: UpdateSubscription call with nothing to update" {
		t.Fatalf("Expected no attributes to be updated, error: %v", err)
	}
}

// NOTE: This test should be skipped by open source contributors. It requires
// allowlisting, a (gsuite) organization project, and specific permissions.
func TestIntegration_UpdateTopicLabels(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	compareConfig := func(got TopicConfig, wantLabels map[string]string) bool {
		return testutil.Equal(got.Labels, wantLabels)
	}

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	got, err := topic.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !compareConfig(got, nil) {
		t.Fatalf("\ngot  %+v\nwant no labels", got)
	}

	labels := map[string]string{"label": "value"}
	got, err = topic.Update(ctx, TopicConfigToUpdate{Labels: labels})
	if err != nil {
		t.Fatal(err)
	}
	if !compareConfig(got, labels) {
		t.Fatalf("\ngot  %+v\nwant labels %+v", got, labels)
	}
	// Remove all labels.
	got, err = topic.Update(ctx, TopicConfigToUpdate{Labels: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if !compareConfig(got, nil) {
		t.Fatalf("\ngot  %+v\nwant no labels", got)
	}
}

func TestIntegration_PublicTopic(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	sub, err := createSubWithRetry(ctx, t, client, subIDs.New(), SubscriptionConfig{
		Topic: client.TopicInProject("taxirides-realtime", "pubsub-public-data"),
	})
	if err != nil {
		t.Fatal(err)
	}
	sub.Delete(ctx)
}

func TestIntegration_Errors(t *testing.T) {
	// Test various edge conditions.
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	// Out-of-range retention duration.
	sub, err := client.CreateSubscription(ctx, subIDs.New(), SubscriptionConfig{
		Topic:             topic,
		RetentionDuration: 1 * time.Second,
	})
	if want := codes.InvalidArgument; status.Code(err) != want {
		t.Errorf("got <%v>, want %s", err, want)
	}
	if err == nil {
		sub.Delete(ctx)
	}

	// Ack deadline less than minimum.
	sub, err = client.CreateSubscription(ctx, subIDs.New(), SubscriptionConfig{
		Topic:       topic,
		AckDeadline: 5 * time.Second,
	})
	if want := codes.Unknown; status.Code(err) != want {
		t.Errorf("got <%v>, want %s", err, want)
	}
	if err == nil {
		sub.Delete(ctx)
	}

	// Updating a non-existent subscription.
	sub = client.Subscription(subIDs.New())
	_, err = sub.Update(ctx, SubscriptionConfigToUpdate{AckDeadline: 20 * time.Second})
	if want := codes.NotFound; status.Code(err) != want {
		t.Errorf("got <%v>, want %s", err, want)
	}
	// Deleting a non-existent subscription.
	err = sub.Delete(ctx)
	if want := codes.NotFound; status.Code(err) != want {
		t.Errorf("got <%v>, want %s", err, want)
	}

	// Updating out-of-range retention duration.
	sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), SubscriptionConfig{Topic: topic})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Delete(ctx)
	_, err = sub.Update(ctx, SubscriptionConfigToUpdate{RetentionDuration: 1000 * time.Hour})
	if want := codes.InvalidArgument; status.Code(err) != want {
		t.Errorf("got <%v>, want %s", err, want)
	}
}

func TestIntegration_MessageStoragePolicy_TopicLevel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	// Specify some regions to set.
	regions := []string{"asia-east1", "us-east1"}
	cfg, err := topic.Update(ctx, TopicConfigToUpdate{
		MessageStoragePolicy: &MessageStoragePolicy{
			AllowedPersistenceRegions: regions,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.MessageStoragePolicy.AllowedPersistenceRegions
	want := regions
	if !testutil.Equal(got, want) {
		t.Fatalf("\ngot  %+v\nwant regions%+v", got, want)
	}

	// Removing all regions should fail
	updateCfg := TopicConfigToUpdate{
		MessageStoragePolicy: &MessageStoragePolicy{
			AllowedPersistenceRegions: []string{},
		},
	}
	if _, err = topic.Update(ctx, updateCfg); err == nil {
		t.Fatalf("Unexpected succeeded in removing all regions\n%+v\n", got)
	}
}

// NOTE: This test should be skipped by open source contributors. It requires
// a (gsuite) organization project, and specific permissions. The test for MessageStoragePolicy
// on a topic level can be run on any topic and is covered by the previous test.
//
// Googlers, see internal bug 77920644. Furthermore, be sure to add your
// service account as an owner of ps-geofencing-test.
func TestIntegration_MessageStoragePolicy_ProjectLevel(t *testing.T) {
	// Verify that the message storage policy is populated.
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	t.Parallel()
	ctx := context.Background()
	// If a message storage policy is not set on a topic, the policy depends on the Resource Location
	// Restriction which is specified on an organization level. The usual testing project is in the
	// google.com org, which has no resource location restrictions. Use a project in another org that
	// does have a restriction set ("us-east1").
	projID := "ps-geofencing-test"
	// We can use the same creds as always because the service account of the default testing project
	// has permission to use the above project. This test will fail if a different service account
	// is used for testing.
	ts := testutil.TokenSource(ctx, ScopePubSub, ScopeCloudPlatform)
	if ts == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	opts := withGRPCHeadersAssertion(t, option.WithTokenSource(ts))
	client, err := NewClient(ctx, projID, opts...)
	if err != nil {
		t.Fatalf("Creating client error: %v", err)
	}
	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	config, err := topic.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := config.MessageStoragePolicy.AllowedPersistenceRegions
	want := []string{"us-east1"}
	if !testutil.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestIntegration_CreateTopic_KMS(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	kmsClient, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		t.Fatal(err)
	}

	keyRingID := "test-key-ring"
	want := "test-key2"

	// Get the test KMS key ring, optionally creating it if it doesn't exist.
	keyRing, err := kmsClient.GetKeyRing(ctx, &kmspb.GetKeyRingRequest{
		Name: fmt.Sprintf("projects/%s/locations/global/keyRings/%s", testutil.ProjID(), keyRingID),
	})
	if err != nil {
		if status.Code(err) != codes.NotFound {
			t.Fatal(err)
		}
		createKeyRingReq := &kmspb.CreateKeyRingRequest{
			Parent:    fmt.Sprintf("projects/%s/locations/global", testutil.ProjID()),
			KeyRingId: keyRingID,
		}
		keyRing, err = kmsClient.CreateKeyRing(ctx, createKeyRingReq)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Get the test KMS crypto key, optionally creating it if it doesn't exist.
	key, err := kmsClient.GetCryptoKey(ctx, &kmspb.GetCryptoKeyRequest{
		Name: fmt.Sprintf("%s/cryptoKeys/%s", keyRing.GetName(), want),
	})
	if err != nil {
		if status.Code(err) != codes.NotFound {
			t.Fatal(err)
		}
		createKeyReq := &kmspb.CreateCryptoKeyRequest{
			Parent:      keyRing.GetName(),
			CryptoKeyId: want,
			CryptoKey: &kmspb.CryptoKey{
				Purpose: 1, // ENCRYPT_DECRYPT purpose
			},
		}
		key, err = kmsClient.CreateCryptoKey(ctx, createKeyReq)
		if err != nil {
			t.Fatal(err)
		}
	}

	tc := TopicConfig{
		KMSKeyName: key.GetName(),
	}
	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), &tc)
	if err != nil {
		t.Fatalf("CreateTopicWithConfig error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	cfg, err := topic.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.KMSKeyName

	if got != key.GetName() {
		t.Errorf("got %v, want %v", got, key.GetName())
	}
}

func TestIntegration_CreateTopic_MessageStoragePolicy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	tc := TopicConfig{
		MessageStoragePolicy: MessageStoragePolicy{
			AllowedPersistenceRegions: []string{"us-east1"},
		},
	}
	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), &tc)
	if err != nil {
		t.Fatalf("CreateTopicWithConfig error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	got, err := topic.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := tc
	if diff := testutil.Diff(got.MessageStoragePolicy, want.MessageStoragePolicy); diff != "" {
		t.Fatalf("\ngot: - want: +\n%s", diff)
	}
}

func TestIntegration_OrderedKeys_Basic(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t, option.WithEndpoint("us-west1-pubsub.googleapis.com:443"))
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()
	exists, err := topic.Exists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("topic %v should exist, but it doesn't", topic)
	}
	var sub *Subscription
	if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), SubscriptionConfig{
		Topic:                 topic,
		EnableMessageOrdering: true,
	}); err != nil {
		t.Fatal(err)
	}
	defer sub.Delete(ctx)
	exists, err = sub.Exists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("subscription %s should exist, but it doesn't", sub.ID())
	}

	topic.PublishSettings.DelayThreshold = time.Second
	topic.EnableMessageOrdering = true

	orderingKey := "some-ordering-key"
	numItems := 1000
	for i := 0; i < numItems; i++ {
		r := topic.Publish(ctx, &Message{
			ID:          fmt.Sprintf("id-%d", i),
			Data:        []byte(fmt.Sprintf("item-%d", i)),
			OrderingKey: orderingKey,
		})
		go func() {
			if _, err := r.Get(ctx); err != nil {
				t.Error(err)
			}
		}()
	}

	received := make(chan string, numItems)
	ctx2, cancel := context.WithCancel(ctx)
	go func() {
		for i := 0; i < numItems; i++ {
			select {
			case r := <-received:
				if got, want := r, fmt.Sprintf("item-%d", i); got != want {
					t.Errorf("%d: got %s, want %s", i, got, want)
				}
			case <-time.After(30 * time.Second):
				t.Errorf("timed out after 30s waiting for item %d", i)
				cancel()
			}
		}
		cancel()
	}()

	if err := sub.Receive(ctx2, func(ctx context.Context, msg *Message) {
		defer msg.Ack()
		if msg.OrderingKey != orderingKey {
			t.Errorf("got ordering key %s, expected %s", msg.OrderingKey, orderingKey)
		}

		received <- string(msg.Data)
	}); err != nil && !errors.Is(err, context.Canceled) {
		t.Error(err)
	}
}

func TestIntegration_OrderedKeys_JSON(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t, option.WithEndpoint("us-west1-pubsub.googleapis.com:443"))
	defer client.Close()

	testutil.Retry(t, 2, 1*time.Second, func(r *testutil.R) {
		topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
		if err != nil {
			r.Errorf("createTopicWithRetry err: %v", err)
		}
		defer topic.Delete(ctx)
		defer topic.Stop()
		exists, err := topic.Exists(ctx)
		if err != nil {
			r.Errorf("topic.Exists err: %v", err)
		}
		if !exists {
			r.Errorf("topic %v should exist, but it doesn't", topic)
		}
		var sub *Subscription
		if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), SubscriptionConfig{
			Topic:                 topic,
			EnableMessageOrdering: true,
		}); err != nil {
			r.Errorf("creteSubWithRetry err: %v", err)
		}
		defer sub.Delete(ctx)
		exists, err = sub.Exists(ctx)
		if err != nil {
			r.Errorf("sub.Exists err: %v", err)
		}
		if !exists {
			r.Errorf("subscription %s should exist, but it doesn't", sub.ID())
		}

		topic.PublishSettings.DelayThreshold = time.Second
		topic.EnableMessageOrdering = true

		inFile, err := os.Open("testdata/publish.csv")
		if err != nil {
			r.Errorf("os.Open err: %v", err)
		}
		defer inFile.Close()

		mu := sync.Mutex{}
		var publishData []testutil2.OrderedKeyMsg
		var receiveData []testutil2.OrderedKeyMsg
		// Keep track of duplicate messages to avoid negative waitgroup counter.
		receiveSet := make(map[string]struct{})

		wg := sync.WaitGroup{}
		scanner := bufio.NewScanner(inFile)
		for scanner.Scan() {
			line := scanner.Text()
			// TODO: use strings.ReplaceAll once we only support 1.11+.
			line = strings.Replace(line, "\"", "", -1)
			parts := strings.Split(line, ",")
			key := parts[0]
			msg := parts[1]
			publishData = append(publishData, testutil2.OrderedKeyMsg{Key: key, Data: msg})
			res := topic.Publish(ctx, &Message{
				Data:        []byte(msg),
				OrderingKey: key,
			})
			go func() {
				_, err := res.Get(ctx)
				if err != nil {
					// Can't fail inside goroutine, so just log the error.
					r.Logf("publish error for message(%s): %v", msg, err)
				}
			}()
			wg.Add(1)
		}
		if err := scanner.Err(); err != nil {
			r.Errorf("scanner.Err(): %v", err)
		}

		go func() {
			sub.Receive(ctx, func(ctx context.Context, msg *Message) {
				mu.Lock()
				defer mu.Unlock()
				// Messages are deduped using the data field, since in this case all
				// messages are unique.
				if _, ok := receiveSet[string(msg.Data)]; ok {
					r.Logf("received duplicate message: %s", msg.Data)
					return
				}
				receiveSet[string(msg.Data)] = struct{}{}
				receiveData = append(receiveData, testutil2.OrderedKeyMsg{Key: msg.OrderingKey, Data: string(msg.Data)})
				wg.Done()
				msg.Ack()
			})
		}()

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(2 * time.Minute):
			r.Errorf("timed out after 2m waiting for all messages to be received")
		}

		mu.Lock()
		defer mu.Unlock()
		if err := testutil2.VerifyKeyOrdering(publishData, receiveData); err != nil {
			r.Errorf("VerifyKeyOrdering error: %v", err)
		}
	})
}

func TestIntegration_OrderedKeys_ResumePublish(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t, option.WithEndpoint("us-west1-pubsub.googleapis.com:443"))
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()
	exists, err := topic.Exists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("topic %v should exist, but it doesn't", topic)
	}

	topic.PublishSettings.BufferedByteLimit = 100
	topic.EnableMessageOrdering = true

	orderingKey := "some-ordering-key2"
	// Publish a message that is too large so we'll get an error that
	// pauses publishing for this ordering key.
	r := topic.Publish(ctx, &Message{
		Data:        bytes.Repeat([]byte("A"), 1000),
		OrderingKey: orderingKey,
	})
	if _, err := r.Get(ctx); err == nil {
		t.Fatalf("expected bundle byte limit error, got nil")
	}
	// Publish a normal sized message now, which should fail
	// since publishing on this ordering key is paused.
	r = topic.Publish(ctx, &Message{
		Data:        []byte("should fail"),
		OrderingKey: orderingKey,
	})
	if _, err := r.Get(ctx); err == nil || !errors.As(err, &ErrPublishingPaused{}) {
		t.Fatalf("expected ordering keys publish error, got %v", err)
	}

	// Lastly, call ResumePublish and make sure subsequent publishes succeed.
	topic.ResumePublish(orderingKey)
	r = topic.Publish(ctx, &Message{
		Data:        []byte("should succeed"),
		OrderingKey: orderingKey,
	})
	if _, err := r.Get(ctx); err != nil {
		t.Fatalf("got error while publishing message: %v", err)
	}
}

// TestIntegration_OrderedKeys_SubscriptionOrdering tests that messages
// with ordering keys are not processed as such if the subscription
// does not have message ordering enabled.
func TestIntegration_OrderedKeys_SubscriptionOrdering(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t, option.WithEndpoint("us-west1-pubsub.googleapis.com:443"))
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()
	exists, err := topic.Exists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("topic %v should exist, but it doesn't", topic)
	}
	topic.EnableMessageOrdering = true

	// Explicitly disable message ordering on the subscription.
	enableMessageOrdering := false
	subCfg := SubscriptionConfig{
		Topic:                 topic,
		EnableMessageOrdering: enableMessageOrdering,
	}
	sub, err := createSubWithRetry(ctx, t, client, subIDs.New(), subCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Delete(ctx)

	publishSync(ctx, t, topic, &Message{
		Data:        []byte("message-1"),
		OrderingKey: "ordering-key-1",
	})

	publishSync(ctx, t, topic, &Message{
		Data:        []byte("message-2"),
		OrderingKey: "ordering-key-1",
	})

	sub.ReceiveSettings.Synchronous = true
	ctx2, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	var numAcked int32
	sub.Receive(ctx2, func(_ context.Context, msg *Message) {
		// Create artificial constraints on message processing time.
		if string(msg.Data) == "message-1" {
			time.Sleep(10 * time.Second)
		} else {
			time.Sleep(5 * time.Second)
		}
		msg.Ack()
		atomic.AddInt32(&numAcked, 1)
	})
	// If the messages were received on a subscription with the EnableMessageOrdering=true,
	// total processing would exceed the timeout and only one message would be processed.
	if numAcked < 2 {
		t.Fatalf("did not process all messages in time, numAcked: %d", numAcked)
	}
}

func TestIntegration_OrderingWithExactlyOnce(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t, option.WithEndpoint("us-west1-pubsub.googleapis.com:443"))
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()
	exists, err := topic.Exists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("topic %v should exist, but it doesn't", topic)
	}
	var sub *Subscription
	if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), SubscriptionConfig{
		Topic:                     topic,
		EnableMessageOrdering:     true,
		EnableExactlyOnceDelivery: true,
	}); err != nil {
		t.Fatal(err)
	}
	defer sub.Delete(ctx)
	exists, err = sub.Exists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("subscription %s should exist, but it doesn't", sub.ID())
	}

	topic.PublishSettings.DelayThreshold = time.Second
	topic.EnableMessageOrdering = true

	orderingKey := "some-ordering-key"
	numItems := 10
	for i := 0; i < numItems; i++ {
		r := topic.Publish(ctx, &Message{
			ID:          fmt.Sprintf("id-%d", i),
			Data:        []byte(fmt.Sprintf("item-%d", i)),
			OrderingKey: orderingKey,
		})
		go func() {
			if _, err := r.Get(ctx); err != nil {
				t.Error(err)
			}
		}()
	}

	received := make(chan string, numItems)
	ctx2, cancel := context.WithCancel(ctx)
	go func() {
		for i := 0; i < numItems; i++ {
			select {
			case r := <-received:
				if got, want := r, fmt.Sprintf("item-%d", i); got != want {
					t.Errorf("%d: got %s, want %s", i, got, want)
				}
			case <-time.After(30 * time.Second):
				t.Errorf("timed out after 30s waiting for item %d", i)
				cancel()
			}
		}
		cancel()
	}()

	if err := sub.Receive(ctx2, func(ctx context.Context, msg *Message) {
		defer msg.Ack()
		if msg.OrderingKey != orderingKey {
			t.Errorf("got ordering key %s, expected %s", msg.OrderingKey, orderingKey)
		}

		received <- string(msg.Data)
	}); err != nil {
		if c := status.Code(err); c != codes.Canceled {
			t.Error(err)
		}
	}

}

func TestIntegration_CreateSubscription_DeadLetterPolicy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	deadLetterTopic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer deadLetterTopic.Delete(ctx)
	defer deadLetterTopic.Stop()

	// We don't set MaxDeliveryAttempts in DeadLetterPolicy so that we can test
	// that MaxDeliveryAttempts defaults properly to 5 if not set.
	cfg := SubscriptionConfig{
		Topic: topic,
		DeadLetterPolicy: &DeadLetterPolicy{
			DeadLetterTopic: deadLetterTopic.String(),
		},
	}
	var sub *Subscription
	if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), cfg); err != nil {
		t.Fatalf("CreateSub error: %v", err)
	}
	defer sub.Delete(ctx)

	got, err := sub.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := &DeadLetterPolicy{
		DeadLetterTopic:     deadLetterTopic.String(),
		MaxDeliveryAttempts: 5,
	}
	if diff := testutil.Diff(got.DeadLetterPolicy, want); diff != "" {
		t.Fatalf("\ngot: - want: +\n%s", diff)
	}

	res := topic.Publish(ctx, &Message{
		Data: []byte("failed message"),
	})
	if _, err := res.Get(ctx); err != nil {
		t.Fatalf("Publish message error: %v", err)
	}

	ctx2, cancel := context.WithCancel(ctx)
	numAttempts := 1
	err = sub.Receive(ctx2, func(_ context.Context, m *Message) {
		if numAttempts >= 5 {
			cancel()
			m.Ack()
			return
		}
		if *m.DeliveryAttempt != numAttempts {
			t.Fatalf("Message delivery attempt: %d does not match numAttempts: %d\n", m.DeliveryAttempt, numAttempts)
		}
		numAttempts++
		m.Nack()
	})
	if err != nil {
		t.Fatalf("Streaming pull error: %v\n", err)
	}
}

// Test that the DeliveryAttempt field is nil when dead lettering is not enabled.
func TestIntegration_DeadLetterPolicy_DeliveryAttempt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	cfg := SubscriptionConfig{
		Topic: topic,
	}
	var sub *Subscription
	if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), cfg); err != nil {
		t.Fatalf("CreateSub error: %v", err)
	}
	defer sub.Delete(ctx)

	res := topic.Publish(ctx, &Message{
		Data: []byte("failed message"),
	})
	if _, err := res.Get(ctx); err != nil {
		t.Fatalf("Publish message error: %v", err)
	}

	ctx2, cancel := context.WithCancel(ctx)
	err = sub.Receive(ctx2, func(_ context.Context, m *Message) {
		defer m.Ack()
		defer cancel()
		if m.DeliveryAttempt != nil {
			t.Fatalf("DeliveryAttempt should be nil when dead lettering is disabled")
		}
	})
	if err != nil {
		t.Fatalf("Streaming pull error: %v\n", err)
	}
}

func TestIntegration_DeadLetterPolicy_ClearDeadLetter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	deadLetterTopic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer deadLetterTopic.Delete(ctx)
	defer deadLetterTopic.Stop()

	cfg := SubscriptionConfig{
		Topic: topic,
		DeadLetterPolicy: &DeadLetterPolicy{
			DeadLetterTopic: deadLetterTopic.String(),
		},
	}
	var sub *Subscription
	if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), cfg); err != nil {
		t.Fatalf("CreateSub error: %v", err)
	}
	defer sub.Delete(ctx)

	sub.Update(ctx, SubscriptionConfigToUpdate{
		DeadLetterPolicy: &DeadLetterPolicy{},
	})

	got, err := sub.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.DeadLetterPolicy != nil {
		t.Fatalf("config.DeadLetterPolicy; got: %v want: nil", got.DeadLetterPolicy)
	}
}

// TestIntegration_BadEndpoint tests that specifying a bad
// endpoint will cause an error in RPCs.
func TestIntegration_BadEndpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	opts := withGRPCHeadersAssertion(t,
		option.WithEndpoint("example.googleapis.com:443"),
	)
	client := integrationTestClient(ctx, t, opts...)
	defer client.Close()
	if _, err := client.CreateTopic(ctx, topicIDs.New()); err == nil {
		t.Fatalf("CreateTopic should fail with fake endpoint, got nil err")
	}
}

func TestIntegration_Filter_CreateSubscription(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()
	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()
	cfg := SubscriptionConfig{
		Topic:  topic,
		Filter: "attributes.event_type = \"1\"",
	}
	var sub *Subscription
	if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), cfg); err != nil {
		t.Fatalf("CreateSub error: %v", err)
	}
	defer sub.Delete(ctx)
	got, err := sub.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := cfg.Filter
	if got.Filter != want {
		t.Fatalf("subcfg.Filter mismatch; got: %s, want: %s", got.Filter, want)
	}
	attrs := make(map[string]string)
	attrs["event_type"] = "1"
	res := topic.Publish(ctx, &Message{
		Data:       []byte("hello world"),
		Attributes: attrs,
	})
	if _, err := res.Get(ctx); err != nil {
		t.Fatalf("Publish message error: %v", err)
	}
	// Publish the same message with a different event_type
	// and check it is filtered out.
	attrs["event_type"] = "2"
	res = topic.Publish(ctx, &Message{
		Data:       []byte("hello world"),
		Attributes: attrs,
	})
	if _, err := res.Get(ctx); err != nil {
		t.Fatalf("Publish message error: %v", err)
	}
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err = sub.Receive(ctx2, func(_ context.Context, m *Message) {
		defer m.Ack()
		if m.Attributes["event_type"] != "1" {
			t.Fatalf("Got message with attributes that should be filtered out: %v", m.Attributes)
		}
	})
	if err != nil {
		t.Fatalf("Streaming pull error: %v\n", err)
	}
}

func TestIntegration_RetryPolicy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	cfg := SubscriptionConfig{
		Topic: topic,
		RetryPolicy: &RetryPolicy{
			MinimumBackoff: 20 * time.Second,
			MaximumBackoff: 500 * time.Second,
		},
	}
	var sub *Subscription
	if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), cfg); err != nil {
		t.Fatalf("CreateSub error: %v", err)
	}
	defer sub.Delete(ctx)

	got, err := sub.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := SubscriptionConfig{
		Topic:               topic,
		AckDeadline:         10 * time.Second,
		RetainAckedMessages: false,
		RetentionDuration:   defaultRetentionDuration,
		ExpirationPolicy:    defaultExpirationPolicy,
		RetryPolicy: &RetryPolicy{
			MinimumBackoff: 20 * time.Second,
			MaximumBackoff: 500 * time.Second,
		},
	}
	if diff := testutil.Diff(got.RetryPolicy, want.RetryPolicy); diff != "" {
		t.Fatalf("\ngot: - want: +\n%s", diff)
	}

	// Test clearing the RetryPolicy
	cfgToUpdate := SubscriptionConfigToUpdate{
		RetryPolicy: &RetryPolicy{},
	}
	_, err = sub.Update(ctx, cfgToUpdate)
	if err != nil {
		t.Fatalf("got error while updating sub: %v", err)
	}

	got, err = sub.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want.RetryPolicy = nil
	if diff := testutil.Diff(got.RetryPolicy, want.RetryPolicy); diff != "" {
		t.Fatalf("\ngot: - want: +\n%s", diff)
	}
}

func TestIntegration_DetachSubscription(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	cfg := SubscriptionConfig{
		Topic: topic,
	}
	var sub *Subscription
	if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), cfg); err != nil {
		t.Fatalf("CreateSub error: %v", err)
	}
	defer sub.Delete(ctx)

	if _, err := client.DetachSubscription(ctx, sub.String()); err != nil {
		t.Fatalf("DetachSubscription error: %v", err)
	}

	newSub := client.Subscription(sub.ID())
	got, err := newSub.Config(ctx)
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if !got.Detached {
		t.Fatal("SubscriptionConfig not detached after calling detach")
	}
}

func TestIntegration_SchemaAdmin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := integrationTestSchemaClient(ctx, t)
	defer c.Close()

	for _, tc := range []struct {
		desc       string
		schemaType SchemaType
		path       string
	}{
		{
			desc:       "avro schema",
			schemaType: SchemaAvro,
			path:       "testdata/schema/us-states.avsc",
		},
		{
			desc:       "protocol buffer schema",
			schemaType: SchemaProtocolBuffer,
			path:       "testdata/schema/us-states.proto",
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			content, err := ioutil.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			schema := string(content)
			schemaID := schemaIDs.New()
			schemaPath := fmt.Sprintf("projects/%s/schemas/%s", testutil.ProjID(), schemaID)
			sc := SchemaConfig{
				Type:       tc.schemaType,
				Definition: schema,
			}
			got, err := c.CreateSchema(ctx, schemaID, sc)
			if err != nil {
				t.Fatalf("SchemaClient.CreateSchema error: %v", err)
			}

			want := &SchemaConfig{
				Name:       schemaPath,
				Type:       tc.schemaType,
				Definition: schema,
			}
			if diff := testutil.Diff(got, want, cmpopts.IgnoreFields(SchemaConfig{}, "RevisionID", "RevisionCreateTime")); diff != "" {
				t.Fatalf("\ngot: - want: +\n%s", diff)
			}

			got, err = c.Schema(ctx, schemaID, SchemaViewFull)
			if err != nil {
				t.Fatalf("SchemaClient.Schema error: %v", err)
			}
			if diff := testutil.Diff(got, want, cmpopts.IgnoreFields(SchemaConfig{}, "RevisionID", "RevisionCreateTime")); diff != "" {
				t.Fatalf("\ngot: - want: +\n%s", diff)
			}

			err = c.DeleteSchema(ctx, schemaID)
			if err != nil {
				t.Fatalf("SchemaClient.DeleteSchema error: %v", err)
			}
		})
	}
}

func TestIntegration_ValidateSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := integrationTestSchemaClient(ctx, t)
	defer c.Close()

	for _, tc := range []struct {
		desc       string
		schemaType SchemaType
		path       string
		wantErr    error
	}{
		{
			desc:       "avro schema",
			schemaType: SchemaAvro,
			path:       "testdata/schema/us-states.avsc",
			wantErr:    nil,
		},
		{
			desc:       "protocol buffer schema",
			schemaType: SchemaProtocolBuffer,
			path:       "testdata/schema/us-states.proto",
			wantErr:    nil,
		},
		{
			desc:       "protocol buffer schema",
			schemaType: SchemaProtocolBuffer,
			path:       "testdata/schema/invalid.avsc",
			wantErr:    status.Errorf(codes.InvalidArgument, "Request contains an invalid argument."),
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			content, err := ioutil.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			def := string(content)
			cfg := SchemaConfig{
				Type:       tc.schemaType,
				Definition: def,
			}
			_, gotErr := c.ValidateSchema(ctx, cfg)
			if status.Code(gotErr) != status.Code(tc.wantErr) {
				t.Fatalf("got err: %v\nwant err: %v", gotErr, tc.wantErr)
			}
		})
	}
}

func TestIntegration_ValidateMessage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := integrationTestSchemaClient(ctx, t)
	defer c.Close()

	for _, tc := range []struct {
		desc        string
		schemaType  SchemaType
		schemaPath  string
		encoding    SchemaEncoding
		messagePath string
		wantErr     error
	}{
		{
			desc:        "avro json encoding",
			schemaType:  SchemaAvro,
			schemaPath:  "testdata/schema/us-states.avsc",
			encoding:    EncodingJSON,
			messagePath: "testdata/schema/alaska.json",
			wantErr:     nil,
		},
		{
			desc:        "avro binary encoding",
			schemaType:  SchemaAvro,
			schemaPath:  "testdata/schema/us-states.avsc",
			encoding:    EncodingBinary,
			messagePath: "testdata/schema/alaska.avro",
			wantErr:     nil,
		},
		{
			desc:        "proto json encoding",
			schemaType:  SchemaProtocolBuffer,
			schemaPath:  "testdata/schema/us-states.proto",
			encoding:    EncodingJSON,
			messagePath: "testdata/schema/alaska.json",
			wantErr:     nil,
		},
		{
			desc:        "protocol buffer schema",
			schemaType:  SchemaProtocolBuffer,
			schemaPath:  "testdata/schema/invalid.avsc",
			encoding:    EncodingBinary,
			messagePath: "testdata/schema/invalid.avsc",
			wantErr:     status.Errorf(codes.InvalidArgument, "Request contains an invalid argument."),
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			content, err := ioutil.ReadFile(tc.schemaPath)
			if err != nil {
				t.Fatal(err)
			}
			def := string(content)
			cfg := SchemaConfig{
				Type:       tc.schemaType,
				Definition: def,
			}

			msg, err := ioutil.ReadFile(tc.messagePath)
			if err != nil {
				t.Fatal(err)
			}
			_, gotErr := c.ValidateMessageWithConfig(ctx, msg, tc.encoding, cfg)
			if status.Code(gotErr) != status.Code(tc.wantErr) {
				t.Fatalf("got err: %v\nwant err: %v", gotErr, tc.wantErr)
			}
		})
	}
}

func TestIntegration_TopicRetention(t *testing.T) {
	ctx := context.Background()
	c := integrationTestClient(ctx, t)
	defer c.Close()

	tc := TopicConfig{
		RetentionDuration: 31 * 24 * time.Hour, // max retention duration
	}

	topic, err := createTopicWithRetry(ctx, t, c, topicIDs.New(), &tc)
	if err != nil {
		t.Fatalf("failed to create topic: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	newDur := 11 * time.Minute
	cfg, err := topic.Update(ctx, TopicConfigToUpdate{
		RetentionDuration: newDur,
	})
	if err != nil {
		t.Fatalf("failed to update topic: %v", err)
	}
	if got := cfg.RetentionDuration; got != newDur {
		t.Fatalf("cfg.RetentionDuration, got: %v, want: %v", got, newDur)
	}

	// Create a subscription on the topic and read TopicMessageRetentionDuration.
	s, err := createSubWithRetry(ctx, t, c, subIDs.New(), SubscriptionConfig{
		Topic: topic,
	})
	if err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}
	defer s.Delete(ctx)
	sCfg, err := s.Config(ctx)
	if err != nil {
		t.Fatalf("failed to get sub config: %v", err)
	}
	if got := sCfg.TopicMessageRetentionDuration; got != newDur {
		t.Fatalf("sCfg.TopicMessageRetentionDuration, got: %v, want: %v", got, newDur)
	}

	// Clear retention duration by setting to a negative value.
	cfg, err = topic.Update(ctx, TopicConfigToUpdate{
		RetentionDuration: -1 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.RetentionDuration; got != nil {
		t.Fatalf("expected cleared retention duration, got: %v", got)
	}
}

func TestIntegration_ExactlyOnceDelivery_PublishReceive(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t)

	for _, maxMsgs := range []int{0, 3, -1} { // MaxOutstandingMessages = default, 3, unlimited
		testPublishAndReceive(t, client, maxMsgs, false, true, 10, 0)
	}
}

func TestIntegration_TopicUpdateSchema(t *testing.T) {
	ctx := context.Background()
	c := integrationTestClient(ctx, t)
	defer c.Close()

	sc := integrationTestSchemaClient(ctx, t)
	defer sc.Close()

	schemaContent, err := ioutil.ReadFile("testdata/schema/us-states.avsc")
	if err != nil {
		t.Fatal(err)
	}

	schemaID := schemaIDs.New()
	schemaCfg, err := sc.CreateSchema(ctx, schemaID, SchemaConfig{
		Type:       SchemaAvro,
		Definition: string(schemaContent),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sc.DeleteSchema(ctx, schemaID)

	topic, err := createTopicWithRetry(ctx, t, c, topicIDs.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	schema := &SchemaSettings{
		Schema:   schemaCfg.Name,
		Encoding: EncodingJSON,
	}
	cfg, err := topic.Update(ctx, TopicConfigToUpdate{
		SchemaSettings: schema,
	})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(cfg.SchemaSettings, schema); diff != "" {
		t.Fatalf("schema settings for update -want, +got: %v", diff)
	}
}

func TestIntegration_DetectProjectID(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	ctx := context.Background()
	testCreds := testutil.Credentials(ctx)
	if testCreds == nil {
		t.Skip("test credentials not present, skipping")
	}

	goodClient, err := NewClient(ctx, DetectProjectID, option.WithCredentials(testCreds))
	if err != nil {
		t.Errorf("test pubsub.NewClient: %v", err)
	}
	if goodClient.Project() != testutil.ProjID() {
		t.Errorf("client.Project() got %q, want %q", goodClient.Project(), testutil.ProjID())
	}

	badTS := testutil.ErroringTokenSource{}
	if badClient, err := NewClient(ctx, DetectProjectID, option.WithTokenSource(badTS)); err == nil {
		t.Errorf("expected error from bad token source, NewClient succeeded with project: %s", badClient.projectID)
	}
}

func TestIntegration_PublishCompression(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	topic.PublishSettings.EnableCompression = true
	topic.PublishSettings.CompressionBytesThreshold = 50

	const messageSizeBytes = 1000

	msg := &Message{Data: bytes.Repeat([]byte{'A'}, int(messageSizeBytes))}
	res := topic.Publish(ctx, msg)

	_, err = res.Get(ctx)
	if err != nil {
		t.Errorf("publish result got err: %v", err)
	}
}

// createTopicWithRetry creates a topic, wrapped with testutil.Retry and returns the created topic or an error.
func createTopicWithRetry(ctx context.Context, t *testing.T, c *Client, topicID string, cfg *TopicConfig) (*Topic, error) {
	var topic *Topic
	var err error
	testutil.Retry(t, 5, 1*time.Second, func(r *testutil.R) {
		if cfg != nil {
			topic, err = c.CreateTopicWithConfig(ctx, topicID, cfg)
			if err != nil {
				r.Errorf("CreateTopic error: %v", err)
			}
		} else {
			topic, err = c.CreateTopic(ctx, topicID)
			if err != nil {
				r.Errorf("CreateTopic error: %v", err)
			}
		}
	})
	return topic, err
}

// createSubWithRetry creates a subscription, wrapped with testutil.Retry and returns the created subscription or an error.
func createSubWithRetry(ctx context.Context, t *testing.T, c *Client, subID string, cfg SubscriptionConfig) (*Subscription, error) {
	var sub *Subscription
	var err error
	testutil.Retry(t, 5, 1*time.Second, func(r *testutil.R) {
		sub, err = c.CreateSubscription(ctx, subID, cfg)
		if err != nil {
			r.Errorf("CreateSub error: %v", err)
		}
	})
	return sub, err
}
