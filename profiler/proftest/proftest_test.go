// Copyright 2020 Google LLC
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

package proftest

import (
	"regexp"
	"testing"
	"time"
)

func TestParseBenchmarkNumber(t *testing.T) {
	benchNumRE, err := regexp.Compile("benchmark (\\d+):")
	if err != nil {
		t.Fatalf("failed to get benchmark regexp: %v", err)
	}
	numBenchmarks := 150

	for _, tc := range []struct {
		desc         string
		line         string
		wantBenchNum int
		wantErr      bool
	}{
		{
			desc:         "zero parsed successfully as benchmark number",
			line:         "benchmark 0: 2020/05/15 23:37:56 start uploading profile",
			wantBenchNum: 0,
		},
		{
			desc:         "benchmark number for empty line can be parsed",
			line:         "benchmark 5:",
			wantBenchNum: 5,
		},
		{
			desc:         "multi-digit benchmark number can be parsed",
			line:         "benchmark 149: line",
			wantBenchNum: 149,
		},
		{
			desc:         "benchmark number can be parsed when it is not at the start of the line",
			line:         "Mon May 18 00:00:00 UTC 2020: benchmark 5: line",
			wantBenchNum: 5,
		},
		{
			desc:    "an error is returned when benchmark number is outside the expected range of benchmark numbers",
			line:    "benchmark 150: line",
			wantErr: true,
		},
		{
			desc:    "an error is returned when benchmark number is not a number",
			line:    "benchmark abc: 2020/05/15 23:40:46 creating a new profile via profiler service",
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			benchNum, err := parseBenchmarkNumber(tc.line, numBenchmarks, benchNumRE)
			if (err != nil) != tc.wantErr {
				t.Errorf("got err = %v; want (err != nil) = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if benchNum != tc.wantBenchNum {
				t.Errorf("got benchmark number = %v, want %v", benchNum, tc.wantBenchNum)
			}
		})
	}
}

func TestParseLogTime(t *testing.T) {
	for _, tc := range []struct {
		desc     string
		line     string
		wantTime time.Time
		wantErr  bool
	}{
		{
			desc:     "a valid timestamp is parsed correctly",
			line:     "Fri May 15 23:39:53 UTC 2020: benchmark 31: creating a new profile via profiler service",
			wantTime: time.Date(2020, 5, 15, 23, 39, 53, 0, time.UTC),
		},
		{
			desc:    "an error is returned when the timestamp is invalid",
			line:    "Fri May 15 26:39:53 UTC 2020: benchmark 31: creating a new profile via profiler service",
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			logTime, err := parseLogTime(tc.line)
			if (err != nil) != tc.wantErr {
				t.Errorf("got err = %v; want (err != nil) = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if !logTime.Equal(tc.wantTime) {
				t.Errorf("got log time = %v, want %v", logTime, tc.wantTime)
			}
		})
	}
}

func TestParseBackoffDuration(t *testing.T) {
	for _, tc := range []struct {
		desc           string
		line           string
		wantBackoffDur time.Duration
		wantErr        bool
	}{
		{
			desc:           "a valid backoff duration is parsed correctly",
			line:           "Fri May 15 22:05:01 UTC 2020: benchmark 0: failed to create profile, will retry: rpc error: code = Aborted desc = generic::aborted: action throttled, backoff for 32m0s",
			wantBackoffDur: 32 * time.Minute,
		},
		{
			desc:           "a floating-point backoff duration is parsed correctly",
			line:           "Fri May 15 22:05:01 UTC 2020: benchmark 0: failed to create profile, will retry: rpc error: code = Aborted desc = generic::aborted: action throttled, backoff for 2000.000s",
			wantBackoffDur: 2000 * time.Second,
		},
		{
			desc:    "an error is returned when the backoff duration is invalid",
			line:    "Fri May 15 22:05:01 UTC 2020: benchmark 0: failed to create profile, will retry: rpc error: code = Aborted desc = generic::aborted: action throttled, backoff for 32..0.s",
			wantErr: true,
		},
		{
			desc:           "a backoff duration specifying hours, minutes, seconds, milliseconds and microseconds is parsed correctly.",
			line:           "Fri May 15 22:05:01 UTC 2020: benchmark 0: failed to create profile, will retry: rpc error: code = Aborted desc = generic::aborted: action throttled, backoff for 1h1m1s1ms1us",
			wantBackoffDur: time.Hour + time.Minute + time.Second + time.Millisecond + time.Microsecond,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			backoffDur, err := parseBackoffDuration(tc.line)
			if (err != nil) != tc.wantErr {
				t.Errorf("got err = %v; want (err != nil) = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if backoffDur != tc.wantBackoffDur {
				t.Errorf("backoff duration: got %v, want %v", backoffDur, tc.wantBackoffDur)
			}
		})
	}
}

