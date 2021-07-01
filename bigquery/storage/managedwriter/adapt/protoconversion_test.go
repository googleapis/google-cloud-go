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

package adapt

import (
	"testing"

	"cloud.google.com/go/internal/testutil"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

func TestSchemaToProtoConversion(t *testing.T) {
	testCases := []struct {
		description string
		bq          *storagepb.TableSchema
		want        *descriptorpb.DescriptorProto
	}{
		{
			description: "basic",
			bq: &storagepb.TableSchema{
				Fields: []*storagepb.TableFieldSchema{
					{Name: "foo", Type: storagepb.TableFieldSchema_STRING, Mode: storagepb.TableFieldSchema_NULLABLE},
					{Name: "bar", Type: storagepb.TableFieldSchema_INT64, Mode: storagepb.TableFieldSchema_REQUIRED},
					{Name: "baz", Type: storagepb.TableFieldSchema_BYTES, Mode: storagepb.TableFieldSchema_REPEATED},
				}},
			want: &descriptorpb.DescriptorProto{
				Name: proto.String("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("foo"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()},
					{Name: proto.String("bar"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum()},
					{Name: proto.String("baz"), Number: proto.Int32(3), Type: descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()},
				},
			},
		},
	}
	for _, tc := range testCases {
		d, err := StorageSchemaToFileDescriptorSet(tc.bq, "root", nil)
		if err != nil {
			t.Errorf("case (%s) failed conversion: %v", tc.description, err)
		}

		// convert it to DP form
		mDesc, ok := d.(protoreflect.MessageDescriptor)
		if !ok {
			t.Fatalf("%s: couldn't convert to messagedescriptor", tc.description)
		}
		gotDP := protodesc.ToDescriptorProto(mDesc)

		if diff := testutil.Diff(gotDP, tc.want); diff != "" {
			t.Fatalf("%s: -got, +want:\n%s", tc.description, diff)
		}
	}
}

func TestProtoManipulation(t *testing.T) {
	in := &storagepb.TableSchema{
		Fields: []*storagepb.TableFieldSchema{
			{Name: "foo", Type: storagepb.TableFieldSchema_STRING, Mode: storagepb.TableFieldSchema_NULLABLE},
			{Name: "bar", Type: storagepb.TableFieldSchema_INT64, Mode: storagepb.TableFieldSchema_REQUIRED},
			{Name: "baz", Type: storagepb.TableFieldSchema_DOUBLE, Mode: storagepb.TableFieldSchema_REPEATED},
		}}

	rootName := "root"
	d, err := StorageSchemaToFileDescriptorSet(in, rootName, nil)
	if err != nil {
		t.Fatalf("StorageSchemaToFileDescriptorSet: %v", err)
	}

	md, ok := d.(protoreflect.MessageDescriptor)
	if !ok {
		t.Fatalf("descriptor not messagedescriptor, was %T", d)
	}

	// build a dynamic message
	m := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName(protoreflect.Name("foo"))
	if fd == nil {
		t.Fatalf("couldn't find field foo")
	}
	m.Set(fd, protoreflect.ValueOfString("hello"))

	fd = md.Fields().ByName(protoreflect.Name("bar"))
	if fd == nil {
		t.Fatalf("couldn't find field bar")
	}
	m.Set(fd, protoreflect.ValueOfInt64(123))

	fd = md.Fields().ByName(protoreflect.Name("baz"))
	if fd == nil {
		t.Fatalf("couldn't find field baz")
	}

	list := m.Mutable(fd).List()
	list.Append(protoreflect.ValueOfFloat64(1.2))
	list.Append(protoreflect.ValueOfFloat64(2.4))

	b, err := protojson.Marshal(m)
	if err != nil {
		t.Fatalf("couldn't Marshall as textproto: %v", err)
	}

	got := string(b)
	want := `{"foo":"hello","bar":"123","baz":[1.2,2.4]}`

	if got != want {
		t.Fatalf("json representation differs:  got %s, want %s", got, want)
	}

}
