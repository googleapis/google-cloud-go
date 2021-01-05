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

func TestValidateZone(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		input   string
		wantErr bool
	}{
		{
			desc:    "valid",
			input:   "us-central1-a",
			wantErr: false,
		},
		{
			desc:    "invalid: insufficient dashes",
			input:   "us-central1",
			wantErr: true,
		},
		{
			desc:    "invalid: excess dashes",
			input:   "us-central1-a-b",
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			err := ValidateZone(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateZone(%q) = %v, want err=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

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
