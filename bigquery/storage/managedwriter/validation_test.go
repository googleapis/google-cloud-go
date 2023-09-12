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
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
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
		{
			description: "proto2 unpacked repeated non-empty",
			tableSchema: testdata.ValidationRepeatedSchema,
			inputRow: &testdata.ValidationP2UnpackedRepeated{
				Id:               proto.Int64(2022),
				DoubleRepeated:   []float64{1.1, 2.2, 3.3},
				FloatRepeated:    []float32{-4.4, -5.5, -6.6, -7.7},
				Int32Repeated:    []int32{2, 4, 6, 6, 10},
				Int64Repeated:    []int64{100, 200, 300, 300, 300, 600, 700},
				Uint32Repeated:   []uint32{8675309, 8675309, 8675309, 8675309, 8675309, 8675309, 8675309},
				Sint32Repeated:   []int32{-1, -2, -3, 4, 5, 6},
				Sint64Repeated:   []int64{2},
				Fixed32Repeated:  []uint32{},
				Sfixed32Repeated: []int32{-88, 100, -99, -88},
				Sfixed64Repeated: []int64{88, -100, 99, -2020},
				BoolRepeated:     []bool{true, true, false, true},
				EnumRepeated:     []testdata.Proto2ExampleEnum{testdata.Proto2ExampleEnum_P2_OTHER_THING, testdata.Proto2ExampleEnum_P2_THIRD_THING, testdata.Proto2ExampleEnum_P2_OTHER_THING},
			},
			constraints: []constraintOption{
				withExactRowCount(1),

				withArrayLength("double_repeated", 3, 1),
				withArrayLength("float_repeated", 4, 1),
				withArrayLength("int32_repeated", 5, 1),
				withArrayLength("int64_repeated", 7, 1),
				withArrayLength("uint32_repeated", 7, 1),
				withArrayLength("sint32_repeated", 6, 1),
				withArrayLength("sint64_repeated", 1, 1),
				withArrayLength("fixed32_repeated", 0, 1),
				withArrayLength("sfixed32_repeated", 4, 1),
				withArrayLength("sfixed64_repeated", 4, 1),
				withArrayLength("bool_repeated", 4, 1),
				withArrayLength("enum_repeated", 3, 1),

				withDistinctArrayValues("double_repeated", 3, 1),
				withDistinctArrayValues("float_repeated", 4, 1),
				withDistinctArrayValues("int32_repeated", 4, 1),
				withDistinctArrayValues("int64_repeated", 5, 1),
				withDistinctArrayValues("uint32_repeated", 1, 1),
				withDistinctArrayValues("sint32_repeated", 6, 1),
				withDistinctArrayValues("sint64_repeated", 1, 1),
				withDistinctArrayValues("fixed32_repeated", 0, 1),
				withDistinctArrayValues("sfixed32_repeated", 3, 1),
				withDistinctArrayValues("sfixed64_repeated", 4, 1),
				withDistinctArrayValues("bool_repeated", 2, 1),
				withDistinctArrayValues("enum_repeated", 2, 1),

				withFloatArraySum("double_repeated", 6.6, 1),
				withFloatArraySum("float_repeated", -24.2, 1),

				withIntegerArraySum("int32_repeated", 28, 1),
				withIntegerArraySum("int64_repeated", 2500, 1),
				withIntegerArraySum("uint32_repeated", 60727163, 1),
				withIntegerArraySum("sint32_repeated", 9, 1),
				withIntegerArraySum("sint64_repeated", 2, 1),
				withIntegerArraySum("enum_repeated", 7, 1),
			},
		},
		{
			description: "proto2 packed repeated non-empty",
			tableSchema: testdata.ValidationRepeatedSchema,
			inputRow: &testdata.ValidationP2PackedRepeated{
				Id:               proto.Int64(2022),
				DoubleRepeated:   []float64{1.1, 2.2, 3.3},
				FloatRepeated:    []float32{-4.4, -5.5, -6.6, -7.7},
				Int32Repeated:    []int32{2, 4, 6, 6, 10},
				Int64Repeated:    []int64{100, 200, 300, 300, 300, 600, 700},
				Uint32Repeated:   []uint32{8675309, 8675309, 8675309, 8675309, 8675309, 8675309, 8675309},
				Sint32Repeated:   []int32{-1, -2, -3, 4, 5, 6},
				Sint64Repeated:   []int64{2},
				Fixed32Repeated:  []uint32{},
				Sfixed32Repeated: []int32{-88, 100, -99, -88},
				Sfixed64Repeated: []int64{88, -100, 99, -2020},
				BoolRepeated:     []bool{true, true, false, true},
				EnumRepeated:     []testdata.Proto2ExampleEnum{testdata.Proto2ExampleEnum_P2_OTHER_THING, testdata.Proto2ExampleEnum_P2_THIRD_THING, testdata.Proto2ExampleEnum_P2_OTHER_THING},
			},
			constraints: []constraintOption{
				withExactRowCount(1),

				withArrayLength("double_repeated", 3, 1),
				withArrayLength("float_repeated", 4, 1),
				withArrayLength("int32_repeated", 5, 1),
				withArrayLength("int64_repeated", 7, 1),
				withArrayLength("uint32_repeated", 7, 1),
				withArrayLength("sint32_repeated", 6, 1),
				withArrayLength("sint64_repeated", 1, 1),
				withArrayLength("fixed32_repeated", 0, 1),
				withArrayLength("sfixed32_repeated", 4, 1),
				withArrayLength("sfixed64_repeated", 4, 1),
				withArrayLength("bool_repeated", 4, 1),
				withArrayLength("enum_repeated", 3, 1),

				withDistinctArrayValues("double_repeated", 3, 1),
				withDistinctArrayValues("float_repeated", 4, 1),
				withDistinctArrayValues("int32_repeated", 4, 1),
				withDistinctArrayValues("int64_repeated", 5, 1),
				withDistinctArrayValues("uint32_repeated", 1, 1),
				withDistinctArrayValues("sint32_repeated", 6, 1),
				withDistinctArrayValues("sint64_repeated", 1, 1),
				withDistinctArrayValues("fixed32_repeated", 0, 1),
				withDistinctArrayValues("sfixed32_repeated", 3, 1),
				withDistinctArrayValues("sfixed64_repeated", 4, 1),
				withDistinctArrayValues("bool_repeated", 2, 1),
				withDistinctArrayValues("enum_repeated", 2, 1),

				withFloatArraySum("double_repeated", 6.6, 1),
				withFloatArraySum("float_repeated", -24.2, 1),

				withIntegerArraySum("int32_repeated", 28, 1),
				withIntegerArraySum("int64_repeated", 2500, 1),
				withIntegerArraySum("uint32_repeated", 60727163, 1),
				withIntegerArraySum("sint32_repeated", 9, 1),
				withIntegerArraySum("sint64_repeated", 2, 1),
				withIntegerArraySum("enum_repeated", 7, 1),
			},
		},
		{
			description: "proto3 packed repeated non-empty",
			tableSchema: testdata.ValidationRepeatedSchema,
			inputRow: &testdata.ValidationP3PackedRepeated{
				Id:               proto.Int64(2022),
				DoubleRepeated:   []float64{1.1, 2.2, 3.3},
				FloatRepeated:    []float32{-4.4, -5.5, -6.6, -7.7},
				Int32Repeated:    []int32{2, 4, 6, 6, 10},
				Int64Repeated:    []int64{100, 200, 300, 300, 300, 600, 700},
				Uint32Repeated:   []uint32{8675309, 8675309, 8675309, 8675309, 8675309, 8675309, 8675309},
				Sint32Repeated:   []int32{-1, -2, -3, 4, 5, 6},
				Sint64Repeated:   []int64{2},
				Fixed32Repeated:  []uint32{},
				Sfixed32Repeated: []int32{-88, 100, -99, -88},
				Sfixed64Repeated: []int64{88, -100, 99, -2020},
				BoolRepeated:     []bool{true, true, false, true},
				EnumRepeated:     []testdata.Proto3ExampleEnum{testdata.Proto3ExampleEnum_P3_OTHER_THING, testdata.Proto3ExampleEnum_P3_THIRD_THING, testdata.Proto3ExampleEnum_P3_OTHER_THING},
			},
			constraints: []constraintOption{
				withExactRowCount(1),

				withArrayLength("double_repeated", 3, 1),
				withArrayLength("float_repeated", 4, 1),
				withArrayLength("int32_repeated", 5, 1),
				withArrayLength("int64_repeated", 7, 1),
				withArrayLength("uint32_repeated", 7, 1),
				withArrayLength("sint32_repeated", 6, 1),
				withArrayLength("sint64_repeated", 1, 1),
				withArrayLength("fixed32_repeated", 0, 1),
				withArrayLength("sfixed32_repeated", 4, 1),
				withArrayLength("sfixed64_repeated", 4, 1),
				withArrayLength("bool_repeated", 4, 1),
				withArrayLength("enum_repeated", 3, 1),

				withDistinctArrayValues("double_repeated", 3, 1),
				withDistinctArrayValues("float_repeated", 4, 1),
				withDistinctArrayValues("int32_repeated", 4, 1),
				withDistinctArrayValues("int64_repeated", 5, 1),
				withDistinctArrayValues("uint32_repeated", 1, 1),
				withDistinctArrayValues("sint32_repeated", 6, 1),
				withDistinctArrayValues("sint64_repeated", 1, 1),
				withDistinctArrayValues("fixed32_repeated", 0, 1),
				withDistinctArrayValues("sfixed32_repeated", 3, 1),
				withDistinctArrayValues("sfixed64_repeated", 4, 1),
				withDistinctArrayValues("bool_repeated", 2, 1),
				withDistinctArrayValues("enum_repeated", 2, 1),

				withFloatArraySum("double_repeated", 6.6, 1),
				withFloatArraySum("float_repeated", -24.2, 1),

				withIntegerArraySum("int32_repeated", 28, 1),
				withIntegerArraySum("int64_repeated", 2500, 1),
				withIntegerArraySum("uint32_repeated", 60727163, 1),
				withIntegerArraySum("sint32_repeated", 9, 1),
				withIntegerArraySum("sint64_repeated", 2, 1),
				withIntegerArraySum("enum_repeated", 7, 1),
			},
		},
		{
			description: "proto2 w/column annotations",
			tableSchema: testdata.ValidationColumnAnnotations,
			inputRow: &testdata.ValidationP2ColumnAnnotations{
				First:  proto.String("first_val"),
				Second: proto.String("second_val"),
				Third:  proto.String("third_val"),
			},
			constraints: []constraintOption{
				withExactRowCount(1),
				withStringValueCount("first", "first_val", 1),
				withStringValueCount("second", "third_val", 1),
				withStringValueCount("特別コラム", "second_val", 1),
			},
		},
	}

	// Common setup.
	mwClient, bqClient := getTestClients(context.Background(), t)
	defer mwClient.Close()
	defer bqClient.Close()

	dataset, cleanup, err := setupTestDataset(context.Background(), t, bqClient, "us-east4")
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

