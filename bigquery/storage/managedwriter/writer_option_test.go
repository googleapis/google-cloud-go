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

	"github.com/google/go-cmp/cmp"
)

func TestWriterOptions(t *testing.T) {

	testCases := []struct {
		desc    string
		options []WriterOption
		want    *ManagedStream
	}{
		{
			desc:    "WithType",
			options: []WriterOption{WithType(BufferedStream)},
			want: func() *ManagedStream {
				ms := &ManagedStream{
					streamSettings: defaultStreamSettings(),
				}
				ms.streamSettings.streamType = BufferedStream
				return ms
			}(),
		},
		{
			desc:    "WithMaxInflightRequests",
			options: []WriterOption{WithMaxInflightRequests(2)},
			want: func() *ManagedStream {
				ms := &ManagedStream{
					streamSettings: defaultStreamSettings(),
				}
				ms.streamSettings.MaxInflightRequests = 2
				return ms
			}(),
		},
		{
			desc:    "WithMaxInflightBytes",
			options: []WriterOption{WithMaxInflightBytes(5)},
			want: func() *ManagedStream {
				ms := &ManagedStream{
					streamSettings: defaultStreamSettings(),
				}
				ms.streamSettings.MaxInflightBytes = 5
				return ms
			}(),
		},
		{
			desc:    "WithTracePrefix",
			options: []WriterOption{WithTracePrefix("foo")},
			want: func() *ManagedStream {
				ms := &ManagedStream{
					streamSettings: defaultStreamSettings(),
				}
				ms.streamSettings.TracePrefix = "foo"
				return ms
			}(),
		},
		{
			desc:    "WithDestinationTable",
			options: []WriterOption{WithDestinationTable("foo")},
			want: func() *ManagedStream {
				ms := &ManagedStream{
					streamSettings:   defaultStreamSettings(),
					destinationTable: "foo",
				}
				return ms
			}(),
		},
		{
			desc: "multiple",
			options: []WriterOption{
				WithType(PendingStream),
				WithMaxInflightBytes(5),
				WithTracePrefix("pre"),
			},
			want: func() *ManagedStream {
				ms := &ManagedStream{
					streamSettings: defaultStreamSettings(),
				}
				ms.streamSettings.MaxInflightBytes = 5
				ms.streamSettings.streamType = PendingStream
				ms.streamSettings.TracePrefix = "pre"
				return ms
			}(),
		},
	}

	for _, tc := range testCases {
		got := &ManagedStream{
			streamSettings: defaultStreamSettings(),
		}
		for _, o := range tc.options {
			o(got)
		}

		if diff := cmp.Diff(got, tc.want,
			cmp.AllowUnexported(ManagedStream{}, streamSettings{})); diff != "" {
			t.Errorf("diff in case (%s):\n%v", tc.desc, diff)
		}
	}
}
