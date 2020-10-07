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

import "testing"

func TestZoneToRegion(t *testing.T) {
	zone := CloudZone("europe-west1-d")
	got := zone.Region()
	want := CloudRegion("europe-west1")
	if got != want {
		t.Errorf("CloudZone(%q).Region() = %v, want %v", zone, got, want)
	}
}

func TestParseZone(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		wantZone CloudZone
		wantErr  bool
	}{
		{
			name:     "valid",
			input:    "us-central1-a",
			wantZone: CloudZone("us-central1-a"),
		},
		{
			name:    "invalid: insufficient dashes",
			input:   "us-central1",
			wantErr: true,
		},
		{
			name:    "invalid: excess dashes",
			input:   "us-central1-a-b",
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotZone, gotErr := ParseZone(tc.input)
			if gotZone != tc.wantZone || (gotErr != nil) != tc.wantErr {
				t.Errorf("ParseZone(%q) = (%v, %v), want (%v, err=%v)", tc.input, gotZone, gotErr, tc.wantZone, tc.wantErr)
			}
		})
	}
}

func TestParseTopicPath(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		wantPath TopicPath
		wantErr  bool
	}{
		{
			name:     "valid: topic path",
			input:    "projects/987654321/locations/europe-west1-d/topics/my-topic",
			wantPath: TopicPath{Project("987654321"), CloudZone("europe-west1-d"), TopicID("my-topic")},
		},
		{
			name:     "valid: sub-resource path",
			input:    "projects/my-project/locations/us-west1-b/topics/my-topic/subresource/name",
			wantPath: TopicPath{Project("my-project"), CloudZone("us-west1-b"), TopicID("my-topic")},
		},
		{
			name:    "invalid: zone",
			input:   "europe-west1-d",
			wantErr: true,
		},
		{
			name:    "invalid: subscription path",
			input:   "projects/987654321/locations/europe-west1-d/subscriptions/my-subs",
			wantErr: true,
		},
		{
			name:    "invalid: invalid zone component",
			input:   "projects/987654321/locations/not_a_zone/topics/my-topic",
			wantErr: true,
		},
		{
			name:    "invalid: missing topic id",
			input:   "projects/987654321/locations/europe-west1-d/topics/",
			wantErr: true,
		},
		{
			name:    "invalid: missing project",
			input:   "projects//locations/europe-west1-d/topics/my-topic",
			wantErr: true,
		},
		{
			name:    "invalid: prefix",
			input:   "prefix/projects/987654321/locations/europe-west1-d/topics/my-topic",
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotPath, gotErr := ParseTopicPath(tc.input)
			if gotPath != tc.wantPath || (gotErr != nil) != tc.wantErr {
				t.Errorf("ParseTopicPath(%q) = (%v, %v), want (%v, err=%v)", tc.input, gotPath, gotErr, tc.wantPath, tc.wantErr)
			}
		})
	}
}

func TestParseSubscriptionPath(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		wantPath SubscriptionPath
		wantErr  bool
	}{
		{
			name:     "valid: subscription path",
			input:    "projects/987654321/locations/europe-west1-d/subscriptions/my-subs",
			wantPath: SubscriptionPath{Project("987654321"), CloudZone("europe-west1-d"), SubscriptionID("my-subs")},
		},
		{
			name:     "valid: sub-resource path",
			input:    "projects/my-project/locations/us-west1-b/subscriptions/my-subs/subresource/name",
			wantPath: SubscriptionPath{Project("my-project"), CloudZone("us-west1-b"), SubscriptionID("my-subs")},
		},
		{
			name:    "invalid: zone",
			input:   "europe-west1-d",
			wantErr: true,
		},
		{
			name:    "invalid: topic path",
			input:   "projects/987654321/locations/europe-west1-d/topics/my-topic",
			wantErr: true,
		},
		{
			name:    "invalid: invalid zone component",
			input:   "projects/987654321/locations/not_a_zone/subscriptions/my-subs",
			wantErr: true,
		},
		{
			name:    "invalid: missing subscription id",
			input:   "projects/987654321/locations/europe-west1-d/subscriptions/",
			wantErr: true,
		},
		{
			name:    "invalid: missing project",
			input:   "projects//locations/europe-west1-d/subscriptions/my-subs",
			wantErr: true,
		},
		{
			name:    "invalid: prefix",
			input:   "prefix/projects/987654321/locations/europe-west1-d/subscriptions/my-subs",
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotPath, gotErr := ParseSubscriptionPath(tc.input)
			if gotPath != tc.wantPath || (gotErr != nil) != tc.wantErr {
				t.Errorf("ParseSubscriptionPath(%q) = (%v, %v), want (%v, err=%v)", tc.input, gotPath, gotErr, tc.wantPath, tc.wantErr)
			}
		})
	}
}
