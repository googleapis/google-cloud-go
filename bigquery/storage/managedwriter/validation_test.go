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
	"context"
	"math"
	"testing"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter/adapt"
	"cloud.google.com/go/bigquery/storage/managedwriter/testdata"
	"google.golang.org/protobuf/proto"
)

func TestValidation_Values(t *testing.T) {

	testcases := []struct {
		description string
		tableSchema bigquery.Schema
		inputRow    proto.Message
		constraints []constraintOption
	}{
		{
			description: "proto2 optional w/nulls",
			tableSchema: testdata.ValidationBaseSchema,
			inputRow:    &testdata.ValidationP2Optional{},
			constraints: []constraintOption{
				withExactRowCount(1),
				withNullCount("double_field", 1),
				withNullCount("float_field", 1),
				withNullCount("int32_field", 1),
				withNullCount("int64_field", 1),
				withNullCount("uint32_field", 1),
				withNullCount("sint32_field", 1),
				withNullCount("sint64_field", 1),
				withNullCount("fixed32_field", 1),
				withNullCount("sfixed32_field", 1),
				withNullCount("sfixed64_field", 1),
				withNullCount("bool_field", 1),
				withNullCount("string_field", 1),
				withNullCount("bytes_field", 1),
				withNullCount("enum_field", 1),
			},
		},
		{
			description: "proto2 optionals w/values",
			tableSchema: testdata.ValidationBaseSchema,
			inputRow: &testdata.ValidationP2Optional{
				DoubleField:   proto.Float64(math.Inf(1)),
				FloatField:    proto.Float32(2.0),
				Int32Field:    proto.Int32(11),
				Int64Field:    proto.Int64(-22),
				Uint32Field:   proto.Uint32(365),
				Sint32Field:   proto.Int32(123),
				Sint64Field:   proto.Int64(45),
				Fixed32Field:  proto.Uint32(1000),
				Sfixed32Field: proto.Int32(999),
				Sfixed64Field: proto.Int64(33),
				BoolField:     proto.Bool(true),
				StringField:   proto.String("test"),
				BytesField:    []byte("some byte data"),
				EnumField:     testdata.Proto2ExampleEnum_P2_THING.Enum(),
			},
			constraints: []constraintOption{
				withExactRowCount(1),
				withFloatValueCount("double_field", math.Inf(1), 1),
				withFloatValueCount("float_field", 2.0, 1),
				withIntegerValueCount("int32_field", 11, 1),
				withIntegerValueCount("int64_field", -22, 1),
				withIntegerValueCount("uint32_field", 365, 1),
				withIntegerValueCount("sint32_field", 123, 1),
				withIntegerValueCount("sint64_field", 45, 1),
				withIntegerValueCount("fixed32_field", 1000, 1),
				withIntegerValueCount("sfixed32_field", 999, 1),
				withIntegerValueCount("sfixed64_field", 33, 1),
				withBoolValueCount("bool_field", true, 1),
				withStringValueCount("string_field", "test", 1),
				withBytesValueCount("bytes_field", []byte("some byte data"), 1),
				withIntegerValueCount("enum_field", int64(testdata.Proto2ExampleEnum_P2_THING), 1),
			},
		},
		{
			description: "proto2 required",
			tableSchema: testdata.ValidationBaseSchema,
			inputRow: &testdata.ValidationP2Required{
				DoubleField:   proto.Float64(math.Inf(1)),
				FloatField:    proto.Float32(2.0),
				Int32Field:    proto.Int32(11),
				Int64Field:    proto.Int64(-22),
				Uint32Field:   proto.Uint32(365),
				Sint32Field:   proto.Int32(123),
				Sint64Field:   proto.Int64(45),
				Fixed32Field:  proto.Uint32(1000),
				Sfixed32Field: proto.Int32(999),
				Sfixed64Field: proto.Int64(33),
				BoolField:     proto.Bool(true),
				StringField:   proto.String("test"),
				BytesField:    []byte("some byte data"),
				EnumField:     testdata.Proto2ExampleEnum_P2_THING.Enum(),
			},
			constraints: []constraintOption{
				withExactRowCount(1),
				withFloatValueCount("double_field", math.Inf(1), 1),
				withFloatValueCount("float_field", 2.0, 1),
				withIntegerValueCount("int32_field", 11, 1),
				withIntegerValueCount("int64_field", -22, 1),
				withIntegerValueCount("uint32_field", 365, 1),
				withIntegerValueCount("sint32_field", 123, 1),
				withIntegerValueCount("sint64_field", 45, 1),
				withIntegerValueCount("fixed32_field", 1000, 1),
				withIntegerValueCount("sfixed32_field", 999, 1),
				withIntegerValueCount("sfixed64_field", 33, 1),
				withBoolValueCount("bool_field", true, 1),
				withStringValueCount("string_field", "test", 1),
				withBytesValueCount("bytes_field", []byte("some byte data"), 1),
				withIntegerValueCount("enum_field", int64(testdata.Proto2ExampleEnum_P2_THING), 1),
			},
		},
		{
			description: "proto2 default values w/nulls",
			tableSchema: testdata.ValidationBaseSchema,
			inputRow:    &testdata.ValidationP2OptionalWithDefaults{},
			constraints: []constraintOption{
				withExactRowCount(1),
				withFloatValueCount("double_field", 1.11, 1),
				withFloatValueCount("float_field", 2.22, 1),
				withIntegerValueCount("int32_field", 3, 1),
				withIntegerValueCount("int64_field", 4, 1),
				withIntegerValueCount("uint32_field", 5, 1),
				withIntegerValueCount("sint32_field", 7, 1),
				withIntegerValueCount("sint64_field", 8, 1),
				withIntegerValueCount("fixed32_field", 9, 1),
				withIntegerValueCount("sfixed32_field", 11, 1),
				withIntegerValueCount("sfixed64_field", 12, 1),
				withBoolValueCount("bool_field", true, 1),
				withStringValueCount("string_field", "custom default", 1),
				withBytesValueCount("bytes_field", []byte("optional bytes"), 1),
				withIntegerValueCount("enum_field", int64(testdata.Proto2ExampleEnum_P2_OTHER_THING), 1),
			},
		},

		{
			description: "proto3 default values",
			tableSchema: testdata.ValidationBaseSchema,
			inputRow:    &testdata.ValidationP3Defaults{},
			constraints: []constraintOption{
				withExactRowCount(1),
				withFloatValueCount("double_field", 0, 1),
				withFloatValueCount("float_field", 0, 1),
				withIntegerValueCount("int32_field", 0, 1),
				withIntegerValueCount("int64_field", 0, 1),
				withIntegerValueCount("uint32_field", 0, 1),
				withIntegerValueCount("sint32_field", 0, 1),
				withIntegerValueCount("sint64_field", 0, 1),
				withIntegerValueCount("fixed32_field", 0, 1),
				withIntegerValueCount("sfixed32_field", 0, 1),
				withIntegerValueCount("sfixed64_field", 0, 1),
				withBoolValueCount("bool_field", false, 1),
				withStringValueCount("string_field", "", 1),
				withBytesValueCount("bytes_field", []byte(""), 1),
				withIntegerValueCount("enum_field", int64(0), 1),
			},
		},
		/*
			BACKEND BEHAVIOR FOR WRAPPER TYPES CURRENTLY INCORRECT
			{
				description: "proto3 with wrapper types",
				tableSchema: testdata.ValidationBaseSchema,
				inputRow: &testdata.ValidationP3Wrappers{
					DoubleField: &wrapperspb.DoubleValue{Value: 1.0},
				},
				constraints: []constraintOption{
					withExactRowCount(1),
					withFloatValueCount("double_field", 0, 1),
				},
			},
		*/
		{
			description: "proto3 optional presence w/o explicit values",
			tableSchema: testdata.ValidationBaseSchema,
			inputRow:    &testdata.ValidationP3Optional{},
			constraints: []constraintOption{
				withExactRowCount(1),
				withNullCount("double_field", 1),
				withNullCount("float_field", 1),
				withNullCount("int32_field", 1),
				withNullCount("int64_field", 1),
				withNullCount("uint32_field", 1),
				withNullCount("sint32_field", 1),
				withNullCount("sint64_field", 1),
				withNullCount("fixed32_field", 1),
				withNullCount("sfixed32_field", 1),
				withNullCount("sfixed64_field", 1),
				withNullCount("bool_field", 1),
				withNullCount("string_field", 1),
				withNullCount("bytes_field", 1),
				withNullCount("enum_field", 1),
			},
		},
	}

	// Common setup.
	mwClient, bqClient := getTestClients(context.Background(), t)
	defer mwClient.Close()
	defer bqClient.Close()

	dataset, cleanup, err := setupTestDataset(context.Background(), t, bqClient, "us-east1")
	if err != nil {
		t.Fatalf("failed to init test dataset: %v", err)
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()

	for _, tc := range testcases {
		// Define a test table for each row.
		testTable := dataset.Table(tableIDs.New())
		if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: tc.tableSchema}); err != nil {
			t.Errorf("%s: failed to create test table %q: %v", tc.description, testTable.FullyQualifiedName(), err)
			continue
		}

		// normalize the proto schema based on the provided message.
		descriptor, err := adapt.NormalizeDescriptor(tc.inputRow.ProtoReflect().Descriptor())
		if err != nil {
			t.Errorf("%s: failed to normalize descriptor: %v", tc.description, err)
			continue
		}
		// setup a new stream.
		ms, err := mwClient.NewManagedStream(ctx,
			WithDestinationTable(TableParentFromParts(testTable.ProjectID, testTable.DatasetID, testTable.TableID)),
			WithType(DefaultStream),
			WithSchemaDescriptor(descriptor),
		)
		if err != nil {
			t.Errorf("%s: NewManagedStream: %v", tc.description, err)
			continue
		}

		// serialize message to wire format and send append.
		b, err := proto.Marshal(tc.inputRow)
		if err != nil {
			t.Errorf("%s failed proto.Marshall(): %v", tc.description, err)
			continue
		}
		data := [][]byte{b}
		result, err := ms.AppendRows(ctx, data)
		if err != nil {
			t.Errorf("%s append failed: %v", tc.description, err)
			continue
		}
		if _, err = result.GetResult(ctx); err != nil {
			t.Errorf("%s append response error: %v", tc.description, err)
			continue
		}
		// Validate table.
		validateTableConstraints(ctx, t, bqClient, testTable, tc.description, tc.constraints...)
	}
}