func TestCheckSerialOutputForBackoffs(t *testing.T) {
	for _, tc := range []struct {
		desc                   string
		logs                   string
		numBenchmarks          int
		serverBackoffSubstring string
		createProfileSubstring string
		benchmarkNumPrefix     string
		wantErr                bool
	}{
		{
			desc: "no error when one benchmark running and the next profile is created after backoff interval",
			logs: `
			Fri May 15 22:05:00 UTC 2020: benchmark 0: creating a new profile via profiler service
			Fri May 15 22:05:01 UTC 2020: benchmark 0: failed to create profile, will retry: rpc error: code = Aborted desc = generic::aborted: action throttled, backoff for 32m0s
			Fri May 15 22:37:01 UTC 2020: benchmark 0: creating a new profile via profiler service
			`,
			numBenchmarks:          1,
			serverBackoffSubstring: "action throttled, backoff for",
			createProfileSubstring: "creating a new profile via profiler service",
			benchmarkNumPrefix:     "benchmark",
		},
		{
			desc: "no error when one benchmark running and the next profile is created 1 minute after backoff interval has elapsed",
			logs: `
			Fri May 15 22:05:00 UTC 2020: benchmark 0: creating a new profile via profiler service
			Fri May 15 22:05:01 UTC 2020: benchmark 0: failed to create profile, will retry: rpc error: code = Aborted desc = generic::aborted: action throttled, backoff for 50m0s
			Fri May 15 22:56:01 UTC 2020: benchmark 0: creating a new profile via profiler service
			`,
			numBenchmarks:          1,
			serverBackoffSubstring: "action throttled, backoff for",
			createProfileSubstring: "creating a new profile via profiler service",
			benchmarkNumPrefix:     "benchmark",
		},
		{
			desc: "no error when one benchmark running and the next profile is created 1 minute before backoff interval has elapsed",
			logs: `
			Fri May 15 22:05:00 UTC 2020: benchmark 0: creating a new profile via profiler service
			Fri May 15 22:05:01 UTC 2020: benchmark 0: failed to create profile, will retry: rpc error: code = Aborted desc = generic::aborted: action throttled, backoff for 45m0s
			Fri May 15 22:49:01 UTC 2020: benchmark 0: creating a new profile via profiler service
			`,
			numBenchmarks:          1,
			serverBackoffSubstring: "action throttled, backoff for",
			createProfileSubstring: "creating a new profile via profiler service",
			benchmarkNumPrefix:     "benchmark",
		},
		{
			desc: "error when one benchmark running and the next profile is created more than 1 minute before backoff interval has elapsed",
			logs: `
			Fri May 15 22:05:00 UTC 2020: benchmark 0: creating a new profile via profiler service
			Fri May 15 22:05:01 UTC 2020: benchmark 0: failed to create profile, will retry: rpc error: code = Aborted desc = generic::aborted: action throttled, backoff for 32m0s
			Fri May 15 22:36:00 UTC 2020: benchmark 0: creating a new profile via profiler service
			`,
			numBenchmarks:          1,
			serverBackoffSubstring: "action throttled, backoff for",
			createProfileSubstring: "creating a new profile via profiler service",
			benchmarkNumPrefix:     "benchmark",
			wantErr:                true,
		},
		{
			desc: "error when one benchmark running and the next profile is created more than 1 minute after backoff interval has elapsed",
			logs: `
			Fri May 15 22:05:00 UTC 2020: benchmark 0: creating a new profile via profiler service
			Fri May 15 22:05:01 UTC 2020: benchmark 0: failed to create profile, will retry: rpc error: code = Aborted desc = generic::aborted: action throttled, backoff for 32m0s
			Fri May 15 22:38:02 UTC 2020: benchmark 0: creating a new profile via profiler service
			`,
			numBenchmarks:          1,
			serverBackoffSubstring: "action throttled, backoff for",
			createProfileSubstring: "creating a new profile via profiler service",
			benchmarkNumPrefix:     "benchmark",
			wantErr:                true,
		},
		{
			desc: "error when there are no log entries indicating server-specifed backoff",
			logs: `
			Fri May 15 22:05:00 UTC 2020: benchmark 0: creating a new profile via profiler service
			Fri May 15 22:37:01 UTC 2020: benchmark 0: creating a new profile via profiler service
			`,
			numBenchmarks:          1,
			serverBackoffSubstring: "action throttled, backoff for",
			createProfileSubstring: "creating a new profile via profiler service",
			benchmarkNumPrefix:     "benchmark",
			wantErr:                true,
		},
		{
			desc: "error when missing CreateProfile requests after server specified backoff",
			logs: `
			Fri May 15 22:05:00 UTC 2020: benchmark 0: creating a new profile via profiler service
			Fri May 15 22:05:00 UTC 2020: benchmark 1: creating a new profile via profiler service
			Fri May 15 22:05:01 UTC 2020: benchmark 0: failed to create profile, will retry: rpc error: code = Aborted desc = generic::aborted: action throttled, backoff for 15m0s
			Fri May 15 22:05:01 UTC 2020: benchmark 1: failed to create profile, will retry: rpc error: code = Aborted desc = generic::aborted: action throttled, backoff for 32m0s
			Fri May 15 22:37:01 UTC 2020: benchmark 1: creating a new profile via profiler service
			`,
			numBenchmarks:          2,
			serverBackoffSubstring: "action throttled, backoff for",
			createProfileSubstring: "creating a new profile via profiler service",
			benchmarkNumPrefix:     "benchmark",
			wantErr:                true,
		},
		{
			desc: "no error when no missing CreateProfile requests after server specified backoff because benchmarks finished before next request could happen.",
			logs: `
			Fri May 15 22:05:00 UTC 2020: benchmark 0: creating a new profile via profiler service
			Fri May 15 22:05:00 UTC 2020: benchmark 1: creating a new profile via profiler service
			Fri May 15 22:05:01 UTC 2020: benchmark 0: failed to create profile, will retry: rpc error: code = Aborted desc = generic::aborted: action throttled, backoff for 32m0s
			Fri May 15 22:05:01 UTC 2020: benchmark 1: failed to create profile, will retry: rpc error: code = Aborted desc = generic::aborted: action throttled, backoff for 32m0s
			Fri May 15 22:37:01 UTC 2020: benchmark 0: creating a new profile via profiler service
			`,
			numBenchmarks:          2,
			serverBackoffSubstring: "action throttled, backoff for",
			createProfileSubstring: "creating a new profile via profiler service",
			benchmarkNumPrefix:     "benchmark",
		},
		{
			desc: "no error when there are non-benchmark logs",
			logs: `
			Fri May 15 22:00:00 UTC 2020: prologue
			Fri May 15 22:05:00 UTC 2020: benchmark 0: creating a new profile via profiler service
			Fri May 15 22:05:01 UTC 2020: benchmark 0: failed to create profile, will retry: rpc error: code = Aborted desc = generic::aborted: action throttled, backoff for 32m0s
			Fri May 15 22:37:01 UTC 2020: benchmark 0: creating a new profile via profiler service
			Fri May 15 22:40:00 UTC 2020: epilogue
			`,
			numBenchmarks:          1,
			serverBackoffSubstring: "action throttled, backoff for",
			createProfileSubstring: "creating a new profile via profiler service",
			benchmarkNumPrefix:     "benchmark",
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			if err := CheckSerialOutputForBackoffs(tc.logs, tc.numBenchmarks, tc.serverBackoffSubstring, tc.createProfileSubstring, tc.benchmarkNumPrefix); (err != nil) != tc.wantErr {
				t.Errorf("got err = %v; want (err != nil) = %v", err, tc.wantErr)
			}
		})
	}
}
