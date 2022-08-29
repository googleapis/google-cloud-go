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
	"testing"

	"cloud.google.com/go/internal/testutil"
	pslinternal "cloud.google.com/go/pubsublite/internal"
)

func TestParseVersion(t *testing.T) {
	for _, tc := range []struct {
		desc        string
		input       string
		wantVersion version
		wantOk      bool
	}{
		{
			desc:        "valid 3 components",
			input:       "1.2.2",
			wantVersion: version{Major: 1, Minor: 2},
			wantOk:      true,
		},
		{
			desc:        "valid 2 components",
			input:       "2.3",
			wantVersion: version{Major: 2, Minor: 3},
			wantOk:      true,
		},
		{
			desc:   "version empty",
			wantOk: false,
		},
		{
			desc:        "minor version invalid",
			input:       "1.a.2",
			wantVersion: version{Major: 1},
			wantOk:      false,
		},
		{
			desc:   "major version invalid",
			input:  "b.1.2",
			wantOk: false,
		},
		{
			desc:   "minor version missing",
			input:  "4",
			wantOk: false,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			if gotVersion, gotOk := parseVersion(tc.input); !testutil.Equal(gotVersion, tc.wantVersion) || gotOk != tc.wantOk {
				t.Errorf("parseVersion(): got (%v, %v), want (%v, %v)", gotVersion, gotOk, tc.wantVersion, tc.wantOk)
			}
		})
	}
}

func TestParseCurrentVersion(t *testing.T) {
	// Ensure the current version is parseable.
	if _, ok := parseVersion(pslinternal.Version); !ok {
		t.Errorf("Cannot parse pubsublite version: %q", pslinternal.Version)
	}
}
