// Copyright 2021 Google LLC
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
// limitations under the License.

package managedwriter

import (
	"testing"
)

func TestTableParentFromStreamName(t *testing.T) {
	testCases := []struct {
		in   string
		want string
	}{
		{
			"bad",
			"bad",
		},
		{
			"projects/foo/datasets/bar/tables/baz",
			"projects/foo/datasets/bar/tables/baz",
		},
		{
			"projects/foo/datasets/bar/tables/baz/zip/zam/zoomie",
			"projects/foo/datasets/bar/tables/baz",
		},
		{
			"projects/foo/datasets/bar/tables/baz/_default",
			"projects/foo/datasets/bar/tables/baz",
		},
	}

	for _, tc := range testCases {
		got := TableParentFromStreamName(tc.in)
		if got != tc.want {
			t.Errorf("mismatch on %s: got %s want %s", tc.in, got, tc.want)
		}
	}
}
