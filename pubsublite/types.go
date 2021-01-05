// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and

package pubsublite

import (
	"fmt"
	"regexp"
	"strings"

	"cloud.google.com/go/pubsublite/internal/wire"
)

// LocationPath stores a path consisting of a project and zone.
type LocationPath struct {
	// A Google Cloud project. The project ID (e.g. "my-project") or the project
	// number (e.g. "987654321") can be provided.
	Project string

	// A Google Cloud zone, for example "us-central1-a".
	// See https://cloud.google.com/pubsub/lite/docs/locations for the list of
	// zones where Cloud Pub/Sub Lite is available.
	Zone string
}

func (l LocationPath) String() string {
	return fmt.Sprintf("projects/%s/locations/%s", l.Project, l.Zone)
}

// TopicPath stores the full path of a Cloud Pub/Sub Lite topic.
// See https://cloud.google.com/pubsub/lite/docs/topics for more information.
type TopicPath struct {
	// A Google Cloud project. The project ID (e.g. "my-project") or the project
	// number (e.g. "987654321") can be provided.
	Project string

	// A Google Cloud zone, for example "us-central1-a".
	// See https://cloud.google.com/pubsub/lite/docs/locations for the list of
	// zones where Cloud Pub/Sub Lite is available.
	Zone string

	// The ID of the Cloud Pub/Sub Lite topic, for example "my-topic-name".
	// See https://cloud.google.com/pubsub/docs/admin#resource_names for more
	// information.
	TopicID string
}

func (t TopicPath) String() string {
	return fmt.Sprintf("projects/%s/locations/%s/topics/%s", t.Project, t.Zone, t.TopicID)
}

func (t TopicPath) location() LocationPath {
	return LocationPath{Project: t.Project, Zone: t.Zone}
}

var topicPathRE = regexp.MustCompile(`^projects/([^/]+)/locations/([^/]+)/topics/([^/]+)$`)

// parseTopicPath parses the full path of a Cloud Pub/Sub Lite topic, which
// should have the format: `projects/{project}/locations/{zone}/topics/{id}`.
func parseTopicPath(input string) (TopicPath, error) {
	parts := topicPathRE.FindStringSubmatch(input)
	if len(parts) < 4 {
		return TopicPath{}, fmt.Errorf("pubsublite: invalid topic path %q", input)
	}
	return TopicPath{Project: parts[1], Zone: parts[2], TopicID: parts[3]}, nil
}

// SubscriptionPath stores the full path of a Cloud Pub/Sub Lite subscription.
// See https://cloud.google.com/pubsub/lite/docs/subscriptions for more
// information.
type SubscriptionPath struct {
	// A Google Cloud project. The project ID (e.g. "my-project") or the project
	// number (e.g. "987654321") can be provided.
	Project string

	// A Google Cloud zone. An example zone is "us-central1-a".
	// See https://cloud.google.com/pubsub/lite/docs/locations for the list of
	// zones where Cloud Pub/Sub Lite is available.
	Zone string

	// The ID of the Cloud Pub/Sub Lite subscription, for example
	// "my-subscription-name".
	// See https://cloud.google.com/pubsub/docs/admin#resource_names for more
	// information.
	SubscriptionID string
}

func (s SubscriptionPath) String() string {
	return fmt.Sprintf("projects/%s/locations/%s/subscriptions/%s", s.Project, s.Zone, s.SubscriptionID)
}

func (s SubscriptionPath) location() LocationPath {
	return LocationPath{Project: s.Project, Zone: s.Zone}
}

var subsPathRE = regexp.MustCompile(`^projects/([^/]+)/locations/([^/]+)/subscriptions/([^/]+)$`)

// parseSubscriptionPath parses the full path of a Cloud Pub/Sub Lite
// subscription, which should have the format:
// `projects/{project}/locations/{zone}/subscriptions/{id}`.
func parseSubscriptionPath(input string) (SubscriptionPath, error) {
	parts := subsPathRE.FindStringSubmatch(input)
	if len(parts) < 4 {
		return SubscriptionPath{}, fmt.Errorf("pubsublite: invalid subscription path %q", input)
	}
	return SubscriptionPath{Project: parts[1], Zone: parts[2], SubscriptionID: parts[3]}, nil
}

// ZoneToRegion returns the region that the given zone is in.
func ZoneToRegion(zone string) (string, error) {
	if err := wire.ValidateZone(zone); err != nil {
		return "", err
	}
	return zone[0:strings.LastIndex(zone, "-")], nil
}
