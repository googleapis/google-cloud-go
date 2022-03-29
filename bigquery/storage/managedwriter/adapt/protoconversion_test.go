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

	"cloud.google.com/go/bigquery/storage/managedwriter/testdata"
	"github.com/google/go-cmp/cmp"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
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
		wantProto2  *descriptorpb.DescriptorProto
		wantProto3  *descriptorpb.DescriptorProto
	}{
		{
			description: "basic",
			bq: &storagepb.TableSchema{
				Fields: []*storagepb.TableFieldSchema{
					{Name: "foo", Type: storagepb.TableFieldSchema_STRING, Mode: storagepb.TableFieldSchema_NULLABLE},
					{Name: "bar", Type: storagepb.TableFieldSchema_INT64, Mode: storagepb.TableFieldSchema_REQUIRED},
					{Name: "baz", Type: storagepb.TableFieldSchema_BYTES, Mode: storagepb.TableFieldSchema_REPEATED},
				}},
			wantProto2: &descriptorpb.DescriptorProto{
				Name: proto.String("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("foo"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()},
					{Name: proto.String("bar"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum()},
					{Name: proto.String("baz"), Number: proto.Int32(3), Type: descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()},
				},
			},
			wantProto3: &descriptorpb.DescriptorProto{
				Name: proto.String("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("foo"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: proto.String(".google.protobuf.StringValue"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()},
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
			wantProto2: &descriptorpb.DescriptorProto{
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
			wantProto3: &descriptorpb.DescriptorProto{
				Name: proto.String("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("curdate"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: proto.String(".google.protobuf.Int32Value"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
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
			wantProto2: &descriptorpb.DescriptorProto{
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
			wantProto3: &descriptorpb.DescriptorProto{
				Name: proto.String("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("curdate"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: proto.String(".google.protobuf.Int32Value"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
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
		p2d, err := StorageSchemaToProto2Descriptor(tc.bq, "root")
		if err != nil {
			t.Errorf("case (%s) failed proto2 conversion: %v", tc.description, err)
		}

		// convert it to DP form
		mDesc, ok := p2d.(protoreflect.MessageDescriptor)
		if !ok {
			t.Errorf("%s: couldn't convert proto2 to messagedescriptor", tc.description)
		}
		gotDP := protodesc.ToDescriptorProto(mDesc)
		if diff := cmp.Diff(gotDP, tc.wantProto2, protocmp.Transform()); diff != "" {
			t.Errorf("%s proto2: -got, +want:\n%s", tc.description, diff)
		}

		p3d, err := StorageSchemaToProto3Descriptor(tc.bq, "root")
		if err != nil {
			t.Fatalf("case (%s) failed proto3 conversion: %v", tc.description, err)
		}

		mDesc, ok = p3d.(protoreflect.MessageDescriptor)
		if !ok {
			t.Errorf("%s: couldn't convert proto3 to messagedescriptor", tc.description)
		}
		gotDP = protodesc.ToDescriptorProto(mDesc)
		if diff := cmp.Diff(gotDP, tc.wantProto3, protocmp.Transform()); diff != "" {
			t.Errorf("%s proto3: -got, +want:\n%s", tc.description, diff)
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

	descriptor, err := StorageSchemaToProto2Descriptor(sourceSchema, "root")
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
				Label:  descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
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

func TestNormalizeDescriptor(t *testing.T) {
	testCases := []struct {
		description string
		in          protoreflect.MessageDescriptor
		wantErr     bool
		want        *descriptorpb.DescriptorProto
	}{
		{
			description: "nil",
			in:          nil,
			wantErr:     true,
		},
		{
			description: "AllSupportedTypes",
			in:          (&testdata.AllSupportedTypes{}).ProtoReflect().Descriptor(),
			want: &descriptorpb.DescriptorProto{
				Name: proto.String("testdata_AllSupportedTypes"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("int32_value"),
						JsonName: proto.String("int32Value"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     proto.String("int64_value"),
						JsonName: proto.String("int64Value"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     proto.String("uint32_value"),
						JsonName: proto.String("uint32Value"),
						Number:   proto.Int32(3),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_UINT32.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     proto.String("uint64_value"),
						JsonName: proto.String("uint64Value"),
						Number:   proto.Int32(4),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_UINT64.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     proto.String("float_value"),
						JsonName: proto.String("floatValue"),
						Number:   proto.Int32(5),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_FLOAT.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     proto.String("double_value"),
						JsonName: proto.String("doubleValue"),
						Number:   proto.Int32(6),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_DOUBLE.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     proto.String("bool_value"),
						JsonName: proto.String("boolValue"),
						Number:   proto.Int32(7),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     proto.String("enum_value"),
						JsonName: proto.String("enumValue"),
						TypeName: proto.String("testdata_TestEnum_E.TestEnum"),
						Number:   proto.Int32(8),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     proto.String("string_value"),
						JsonName: proto.String("stringValue"),
						Number:   proto.Int32(9),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
					},
					{
						Name:     proto.String("fixed64_value"),
						JsonName: proto.String("fixed64Value"),
						Number:   proto.Int32(10),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_FIXED64.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("testdata_TestEnum_E"),
						EnumType: []*descriptorpb.EnumDescriptorProto{
							{
								Name: proto.String("TestEnum"),
								Value: []*descriptorpb.EnumValueDescriptorProto{
									{
										Name:   proto.String("TestEnum0"),
										Number: proto.Int32(0),
									},
									{
										Name:   proto.String("TestEnum1"),
										Number: proto.Int32(1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			description: "ContainsRecursive",
			in:          (&testdata.ContainsRecursive{}).ProtoReflect().Descriptor(),
			wantErr:     true,
		},
		{
			description: "RecursiveTypeTopMessage",
			in:          (&testdata.RecursiveTypeTopMessage{}).ProtoReflect().Descriptor(),
			wantErr:     true,
		},
		{
			description: "ComplexType",
			in:          (&testdata.ComplexType{}).ProtoReflect().Descriptor(),
			want: &descriptorpb.DescriptorProto{
				Name: proto.String("testdata_ComplexType"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("nested_repeated_type"),
						JsonName: proto.String("nestedRepeatedType"),
						Number:   proto.Int32(1),
						TypeName: proto.String("testdata_NestedType"),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
					{
						Name:     proto.String("inner_type"),
						JsonName: proto.String("innerType"),
						Number:   proto.Int32(2),
						TypeName: proto.String("testdata_InnerType"),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("testdata_InnerType"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     proto.String("value"),
								JsonName: proto.String("value"),
								Number:   proto.Int32(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
							},
						},
					},
					{
						Name: proto.String("testdata_NestedType"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     proto.String("inner_type"),
								JsonName: proto.String("innerType"),
								Number:   proto.Int32(1),
								TypeName: proto.String("testdata_InnerType"),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
							},
						},
					},
				},
			},
		},
		{
			description: "WithWellKnownTypes",
			in:          (&testdata.WithWellKnownTypes{}).ProtoReflect().Descriptor(),
			want: &descriptorpb.DescriptorProto{
				Name: proto.String("testdata_WithWellKnownTypes"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("int64_value"),
						JsonName: proto.String("int64Value"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     proto.String("wrapped_int64"),
						JsonName: proto.String("wrappedInt64"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: proto.String("google_protobuf_Int64Value"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     proto.String("string_value"),
						JsonName: proto.String("stringValue"),
						Number:   proto.Int32(3),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
					{
						Name:     proto.String("wrapped_string"),
						JsonName: proto.String("wrappedString"),
						Number:   proto.Int32(4),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: proto.String("google_protobuf_StringValue"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("google_protobuf_Int64Value"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     proto.String("value"),
								JsonName: proto.String("value"),
								Number:   proto.Int32(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
					{
						Name: proto.String("google_protobuf_StringValue"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     proto.String("value"),
								JsonName: proto.String("value"),
								Number:   proto.Int32(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
				},
			},
		},
		{
			description: "WithOneOf",
			in:          (&testdata.WithOneOf{}).ProtoReflect().Descriptor(),
			want: &descriptorpb.DescriptorProto{
				Name: proto.String("testdata_WithOneOf"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("int32_value"),
						JsonName: proto.String("int32Value"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     proto.String("string_value"),
						JsonName: proto.String("stringValue"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     proto.String("double_value"),
						JsonName: proto.String("doubleValue"),
						Number:   proto.Int32(3),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_DOUBLE.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
		},
		{
			description: "WithExternalEnum",
			in:          (&testdata.ExternalEnumMessage{}).ProtoReflect().Descriptor(),
			want: &descriptorpb.DescriptorProto{
				Name: proto.String("testdata_ExternalEnumMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("msg_a"),
						JsonName: proto.String("msgA"),
						Number:   proto.Int32(1),
						TypeName: proto.String("testdata_EnumMsgA"),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     proto.String("msg_b"),
						JsonName: proto.String("msgB"),
						Number:   proto.Int32(2),
						TypeName: proto.String("testdata_EnumMsgB"),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("testdata_EnumMsgA"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     proto.String("foo"),
								JsonName: proto.String("foo"),
								Number:   proto.Int32(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
							{
								Name:     proto.String("bar"),
								JsonName: proto.String("bar"),
								Number:   proto.Int32(2),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
								TypeName: proto.String("testdata_ExtEnum_E.ExtEnum"),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
						NestedType: []*descriptorpb.DescriptorProto{
							{
								Name: proto.String("testdata_ExtEnum_E"),
								EnumType: []*descriptorpb.EnumDescriptorProto{
									{
										Name: proto.String("ExtEnum"),
										Value: []*descriptorpb.EnumValueDescriptorProto{
											{
												Name:   proto.String("UNDEFINED"),
												Number: proto.Int32(0),
											},
											{
												Name:   proto.String("THING"),
												Number: proto.Int32(1),
											},
											{
												Name:   proto.String("OTHER_THING"),
												Number: proto.Int32(2),
											},
										},
									},
								},
							},
						},
					},
					{
						Name: proto.String("testdata_EnumMsgB"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     proto.String("baz"),
								JsonName: proto.String("baz"),
								Number:   proto.Int32(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
								TypeName: proto.String("testdata_ExtEnum_E.ExtEnum"),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		gotDP, err := NormalizeDescriptor(tc.in)

		if tc.wantErr && err == nil {
			t.Errorf("%s: wanted err but got success", tc.description)
			continue
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%s: wanted success, got err: %v", tc.description, err)
			continue
		}
		if diff := cmp.Diff(gotDP, tc.want, protocmp.Transform()); diff != "" {
			t.Errorf("%s: -got, +want:\n%s", tc.description, diff)
		}
	}
}
