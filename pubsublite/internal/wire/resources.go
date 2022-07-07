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

// LocationToRegion returns the region that the given location is in.
func LocationToRegion(location string) (string, error) {
	parts := strings.Split(location, "-")
	if len(parts) == 3 {
		return strings.Join(parts[0:len(parts)-1], "-"), nil
	}
	if len(parts) == 2 {
		return location, nil
	}
	return "", fmt.Errorf("pubsublite: location %q is not a valid zone or region", location)
}

// LocationPath stores a path consisting of a project and zone/region.
type LocationPath struct {
	// A Google Cloud project. The project ID (e.g. "my-project") or the project
	// number (e.g. "987654321") can be provided.
	Project string

	// A Google Cloud zone (e.g. "us-central1-a") or region (e.g. "us-central1").
	Location string
}

func (l LocationPath) String() string {
	return fmt.Sprintf("projects/%s/locations/%s", l.Project, l.Location)
}

var locPathRE = regexp.MustCompile(`^projects/([^/]+)/locations/([^/]+)$`)

// ParseLocationPath parses a project/location path.
func ParseLocationPath(input string) (LocationPath, error) {
	parts := locPathRE.FindStringSubmatch(input)
	if len(parts) < 3 {
		return LocationPath{}, fmt.Errorf("pubsublite: invalid location path %q. valid format is %q",
			input, "projects/PROJECT_ID/locations/LOCATION")
	}
	return LocationPath{Project: parts[1], Location: parts[2]}, nil
}

// TopicPath stores the full path of a Pub/Sub Lite topic.
type TopicPath struct {
	// A Google Cloud project. The project ID (e.g. "my-project") or the project
	// number (e.g. "987654321") can be provided.
	Project string

	// A Google Cloud zone (e.g. "us-central1-a") or region (e.g. "us-central1").
	Location string

	// The ID of the Pub/Sub Lite topic, for example "my-topic-name".
	TopicID string
}

func (t TopicPath) String() string {
	return fmt.Sprintf("projects/%s/locations/%s/topics/%s", t.Project, t.Location, t.TopicID)
}

// LocationPath returns the topic's location path.
func (t TopicPath) LocationPath() LocationPath {
	return LocationPath{Project: t.Project, Location: t.Location}
}

var topicPathRE = regexp.MustCompile(`^projects/([^/]+)/locations/([^/]+)/topics/([^/]+)$`)

// ParseTopicPath parses the full path of a Pub/Sub Lite topic.
func ParseTopicPath(input string) (TopicPath, error) {
	parts := topicPathRE.FindStringSubmatch(input)
	if len(parts) < 4 {
		return TopicPath{}, fmt.Errorf("pubsublite: invalid topic path %q. valid format is %q",
			input, "projects/PROJECT_ID/locations/ZONE_OR_REGION/topics/TOPIC_ID")
	}
	return TopicPath{Project: parts[1], Location: parts[2], TopicID: parts[3]}, nil
}

// SubscriptionPath stores the full path of a Pub/Sub Lite subscription.
type SubscriptionPath struct {
	// A Google Cloud project. The project ID (e.g. "my-project") or the project
	// number (e.g. "987654321") can be provided.
	Project string

	// A Google Cloud zone (e.g. "us-central1-a") or region (e.g. "us-central1").
	Location string

	// The ID of the Pub/Sub Lite subscription, for example
	// "my-subscription-name".
	SubscriptionID string
}

func (s SubscriptionPath) String() string {
	return fmt.Sprintf("projects/%s/locations/%s/subscriptions/%s", s.Project, s.Location, s.SubscriptionID)
}

// LocationPath returns the subscription's location path.
func (s SubscriptionPath) LocationPath() LocationPath {
	return LocationPath{Project: s.Project, Location: s.Location}
}

var subsPathRE = regexp.MustCompile(`^projects/([^/]+)/locations/([^/]+)/subscriptions/([^/]+)$`)

// ParseSubscriptionPath parses the full path of a Pub/Sub Lite subscription.
func ParseSubscriptionPath(input string) (SubscriptionPath, error) {
	parts := subsPathRE.FindStringSubmatch(input)
	if len(parts) < 4 {
		return SubscriptionPath{}, fmt.Errorf("pubsublite: invalid subscription path %q. valid format is %q",
			input, "projects/PROJECT_ID/locations/ZONE_OR_REGION/subscriptions/SUBSCRIPTION_ID")
	}
	return SubscriptionPath{Project: parts[1], Location: parts[2], SubscriptionID: parts[3]}, nil
}

// ReservationPath stores the full path of a Pub/Sub Lite reservation.
type ReservationPath struct {
	// A Google Cloud project. The project ID (e.g. "my-project") or the project
	// number (e.g. "987654321") can be provided.
	Project string

	// A Google Cloud region. An example region is "us-central1".
	Region string

	// The ID of the Pub/Sub Lite reservation, for example "my-reservation-name".
	ReservationID string
}

func (r ReservationPath) String() string {
	return fmt.Sprintf("projects/%s/locations/%s/reservations/%s", r.Project, r.Region, r.ReservationID)
}

// Location returns the reservation's location path.
func (r ReservationPath) Location() LocationPath {
	return LocationPath{Project: r.Project, Location: r.Region}
}

var reservationPathRE = regexp.MustCompile(`^projects/([^/]+)/locations/([^/]+)/reservations/([^/]+)$`)

// ParseReservationPath parses the full path of a Pub/Sub Lite reservation.
func ParseReservationPath(input string) (ReservationPath, error) {
	parts := reservationPathRE.FindStringSubmatch(input)
	if len(parts) < 4 {
		return ReservationPath{}, fmt.Errorf("pubsublite: invalid reservation path %q. valid format is %q",
			input, "projects/PROJECT_ID/locations/REGION/reservations/RESERVATION_ID")
	}
	return ReservationPath{Project: parts[1], Region: parts[2], ReservationID: parts[3]}, nil
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

// MessageMetadata holds properties of a message published to the Pub/Sub Lite
// service.
//
// NOTE: This is duplicated in the pscompat package in order to generate nicer
// docs and should be kept consistent.
type MessageMetadata struct {
	// The topic partition the message was published to.
	Partition int

	// The offset the message was assigned.
	Offset int64
}

func (m *MessageMetadata) String() string {
	return fmt.Sprintf("%d:%d", m.Partition, m.Offset)
}
