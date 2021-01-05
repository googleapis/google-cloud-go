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

package common

import (
	"testing"

	"cloud.google.com/go/internal/testutil"
)

func TestPublishMetadataStringEncoding(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		input   string
		want    *PublishMetadata
		wantErr bool
	}{
		{
			desc:  "valid: zero",
			input: "0:0",
			want:  &PublishMetadata{Partition: 0, Offset: 0},
		},
		{
			desc:  "valid: non-zero",
			input: "3:1234",
			want:  &PublishMetadata{Partition: 3, Offset: 1234},
		},
		{
			desc:    "invalid: number",
			input:   "1234",
			wantErr: true,
		},
		{
			desc:    "invalid: partition",
			input:   "p:1234",
			wantErr: true,
		},
		{
			desc:    "invalid: offset",
			input:   "10:9offset",
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			got, gotErr := ParsePublishMetadata(tc.input)
			if !testutil.Equal(got, tc.want) || (gotErr != nil) != tc.wantErr {
				t.Errorf("ParsePublishMetadata(%q): got (%v, %v), want (%v, err=%v)", tc.input, got, gotErr, tc.want, tc.wantErr)
			}

			if tc.want != nil {
				if got := tc.want.String(); got != tc.input {
					t.Errorf("PublishMetadata(%v).String(): got %q, want: %q", tc.want, got, tc.input)
				}
			}
		})
	}
}
