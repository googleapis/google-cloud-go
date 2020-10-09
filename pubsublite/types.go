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
)

// CloudRegion identifies a geographical location where resources are hosted. An
// example region is "us-central1".
// See https://cloud.google.com/compute/docs/regions-zones for more information.
type CloudRegion string

func (r CloudRegion) String() string {
	return string(r)
}

// CloudZone identifies a subset of a Google Cloud region. An example zone is
// "us-central1-a".
// See https://cloud.google.com/compute/docs/regions-zones for more information.
type CloudZone string

// ParseZone verifies that the `input` string has the format of a valid zone and
// returns it as a CloudZone type. If `input` is guaranteed to be a valid zone,
// the type conversion `CloudZone(input)` can be used instead.
func ParseZone(input string) (CloudZone, error) {
	parts := strings.Split(input, "-")
	if len(parts) != 3 {
		return CloudZone(""), fmt.Errorf("pubsublite: invalid zone %q", input)
	}
	return CloudZone(input), nil
}

func (z CloudZone) String() string {
	return string(z)
}

// Region returns the region that this zone is in.
func (z CloudZone) Region() CloudRegion {
	return CloudRegion(z[0:strings.LastIndex(z.String(), "-")])
}

// Project identifies a Google Cloud project. The project ID (e.g. "my-project")
// or the project number (e.g. 987654321) can be provided.
// See https://cloud.google.com/resource-manager/docs/creating-managing-projects
// for more information about project identifiers.
type Project string

func (p Project) String() string {
	return string(p)
}

// LocationPath stores a path consisting of a project and zone.
type LocationPath struct {
	Project Project
	Zone    CloudZone
}

func (l LocationPath) String() string {
	return fmt.Sprintf("projects/%s/locations/%s", l.Project, l.Zone)
}

// TopicID identifies a Google Pub/Sub Lite topic.
// See https://cloud.google.com/pubsub/lite/docs/topics for more information.
type TopicID string

func (t TopicID) String() string {
	return string(t)
}

// TopicPath stores the full path of a Google Pub/Sub Lite topic.
type TopicPath struct {
	Project Project
	Zone    CloudZone
	ID      TopicID
}

var topicPathRE = regexp.MustCompile(`^projects/([^/]+)/locations/([^/]+)/topics/([^/]+)`)

// ParseTopicPath parses the full path of a Google Pub/Sub Lite topic, which
// should have the format: `projects/{project}/locations/{zone}/topics/{id}`.
func ParseTopicPath(input string) (TopicPath, error) {
	parts := topicPathRE.FindStringSubmatch(input)
	if len(parts) < 4 {
		return TopicPath{}, fmt.Errorf("pubsublite: invalid topic path %q", input)
	}
	zone, err := ParseZone(parts[2])
	if err != nil {
		return TopicPath{}, fmt.Errorf("pubsublite: topic path %q contains an invalid zone", input)
	}
	return TopicPath{Project(parts[1]), zone, TopicID(parts[3])}, nil
}

func (t TopicPath) String() string {
	return fmt.Sprintf("projects/%s/locations/%s/topics/%s", t.Project, t.Zone, t.ID)
}

func (t TopicPath) Location() LocationPath {
	return LocationPath{t.Project, t.Zone}
}

// SubscriptionID identifies a Google Pub/Sub Lite subscription.
// See https://cloud.google.com/pubsub/lite/docs/subscriptions for more
// information.
type SubscriptionID string

func (s SubscriptionID) String() string {
	return string(s)
}

// SubscriptionPath stores the full path of a Google Pub/Sub Lite subscription.
type SubscriptionPath struct {
	Project Project
	Zone    CloudZone
	ID      SubscriptionID
}

var subsPathRE = regexp.MustCompile(`^projects/([^/]+)/locations/([^/]+)/subscriptions/([^/]+)`)

// ParseSubscriptionPath parses the full path of a Google Pub/Sub Lite
// subscription, which should have the format:
// `projects/{project}/locations/{zone}/subscriptions/{id}`.
func ParseSubscriptionPath(input string) (SubscriptionPath, error) {
	parts := subsPathRE.FindStringSubmatch(input)
	if len(parts) < 4 {
		return SubscriptionPath{}, fmt.Errorf("pubsublite: invalid subscription path %q", input)
	}
	zone, err := ParseZone(parts[2])
	if err != nil {
		return SubscriptionPath{}, fmt.Errorf("pubsublite: subscription path %q contains an invalid zone", input)
	}
	return SubscriptionPath{Project(parts[1]), zone, SubscriptionID(parts[3])}, nil
}

func (s SubscriptionPath) String() string {
	return fmt.Sprintf("projects/%s/locations/%s/subscriptions/%s", s.Project, s.Zone, s.ID)
}

func (s SubscriptionPath) Location() LocationPath {
	return LocationPath{s.Project, s.Zone}
}
