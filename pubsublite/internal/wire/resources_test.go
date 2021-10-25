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

import "testing"

func TestValidateRegion(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		input   string
		wantErr bool
	}{
		{
			desc:    "valid",
			input:   "europe-west1",
			wantErr: false,
		},
		{
			desc:    "invalid: insufficient dashes",
			input:   "europewest1",
			wantErr: true,
		},
		{
			desc:    "invalid: excess dashes",
			input:   "europe-west1-b",
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			err := ValidateRegion(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateRegion(%q) = %v, want err=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestLocationToRegion(t *testing.T) {
	for _, tc := range []struct {
		desc       string
		zone       string
		wantRegion string
		wantErr    bool
	}{
		{
			desc:       "valid zone",
			zone:       "europe-west1-d",
			wantRegion: "europe-west1",
			wantErr:    false,
		},
		{
			desc:       "valid region",
			zone:       "europe-west1",
			wantRegion: "europe-west1",
			wantErr:    false,
		},
		{
			desc:    "invalid: too many dashes",
			zone:    "europe-west1-b-d",
			wantErr: true,
		},
		{
			desc:    "invalid: no dashes",
			zone:    "europewest1",
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			gotRegion, gotErr := LocationToRegion(tc.zone)
			if gotRegion != tc.wantRegion || (gotErr != nil) != tc.wantErr {
				t.Errorf("LocationToRegion(%q) = (%v, %v), want (%v, err=%v)", tc.zone, gotRegion, gotErr, tc.wantRegion, tc.wantErr)
			}
		})
	}
}

func TestParseLocationPath(t *testing.T) {
	for _, tc := range []struct {
		desc     string
		input    string
		wantPath LocationPath
		wantErr  bool
	}{
		{
			desc:     "valid: zone path",
			input:    "projects/987654321/locations/europe-west1-d",
			wantPath: LocationPath{Project: "987654321", Location: "europe-west1-d"},
		},
		{
			desc:     "valid: region path",
			input:    "projects/987654321/locations/europe-west1",
			wantPath: LocationPath{Project: "987654321", Location: "europe-west1"},
		},
		{
			desc:    "invalid: zone",
			input:   "europe-west1-d",
			wantErr: true,
		},
		{
			desc:    "invalid: missing project",
			input:   "projects//locations/europe-west1-d",
			wantErr: true,
		},
		{
			desc:    "invalid: missing zone",
			input:   "projects/987654321/locations/",
			wantErr: true,
		},
		{
			desc:    "invalid: has prefix",
			input:   "prefix/projects/987654321/locations/europe-west1-d",
			wantErr: true,
		},
		{
			desc:    "invalid: has suffix",
			input:   "projects/987654321/locations/europe-west1-d/subscriptions/my-subs",
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			gotPath, gotErr := ParseLocationPath(tc.input)
			if gotPath != tc.wantPath || (gotErr != nil) != tc.wantErr {
				t.Errorf("ParseLocationPath(%q) = (%v, %v), want (%v, err=%v)", tc.input, gotPath, gotErr, tc.wantPath, tc.wantErr)
			}
		})
	}
}

func TestParseTopicPath(t *testing.T) {
	for _, tc := range []struct {
		desc     string
		input    string
		wantPath TopicPath
		wantErr  bool
	}{
		{
			desc:     "valid: topic in zone",
			input:    "projects/987654321/locations/europe-west1-d/topics/my-topic",
			wantPath: TopicPath{Project: "987654321", Location: "europe-west1-d", TopicID: "my-topic"},
		},
		{
			desc:     "valid: topic in region",
			input:    "projects/987654321/locations/europe-west1/topics/my-topic",
			wantPath: TopicPath{Project: "987654321", Location: "europe-west1", TopicID: "my-topic"},
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
			gotPath, gotErr := ParseTopicPath(tc.input)
			if gotPath != tc.wantPath || (gotErr != nil) != tc.wantErr {
				t.Errorf("ParseTopicPath(%q) = (%v, %v), want (%v, err=%v)", tc.input, gotPath, gotErr, tc.wantPath, tc.wantErr)
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
			desc:     "valid: subscription in zone",
			input:    "projects/987654321/locations/europe-west1-d/subscriptions/my-subs",
			wantPath: SubscriptionPath{Project: "987654321", Location: "europe-west1-d", SubscriptionID: "my-subs"},
		},
		{
			desc:     "valid: subscription in region",
			input:    "projects/987654321/locations/europe-west1/subscriptions/my-subs",
			wantPath: SubscriptionPath{Project: "987654321", Location: "europe-west1", SubscriptionID: "my-subs"},
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
			gotPath, gotErr := ParseSubscriptionPath(tc.input)
			if gotPath != tc.wantPath || (gotErr != nil) != tc.wantErr {
				t.Errorf("ParseSubscriptionPath(%q) = (%v, %v), want (%v, err=%v)", tc.input, gotPath, gotErr, tc.wantPath, tc.wantErr)
			}
		})
	}
}

func TestParseReservationPath(t *testing.T) {
	for _, tc := range []struct {
		desc     string
		input    string
		wantPath ReservationPath
		wantErr  bool
	}{
		{
			desc:     "valid: reservation path",
			input:    "projects/987654321/locations/europe-west1/reservations/my-reservation",
			wantPath: ReservationPath{Project: "987654321", Region: "europe-west1", ReservationID: "my-reservation"},
		},
		{
			desc:    "invalid: region only",
			input:   "europe-west1",
			wantErr: true,
		},
		{
			desc:    "invalid: topic path",
			input:   "projects/987654321/locations/europe-west1-d/topics/my-topic",
			wantErr: true,
		},
		{
			desc:    "invalid: missing project",
			input:   "projects//locations/europe-west1/reservations/my-reservation",
			wantErr: true,
		},
		{
			desc:    "invalid: missing region",
			input:   "projects/987654321/locations//reservations/my-reservation",
			wantErr: true,
		},
		{
			desc:    "invalid: missing reservation id",
			input:   "projects/987654321/locations/europe-west1/reservations/",
			wantErr: true,
		},
		{
			desc:    "invalid: has prefix",
			input:   "prefix/projects/987654321/locations/europe-west1/reservations/my-reservation",
			wantErr: true,
		},
		{
			desc:    "invalid: has suffix",
			input:   "projects/my-project/locations/us-west1/reservations/my-reservation/subresource/desc",
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			gotPath, gotErr := ParseReservationPath(tc.input)
			if gotPath != tc.wantPath || (gotErr != nil) != tc.wantErr {
				t.Errorf("ParseReservationPath(%q) = (%v, %v), want (%v, err=%v)", tc.input, gotPath, gotErr, tc.wantPath, tc.wantErr)
			}
		})
	}
}
