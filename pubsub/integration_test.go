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
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/iam"
	"cloud.google.com/go/internal"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"cloud.google.com/go/internal/version"
	kms "cloud.google.com/go/kms/apiv1"
	testutil2 "cloud.google.com/go/pubsub/internal/testutil"
	"github.com/golang/protobuf/proto"
	gax "github.com/googleapis/gax-go/v2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	kmspb "google.golang.org/genproto/googleapis/cloud/kms/v1"
	pb "google.golang.org/genproto/googleapis/pubsub/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var (
	topicIDs = uid.NewSpace("topic", nil)
	subIDs   = uid.NewSpace("sub", nil)
)

// messageData is used to hold the contents of a message so that it can be compared against the contents
// of another message without regard to irrelevant fields.
type messageData struct {
	ID         string
	Data       []byte
	Attributes map[string]string
}

func extractMessageData(m *Message) *messageData {
	return &messageData{
		ID:         m.ID,
		Data:       m.Data,
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

func integrationTestClient(ctx context.Context, t *testing.T) *Client {
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
	opts := withGRPCHeadersAssertion(t, option.WithTokenSource(ts))
	client, err := NewClient(ctx, projID, opts...)
	if err != nil {
		t.Fatalf("Creating client error: %v", err)
	}
	return client
}

func TestIntegration_All(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := client.CreateTopic(ctx, topicIDs.New())
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
	if sub, err = client.CreateSubscription(ctx, subIDs.New(), SubscriptionConfig{Topic: topic}); err != nil {
		t.Errorf("CreateSub error: %v", err)
	}
	exists, err = sub.Exists(ctx)
	if err != nil {
		t.Fatalf("SubExists error: %v", err)
	}
	if !exists {
		t.Errorf("subscription %s should exist, but it doesn't", sub.ID())
	}

	for _, sync := range []bool{false, true} {
		for _, maxMsgs := range []int{0, 3, -1} { // MaxOutstandingMessages = default, 3, unlimited
			testPublishAndReceive(t, topic, sub, maxMsgs, sync, 10, 0)
		}

		// Tests for large messages (larger than the 4MB gRPC limit).
		testPublishAndReceive(t, topic, sub, 0, sync, 1, 5*1024*1024)
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

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	err = internal.Retry(timeoutCtx, gax.Backoff{}, func() (bool, error) {
		snapIt := client.Snapshots(timeoutCtx)
		for {
			s, err := snapIt.Next()
			if err == nil && s.name == snap.name {
				return true, nil
			}
			if err == iterator.Done {
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

func testPublishAndReceive(t *testing.T, topic *Topic, sub *Subscription, maxMsgs int, synchronous bool, numMsgs, extraBytes int) {
	ctx := context.Background()
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
	want := make(map[string]*messageData)
	for _, res := range rs {
		id, err := res.r.Get(ctx)
		if err != nil {
			t.Fatal(err)
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
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	gotMsgs, err := pullN(timeoutCtx, sub, len(want), func(ctx context.Context, m *Message) {
		m.Ack()
	})
	if err != nil {
		if c := status.Convert(err); c.Code() == codes.Canceled {
			if time.Now().Sub(now) >= time.Minute {
				t.Fatal("pullN took too long")
			}
		} else {
			t.Fatalf("Pull: %v", err)
		}
	}
	got := make(map[string]*messageData)
	for _, m := range gotMsgs {
		md := extractMessageData(m)
		got[md.ID] = md
	}
	if !testutil.Equal(got, want) {
		t.Fatalf("MaxOutstandingMessages=%d, Synchronous=%t, messages got: %v, messages want: %v",
			maxMsgs, synchronous, got, want)
	}
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

	topic, err := client.CreateTopic(ctx, topicIDs.New())
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
	pbMsgOverhead := 1 + proto.SizeVarint(uint64(length))
	dataOverhead := 1 + proto.SizeVarint(uint64(length-pbMsgOverhead))
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
	ctx, cancel := context.WithCancel(context.Background())
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := client.CreateTopic(ctx, topicIDs.New())
	if err != nil {
		t.Fatal(err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	var sub *Subscription
	if sub, err = client.CreateSubscription(ctx, subIDs.New(), SubscriptionConfig{Topic: topic}); err != nil {
		t.Fatal(err)
	}
	defer sub.Delete(ctx)

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
		defer close(doneReceiving)
		err = sub.Receive(ctx, func(_ context.Context, msg *Message) {
			cancel()
			time.AfterFunc(5*time.Second, msg.Ack)
		})
		if err != nil {
			t.Error(err)
		}
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

	topic, err := client.CreateTopic(ctx, topicIDs.New())
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
	if sub, err = client.CreateSubscription(ctx, subIDs.New(), cfg); err != nil {
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
		ExpirationPolicy:    time.Duration(0),
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Fatalf("\ngot: - want: +\n%s", diff)
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

	topic, err := client.CreateTopic(ctx, topicIDs.New())
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
	if sub, err = client.CreateSubscription(ctx, subIDs.New(), sCfg); err != nil {
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
	}
	if diff := testutil.Diff(got, want); diff != "" {
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
	}

	if !testutil.Equal(got, want) {
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

	if !testutil.Equal(got, want) {
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
	if !testutil.Equal(got, want) {
		t.Fatalf("\ngot  %+v\nwant %+v", got, want)
	}
	// If nothing changes, our client returns an error.
	_, err = sub.Update(ctx, SubscriptionConfigToUpdate{})
	if err == nil {
		t.Fatal("got nil, wanted error")
	}
}

func TestIntegration_UpdateSubscription_ExpirationPolicy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := client.CreateTopic(ctx, topicIDs.New())
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	var sub *Subscription
	if sub, err = client.CreateSubscription(ctx, subIDs.New(), SubscriptionConfig{Topic: topic}); err != nil {
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
	want := SubscriptionConfig{
		Topic:             topic,
		AckDeadline:       2 * time.Minute,
		RetentionDuration: 2 * time.Hour,
		ExpirationPolicy:  25 * time.Hour,
	}
	// Pubsub service issue: PushConfig attributes are not removed.
	// TODO(jba): remove when issue resolved.
	want.PushConfig.Attributes = map[string]string{"x-goog-version": "v1"}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Fatalf("\ngot: - want: +\n%s", diff)
	}

	// ExpirationPolicy to never expire.
	got, err = sub.Update(ctx, SubscriptionConfigToUpdate{
		ExpirationPolicy: time.Duration(0),
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v\n", err)
	}
	want.ExpirationPolicy = time.Duration(0)
	if diff := testutil.Diff(got, want); diff != "" {
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
// whitelisting, a (gsuite) organization project, and specific permissions.
func TestIntegration_UpdateTopicLabels(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	compareConfig := func(got TopicConfig, wantLabels map[string]string) bool {
		if !testutil.Equal(got.Labels, wantLabels) {
			return false
		}
		// For MessageStoragePolicy, we don't want to check for an exact set of regions.
		// That set may change at any time. Instead, just make sure that the set isn't empty.
		if len(got.MessageStoragePolicy.AllowedPersistenceRegions) == 0 {
			return false
		}
		return true
	}

	topic, err := client.CreateTopic(ctx, topicIDs.New())
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

	sub, err := client.CreateSubscription(ctx, subIDs.New(), SubscriptionConfig{
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

	topic, err := client.CreateTopic(ctx, topicIDs.New())
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
	sub, err = client.CreateSubscription(ctx, subIDs.New(), SubscriptionConfig{Topic: topic})
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

	topic, err := client.CreateTopic(ctx, topicIDs.New())
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	// Initially the message storage policy should just be non-empty
	got, err := topic.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.MessageStoragePolicy.AllowedPersistenceRegions) == 0 {
		t.Fatalf("Empty AllowedPersistenceRegions in :\n%+v", got)
	}

	// Specify some regions to set.
	regions := []string{"asia-east1", "us-east1"}
	got, err = topic.Update(ctx, TopicConfigToUpdate{
		MessageStoragePolicy: &MessageStoragePolicy{
			AllowedPersistenceRegions: regions,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := TopicConfig{
		MessageStoragePolicy: MessageStoragePolicy{
			AllowedPersistenceRegions: regions,
		},
	}
	if !testutil.Equal(got, want) {
		t.Fatalf("\ngot  %+v\nwant regions%+v", got, want)
	}

	// Reset all allowed regions to project default.
	got, err = topic.Update(ctx, TopicConfigToUpdate{
		MessageStoragePolicy: &MessageStoragePolicy{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.MessageStoragePolicy.AllowedPersistenceRegions) == 0 {
		t.Fatalf("Unexpectedly got empty MessageStoragePolicy.AllowedPersistenceRegions in:\n%+v", got)
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
	topic, err := client.CreateTopic(ctx, topicIDs.New())
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
	topic, err := client.CreateTopicWithConfig(ctx, topicIDs.New(), &tc)
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
	topic, err := client.CreateTopicWithConfig(ctx, topicIDs.New(), &tc)
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
	if diff := testutil.Diff(got, want); diff != "" {
		t.Fatalf("\ngot: - want: +\n%s", diff)
	}
}

func TestIntegration_OrderedKeys_Basic(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := client.CreateTopic(ctx, topicIDs.New())
	if err != nil {
		t.Fatal(err)
	}
	defer topic.Stop()
	exists, err := topic.Exists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("topic %v should exist, but it doesn't", topic)
	}
	var sub *Subscription
	if sub, err = client.CreateSubscription(ctx, subIDs.New(), SubscriptionConfig{
		Topic:                 topic,
		EnableMessageOrdering: true,
	}); err != nil {
		t.Fatal(err)
	}
	_ = sub
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
			<-r.ready
			if r.err != nil {
				t.Error(r.err)
			}
		}()
	}

	received := make(chan string, numItems)
	go func() {
		if err := sub.Receive(ctx, func(ctx context.Context, msg *Message) {
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
	}()

	for i := 0; i < numItems; i++ {
		select {
		case r := <-received:
			if got, want := r, fmt.Sprintf("item-%d", i); got != want {
				t.Fatalf("%d: got %s, want %s", i, got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out after 5s waiting for item %d", i)
		}
	}
}

func TestIntegration_OrderedKeys_JSON(t *testing.T) {
	t.Skip("Flaky, see https://github.com/googleapis/google-cloud-go/issues/1872")
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := client.CreateTopic(ctx, topicIDs.New())
	if err != nil {
		t.Fatal(err)
	}
	defer topic.Stop()
	exists, err := topic.Exists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("topic %v should exist, but it doesn't", topic)
	}
	var sub *Subscription
	if sub, err = client.CreateSubscription(ctx, subIDs.New(), SubscriptionConfig{
		Topic:                 topic,
		EnableMessageOrdering: true,
	}); err != nil {
		t.Fatal(err)
	}
	_ = sub
	exists, err = sub.Exists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("subscription %s should exist, but it doesn't", sub.ID())
	}

	topic.PublishSettings.DelayThreshold = time.Second
	topic.EnableMessageOrdering = true

	inFile, err := os.Open("testdata/publish.csv")
	if err != nil {
		t.Fatal(err)
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
		topic.Publish(ctx, &Message{
			ID:          msg,
			Data:        []byte(msg),
			OrderingKey: key,
		})
		wg.Add(1)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	go func() {
		if err := sub.Receive(ctx, func(ctx context.Context, msg *Message) {
			defer msg.Ack()
			mu.Lock()
			defer mu.Unlock()
			if _, ok := receiveSet[msg.ID]; ok {
				return
			}
			receiveSet[msg.ID] = struct{}{}
			receiveData = append(receiveData, testutil2.OrderedKeyMsg{Key: msg.OrderingKey, Data: string(msg.Data)})
			wg.Done()
		}); err != nil {
			if c := status.Code(err); c != codes.Canceled {
				t.Error(err)
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out after 30s waiting for all messages to be received")
	}

	mu.Lock()
	defer mu.Unlock()
	if err := testutil2.VerifyKeyOrdering(publishData, receiveData); err != nil {
		t.Fatal(err)
		t.Fatalf("CreateTopic error: %v", err)
	}
}

func TestIntegration_OrderedKeys_ResumePublish(t *testing.T) {
	t.Skip("kokoro failing in https://github.com/googleapis/google-cloud-go/issues/1850")
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := client.CreateTopic(ctx, topicIDs.New())
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

	topic.PublishSettings.DelayThreshold = time.Second
	topic.EnableMessageOrdering = true

	orderingKey := "some-ordering-key2"
	// Publish a message that is too large so we'll get an error that
	// pauses publishing for this ordering key.
	r := topic.Publish(ctx, &Message{
		ID:          "1",
		Data:        bytes.Repeat([]byte("A"), 1e10),
		OrderingKey: orderingKey,
	})
	<-r.ready
	if r.err == nil {
		t.Fatalf("expected bundle byte limit error, got nil")
	}
	// Publish a normal sized message now, which should fail
	// since publishing on this ordering key is paused.
	r = topic.Publish(ctx, &Message{
		ID:          "2",
		Data:        []byte("failed message"),
		OrderingKey: orderingKey,
	})
	<-r.ready
	if r.err == nil || !strings.Contains(r.err.Error(), "pubsub: Publishing for ordering key") {
		t.Fatalf("expected ordering keys publish error, got %v", r.err)
	}

	// Lastly, call ResumePublish and make sure subsequent publishes succeed.
	topic.ResumePublish(orderingKey)
	r = topic.Publish(ctx, &Message{
		ID:          "4",
		Data:        []byte("normal message"),
		OrderingKey: orderingKey,
	})
	<-r.ready
	if r.err != nil {
		t.Fatalf("got error while publishing message: %v", r.err)
	}
}

func TestIntegration_CreateSubscription_DeadLetterPolicy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := client.CreateTopic(ctx, topicIDs.New())
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}

	defer topic.Delete(ctx)
	defer topic.Stop()

	deadLetterTopic, err := client.CreateTopic(ctx, topicIDs.New())
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
	if sub, err = client.CreateSubscription(ctx, subIDs.New(), cfg); err != nil {
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
		DeadLetterPolicy: &DeadLetterPolicy{
			DeadLetterTopic:     deadLetterTopic.String(),
			MaxDeliveryAttempts: 5,
		},
	}
	if diff := testutil.Diff(got, want); diff != "" {
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

	topic, err := client.CreateTopic(ctx, topicIDs.New())
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	cfg := SubscriptionConfig{
		Topic: topic,
	}
	var sub *Subscription
	if sub, err = client.CreateSubscription(ctx, subIDs.New(), cfg); err != nil {
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
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Fatalf("SubsciptionConfig; got: - want: +\n%s", diff)
	}

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

	topic, err := client.CreateTopic(ctx, topicIDs.New())
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	deadLetterTopic, err := client.CreateTopic(ctx, topicIDs.New())
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
	if sub, err = client.CreateSubscription(ctx, subIDs.New(), cfg); err != nil {
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
	want := SubscriptionConfig{
		Topic:               topic,
		AckDeadline:         10 * time.Second,
		RetainAckedMessages: false,
		RetentionDuration:   defaultRetentionDuration,
		ExpirationPolicy:    defaultExpirationPolicy,
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Fatalf("SubsciptionConfig; got: - want: +\n%s", diff)
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
	client, err := NewClient(ctx, testutil.ProjID(), opts...)
	if err != nil {
		t.Fatalf("Creating client error: %v", err)
	}
	if _, err = client.CreateTopic(ctx, topicIDs.New()); err == nil {
		t.Fatalf("CreateTopic should fail with fake endpoint, got nil err")
	}
}