func TestValidationRoundtripRepeated(t *testing.T) {
	// This test exists to confirm packed values are backwards compatible.
	// We create a message using a packed descriptor, and normalize it which
	// loses the packed option, and confirm we can decode the values using the
	// normalized descriptor.
	input := &testdata.ValidationP3PackedRepeated{
		Id:            proto.Int64(2022),
		Int64Repeated: []int64{1, 2, 4, -88},
	}
	b, err := proto.Marshal(input)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}

	// Verify original packed option (proto3 is default packed)
	origDescriptor := input.ProtoReflect().Descriptor()
	origFD := origDescriptor.Fields().ByName(protoreflect.Name("int64_repeated"))
	if !origFD.IsPacked() {
		t.Errorf("expected original field to be packed, wasn't")
	}

	// Normalize and use it to get a new descriptor.
	normalized, err := adapt.NormalizeDescriptor(input.ProtoReflect().Descriptor())
	if err != nil {
		t.Fatalf("NormalizeDescriptor: %v", err)
	}
	fdp := &descriptorpb.FileDescriptorProto{
		MessageType: []*descriptorpb.DescriptorProto{normalized},
		Name:        proto.String("lookup"),
		Syntax:      proto.String("proto2"),
	}
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{fdp},
	}
	files, err := protodesc.NewFiles(fds)
	if err != nil {
		t.Fatalf("protodesc.NewFiles: %v", err)
	}
	found, err := files.FindDescriptorByName("testdata_ValidationP3PackedRepeated")
	if err != nil {
		t.Fatalf("FindDescriptorByName: %v", err)
	}
	md := found.(protoreflect.MessageDescriptor)

	// Use the new, normalized descriptor to unmarshal the bytes and verify.
	msg := dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(b, msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	//
	int64FD := msg.Descriptor().Fields().ByName(protoreflect.Name("int64_repeated")) // int64_repeated again
	if int64FD == nil {
		t.Fatalf("failed to get field")
	}
	if int64FD.IsPacked() {
		t.Errorf("expected normalized descriptor to be un-packed, but it was packed")
	}
	// Ensure we got the expected values out the other side.
	list := msg.Get(int64FD).List()
	wantLen := 4
	if list.Len() != wantLen {
		t.Errorf("wanted %d values, got %d", wantLen, list.Len())
	}
	// Confirm the same values out the other side.
	wantVals := []int64{1, 2, 4, -88}
	for i := 0; i < list.Len(); i++ {
		got := list.Get(i).Int()
		if got != wantVals[i] {
			t.Errorf("expected elem %d to be %d, was %d", i, wantVals[i], got)
		}
	}
}
