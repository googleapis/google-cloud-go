// Copyright 2017 Google LLC
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

import "testing"

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
	}
}

func Test_filterByAttrs(t *testing.T) {
	tt := []struct {
		filter string
		want   []int
	}{
		{
			filter: "attributes.name = \"com\"",
			want:   []int{1, 4, 9},
		},

		{
			filter: "attributes.name != \"com\"",
			want:   []int{2, 3, 5, 6, 7, 8},
		},
		{
			filter: "hasPrefix(attributes.name, \"co\")",
			want:   []int{1, 4, 7, 9},
		},
		{
			filter: "attributes:name",
			want:   []int{1, 2, 3, 4, 5, 6, 7, 9},
		},
		{
			filter: "NOT attributes:name",
			want:   []int{8},
		},
		{
			filter: "(NOT attributes:name) OR attributes.name = \"co\"",
			want:   []int{7, 8},
		},
		{
			filter: "NOT (attributes:name OR attributes.lang = \"jp\")",
			want:   []int{},
		},
		{
			filter: "attributes.name = \"com\" AND -attributes:\"lang\"",
			want:   []int{9},
		},
	}
	for _, tc := range tt {
		t.Run(tc.filter, func(t *testing.T) {
			filter, err := parseFilter(tc.filter)
			if err != nil {
				t.Error(err)
			}
			attrs := getAttrs()
			filterByAttrs(attrs, &filter, func(msgAttrs messageAttrs) messageAttrs { return msgAttrs })
			if !checkKeys(attrs, tc.want) {
				t.Errorf("filterByAttrs(%v, %v) = %v, want keys %v", attrs, tc.filter, attrs, tc.want)
			}
		})
	}
}
