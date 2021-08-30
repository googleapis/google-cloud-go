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

package adapt_test

import (
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter/adapt"
	"cloud.google.com/go/bigquery/storage/managedwriter/testdata"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var benchDescriptor protoreflect.Descriptor

func BenchmarkStorageSchemaToDescriptor(b *testing.B) {
	syntaxLabels := []string{"proto2", "proto3"}
	for _, bm := range []struct {
		name string
		in   bigquery.Schema
	}{
		{
			name: "SingleField",
			in: bigquery.Schema{
				{Name: "field", Type: bigquery.StringFieldType},
			},
		},
		{
			name: "NestedRecord",
			in: bigquery.Schema{
				{Name: "field1", Type: bigquery.StringFieldType},
				{Name: "field2", Type: bigquery.IntegerFieldType},
				{Name: "field3", Type: bigquery.BooleanFieldType},
				{
					Name: "field4",
					Type: bigquery.RecordFieldType,
					Schema: bigquery.Schema{
						{Name: "recordfield1", Type: bigquery.GeographyFieldType},
						{Name: "recordfield2", Type: bigquery.TimestampFieldType},
					},
				},
			},
		},
		{
			name: "SimpleMessage",
			in:   testdata.SimpleMessageSchema,
		},
		{
			name: "GithubArchiveSchema",
			in:   testdata.GithubArchiveSchema,
		},
	} {
		for _, s := range syntaxLabels {
			b.Run(fmt.Sprintf("%s-%s", bm.name, s), func(b *testing.B) {
				convSchema, err := adapt.BQSchemaToStorageTableSchema(bm.in)
				if err != nil {
					b.Errorf("%q: schema conversion fail: %v", bm.name, err)
				}
				for n := 0; n < b.N; n++ {
					if s == "proto3" {
						benchDescriptor, err = adapt.StorageSchemaToProto3Descriptor(convSchema, "root")
					} else {
						benchDescriptor, err = adapt.StorageSchemaToProto2Descriptor(convSchema, "root")
					}
					if err != nil {
						b.Errorf("failed to convert %q: %v", bm.name, err)
					}
				}
			})
		}
	}
}

var staticBytes []byte

func BenchmarkStaticProtoSerialization(b *testing.B) {
	for _, bm := range []struct {
		name    string
		in      bigquery.Schema
		syntax  string
		setterF func() protoreflect.ProtoMessage
	}{
		{
			name: "SimpleMessageProto2",
			setterF: func() protoreflect.ProtoMessage {
				return &testdata.SimpleMessageProto2{
					Name:  proto.String(fmt.Sprintf("test-%d", time.Now().UnixNano())),
					Value: proto.Int64(time.Now().UnixNano()),
				}
			},
		},
		{
			name: "SimpleMessageProto3",
			setterF: func() protoreflect.ProtoMessage {
				return &testdata.SimpleMessageProto3{
					Name:  fmt.Sprintf("test-%d", time.Now().UnixNano()),
					Value: &wrapperspb.Int64Value{Value: time.Now().UnixNano()},
				}
			},
		},
		{
			name: "GithubArchiveProto2",
			setterF: func() protoreflect.ProtoMessage {
				nowNano := time.Now().UnixNano()
				return &testdata.GithubArchiveMessageProto2{
					Type:    proto.String("SomeEvent"),
					Public:  proto.Bool(nowNano%2 == 0),
					Payload: proto.String(fmt.Sprintf("stuff %d", nowNano)),
					Repo: &testdata.GithubArchiveRepoProto2{
						Id:   proto.Int64(nowNano),
						Name: proto.String("staticname"),
						Url:  proto.String(fmt.Sprintf("foo.com/%d", nowNano)),
					},
					Actor: &testdata.GithubArchiveEntityProto2{
						Id:         proto.Int64(nowNano % 1000),
						Login:      proto.String(fmt.Sprintf("login-%d", nowNano%1000)),
						GravatarId: proto.String(fmt.Sprintf("grav-%d", nowNano%1000000)),
						AvatarUrl:  proto.String(fmt.Sprintf("https://something.com/img/%d", nowNano%10000000)),
						Url:        proto.String(fmt.Sprintf("https://something.com/img/%d", nowNano%10000000)),
					},
					Org: &testdata.GithubArchiveEntityProto2{
						Id:         proto.Int64(nowNano % 1000),
						Login:      proto.String(fmt.Sprintf("login-%d", nowNano%1000)),
						GravatarId: proto.String(fmt.Sprintf("grav-%d", nowNano%1000000)),
						AvatarUrl:  proto.String(fmt.Sprintf("https://something.com/img/%d", nowNano%10000000)),
						Url:        proto.String(fmt.Sprintf("https://something.com/img/%d", nowNano%10000000)),
					},
					CreatedAt: proto.Int64(nowNano),
					Id:        proto.String(fmt.Sprintf("id%d", nowNano)),
					Other:     proto.String("other"),
				}
			},
		},
		{
			// Only set a single top-level field in a larger message.
			name: "GithubArchiveProto2_Sparse",
			setterF: func() protoreflect.ProtoMessage {
				nowNano := time.Now().UnixNano()
				return &testdata.GithubArchiveMessageProto2{
					Id: proto.String(fmt.Sprintf("id%d", nowNano)),
				}
			},
		},
		{
			name: "GithubArchiveProto3",
			setterF: func() protoreflect.ProtoMessage {
				nowNano := time.Now().UnixNano()
				return &testdata.GithubArchiveMessageProto3{
					Type:    &wrapperspb.StringValue{Value: "SomeEvent"},
					Public:  &wrapperspb.BoolValue{Value: nowNano%2 == 0},
					Payload: &wrapperspb.StringValue{Value: fmt.Sprintf("stuff %d", nowNano)},
					Repo: &testdata.GithubArchiveRepoProto3{
						Id:   &wrapperspb.Int64Value{Value: nowNano},
						Name: &wrapperspb.StringValue{Value: "staticname"},
						Url:  &wrapperspb.StringValue{Value: fmt.Sprintf("foo.com/%d", nowNano)},
					},
					Actor: &testdata.GithubArchiveEntityProto3{
						Id:         &wrapperspb.Int64Value{Value: nowNano % 1000},
						Login:      &wrapperspb.StringValue{Value: fmt.Sprintf("login-%d", nowNano%1000)},
						GravatarId: &wrapperspb.StringValue{Value: fmt.Sprintf("grav-%d", nowNano%1000000)},
						AvatarUrl:  &wrapperspb.StringValue{Value: fmt.Sprintf("https://something.com/img/%d", nowNano%10000000)},
						Url:        &wrapperspb.StringValue{Value: fmt.Sprintf("https://something.com/img/%d", nowNano%10000000)},
					},
					Org: &testdata.GithubArchiveEntityProto3{
						Id:         &wrapperspb.Int64Value{Value: nowNano % 1000},
						Login:      &wrapperspb.StringValue{Value: fmt.Sprintf("login-%d", nowNano%1000)},
						GravatarId: &wrapperspb.StringValue{Value: fmt.Sprintf("grav-%d", nowNano%1000000)},
						AvatarUrl:  &wrapperspb.StringValue{Value: fmt.Sprintf("https://something.com/img/%d", nowNano%10000000)},
						Url:        &wrapperspb.StringValue{Value: fmt.Sprintf("https://something.com/img/%d", nowNano%10000000)},
					},
					CreatedAt: &wrapperspb.Int64Value{Value: nowNano},
					Id:        &wrapperspb.StringValue{Value: fmt.Sprintf("id%d", nowNano)},
					Other:     &wrapperspb.StringValue{Value: "other"},
				}
			},
		},
		{
			// Only set a single field in a larger message.
			name: "GithubArchiveProto3_Sparse",
			setterF: func() protoreflect.ProtoMessage {
				nowNano := time.Now().UnixNano()
				return &testdata.GithubArchiveMessageProto3{
					Id: &wrapperspb.StringValue{Value: fmt.Sprintf("id%d", nowNano)},
				}
			},
		},
	} {
		b.Run(bm.name, func(b *testing.B) {
			var totalBytes int64
			for n := 0; n < b.N; n++ {
				m := bm.setterF()
				out, err := proto.Marshal(m)
				if err != nil {
					b.Errorf("%q %q: Marshal: %v", bm.name, bm.syntax, err)
				}
				totalBytes = totalBytes + int64(len(out))
				staticBytes = out
			}
			b.Logf("N=%d, avg bytes/message: %d", b.N, totalBytes/int64(b.N))
		})
	}
}
