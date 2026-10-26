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
	"fmt"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"cloud.google.com/go/bigquery/storage/managedwriter/testdata"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// TestSchemaToProtoConversion validates behavior around converting table schemas to
// a descriptor.  The challenges here are that we use dynamic proto registries to
// do this work, which means that we're unable to do things like proto.Equal comparisons
// between MessageDescriptors directly.
//
// Instead, we compare to two forms of the message in DescriptorProto form:  In the first,
// we ensure the structure of the outer message is as expected.  In the second, we compare
// to the normalized form of the DescriptorProto as that encapsulates all the dependencies
// within the NestedTypes definition.
func TestSchemaToProtoConversion(t *testing.T) {
	testCases := []struct {
		description string
		bq          *storagepb.TableSchema
		// The un-normalized descriptor (sans dependencies)
		wantProto2 *descriptorpb.DescriptorProto
		// Normalized descriptor (all dependencies nested)
		wantProto2Normalized *descriptorpb.DescriptorProto

		// The un-normalized descriptor (sans dependencies)
		wantProto3 *descriptorpb.DescriptorProto
		// Normalized descriptor
		wantProto3Normalized *descriptorpb.DescriptorProto

		// proto conversion options
		options []ProtoConversionOption
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
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   new("foo"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()},
					{Name: new("bar"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum()},
					{Name: new("baz"), Number: proto.Int32(3), Type: descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()},
				},
			},
			wantProto2Normalized: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   new("foo"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()},
					{Name: new("bar"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum()},
					{Name: new("baz"), Number: proto.Int32(3), Type: descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()},
				},
			},
			wantProto3: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("foo"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".google.protobuf.StringValue"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()},
					{Name: new("bar"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()},
					{Name: new("baz"), Number: proto.Int32(3), Type: descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()},
				},
			},
		},
		{
			description: "json type",
			bq: &storagepb.TableSchema{
				Fields: []*storagepb.TableFieldSchema{
					{Name: "json_required", Type: storagepb.TableFieldSchema_JSON, Mode: storagepb.TableFieldSchema_REQUIRED},
					{Name: "json_optional", Type: storagepb.TableFieldSchema_JSON, Mode: storagepb.TableFieldSchema_NULLABLE},
				}},
			wantProto2: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   new("json_required"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum()},
					{
						Name:   new("json_optional"),
						Number: proto.Int32(2),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
			wantProto2Normalized: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   new("json_required"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
					},
					{
						Name:   new("json_optional"),
						Number: proto.Int32(2),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
			wantProto3: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   new("json_required"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("json_optional"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".google.protobuf.StringValue"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
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
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   new("curdate"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("rec"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".root__rec"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
			wantProto2Normalized: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   new("curdate"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("rec"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new("root__rec"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: new("root__rec"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   new("userid"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
							},
							{
								Name:   new("location"),
								Number: proto.Int32(2),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
				},
			},
			wantProto3: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("curdate"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".google.protobuf.Int32Value"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("rec"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".root__rec"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
		},
		{
			// exercise construct of a submessage
			description: "range types",
			bq: &storagepb.TableSchema{
				Fields: []*storagepb.TableFieldSchema{
					{
						Name:             "rangedate",
						Type:             storagepb.TableFieldSchema_RANGE,
						Mode:             storagepb.TableFieldSchema_NULLABLE,
						RangeElementType: &storagepb.TableFieldSchema_FieldElementType{Type: storagepb.TableFieldSchema_DATE},
					},
					{
						Name:             "rangedatetime",
						Type:             storagepb.TableFieldSchema_RANGE,
						Mode:             storagepb.TableFieldSchema_NULLABLE,
						RangeElementType: &storagepb.TableFieldSchema_FieldElementType{Type: storagepb.TableFieldSchema_DATETIME},
					},
					{
						Name:             "rangetimestamp",
						Type:             storagepb.TableFieldSchema_RANGE,
						Mode:             storagepb.TableFieldSchema_NULLABLE,
						RangeElementType: &storagepb.TableFieldSchema_FieldElementType{Type: storagepb.TableFieldSchema_TIMESTAMP},
					},
					{
						Name:             "duplicate_rangedate",
						Type:             storagepb.TableFieldSchema_RANGE,
						Mode:             storagepb.TableFieldSchema_NULLABLE,
						RangeElementType: &storagepb.TableFieldSchema_FieldElementType{Type: storagepb.TableFieldSchema_DATE},
					},
				},
			},
			wantProto2: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("rangedate"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".rangemessage_range_date"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("rangedatetime"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".rangemessage_range_datetime"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("rangetimestamp"),
						Number:   proto.Int32(3),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".rangemessage_range_timestamp"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("duplicate_rangedate"),
						Number:   proto.Int32(4),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".rangemessage_range_date"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
			wantProto2Normalized: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("rangedate"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new("rangemessage_range_date"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("rangedatetime"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new("rangemessage_range_datetime"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("rangetimestamp"),
						Number:   proto.Int32(3),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new("rangemessage_range_timestamp"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("duplicate_rangedate"),
						Number:   proto.Int32(4),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new("rangemessage_range_date"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: new("rangemessage_range_date"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   new("start"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
							{
								Name:   new("end"),
								Number: proto.Int32(2),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
					{
						Name: new("rangemessage_range_datetime"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   new("start"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
							{
								Name:   new("end"),
								Number: proto.Int32(2),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
					{
						Name: new("rangemessage_range_timestamp"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   new("start"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
							{
								Name:   new("end"),
								Number: proto.Int32(2),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
				},
			},
			wantProto3: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("rangedate"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".rangemessage_range_date"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("rangedatetime"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".rangemessage_range_datetime"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("rangetimestamp"),
						Number:   proto.Int32(3),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".rangemessage_range_timestamp"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("duplicate_rangedate"),
						Number:   proto.Int32(4),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".rangemessage_range_date"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
		},
		{
			description: "nested-uppercase",
			bq: &storagepb.TableSchema{
				Fields: []*storagepb.TableFieldSchema{
					{Name: "recordID", Type: storagepb.TableFieldSchema_INT64, Mode: storagepb.TableFieldSchema_REQUIRED},
					{
						Name: "recordDetails",
						Type: storagepb.TableFieldSchema_STRUCT,
						Mode: storagepb.TableFieldSchema_REPEATED,
						Fields: []*storagepb.TableFieldSchema{
							{Name: "key", Type: storagepb.TableFieldSchema_STRING, Mode: storagepb.TableFieldSchema_REQUIRED},
							{Name: "value", Type: storagepb.TableFieldSchema_BYTES, Mode: storagepb.TableFieldSchema_NULLABLE},
						},
					},
				},
			},
			wantProto2: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   new("recordID"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
					},
					{
						Name:     new("recordDetails"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".root__recordDetails"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
				},
			},
			wantProto2Normalized: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   new("recordID"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
					},
					{
						Name:     new("recordDetails"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new("root__recordDetails"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: new("root__recordDetails"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   new("key"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
							},
							{
								Name:   new("value"),
								Number: proto.Int32(2),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
				},
			},
			wantProto3: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   new("recordID"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("recordDetails"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".root__recordDetails"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
				},
			},
		},
		{
			// We expect to re-use the submessage twice, as the schema contains two identical structs.
			description: "nested with duplicate submessage",
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
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   new("curdate"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("rec1"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".root__rec1"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("rec2"),
						Number:   proto.Int32(3),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".root__rec1"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
			wantProto3: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("curdate"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".google.protobuf.Int32Value"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("rec1"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".root__rec1"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("rec2"),
						Number:   proto.Int32(3),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".root__rec1"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
		},
		{
			description: "nested with reused submessage in different levels",
			bq: &storagepb.TableSchema{
				Fields: []*storagepb.TableFieldSchema{
					{
						Name: "reused_inner_struct",
						Type: storagepb.TableFieldSchema_STRUCT,
						Mode: storagepb.TableFieldSchema_REQUIRED,
						Fields: []*storagepb.TableFieldSchema{
							{
								Name: "leaf",
								Type: storagepb.TableFieldSchema_STRING,
								Mode: storagepb.TableFieldSchema_REQUIRED,
							},
						},
					},
					{
						Name: "outer_struct",
						Type: storagepb.TableFieldSchema_STRUCT,
						Mode: storagepb.TableFieldSchema_REQUIRED,
						Fields: []*storagepb.TableFieldSchema{
							{
								Name: "another_inner_struct",
								Type: storagepb.TableFieldSchema_STRUCT,
								Mode: storagepb.TableFieldSchema_REQUIRED,
								Fields: []*storagepb.TableFieldSchema{
									{
										Name: "another_leaf",
										Type: storagepb.TableFieldSchema_STRING,
										Mode: storagepb.TableFieldSchema_REQUIRED,
									},
								},
							},
							{
								Name: "reused_inner_struct_one",
								Type: storagepb.TableFieldSchema_STRUCT,
								Mode: storagepb.TableFieldSchema_REQUIRED,
								Fields: []*storagepb.TableFieldSchema{
									{
										Name: "leaf",
										Type: storagepb.TableFieldSchema_STRING,
										Mode: storagepb.TableFieldSchema_REQUIRED,
									},
								},
							},
							{
								Name: "reused_inner_struct_two",
								Type: storagepb.TableFieldSchema_STRUCT,
								Mode: storagepb.TableFieldSchema_REQUIRED,
								Fields: []*storagepb.TableFieldSchema{
									{
										Name: "leaf",
										Type: storagepb.TableFieldSchema_STRING,
										Mode: storagepb.TableFieldSchema_REQUIRED,
									},
								},
							},
						},
					},
				},
			},
			wantProto2: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("reused_inner_struct"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".root__reused_inner_struct"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
					},
					{
						Name:     new("outer_struct"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".root__outer_struct"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
					},
				},
			},
			wantProto2Normalized: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("reused_inner_struct"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new("root__reused_inner_struct"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
					},
					{
						Name:     new("outer_struct"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new("root__outer_struct"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: new("root__outer_struct"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     new("another_inner_struct"),
								Number:   proto.Int32(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
								TypeName: new("root__outer_struct__another_inner_struct"),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
							},
							{
								Name:     new("reused_inner_struct_one"),
								Number:   proto.Int32(2),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
								TypeName: new("root__reused_inner_struct"),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
							},
							{
								Name:     new("reused_inner_struct_two"),
								Number:   proto.Int32(3),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
								TypeName: new("root__reused_inner_struct"),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
							},
						},
					},
					{
						Name: new("root__outer_struct__another_inner_struct"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   new("another_leaf"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
							},
						},
					},
					{
						Name: new("root__reused_inner_struct"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   new("leaf"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
							},
						},
					},
				},
			},
			wantProto3: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("reused_inner_struct"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".root__reused_inner_struct"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("outer_struct"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".root__outer_struct"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
		},
		{
			description: "multiple nesting levels",
			bq: &storagepb.TableSchema{
				Fields: []*storagepb.TableFieldSchema{
					{
						Name: "outer_struct",
						Type: storagepb.TableFieldSchema_STRUCT,
						Mode: storagepb.TableFieldSchema_NULLABLE,
						Fields: []*storagepb.TableFieldSchema{
							{
								Name: "inner_struct",
								Type: storagepb.TableFieldSchema_STRUCT,
								Mode: storagepb.TableFieldSchema_NULLABLE,
								Fields: []*storagepb.TableFieldSchema{
									{Name: "leaf_one", Type: storagepb.TableFieldSchema_INT64, Mode: storagepb.TableFieldSchema_NULLABLE},
									{Name: "leaf_two", Type: storagepb.TableFieldSchema_INT64, Mode: storagepb.TableFieldSchema_NULLABLE},
								},
							},
						},
					},
					{
						Name: "other_field",
						Type: storagepb.TableFieldSchema_INT64,
						Mode: storagepb.TableFieldSchema_NULLABLE,
					},
				},
			},
			wantProto2: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("outer_struct"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".root__outer_struct"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:   new("other_field"),
						Number: proto.Int32(2),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
			wantProto2Normalized: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("outer_struct"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new("root__outer_struct"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:   new("other_field"),
						Number: proto.Int32(2),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: new("root__outer_struct"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     new("inner_struct"),
								Number:   proto.Int32(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
								TypeName: new("root__outer_struct__inner_struct"),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
					{
						Name: new("root__outer_struct__inner_struct"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   new("leaf_one"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
							{
								Name:   new("leaf_two"),
								Number: proto.Int32(2),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
				},
			},
			wantProto3: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("outer_struct"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".root__outer_struct"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("other_field"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".google.protobuf.Int64Value"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
		},
		{
			description: "repeated with packed",
			bq: &storagepb.TableSchema{
				Fields: []*storagepb.TableFieldSchema{
					{Name: "name", Type: storagepb.TableFieldSchema_STRING, Mode: storagepb.TableFieldSchema_NULLABLE},
					{Name: "some_lengths", Type: storagepb.TableFieldSchema_INT64, Mode: storagepb.TableFieldSchema_REPEATED},
					{Name: "nicknames", Type: storagepb.TableFieldSchema_STRING, Mode: storagepb.TableFieldSchema_REPEATED},
				}},
			wantProto2: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   new("name"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()},
					{
						Name:   new("some_lengths"),
						Number: proto.Int32(2),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
						Options: &descriptorpb.FieldOptions{
							Packed: new(true),
						},
					},
					{Name: new("nicknames"), Number: proto.Int32(3), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()},
				},
			},
		},
		{
			description: "indirect names",
			bq: &storagepb.TableSchema{
				Fields: []*storagepb.TableFieldSchema{
					{Name: "foo", Type: storagepb.TableFieldSchema_STRING, Mode: storagepb.TableFieldSchema_NULLABLE},
					{Name: "火", Type: storagepb.TableFieldSchema_INT64, Mode: storagepb.TableFieldSchema_REQUIRED},
					{Name: "水_addict", Type: storagepb.TableFieldSchema_BYTES, Mode: storagepb.TableFieldSchema_REPEATED},
					{Name: "0col", Type: storagepb.TableFieldSchema_INT64, Mode: storagepb.TableFieldSchema_NULLABLE},
					{Name: "funny-name", Type: storagepb.TableFieldSchema_INT64, Mode: storagepb.TableFieldSchema_NULLABLE},
				}},
			wantProto2: func() *descriptorpb.DescriptorProto {
				dp := &descriptorpb.DescriptorProto{
					Name: new("root"),
					Field: []*descriptorpb.FieldDescriptorProto{
						{
							Name:   new("foo"),
							Number: proto.Int32(1),
							Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
							Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()},
						{
							Name:    new("col_54Gr"),
							Number:  proto.Int32(2),
							Type:    descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
							Options: &descriptorpb.FieldOptions{},
							Label:   descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum()},
						{
							Name:    new("col_5rC0X2FkZGljdA"),
							Number:  proto.Int32(3),
							Type:    descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(),
							Options: &descriptorpb.FieldOptions{},
							Label:   descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
						},
						{
							Name:    new("col_MGNvbA"),
							Number:  proto.Int32(4),
							Type:    descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
							Options: &descriptorpb.FieldOptions{},
							Label:   descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()},
						{
							Name:    new("col_ZnVubnktbmFtZQ"),
							Number:  proto.Int32(5),
							Type:    descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
							Options: &descriptorpb.FieldOptions{},
							Label:   descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()},
					},
				}
				proto.SetExtension(dp.Field[1].Options, storagepb.E_ColumnName, "火")
				proto.SetExtension(dp.Field[2].Options, storagepb.E_ColumnName, "水_addict")
				proto.SetExtension(dp.Field[3].Options, storagepb.E_ColumnName, "0col")
				proto.SetExtension(dp.Field[4].Options, storagepb.E_ColumnName, "funny-name")
				return dp
			}(),
		},
		{
			description: "multiple nesting levels with proto mapping",
			options: []ProtoConversionOption{
				WithProtoMapping(ProtoMapping{
					FieldType: storagepb.TableFieldSchema_INT64,
					TypeName:  "google.protobuf.Int64Value",
					Type:      descriptorpb.FieldDescriptorProto_TYPE_MESSAGE,
				}), // override all INT64
				WithProtoMapping(ProtoMapping{
					FieldPath: "outer_struct.inner_struct.leaf_one",
					Type:      descriptorpb.FieldDescriptorProto_TYPE_INT64,
				}), // let deeply nested field to still use basic INT64
			},
			bq: &storagepb.TableSchema{
				Fields: []*storagepb.TableFieldSchema{
					{
						Name: "outer_struct",
						Type: storagepb.TableFieldSchema_STRUCT,
						Mode: storagepb.TableFieldSchema_NULLABLE,
						Fields: []*storagepb.TableFieldSchema{
							{
								Name: "inner_struct",
								Type: storagepb.TableFieldSchema_STRUCT,
								Mode: storagepb.TableFieldSchema_NULLABLE,
								Fields: []*storagepb.TableFieldSchema{
									{Name: "leaf_one", Type: storagepb.TableFieldSchema_INT64, Mode: storagepb.TableFieldSchema_NULLABLE},
									{Name: "leaf_two", Type: storagepb.TableFieldSchema_INT64, Mode: storagepb.TableFieldSchema_NULLABLE},
								},
							},
						},
					},
					{
						Name: "other_field",
						Type: storagepb.TableFieldSchema_INT64,
						Mode: storagepb.TableFieldSchema_NULLABLE,
					},
				},
			},
			wantProto2: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("outer_struct"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".root__outer_struct"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("other_field"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".google.protobuf.Int64Value"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
			wantProto2Normalized: &descriptorpb.DescriptorProto{
				Name: new("root"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("outer_struct"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new("root__outer_struct"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("other_field"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new("google_protobuf_Int64Value"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: new("google_protobuf_Int64Value"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:         new("value"),
								Number:       proto.Int32(1),
								DefaultValue: new("0"),
								JsonName:     new("value"),
								Type:         descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
								Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
					{
						Name: new("root__outer_struct"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     new("inner_struct"),
								Number:   proto.Int32(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
								TypeName: new("root__outer_struct__inner_struct"),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
					{
						Name: new("root__outer_struct__inner_struct"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   new("leaf_one"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
							{
								Name:     new("leaf_two"),
								Number:   proto.Int32(2),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
								TypeName: new("google_protobuf_Int64Value"),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(T *testing.T) {
			// Proto2
			p2d, err := StorageSchemaToProtoDescriptorWithOptions(tc.bq, "root", tc.options...)
			if err != nil {
				t.Fatalf("failed proto2 conversion: %v", err)
			}

			// Convert to MessageDescriptor.
			mDesc, ok := p2d.(protoreflect.MessageDescriptor)
			if !ok {
				t.Error("couldn't convert proto2 to messagedescriptor")
			}
			// Check the non-normalized case.
			if tc.wantProto2 != nil {
				gotDP := protodesc.ToDescriptorProto(mDesc)
				if diff := cmp.Diff(gotDP, tc.wantProto2, protocmp.Transform()); diff != "" {
					t.Errorf("proto2: -got, +want:\n%s", diff)
				}
			}
			// Check the normalized case.
			if tc.wantProto2Normalized != nil {
				gotDP, err := NormalizeDescriptor(mDesc)
				if err != nil {
					t.Errorf("failed to normalize: %v", err)
				}
				if diff := cmp.Diff(gotDP, tc.wantProto2Normalized, protocmp.Transform()); diff != "" {
					t.Errorf("proto2normalized: -got, +want:\n%s", diff)
				}
			}

			p3d, err := StorageSchemaToProto3Descriptor(tc.bq, "root")
			if err != nil {
				t.Fatalf("failed proto3 conversion: %v", err)
			}
			// Convert to MessageDescriptor.
			mDesc, ok = p3d.(protoreflect.MessageDescriptor)
			if !ok {
				t.Error("couldn't convert proto3 to messagedescriptor")
			}
			// Check the non-normalized case.
			if tc.wantProto3 != nil {
				gotDP := protodesc.ToDescriptorProto(mDesc)
				if diff := cmp.Diff(gotDP, tc.wantProto3, protocmp.Transform()); diff != "" {
					t.Errorf("proto3: -got, +want:\n%s", diff)
				}
			}
			// Check the normalized case.
			if tc.wantProto3Normalized != nil {
				gotDP, err := NormalizeDescriptor(mDesc)
				if err != nil {
					t.Errorf("failed to normalize: %v", err)
				}
				if diff := cmp.Diff(gotDP, tc.wantProto3Normalized, protocmp.Transform()); diff != "" {
					t.Errorf("proto3normalized: -got, +want:\n%s", diff)
				}
			}
		})
	}
}

func TestProtoJSONSerialization(t *testing.T) {
	sourceSchema := &storagepb.TableSchema{
		Fields: []*storagepb.TableFieldSchema{
			{Name: "record_id", Type: storagepb.TableFieldSchema_INT64, Mode: storagepb.TableFieldSchema_NULLABLE},
			{Name: "created_at", Type: storagepb.TableFieldSchema_TIMESTAMP, Mode: storagepb.TableFieldSchema_NULLABLE},
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

	descriptor, err := StorageSchemaToProtoDescriptorWithOptions(sourceSchema, "root",
		WithProtoMapping(ProtoMapping{
			FieldType: storagepb.TableFieldSchema_TIMESTAMP,
			TypeName:  "google.protobuf.Timestamp",
			Type:      descriptorpb.FieldDescriptorProto_TYPE_MESSAGE,
		}))
	if err != nil {
		t.Fatalf("failed to construct descriptor")
	}

	ts := time.Now().UTC().Format(time.RFC3339)
	sampleRecord := fmt.Appendf(nil, `{"record_id":"12345","created_at": "%s","details":[{"key":"name","value":"jimmy"},{"key":"title","value":"clown"}]}`, ts)

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
		Name: new("root"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{
				Name:   new("record_id"),
				Number: proto.Int32(1),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			},
			{
				Name:     new("created_at"),
				Number:   proto.Int32(2),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				TypeName: new(".google.protobuf.Timestamp"),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			},
			{
				Name:     new("details"),
				Number:   proto.Int32(3),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				TypeName: new(".root__details"),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
			},
		},
	}

	wantInnerDP := &descriptorpb.DescriptorProto{
		Name: new("root__details"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{
				Name:   new("key"),
				Number: proto.Int32(1),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
			},
			{
				Name:   new("value"),
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

	var got, want any
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
				Name: new("testdata_AllSupportedTypes"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("int32_value"),
						JsonName: new("int32Value"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("int64_value"),
						JsonName: new("int64Value"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("uint32_value"),
						JsonName: new("uint32Value"),
						Number:   proto.Int32(3),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_UINT32.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("uint64_value"),
						JsonName: new("uint64Value"),
						Number:   proto.Int32(4),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_UINT64.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("float_value"),
						JsonName: new("floatValue"),
						Number:   proto.Int32(5),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_FLOAT.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("double_value"),
						JsonName: new("doubleValue"),
						Number:   proto.Int32(6),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_DOUBLE.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("bool_value"),
						JsonName: new("boolValue"),
						Number:   proto.Int32(7),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("enum_value"),
						JsonName: new("enumValue"),
						TypeName: new("testdata_TestEnum_E.TestEnum"),
						Number:   proto.Int32(8),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("string_value"),
						JsonName: new("stringValue"),
						Number:   proto.Int32(9),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
					},
					{
						Name:     new("fixed64_value"),
						JsonName: new("fixed64Value"),
						Number:   proto.Int32(10),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_FIXED64.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: new("testdata_TestEnum_E"),
						EnumType: []*descriptorpb.EnumDescriptorProto{
							{
								Name: new("TestEnum"),
								Value: []*descriptorpb.EnumValueDescriptorProto{
									{
										Name:   new("TestEnum0"),
										Number: proto.Int32(0),
									},
									{
										Name:   new("TestEnum1"),
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
				Name: new("testdata_ComplexType"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("nested_repeated_type"),
						JsonName: new("nestedRepeatedType"),
						Number:   proto.Int32(1),
						TypeName: new("testdata_NestedType"),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
					{
						Name:     new("inner_type"),
						JsonName: new("innerType"),
						Number:   proto.Int32(2),
						TypeName: new("testdata_InnerType"),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("range_type"),
						JsonName: new("rangeType"),
						Number:   proto.Int32(3),
						TypeName: new("testdata_RangeTypeTimestamp"),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("json_type"),
						JsonName: new("jsonType"),
						Number:   proto.Int32(4),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: new("testdata_InnerType"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     new("value"),
								JsonName: new("value"),
								Number:   proto.Int32(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
							},
						},
					},
					{
						Name: new("testdata_NestedType"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     new("inner_type"),
								JsonName: new("innerType"),
								Number:   proto.Int32(1),
								TypeName: new("testdata_InnerType"),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
							},
						},
					},
					{
						Name: new("testdata_RangeTypeTimestamp"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     new("start"),
								JsonName: new("start"),
								Number:   proto.Int32(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
							{
								Name:     new("end"),
								JsonName: new("end"),
								Number:   proto.Int32(2),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
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
				Name: new("testdata_WithWellKnownTypes"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("int64_value"),
						JsonName: new("int64Value"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("wrapped_int64"),
						JsonName: new("wrappedInt64"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new("google_protobuf_Int64Value"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("string_value"),
						JsonName: new("stringValue"),
						Number:   proto.Int32(3),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
					{
						Name:     new("wrapped_string"),
						JsonName: new("wrappedString"),
						Number:   proto.Int32(4),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new("google_protobuf_StringValue"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: new("google_protobuf_Int64Value"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:         new("value"),
								JsonName:     new("value"),
								Number:       proto.Int32(1),
								DefaultValue: new("0"),
								Type:         descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
								Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
					{
						Name: new("google_protobuf_StringValue"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:         new("value"),
								JsonName:     new("value"),
								Number:       proto.Int32(1),
								DefaultValue: new(""),
								Type:         descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
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
				Name: new("testdata_WithOneOf"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("int32_value"),
						JsonName: new("int32Value"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("string_value"),
						JsonName: new("stringValue"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("double_value"),
						JsonName: new("doubleValue"),
						Number:   proto.Int32(3),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_DOUBLE.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
		},
		{
			description: "WithProto3Optional",
			in:          (&testdata.SimpleMessageProto3WithOptional{}).ProtoReflect().Descriptor(),
			want: &descriptorpb.DescriptorProto{
				Name: new("testdata_SimpleMessageProto3WithOptional"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:         new("name"),
						JsonName:     new("name"),
						Number:       proto.Int32(1),
						DefaultValue: new(""),
						Type:         descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("value"),
						JsonName: new("value"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
		},
		{
			description: "WithProto3Defaults",
			in:          (&testdata.ValidationP3Defaults{}).ProtoReflect().Descriptor(),
			want: &descriptorpb.DescriptorProto{
				Name: new("testdata_ValidationP3Defaults"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:         new("double_field"),
						JsonName:     new("doubleField"),
						Number:       proto.Int32(1),
						Type:         descriptorpb.FieldDescriptorProto_TYPE_DOUBLE.Enum(),
						Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						DefaultValue: new("0"),
					},
					{
						Name:         new("float_field"),
						JsonName:     new("floatField"),
						Number:       proto.Int32(2),
						Type:         descriptorpb.FieldDescriptorProto_TYPE_FLOAT.Enum(),
						Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						DefaultValue: new("0"),
					},
					{
						Name:         new("int32_field"),
						JsonName:     new("int32Field"),
						Number:       proto.Int32(3),
						Type:         descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
						Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						DefaultValue: new("0"),
					},
					{
						Name:         new("int64_field"),
						JsonName:     new("int64Field"),
						Number:       proto.Int32(4),
						Type:         descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
						Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						DefaultValue: new("0"),
					},
					{
						Name:         new("uint32_field"),
						JsonName:     new("uint32Field"),
						Number:       proto.Int32(5),
						Type:         descriptorpb.FieldDescriptorProto_TYPE_UINT32.Enum(),
						Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						DefaultValue: new("0"),
					},
					{
						Name:         new("sint32_field"),
						JsonName:     new("sint32Field"),
						Number:       proto.Int32(7),
						Type:         descriptorpb.FieldDescriptorProto_TYPE_SINT32.Enum(),
						Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						DefaultValue: new("0"),
					},
					{
						Name:         new("sint64_field"),
						JsonName:     new("sint64Field"),
						Number:       proto.Int32(8),
						Type:         descriptorpb.FieldDescriptorProto_TYPE_SINT64.Enum(),
						Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						DefaultValue: new("0"),
					},
					{
						Name:         new("fixed32_field"),
						JsonName:     new("fixed32Field"),
						Number:       proto.Int32(9),
						Type:         descriptorpb.FieldDescriptorProto_TYPE_FIXED32.Enum(),
						Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						DefaultValue: new("0"),
					},
					{
						Name:         new("sfixed32_field"),
						JsonName:     new("sfixed32Field"),
						Number:       proto.Int32(11),
						Type:         descriptorpb.FieldDescriptorProto_TYPE_SFIXED32.Enum(),
						Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						DefaultValue: new("0"),
					},
					{
						Name:         new("sfixed64_field"),
						JsonName:     new("sfixed64Field"),
						Number:       proto.Int32(12),
						Type:         descriptorpb.FieldDescriptorProto_TYPE_SFIXED64.Enum(),
						Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						DefaultValue: new("0"),
					},
					{
						Name:         new("bool_field"),
						JsonName:     new("boolField"),
						Number:       proto.Int32(13),
						Type:         descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum(),
						Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						DefaultValue: new("false"),
					},
					{
						Name:         new("string_field"),
						JsonName:     new("stringField"),
						Number:       proto.Int32(14),
						Type:         descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						DefaultValue: new(""),
					},
					{
						Name:         new("bytes_field"),
						JsonName:     new("bytesField"),
						Number:       proto.Int32(15),
						Type:         descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(),
						Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						DefaultValue: new(""),
					},
					{
						Name:         new("enum_field"),
						JsonName:     new("enumField"),
						Number:       proto.Int32(16),
						Type:         descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
						TypeName:     new("testdata_Proto3ExampleEnum_E.Proto3ExampleEnum"),
						Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						DefaultValue: new("P3_UNDEFINED"),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: new("testdata_Proto3ExampleEnum_E"),
						EnumType: []*descriptorpb.EnumDescriptorProto{
							{
								Name: new("Proto3ExampleEnum"),
								Value: []*descriptorpb.EnumValueDescriptorProto{
									{
										Name:   new("P3_UNDEFINED"),
										Number: proto.Int32(0),
									},
									{
										Name:   new("P3_THING"),
										Number: proto.Int32(1),
									},
									{
										Name:   new("P3_OTHER_THING"),
										Number: proto.Int32(2),
									},
									{
										Name:   new("P3_THIRD_THING"),
										Number: proto.Int32(3),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			description: "WithExternalEnum",
			in:          (&testdata.ExternalEnumMessage{}).ProtoReflect().Descriptor(),
			want: &descriptorpb.DescriptorProto{
				Name: new("testdata_ExternalEnumMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("msg_a"),
						JsonName: new("msgA"),
						Number:   proto.Int32(1),
						TypeName: new("testdata_EnumMsgA"),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("msg_b"),
						JsonName: new("msgB"),
						Number:   proto.Int32(2),
						TypeName: new("testdata_EnumMsgB"),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: new("testdata_EnumMsgA"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     new("foo"),
								JsonName: new("foo"),
								Number:   proto.Int32(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
							{
								Name:     new("bar"),
								JsonName: new("bar"),
								Number:   proto.Int32(2),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
								TypeName: new("testdata_ExtEnum_E.ExtEnum"),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
					{
						Name: new("testdata_EnumMsgB"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     new("baz"),
								JsonName: new("baz"),
								Number:   proto.Int32(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
								TypeName: new("testdata_ExtEnum_E.ExtEnum"),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
					{
						Name: new("testdata_ExtEnum_E"),
						EnumType: []*descriptorpb.EnumDescriptorProto{
							{
								Name: new("ExtEnum"),
								Value: []*descriptorpb.EnumValueDescriptorProto{
									{
										Name:   new("UNDEFINED"),
										Number: proto.Int32(0),
									},
									{
										Name:   new("THING"),
										Number: proto.Int32(1),
									},
									{
										Name:   new("OTHER_THING"),
										Number: proto.Int32(2),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			description: "OutOfOrderDefinitionProto2",
			in:          (&testdata.OutOfOrderDefinitionProto2{}).ProtoReflect().Descriptor(),
			want: &descriptorpb.DescriptorProto{
				Name: new("testdata_OutOfOrderDefinitionProto2"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("s1"),
						JsonName: new("s1"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("s2"),
						JsonName: new("s2"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("s3"),
						JsonName: new("s3"),
						Number:   proto.Int32(3),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("enum1"),
						JsonName: new("enum1"),
						Number:   proto.Int32(4),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						TypeName: new("testdata_OutOfOrderDefinitionProto2_OutOfOrderEnum_E.OutOfOrderEnum"),
					},
					{
						Name:     new("enum2"),
						JsonName: new("enum2"),
						Number:   proto.Int32(5),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						TypeName: new("testdata_OutOfOrderDefinitionProto2_OutOfOrderEnum_E.OutOfOrderEnum"),
					},
					{
						Name:     new("msg6"),
						JsonName: new("msg6"),
						Number:   proto.Int32(6),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						TypeName: new("testdata_SimpleMessageProto2"),
					},
					{
						Name:     new("msg7"),
						JsonName: new("msg7"),
						Number:   proto.Int32(7),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						TypeName: new("testdata_SimpleMessageProto2"),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: new("testdata_OutOfOrderDefinitionProto2_OutOfOrderEnum_E"),
						EnumType: []*descriptorpb.EnumDescriptorProto{
							{
								Name: new("OutOfOrderEnum"),
								Value: []*descriptorpb.EnumValueDescriptorProto{
									{
										Name:   new("E1"),
										Number: proto.Int32(1),
									},
									{
										Name:   new("E2"),
										Number: proto.Int32(2),
									},
									{
										Name:   new("E3"),
										Number: proto.Int32(3),
									},
								},
							},
						},
					},
					{
						Name: new("testdata_SimpleMessageProto2"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     new("name"),
								JsonName: new("name"),
								Number:   proto.Int32(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
							{
								Name:     new("value"),
								JsonName: new("value"),
								Number:   proto.Int32(2),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
				},
			},
		},
		{
			description: "ValidationP3PackedRepeated",
			in:          (&testdata.ValidationP3PackedRepeated{}).ProtoReflect().Descriptor(),
			want: &descriptorpb.DescriptorProto{
				Name: new("testdata_ValidationP3PackedRepeated"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("id"),
						JsonName: new("id"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     new("double_repeated"),
						JsonName: new("doubleRepeated"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_DOUBLE.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
					{
						Name:     new("float_repeated"),
						JsonName: new("floatRepeated"),
						Number:   proto.Int32(3),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_FLOAT.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
					{
						Name:     new("int32_repeated"),
						JsonName: new("int32Repeated"),
						Number:   proto.Int32(4),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
					{
						Name:     new("int64_repeated"),
						JsonName: new("int64Repeated"),
						Number:   proto.Int32(5),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
					{
						Name:     new("uint32_repeated"),
						JsonName: new("uint32Repeated"),
						Number:   proto.Int32(6),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_UINT32.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
					{
						Name:     new("sint32_repeated"),
						JsonName: new("sint32Repeated"),
						Number:   proto.Int32(7),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_SINT32.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
					{
						Name:     new("sint64_repeated"),
						JsonName: new("sint64Repeated"),
						Number:   proto.Int32(8),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_SINT64.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
					{
						Name:     new("fixed32_repeated"),
						JsonName: new("fixed32Repeated"),
						Number:   proto.Int32(9),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_FIXED32.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
					{
						Name:     new("sfixed32_repeated"),
						JsonName: new("sfixed32Repeated"),
						Number:   proto.Int32(10),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_SFIXED32.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
					{
						Name:     new("sfixed64_repeated"),
						JsonName: new("sfixed64Repeated"),
						Number:   proto.Int32(11),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_SFIXED64.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},

					{
						Name:     new("bool_repeated"),
						JsonName: new("boolRepeated"),
						Number:   proto.Int32(12),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
					{
						Name:     new("enum_repeated"),
						JsonName: new("enumRepeated"),
						Number:   proto.Int32(13),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
						TypeName: new("testdata_Proto3ExampleEnum_E.Proto3ExampleEnum"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: new("testdata_Proto3ExampleEnum_E"),
						EnumType: []*descriptorpb.EnumDescriptorProto{
							{
								Name: new("Proto3ExampleEnum"),
								Value: []*descriptorpb.EnumValueDescriptorProto{
									{
										Name:   new("P3_UNDEFINED"),
										Number: proto.Int32(0),
									},
									{
										Name:   new("P3_THING"),
										Number: proto.Int32(1),
									},
									{
										Name:   new("P3_OTHER_THING"),
										Number: proto.Int32(2),
									},
									{
										Name:   new("P3_THIRD_THING"),
										Number: proto.Int32(3),
									},
								},
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
