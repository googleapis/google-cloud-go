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
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/testing/protocmp"
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
					{
						Name:   proto.String("foo"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()},
					{Name: proto.String("bar"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()},
					{Name: proto.String("baz"), Number: proto.Int32(3), Type: descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()},
				},
			},
		},
		{
			// exercise construct of a submessage
			description: "nested",
			bq: &storagepb.TableSchema{
				Fields: []*storagepb.TableFieldSchema{
					{Name: "curdate", Type: storagepb.TableFieldSchema_DATE, Mode: storagepb.TableFieldSchema_NULLABLE},
					{
						Name: "rec",
						Type: storagepb.TableFieldSchema_STRUCT,
						Mode: storagepb.TableFieldSchema_NULLABLE,
						Fields: []*storagepb.TableFieldSchema{
							{Name: "userid", Type: storagepb.TableFieldSchema_STRING, Mode: storagepb.TableFieldSchema_REQUIRED},
							{Name: "location", Type: storagepb.TableFieldSchema_GEOGRAPHY, Mode: storagepb.TableFieldSchema_NULLABLE},
						},
					},
				},
			},
			want: &descriptorpb.DescriptorProto{
				Name: proto.String("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("curdate"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     proto.String("rec"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: proto.String(".root__rec"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
		},
		{
			// We expect to re-use the submessage twice, as the schema contains two identical structs.
			description: "nested w/duplicate submessage",
			bq: &storagepb.TableSchema{
				Fields: []*storagepb.TableFieldSchema{
					{Name: "curdate", Type: storagepb.TableFieldSchema_DATE, Mode: storagepb.TableFieldSchema_NULLABLE},
					{
						Name: "rec1",
						Type: storagepb.TableFieldSchema_STRUCT,
						Mode: storagepb.TableFieldSchema_NULLABLE,
						Fields: []*storagepb.TableFieldSchema{
							{Name: "userid", Type: storagepb.TableFieldSchema_STRING, Mode: storagepb.TableFieldSchema_REQUIRED},
							{Name: "location", Type: storagepb.TableFieldSchema_GEOGRAPHY, Mode: storagepb.TableFieldSchema_NULLABLE},
						},
					},
					{
						Name: "rec2",
						Type: storagepb.TableFieldSchema_STRUCT,
						Mode: storagepb.TableFieldSchema_NULLABLE,
						Fields: []*storagepb.TableFieldSchema{
							{Name: "userid", Type: storagepb.TableFieldSchema_STRING, Mode: storagepb.TableFieldSchema_REQUIRED},
							{Name: "location", Type: storagepb.TableFieldSchema_GEOGRAPHY, Mode: storagepb.TableFieldSchema_NULLABLE},
						},
					},
				},
			},
			want: &descriptorpb.DescriptorProto{
				Name: proto.String("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("curdate"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     proto.String("rec1"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: proto.String(".root__rec1"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     proto.String("rec2"),
						Number:   proto.Int32(3),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: proto.String(".root__rec1"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		d, err := StorageSchemaToDescriptor(tc.bq, "root")
		if err != nil {
			t.Fatalf("case (%s) failed conversion: %v", tc.description, err)
		}

		// convert it to DP form
		mDesc, ok := d.(protoreflect.MessageDescriptor)
		if !ok {
			t.Fatalf("%s: couldn't convert to messagedescriptor", tc.description)
		}
		gotDP := protodesc.ToDescriptorProto(mDesc)

		if diff := cmp.Diff(gotDP, tc.want, protocmp.Transform()); diff != "" {
			t.Fatalf("%s: -got, +want:\n%s", tc.description, diff)
		}
	}
}

func TestProtoJSONSerialization(t *testing.T) {

	sourceSchema := &storagepb.TableSchema{
		Fields: []*storagepb.TableFieldSchema{
			{Name: "record_id", Type: storagepb.TableFieldSchema_INT64, Mode: storagepb.TableFieldSchema_NULLABLE},
			{
				Name: "details",
				Type: storagepb.TableFieldSchema_STRUCT,
				Mode: storagepb.TableFieldSchema_REPEATED,
				Fields: []*storagepb.TableFieldSchema{
					{Name: "key", Type: storagepb.TableFieldSchema_STRING, Mode: storagepb.TableFieldSchema_REQUIRED},
					{Name: "value", Type: storagepb.TableFieldSchema_STRING, Mode: storagepb.TableFieldSchema_NULLABLE},
				},
			},
		},
	}

	descriptor, err := StorageSchemaToDescriptor(sourceSchema, "root")
	if err != nil {
		t.Fatalf("failed to construct descriptor")
	}

	sampleRecord := []byte(`{"record_id":"12345","details":[{"key":"name","value":"jimmy"},{"key":"title","value":"clown"}]}`)

	messageDescriptor, ok := descriptor.(protoreflect.MessageDescriptor)
	if !ok {
		t.Fatalf("StorageSchemaToDescriptor didn't yield a valid message descriptor, got %T", descriptor)
	}

	// First, ensure we got the expected descriptors.  Check both outer and inner messages.
	gotOuterDP := protodesc.ToDescriptorProto(messageDescriptor)

	innerField := messageDescriptor.Fields().ByName("details")
	if innerField == nil {
		t.Fatalf("couldn't get inner descriptor for details")
	}
	gotInnerDP := protodesc.ToDescriptorProto(innerField.Message())

	wantOuterDP := &descriptorpb.DescriptorProto{
		Name: proto.String("root"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{
				Name:   proto.String("record_id"),
				Number: proto.Int32(1),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			},
			{
				Name:     proto.String("details"),
				Number:   proto.Int32(2),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				TypeName: proto.String(".root__details"),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
			},
		},
	}

	wantInnerDP := &descriptorpb.DescriptorProto{
		Name: proto.String("root__details"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{
				Name:   proto.String("key"),
				Number: proto.Int32(1),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			},
			{
				Name:   proto.String("value"),
				Number: proto.Int32(2),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			},
		},
	}

	if outerDiff := cmp.Diff(gotOuterDP, wantOuterDP, protocmp.Transform()); outerDiff != "" {
		t.Fatalf("DescriptorProto for outer message differs.\n-got, +want:\n%s", outerDiff)
	}
	if innerDiff := cmp.Diff(gotInnerDP, wantInnerDP, protocmp.Transform()); innerDiff != "" {
		t.Fatalf("DescriptorProto for inner message differs.\n-got, +want:\n%s", innerDiff)
	}

	message := dynamicpb.NewMessage(messageDescriptor)

	// Attempt to serialize json record into proto message.
	err = protojson.Unmarshal(sampleRecord, message)
	if err != nil {
		t.Fatalf("failed to Unmarshal json message: %v", err)
	}

	// Serialize message back to json bytes.  We must use options for idempotency, otherwise
	// we'll serialize using the Go name rather than the proto name (recordId vs record_id).
	options := protojson.MarshalOptions{
		UseProtoNames: true,
	}
	gotBytes, err := options.Marshal(message)
	if err != nil {
		t.Fatalf("failed to marshal message: %v", err)
	}

	var got, want interface{}
	if err := json.Unmarshal(gotBytes, &got); err != nil {
		t.Fatalf("couldn't marshal gotBytes: %v", err)
	}
	if err := json.Unmarshal(sampleRecord, &want); err != nil {
		t.Fatalf("couldn't marshal sampleRecord: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mismatched json: got\n%q\nwant\n%q", gotBytes, sampleRecord)
	}

}
