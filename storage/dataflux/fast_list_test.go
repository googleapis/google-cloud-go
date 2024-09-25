// Copyright 2024 Google LLC
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

package dataflux

import (
	"runtime"
	"testing"

	"cloud.google.com/go/storage"
)

func TestUpdateStartEndOffset(t *testing.T) {
	testcase := []struct {
		desc      string
		start     string
		end       string
		prefix    string
		wantStart string
		wantEnd   string
	}{
		// List all objects with the given prefix.
		{
			desc:      "start and end are empty",
			start:     "",
			end:       "",
			prefix:    "pre",
			wantStart: "",
			wantEnd:   "",
		},
		{
			desc:      "start is longer than prefix",
			start:     "abcqre",
			end:       "",
			prefix:    "pre",
			wantStart: "",
			wantEnd:   "",
		},
		{
			desc:      "start value same as prefix",
			start:     "pre",
			end:       "",
			prefix:    "pre",
			wantStart: "",
			wantEnd:   "",
		},
		{
			desc:      "lexicographically start comes before prefix and end after prefix",
			start:     "abc",
			end:       "xyz",
			prefix:    "pre",
			wantStart: "",
			wantEnd:   "",
		},
		// List objects within the given prefix.
		{
			desc:      "start value contains prefix",
			start:     "pre_a",
			end:       "",
			prefix:    "pre",
			wantStart: "_a",
			wantEnd:   "",
		},
		{
			desc:      "end value contains prefix",
			start:     "",
			end:       "pre_x",
			prefix:    "pre",
			wantStart: "",
			wantEnd:   "_x",
		},
		// With empty prefix, start and end will not be affected.
		{
			desc:      "prefix is empty",
			start:     "abc",
			end:       "xyz",
			prefix:    "",
			wantStart: "abc",
			wantEnd:   "xyz",
		},
		{
			desc:      "start is lexicographically higher than end",
			start:     "xyz",
			end:       "abc",
			prefix:    "",
			wantStart: "xyz",
			wantEnd:   "abc",
		},
		// Cases where no objects will be listed when prefix is given.
		{
			desc:      "end is same as prefix",
			start:     "",
			end:       "pre",
			prefix:    "pre",
			wantStart: "pre",
			wantEnd:   "pre",
		},
		{
			desc:      "start is lexicographically higher than end with prefix",
			start:     "xyz",
			end:       "abc",
			prefix:    "pre",
			wantStart: "xyz",
			wantEnd:   "xyz",
		},
		{
			desc:      "start is lexicographically higher than prefix",
			start:     "xyz",
			end:       "",
			prefix:    "pre",
			wantStart: "xyz",
			wantEnd:   "xyz",
		},
	}

	for _, tc := range testcase {
		t.Run(tc.desc, func(t *testing.T) {
			gotStart, gotEnd := updateStartEndOffset(tc.start, tc.end, tc.prefix)
			if gotStart != tc.wantStart || gotEnd != tc.wantEnd {
				t.Errorf("updateStartEndOffset(%q, %q, %q) got = (%q, %q), want = (%q, %q)", tc.start, tc.end, tc.prefix, gotStart, gotEnd, tc.wantStart, tc.wantEnd)
			}
		})
	}
}

func TestNewLister(t *testing.T) {
	gcs := &storage.Client{}
	bucketName := "test-bucket"
	in := ListerInput{
		BucketName:  bucketName,
		Parallelism: 1,
		BatchSize:   0,
	}
	testcase := []struct {
		desc            string
		query           storage.Query
		parallelism     int
		wantStart       string
		wantEnd         string
		wantParallelism int
	}{
		{
			desc:            "start and end are empty",
			query:           storage.Query{Prefix: "pre"},
			parallelism:     1,
			wantStart:       "",
			wantEnd:         "",
			wantParallelism: 1,
		},
		{
			desc:            "start is longer than prefix",
			query:           storage.Query{Prefix: "pre", StartOffset: "pre_a"},
			parallelism:     1,
			wantStart:       "_a",
			wantEnd:         "",
			wantParallelism: 1,
		},
		{
			desc:            "start and end are empty",
			query:           storage.Query{Prefix: "pre"},
			parallelism:     0,
			wantStart:       "",
			wantEnd:         "",
			wantParallelism: 10 * runtime.NumCPU(),
		},
	}

	for _, tc := range testcase {
		t.Run(tc.desc, func(t *testing.T) {
			in.Query = tc.query
			in.Parallelism = tc.parallelism
			df := NewLister(gcs, &in)
			defer df.Close()
			if len(df.ranges) != 1 {
				t.Errorf("NewLister(%v, %v %v, %v) got len of ranges = %v, want = %v", bucketName, 1, 0, tc.query, len(df.ranges), 1)
			}
			ranges := <-df.ranges
			if df.method != open || df.pageToken != "" || ranges.startRange != tc.wantStart || ranges.endRange != tc.wantEnd || df.parallelism != tc.wantParallelism {
				t.Errorf("NewLister(%q, %d, %d, %v) got = (method: %v, token: %q,  start: %q, end: %q, parallelism: %d), want = (method: %v, token: %q,  start: %q, end: %q, parallelism: %d)", bucketName, 1, 0, tc.query, df.method, df.pageToken, ranges.startRange, ranges.endRange, df.parallelism, open, "", tc.wantStart, tc.wantEnd, tc.wantParallelism)
			}

		})
	}
}
