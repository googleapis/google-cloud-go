// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pstest

import (
	"testing"

	filter "cloud.google.com/go/pubsub/v2/pstest/internal"
)

type messageAttrs map[string]string

// checkKeys returns true if the keys of a and b are equal.
func checkKeys(a map[int]messageAttrs, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for _, k := range b {
		if _, ok := a[k]; !ok {
			return false
		}
	}
	return true
}

// getAttrs returns a map of message attributes.
// Used for testing.
func getAttrs() map[int]messageAttrs {
	return map[int]messageAttrs{
		1: {
			"lang":     "en",
			"name":     "com",
			"timezone": "UTC",
		},
		2: {
			"lang":     "en",
			"name":     "net",
			"timezone": "UTC",
		},
		3: {
			"lang":     "en",
			"name":     "org",
			"timezone": "UTC",
		},
		4: {
			"lang":     "cn",
			"name":     "com",
			"timezone": "UTC",
		},
		5: {
			"lang":     "cn",
			"name":     "net",
			"timezone": "UTC",
		},
		6: {
			"lang":     "cn",
			"name":     "org",
			"timezone": "UTC",
		},
		7: {
			"lang":     "jp",
			"name":     "co",
			"timezone": "UTC",
		},
		8: {
			"lang":     "jp",
			"timezone": "UTC",
		},
		9: {
			"name":     "com",
			"timezone": "UTC",
		},
		10: {
			"lang":               "jp",
			"\u307F\u3093\u306A": "dummy1",
		},
		11: {
			"\u307F\u3093\u306A": "dummy2",
		},
		12: {
			"name":               "com",
			"\u307F\u3093\u306A": "dummy3",
		},
		13: {
			"name": "contains\"quote",
		},
	}
}

func Test_filterByAttrs(t *testing.T) {
	tt := []struct {
		filter string
		want   []int
	}{
		{
			filter: "attributes.name = \"com\"",
			want:   []int{1, 4, 9, 12},
		},

		{
			filter: "attributes.name != \"com\"",
			want:   []int{2, 3, 5, 6, 7, 13},
		},
		{
			filter: "attributes.name = \"contains\\\"quote\"",
			want:   []int{13},
		},
		{
			filter: "hasPrefix(attributes.name, \"co\")",
			want:   []int{1, 4, 7, 9, 12, 13},
		},
		{
			filter: "attributes:name",
			want:   []int{1, 2, 3, 4, 5, 6, 7, 9, 12, 13},
		},
		{
			filter: "NOT attributes:name",
			want:   []int{8, 10, 11},
		},
		{
			filter: "(NOT attributes:name) OR attributes.name = \"co\"",
			want:   []int{7, 8, 10, 11},
		},
		{
			filter: "NOT (attributes:name OR attributes.lang = \"jp\")",
			want:   []int{11},
		},
		{
			filter: "attributes.name = \"com\" AND -attributes:\"lang\"",
			want:   []int{9, 12},
		},
		{
			filter: "attributes:\"\u307F\u3093\u306A\"",
			want:   []int{10, 11, 12},
		},
	}
	for _, tc := range tt {
		t.Run(tc.filter, func(t *testing.T) {
			f, err := parseFilter(tc.filter)
			if err != nil {
				t.Error(err)
			}
			attrs := getAttrs()
			for id, msgAttrs := range attrs {
				if !filter.Evaluate(f, msgAttrs) {
					delete(attrs, id)
				}
			}
			if !checkKeys(attrs, tc.want) {
				t.Errorf("filter(%v) = %v, want keys %v", tc.filter, attrs, tc.want)
			}
		})
	}
}

func Test_parseFilter_Errors(t *testing.T) {
	tests := []string{
		"attributes.name = $",        // invalid char
		"attributes.name = ",         // unexpected EOF
		"(attributes.name = \"com\"", // missing closing paren
		"hasPrefix(attributes.name)", // missing args
	}

	for _, tc := range tests {
		t.Run(tc, func(t *testing.T) {
			_, err := parseFilter(tc)
			if err == nil {
				t.Errorf("parseFilter(%q) expected error, got nil", tc)
			}
		})
	}
}
