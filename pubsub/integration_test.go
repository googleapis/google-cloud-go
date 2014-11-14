// Copyright 2014 Google Inc. All Rights Reserved.
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

// +build integration

package pubsub

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/golang/oauth2"
	"github.com/golang/oauth2/google"
	"golang.org/x/net/context"
	"google.golang.org/cloud"
)

const (
	envProjID     = "GCLOUD_TESTS_GOLANG_PROJECT_ID"
	envPrivateKey = "GCLOUD_TESTS_GOLANG_KEY"
)

func TestAll(t *testing.T) {
	ctx := testContext(t)
	topic := fmt.Sprintf("topic-%d", time.Now().Unix())
	subscription := fmt.Sprintf("subscription-%d", time.Now().Unix())

	if err := CreateTopic(ctx, topic); err != nil {
		t.Errorf("CreateTopic error: %v", err)
	}

	if err := CreateSub(ctx, subscription, topic, time.Duration(0), ""); err != nil {
		t.Errorf("CreateSub error: %v", err)
	}

	exists, err := TopicExists(ctx, topic)
	if err != nil {
		t.Errorf("TopicExists error: %v", err)
	}
	if !exists {
		t.Errorf("topic %s should exist, but it doesn't", topic)
	}

	exists, err = SubExists(ctx, subscription)
	if err != nil {
		t.Errorf("SubExists error: %v", err)
	}
	if !exists {
		t.Errorf("subscription %s should exist, but it doesn't", subscription)
	}

	text := fmt.Sprintf("a message from %s", time.Now())
	labels := make(map[string]string)
	labels["foo"] = "bar"
	if err := Publish(ctx, topic, []byte(text), labels); err != nil {
		t.Errorf("Publish (1) error: %v", err)
	}
	// TODO(jbd): Stop publishing twice when the API fixes its latency
	// issues with the initial message on a topic.
	if err := Publish(ctx, topic, []byte(text), labels); err != nil {
		t.Errorf("Publish (2) error: %v", err)
	}
	msg, err := Pull(ctx, subscription)
	if err != nil {
		t.Errorf("Pull error: %v", err)
		return
	}
	if string(msg.Data) != text {
		t.Errorf("message data is expected to be '%s', found '%s'.", text, msg.Data)
	}
	if msg.Labels["foo"] != "bar" {
		t.Errorf("message label foo is expected to be '%s', found '%s'", labels["foo"], msg.Labels["foo"])
	}
	if err := Ack(ctx, subscription, msg.AckID); err != nil {
		t.Errorf("Can't acknowledge the message (1)", err)
	}
	// TODO(jbd): Test PullWait with timeout case.

	// Allow user to publish and pull messages with nil data.
	if err := Publish(ctx, topic, nil, nil); err != nil {
		t.Errorf("Publish with nil data failed with %v", err)
	}
	msg, err = Pull(ctx, subscription)
	if err != nil {
		t.Errorf("Pulling message with nil data failed with %v", err)
	}
	if msg.AckID == "" {
		t.Error("Missing acknowledgement ID for the nil-data message")
	}
	if msg.Data != nil {
		t.Errorf("Message should have been nil for message, found %v", msg.Data)
	}
	if err := Ack(ctx, subscription, msg.AckID); err != nil {
		t.Errorf("Can't acknowledge the message (2)", err)
	}

	err = DeleteSub(ctx, subscription)
	if err != nil {
		t.Errorf("DeleteSub error: %v", err)
	}

	err = DeleteTopic(ctx, topic)
	if err != nil {
		t.Errorf("DeleteTopic error: %v", err)
	}
}

func testContext(t *testing.T) context.Context {
	f, err := oauth2.New(
		google.ServiceAccountJSONKey(os.Getenv(envPrivateKey)),
		oauth2.Scope(ScopePubSub, ScopeCloudPlatform),
	)
	if err != nil {
		t.Fatal(err)
	}
	return cloud.NewContext(
		os.Getenv(envProjID), &http.Client{Transport: f.NewTransport()})
}
