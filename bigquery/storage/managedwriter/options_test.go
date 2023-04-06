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
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

func TestCustomClientOptions(t *testing.T) {
	testCases := []struct {
		desc    string
		options []option.ClientOption
		want    *writerClientConfig
	}{
		{
			desc: "no options",
			want: &writerClientConfig{},
		},
		{
			desc: "multiplex",
			options: []option.ClientOption{
				enableMultiplex(true, 4),
			},
			want: &writerClientConfig{
				useMultiplex:         true,
				maxMultiplexPoolSize: 4,
			},
		},
		{
			desc: "default requests",
			options: []option.ClientOption{
				defaultMaxInflightRequests(42),
			},
			want: &writerClientConfig{
				defaultInflightRequests: 42,
			},
		},
		{
			desc: "default bytes",
			options: []option.ClientOption{
				defaultMaxInflightBytes(123),
			},
			want: &writerClientConfig{
				defaultInflightBytes: 123,
			},
		},
		{
			desc: "default call options",
			options: []option.ClientOption{
				defaultAppendRowsCallOption(gax.WithGRPCOptions(grpc.MaxCallSendMsgSize(1))),
			},
			want: &writerClientConfig{
				defaultAppendRowsCallOptions: []gax.CallOption{
					gax.WithGRPCOptions(grpc.MaxCallSendMsgSize(1)),
				},
			},
		},
		{
			desc: "multiple options",
			options: []option.ClientOption{
				enableMultiplex(true, 10),
				defaultMaxInflightRequests(99),
				defaultMaxInflightBytes(12345),
				defaultAppendRowsCallOption(gax.WithGRPCOptions(grpc.MaxCallSendMsgSize(1))),
			},
			want: &writerClientConfig{
				useMultiplex:            true,
				maxMultiplexPoolSize:    10,
				defaultInflightRequests: 99,
				defaultInflightBytes:    12345,
				defaultAppendRowsCallOptions: []gax.CallOption{
					gax.WithGRPCOptions(grpc.MaxCallSendMsgSize(1)),
				},
			},
		},
	}
	for _, tc := range testCases {
		gotCfg := newWriterClientConfig(tc.options...)

		if diff := cmp.Diff(gotCfg, tc.want, cmp.AllowUnexported(writerClientConfig{})); diff != "" {
			t.Errorf("diff in case (%s):\n%v", tc.desc, diff)
		}
	}
}

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
			desc:    "WithTraceID",
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
					streamSettings: defaultStreamSettings(),
				}
				ms.streamSettings.destinationTable = "foo"
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
				}
				ms.streamSettings.appendCallOptions = append(ms.streamSettings.appendCallOptions,
					gax.WithGRPCOptions(grpc.MaxCallSendMsgSize(1)))
				return ms
			}(),
		},
		{
			desc:    "EnableRetries",
			options: []WriterOption{EnableWriteRetries(true)},
			want: func() *ManagedStream {
				ms := &ManagedStream{
					streamSettings: defaultStreamSettings(),
				}
				ms.retry = newStatelessRetryer()
				return ms
			}(),
		},
		{
			desc: "multiple",
			options: []WriterOption{
				WithType(PendingStream),
				WithMaxInflightBytes(5),
				WithTraceID("traceid"),
				EnableWriteRetries(true),
			},
			want: func() *ManagedStream {
				ms := &ManagedStream{
					streamSettings: defaultStreamSettings(),
				}
				ms.streamSettings.MaxInflightBytes = 5
				ms.streamSettings.streamType = PendingStream
				ms.streamSettings.TraceID = "traceid"
				ms.retry = newStatelessRetryer()
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
			cmp.AllowUnexported(sync.Mutex{}),
			cmpopts.IgnoreUnexported(statelessRetryer{})); diff != "" {
			t.Errorf("diff in case (%s):\n%v", tc.desc, diff)
		}
	}
}
