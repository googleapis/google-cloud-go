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
		want    *ManagedWriteClient
	}{
		{
			desc:    "WithType",
			options: []WriterOption{WithType(BufferedStream)},
			want: func() *ManagedWriteClient {
				mw := &ManagedWriteClient{
					streamSettings: defaultStreamSettings(),
				}
				mw.streamSettings.streamType = BufferedStream
				return mw
			}(),
		},
		{
			desc:    "WithMaxInflightRequests",
			options: []WriterOption{WithMaxInflightRequests(2)},
			want: func() *ManagedWriteClient {
				mw := &ManagedWriteClient{
					streamSettings: defaultStreamSettings(),
				}
				mw.streamSettings.MaxInflightRequests = 2
				return mw
			}(),
		},
		{
			desc:    "WithMaxInflightBytes",
			options: []WriterOption{WithMaxInflightBytes(5)},
			want: func() *ManagedWriteClient {
				mw := &ManagedWriteClient{
					streamSettings: defaultStreamSettings(),
				}
				mw.streamSettings.MaxInflightBytes = 5
				return mw
			}(),
		},
		{
			desc:    "WithTracePrefix",
			options: []WriterOption{WithTracePrefix("foo")},
			want: func() *ManagedWriteClient {
				mw := &ManagedWriteClient{
					streamSettings: defaultStreamSettings(),
				}
				mw.streamSettings.TracePrefix = "foo"
				return mw
			}(),
		},
		{
			desc: "multiple",
			options: []WriterOption{
				WithType(PendingStream),
				WithMaxInflightBytes(5),
				WithTracePrefix("pre"),
			},
			want: func() *ManagedWriteClient {
				mw := &ManagedWriteClient{
					streamSettings: defaultStreamSettings(),
				}
				mw.streamSettings.MaxInflightBytes = 5
				mw.streamSettings.streamType = PendingStream
				mw.streamSettings.TracePrefix = "pre"
				return mw
			}(),
		},
	}

	for _, tc := range testCases {
		got := &ManagedWriteClient{
			streamSettings: defaultStreamSettings(),
		}
		for _, o := range tc.options {
			o(got)
		}

		if diff := cmp.Diff(got, tc.want,
			cmp.AllowUnexported(ManagedWriteClient{}, streamSettings{})); diff != "" {
			t.Errorf("diff in case (%s):\n%v", tc.desc, diff)
		}
	}
}
