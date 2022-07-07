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
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc"
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
			options: []WriterOption{WithTraceID("foo")},
			want: func() *ManagedStream {
				ms := &ManagedStream{
					streamSettings: defaultStreamSettings(),
				}
				ms.streamSettings.TraceID = "foo"
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
			desc:    "WithDataOrigin",
			options: []WriterOption{WithDataOrigin("origin")},
			want: func() *ManagedStream {
				ms := &ManagedStream{
					streamSettings: defaultStreamSettings(),
				}
				ms.streamSettings.dataOrigin = "origin"
				return ms
			}(),
		},
		{
			desc:    "WithCallOption",
			options: []WriterOption{WithAppendRowsCallOption(gax.WithGRPCOptions(grpc.MaxCallSendMsgSize(1)))},
			want: func() *ManagedStream {
				ms := &ManagedStream{
					streamSettings: defaultStreamSettings(),
					callOptions: []gax.CallOption{
						gax.WithGRPCOptions(grpc.MaxCallSendMsgSize(1)),
					},
				}
				return ms
			}(),
		},
		{
			desc: "multiple",
			options: []WriterOption{
				WithType(PendingStream),
				WithMaxInflightBytes(5),
				WithTraceID("id"),
			},
			want: func() *ManagedStream {
				ms := &ManagedStream{
					streamSettings: defaultStreamSettings(),
				}
				ms.streamSettings.MaxInflightBytes = 5
				ms.streamSettings.streamType = PendingStream
				ms.streamSettings.TraceID = "id"
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
			cmp.AllowUnexported(ManagedStream{}, streamSettings{}),
			cmp.AllowUnexported(sync.Mutex{})); diff != "" {
			t.Errorf("diff in case (%s):\n%v", tc.desc, diff)
		}
	}
}
