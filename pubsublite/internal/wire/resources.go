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

package wire

import (
	"fmt"
	"regexp"
	"strings"
)

// ValidateZone verifies that the `input` string has the format of a valid
// Google Cloud zone. An example zone is "europe-west1-b".
// See https://cloud.google.com/compute/docs/regions-zones for more information.
func ValidateZone(input string) error {
	parts := strings.Split(input, "-")
	if len(parts) != 3 {
		return fmt.Errorf("pubsublite: invalid zone %q", input)
	}
	return nil
}

// ValidateRegion verifies that the `input` string has the format of a valid
// Google Cloud region. An example region is "europe-west1".
// See https://cloud.google.com/compute/docs/regions-zones for more information.
func ValidateRegion(input string) error {
	parts := strings.Split(input, "-")
	if len(parts) != 2 {
		return fmt.Errorf("pubsublite: invalid region %q", input)
	}
	return nil
}

// ZoneToRegion returns the region that the given zone is in.
func ZoneToRegion(zone string) (string, error) {
	if err := ValidateZone(zone); err != nil {
		return "", err
	}
	return zone[0:strings.LastIndex(zone, "-")], nil
}

// LocationPath stores a path consisting of a project and zone.
type LocationPath struct {
	// A Google Cloud project. The project ID (e.g. "my-project") or the project
	// number (e.g. "987654321") can be provided.
	Project string

	// A Google Cloud zone, for example "us-central1-a".
	Zone string
}

func (l LocationPath) String() string {
	return fmt.Sprintf("projects/%s/locations/%s", l.Project, l.Zone)
}

var locPathRE = regexp.MustCompile(`^projects/([^/]+)/locations/([^/]+)$`)

// ParseLocationPath parses a project/location path.
func ParseLocationPath(input string) (LocationPath, error) {
	parts := locPathRE.FindStringSubmatch(input)
	if len(parts) < 3 {
		return LocationPath{}, fmt.Errorf("pubsublite: invalid location path %q. valid format is %q",
			input, "projects/PROJECT_ID/locations/ZONE")
	}
	return LocationPath{Project: parts[1], Zone: parts[2]}, nil
}

// TopicPath stores the full path of a Pub/Sub Lite topic.
type TopicPath struct {
	// A Google Cloud project. The project ID (e.g. "my-project") or the project
	// number (e.g. "987654321") can be provided.
	Project string

	// A Google Cloud zone, for example "us-central1-a".
	Zone string

	// The ID of the Pub/Sub Lite topic, for example "my-topic-name".
	TopicID string
}

func (t TopicPath) String() string {
	return fmt.Sprintf("projects/%s/locations/%s/topics/%s", t.Project, t.Zone, t.TopicID)
}

// Location returns the topic's location path.
func (t TopicPath) Location() LocationPath {
	return LocationPath{Project: t.Project, Zone: t.Zone}
}

var topicPathRE = regexp.MustCompile(`^projects/([^/]+)/locations/([^/]+)/topics/([^/]+)$`)

// ParseTopicPath parses the full path of a Pub/Sub Lite topic.
func ParseTopicPath(input string) (TopicPath, error) {
	parts := topicPathRE.FindStringSubmatch(input)
	if len(parts) < 4 {
		return TopicPath{}, fmt.Errorf("pubsublite: invalid topic path %q. valid format is %q",
			input, "projects/PROJECT_ID/locations/ZONE/topics/TOPIC_ID")
	}
	return TopicPath{Project: parts[1], Zone: parts[2], TopicID: parts[3]}, nil
}

// SubscriptionPath stores the full path of a Pub/Sub Lite subscription.
type SubscriptionPath struct {
	// A Google Cloud project. The project ID (e.g. "my-project") or the project
	// number (e.g. "987654321") can be provided.
	Project string

	// A Google Cloud zone. An example zone is "us-central1-a".
	Zone string

	// The ID of the Pub/Sub Lite subscription, for example
	// "my-subscription-name".
	SubscriptionID string
}

func (s SubscriptionPath) String() string {
	return fmt.Sprintf("projects/%s/locations/%s/subscriptions/%s", s.Project, s.Zone, s.SubscriptionID)
}

// Location returns the subscription's location path.
func (s SubscriptionPath) Location() LocationPath {
	return LocationPath{Project: s.Project, Zone: s.Zone}
}

var subsPathRE = regexp.MustCompile(`^projects/([^/]+)/locations/([^/]+)/subscriptions/([^/]+)$`)

// ParseSubscriptionPath parses the full path of a Pub/Sub Lite subscription.
func ParseSubscriptionPath(input string) (SubscriptionPath, error) {
	parts := subsPathRE.FindStringSubmatch(input)
	if len(parts) < 4 {
		return SubscriptionPath{}, fmt.Errorf("pubsublite: invalid subscription path %q. valid format is %q",
			input, "projects/PROJECT_ID/locations/ZONE/subscriptions/SUBSCRIPTION_ID")
	}
	return SubscriptionPath{Project: parts[1], Zone: parts[2], SubscriptionID: parts[3]}, nil
}

type topicPartition struct {
	Path      string
	Partition int
}

func (tp topicPartition) String() string {
	return fmt.Sprintf("%s/partitions/%d", tp.Path, tp.Partition)
}

type subscriptionPartition struct {
	Path      string
	Partition int
}

func (sp subscriptionPartition) String() string {
	return fmt.Sprintf("%s/partitions/%d", sp.Path, sp.Partition)
}
