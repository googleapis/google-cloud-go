// Copyright 2022 Google LLC
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

package reader

import (
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestReadOptions(t *testing.T) {

	testCases := []struct {
		desc    string
		options []ReadOption
		want    *Reader
	}{
		{
			desc:    "WithMaxStreamCount",
			options: []ReadOption{WithMaxStreamCount(1)},
			want: func() *Reader {
				ms := &Reader{
					settings: defaultSettings(),
				}
				ms.settings.MaxStreamCount = 1
				return ms
			}(),
		},
	}

	for _, tc := range testCases {
		got := &Reader{
			settings: defaultSettings(),
		}
		for _, o := range tc.options {
			o(got)
		}

		if diff := cmp.Diff(got, tc.want,
			cmp.AllowUnexported(Reader{}, settings{}),
			cmp.AllowUnexported(sync.Mutex{})); diff != "" {
			t.Errorf("diff in case (%s):\n%v", tc.desc, diff)
		}
	}
}
