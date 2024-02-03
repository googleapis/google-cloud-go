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

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/descriptorpb"
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
			desc: "multiplex enable",
			options: []option.ClientOption{
				WithMultiplexing(),
			},
			want: &writerClientConfig{
				useMultiplex:         true,
				maxMultiplexPoolSize: 1,
			},
		},
		{
			desc: "multiplex max",
			options: []option.ClientOption{
				WithMultiplexPoolLimit(99),
			},
			want: &writerClientConfig{
				maxMultiplexPoolSize: 99,
			},
		},
		{
			desc: "default requests",
			options: []option.ClientOption{
				WithDefaultInflightRequests(42),
			},
			want: &writerClientConfig{
				defaultInflightRequests: 42,
			},
		},
		{
			desc: "default bytes",
			options: []option.ClientOption{
				WithDefaultInflightBytes(123),
			},
			want: &writerClientConfig{
				defaultInflightBytes: 123,
			},
		},
		{
			desc: "default call options",
			options: []option.ClientOption{
				WithDefaultAppendRowsCallOption(gax.WithGRPCOptions(grpc.MaxCallSendMsgSize(1))),
			},
			want: &writerClientConfig{
				defaultAppendRowsCallOptions: []gax.CallOption{
					gax.WithGRPCOptions(grpc.MaxCallSendMsgSize(1)),
				},
			},
		},
		{
			desc: "unusual values",
			options: []option.ClientOption{
				WithMultiplexing(),
				WithMultiplexPoolLimit(-8),
				WithDefaultInflightBytes(-1),
				WithDefaultInflightRequests(-99),
			},
			want: &writerClientConfig{
				useMultiplex:            true,
				maxMultiplexPoolSize:    1,
				defaultInflightRequests: 0,
				defaultInflightBytes:    0,
			},
		},
		{
			desc: "multiple options",
			options: []option.ClientOption{
				WithMultiplexing(),
				WithMultiplexPoolLimit(10),
				WithDefaultInflightRequests(99),
				WithDefaultInflightBytes(12345),
				WithDefaultAppendRowsCallOption(gax.WithGRPCOptions(grpc.MaxCallSendMsgSize(1))),
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
					curTemplate:    newVersionedTemplate(),
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
					curTemplate:    newVersionedTemplate(),
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
					curTemplate:    newVersionedTemplate(),
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
					curTemplate:    newVersionedTemplate(),
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
					curTemplate:    newVersionedTemplate(),
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
					curTemplate:    newVersionedTemplate(),
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
					curTemplate:    newVersionedTemplate(),
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
					curTemplate:    newVersionedTemplate(),
				}
				ms.retry = newStatelessRetryer()
				return ms
			}(),
		},
		{
			desc:    "WithSchemaDescriptor",
			options: []WriterOption{WithSchemaDescriptor(&descriptorpb.DescriptorProto{Name: proto.String("name")})},
			want: func() *ManagedStream {
				ms := &ManagedStream{
					streamSettings: defaultStreamSettings(),
					curTemplate:    newVersionedTemplate(),
				}
				ms.curTemplate.tmpl = &storagepb.AppendRowsRequest{
					Rows: &storagepb.AppendRowsRequest_ProtoRows{
						ProtoRows: &storagepb.AppendRowsRequest_ProtoData{
							WriterSchema: &storagepb.ProtoSchema{
								ProtoDescriptor: &descriptorpb.DescriptorProto{Name: proto.String("name")},
							},
						},
					},
				}
				return ms
			}(),
		},
		{
			desc:    "WithDefaultMissingValueInterpretation",
			options: []WriterOption{WithDefaultMissingValueInterpretation(storagepb.AppendRowsRequest_DEFAULT_VALUE)},
			want: func() *ManagedStream {
				ms := &ManagedStream{
					streamSettings: defaultStreamSettings(),
					curTemplate:    newVersionedTemplate(),
				}
				ms.curTemplate.tmpl = &storagepb.AppendRowsRequest{
					DefaultMissingValueInterpretation: storagepb.AppendRowsRequest_DEFAULT_VALUE,
				}
				return ms
			}(),
		},
		{
			desc: "WithtMissingValueInterpretations",
			options: []WriterOption{WithMissingValueInterpretations(map[string]storagepb.AppendRowsRequest_MissingValueInterpretation{
				"foo": storagepb.AppendRowsRequest_DEFAULT_VALUE,
				"bar": storagepb.AppendRowsRequest_NULL_VALUE,
			})},
			want: func() *ManagedStream {
				ms := &ManagedStream{
					streamSettings: defaultStreamSettings(),
					curTemplate:    newVersionedTemplate(),
				}
				ms.curTemplate.tmpl = &storagepb.AppendRowsRequest{
					MissingValueInterpretations: map[string]storagepb.AppendRowsRequest_MissingValueInterpretation{
						"foo": storagepb.AppendRowsRequest_DEFAULT_VALUE,
						"bar": storagepb.AppendRowsRequest_NULL_VALUE,
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
				WithTraceID("traceid"),
				EnableWriteRetries(true),
				WithSchemaDescriptor(&descriptorpb.DescriptorProto{Name: proto.String("name")}),
				WithDefaultMissingValueInterpretation(storagepb.AppendRowsRequest_DEFAULT_VALUE),
				WithMissingValueInterpretations(map[string]storagepb.AppendRowsRequest_MissingValueInterpretation{
					"foo": storagepb.AppendRowsRequest_DEFAULT_VALUE,
					"bar": storagepb.AppendRowsRequest_NULL_VALUE,
				}),
			},
			want: func() *ManagedStream {
				ms := &ManagedStream{
					streamSettings: defaultStreamSettings(),
					curTemplate:    newVersionedTemplate(),
				}
				ms.curTemplate.tmpl = &storagepb.AppendRowsRequest{
					Rows: &storagepb.AppendRowsRequest_ProtoRows{
						ProtoRows: &storagepb.AppendRowsRequest_ProtoData{
							WriterSchema: &storagepb.ProtoSchema{
								ProtoDescriptor: &descriptorpb.DescriptorProto{Name: proto.String("name")},
							},
						},
					},
					MissingValueInterpretations: map[string]storagepb.AppendRowsRequest_MissingValueInterpretation{
						"foo": storagepb.AppendRowsRequest_DEFAULT_VALUE,
						"bar": storagepb.AppendRowsRequest_NULL_VALUE,
					},
					DefaultMissingValueInterpretation: storagepb.AppendRowsRequest_DEFAULT_VALUE,
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
			curTemplate:    newVersionedTemplate(),
		}
		for _, o := range tc.options {
			o(got)
		}

		if diff := cmp.Diff(got, tc.want,
			cmp.AllowUnexported(ManagedStream{}, streamSettings{}),
			cmp.AllowUnexported(sync.Mutex{}),
			cmp.AllowUnexported(versionedTemplate{}),
			cmpopts.IgnoreFields(versionedTemplate{}, "versionTime", "hashVal"),
			protocmp.Transform(), // versionedTemplate embeds proto messages.
			cmpopts.IgnoreUnexported(statelessRetryer{})); diff != "" {
			t.Errorf("diff in case (%s):\n%v", tc.desc, diff)
		}

	}
}
