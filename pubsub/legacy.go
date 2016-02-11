// Copyright 2016 Google Inc. All Rights Reserved.
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
	"errors"
	"net/http"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"
	raw "google.golang.org/api/pubsub/v1"
	"google.golang.org/cloud/internal"
)

// CreateTopic creates a new topic with the specified name on the backend.
//
// Deprecated: Use Client.NewTopic instead.
//
// It will return an error if topic already exists.
func CreateTopic(ctx context.Context, name string) error {
	_, err := rawService(ctx).Projects.Topics.Create(fullTopicName(internal.ProjID(ctx), name), &raw.Topic{}).Do()
	return err
}

// DeleteTopic deletes the specified topic.
//
// Deprecated: Use TopicHandle.Delete instead.
func DeleteTopic(ctx context.Context, name string) error {
	_, err := rawService(ctx).Projects.Topics.Delete(fullTopicName(internal.ProjID(ctx), name)).Do()
	return err
}

// TopicExists returns true if a topic exists with the specified name.
//
// Deprecated: Use TopicHandle.Exists instead.
func TopicExists(ctx context.Context, name string) (bool, error) {
	_, err := rawService(ctx).Projects.Topics.Get(fullTopicName(internal.ProjID(ctx), name)).Do()
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// DeleteSub deletes the subscription.
//
// Deprecated: Use SubscriptionHandle.Delete instead.
func DeleteSub(ctx context.Context, name string) error {
	_, err := rawService(ctx).Projects.Subscriptions.Delete(fullSubName(internal.ProjID(ctx), name)).Do()
	return err
}

// SubExists returns true if subscription exists.
//
// Deprecated: Use SubscriptionHandle.Exists instead.
func SubExists(ctx context.Context, name string) (bool, error) {
	_, err := rawService(ctx).Projects.Subscriptions.Get(fullSubName(internal.ProjID(ctx), name)).Do()
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// CreateSub creates a Pub/Sub subscription on the backend.
//
// Deprecated: Use TopicHandle.Subscribe instead.
//
// A subscription should subscribe to an existing topic.
//
// The messages that haven't acknowledged will be pushed back to the
// subscription again when the default acknowledgement deadline is
// reached. You can override the default deadline by providing a
// non-zero deadline. Deadline must not be specified to
// precision greater than one second.
//
// As new messages are being queued on the subscription, you
// may recieve push notifications regarding to the new arrivals.
// To receive notifications of new messages in the queue,
// specify an endpoint callback URL.
// If endpoint is an empty string the backend will not notify the
// client of new messages.
//
// If the subscription already exists an error will be returned.
func CreateSub(ctx context.Context, name string, topic string, deadline time.Duration, endpoint string) error {
	sub := &raw.Subscription{
		Topic: fullTopicName(internal.ProjID(ctx), topic),
	}
	if int64(deadline) > 0 {
		if !isSec(deadline) {
			return errors.New("pubsub: deadline must not be specified to precision greater than one second")
		}
		sub.AckDeadlineSeconds = int64(deadline / time.Second)
	}
	if endpoint != "" {
		sub.PushConfig = &raw.PushConfig{PushEndpoint: endpoint}
	}
	_, err := rawService(ctx).Projects.Subscriptions.Create(fullSubName(internal.ProjID(ctx), name), sub).Do()
	return err
}
