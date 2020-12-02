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

func TestParseTopicPath(t *testing.T) {
	for _, tc := range []struct {
		desc     string
		input    string
		wantPath TopicPath
		wantErr  bool
	}{
		{
			desc:     "valid: topic path",
			input:    "projects/987654321/locations/europe-west1-d/topics/my-topic",
			wantPath: TopicPath{Project: "987654321", Zone: "europe-west1-d", TopicID: "my-topic"},
		},
		{
			desc:    "invalid: zone",
			input:   "europe-west1-d",
			wantErr: true,
		},
		{
			desc:    "invalid: subscription path",
			input:   "projects/987654321/locations/europe-west1-d/subscriptions/my-subs",
			wantErr: true,
		},
		{
			desc:    "invalid: missing project",
			input:   "projects//locations/europe-west1-d/topics/my-topic",
			wantErr: true,
		},
		{
			desc:    "invalid: missing zone",
			input:   "projects/987654321/locations//topics/my-topic",
			wantErr: true,
		},
		{
			desc:    "invalid: missing topic id",
			input:   "projects/987654321/locations/europe-west1-d/topics/",
			wantErr: true,
		},
		{
			desc:    "invalid: has prefix",
			input:   "prefix/projects/987654321/locations/europe-west1-d/topics/my-topic",
			wantErr: true,
		},
		{
			desc:    "invalid: has suffix",
			input:   "projects/my-project/locations/us-west1-b/topics/my-topic/subresource/desc",
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			gotPath, gotErr := parseTopicPath(tc.input)
			if gotPath != tc.wantPath || (gotErr != nil) != tc.wantErr {
				t.Errorf("parseTopicPath(%q) = (%v, %v), want (%v, err=%v)", tc.input, gotPath, gotErr, tc.wantPath, tc.wantErr)
			}
		})
	}
}

func TestParseSubscriptionPath(t *testing.T) {
	for _, tc := range []struct {
		desc     string
		input    string
		wantPath SubscriptionPath
		wantErr  bool
	}{
		{
			desc:     "valid: subscription path",
			input:    "projects/987654321/locations/europe-west1-d/subscriptions/my-subs",
			wantPath: SubscriptionPath{Project: "987654321", Zone: "europe-west1-d", SubscriptionID: "my-subs"},
		},
		{
			desc:    "invalid: zone",
			input:   "europe-west1-d",
			wantErr: true,
		},
		{
			desc:    "invalid: topic path",
			input:   "projects/987654321/locations/europe-west1-d/topics/my-topic",
			wantErr: true,
		},
		{
			desc:    "invalid: missing project",
			input:   "projects//locations/europe-west1-d/subscriptions/my-subs",
			wantErr: true,
		},
		{
			desc:    "invalid: missing zone",
			input:   "projects/987654321/locations//subscriptions/my-subs",
			wantErr: true,
		},
		{
			desc:    "invalid: missing subscription id",
			input:   "projects/987654321/locations/europe-west1-d/subscriptions/",
			wantErr: true,
		},
		{
			desc:    "invalid: has prefix",
			input:   "prefix/projects/987654321/locations/europe-west1-d/subscriptions/my-subs",
			wantErr: true,
		},
		{
			desc:    "invalid: has suffix",
			input:   "projects/my-project/locations/us-west1-b/subscriptions/my-subs/subresource/desc",
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			gotPath, gotErr := parseSubscriptionPath(tc.input)
			if gotPath != tc.wantPath || (gotErr != nil) != tc.wantErr {
				t.Errorf("parseSubscriptionPath(%q) = (%v, %v), want (%v, err=%v)", tc.input, gotPath, gotErr, tc.wantPath, tc.wantErr)
			}
		})
	}
}

func TestZoneToRegion(t *testing.T) {
	for _, tc := range []struct {
		desc       string
		zone       string
		wantRegion string
		wantErr    bool
	}{
		{
			desc:       "valid",
			zone:       "europe-west1-d",
			wantRegion: "europe-west1",
			wantErr:    false,
		},
		{
			desc:    "invalid: insufficient dashes",
			zone:    "europe-west1",
			wantErr: true,
		},
		{
			desc:    "invalid: no dashes",
			zone:    "europewest1",
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			gotRegion, gotErr := ZoneToRegion(tc.zone)
			if gotRegion != tc.wantRegion || (gotErr != nil) != tc.wantErr {
				t.Errorf("ZoneToRegion(%q) = (%v, %v), want (%v, err=%v)", tc.zone, gotRegion, gotErr, tc.wantRegion, tc.wantErr)
			}
		})
	}
}
