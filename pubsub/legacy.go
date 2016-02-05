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
	"net/http"

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
