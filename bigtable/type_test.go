/*
Copyright 2024 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bigtable

import (
	"testing"

	btapb "cloud.google.com/go/bigtable/admin/apiv2/adminpb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/proto"
)

func aggregateProto() *btapb.Type {
	return &btapb.Type{
		Kind: &btapb.Type_Int64Type{
			Int64Type: &btapb.Type_Int64{
				Encoding: &btapb.Type_Int64_Encoding{
					Encoding: &btapb.Type_Int64_Encoding_BigEndianBytes_{
						BigEndianBytes: &btapb.Type_Int64_Encoding_BigEndianBytes{},
					},
				},
			},
		},
	}
}

func TestUnknown(t *testing.T) {
	unsupportedType := &btapb.Type{
		Kind: &btapb.Type_Float64Type{
			Float64Type: &btapb.Type_Float64{},
		},
	}
	got, ok := ProtoToType(unsupportedType).(unknown[btapb.Type])
	if !ok {
		t.Errorf("got: %T, wanted unknown[btapb.Type]", got)
	}

	assertType(t, got, unsupportedType)
}

func TestInt64Proto(t *testing.T) {
	want := aggregateProto()
	it := Int64Type{Encoding: BigEndianBytesEncoding{}}

	assertType(t, it, want)
}

func TestStringProto(t *testing.T) {
	want := &btapb.Type{
		Kind: &btapb.Type_StringType{
			StringType: &btapb.Type_String{
				Encoding: &btapb.Type_String_Encoding{
					Encoding: &btapb.Type_String_Encoding_Utf8Raw_{},
				},
			},
		},
	}
	st := StringType{Encoding: StringUtf8Encoding{}}

	assertType(t, st, want)
}

func TestProtoBijection(t *testing.T) {
	want := aggregateProto()
	got := ProtoToType(want).proto()
	if !proto.Equal(got, want) {
		t.Errorf("got type %v, want: %v", got, want)
	}
}

func TestAggregateProto(t *testing.T) {
	intType := &btapb.Type{
		Kind: &btapb.Type_Int64Type{
			Int64Type: &btapb.Type_Int64{
				Encoding: &btapb.Type_Int64_Encoding{
					Encoding: &btapb.Type_Int64_Encoding_BigEndianBytes_{
						BigEndianBytes: &btapb.Type_Int64_Encoding_BigEndianBytes{},
					},
				},
			},
		},
	}

	testCases := []struct {
		name     string
		agg      Aggregator
		protoAgg btapb.Type_Aggregate
	}{
		{
			name: "hll",
			agg:  HllppUniqueCountAggregator{},
			protoAgg: btapb.Type_Aggregate{
				InputType: intType,
				Aggregator: &btapb.Type_Aggregate_HllppUniqueCount{
					HllppUniqueCount: &btapb.Type_Aggregate_HyperLogLogPlusPlusUniqueCount{},
				},
			},
		},
		{
			name: "min",
			agg:  MinAggregator{},
			protoAgg: btapb.Type_Aggregate{
				InputType: intType,
				Aggregator: &btapb.Type_Aggregate_Min_{
					Min: &btapb.Type_Aggregate_Min{},
				},
			},
		},
		{
			name: "max",
			agg:  MaxAggregator{},
			protoAgg: btapb.Type_Aggregate{
				InputType: intType,
				Aggregator: &btapb.Type_Aggregate_Max_{
					Max: &btapb.Type_Aggregate_Max{},
				},
			},
		},
		{
			name: "sum",
			agg:  SumAggregator{},
			protoAgg: btapb.Type_Aggregate{
				InputType: intType,
				Aggregator: &btapb.Type_Aggregate_Sum_{
					Sum: &btapb.Type_Aggregate_Sum{},
				},
			},
		}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			want := &btapb.Type{
				Kind: &btapb.Type_AggregateType{
					AggregateType: &tc.protoAgg,
				},
			}
			at := AggregateType{Input: Int64Type{Encoding: BigEndianBytesEncoding{}}, Aggregator: tc.agg}

			assertType(t, at, want)
		})
	}
}

func assertType(t *testing.T, ty Type, want *btapb.Type) {
	t.Helper()

	got := ty.proto()
	if !proto.Equal(got, want) {
		t.Errorf("got type %v, want: %v", got, want)
	}

	gotJSON, err := MarshalJSON(ty)
	if err != nil {
		t.Fatalf("Error calling MarshalJSON: %v", err)
	}
	result, err := UnmarshalJSON(gotJSON)
	if err != nil {
		t.Fatalf("Error calling UnmarshalJSON: %v", err)
	}
	if diff := cmp.Diff(result, ty, cmpopts.IgnoreUnexported(unknown[btapb.Type]{})); diff != "" {
		t.Errorf("Unexpected diff: \n%s", diff)
	}
	if !Equal(result, ty) {
		t.Errorf("Unexpected result. Got %#v, want %#v", result, ty)
	}
}

func TestNilChecks(t *testing.T) {
	// ProtoToType
	if val, ok := ProtoToType(nil).(unknown[btapb.Type]); !ok {
		t.Errorf("got: %T, wanted unknown[btapb.Type]", val)
	}
	if val, ok := ProtoToType(&btapb.Type{}).(unknown[btapb.Type]); !ok {
		t.Errorf("got: %T, wanted unknown[btapb.Type]", val)
	}

	// bytesEncodingProtoToType
	if val, ok := bytesEncodingProtoToType(nil).(unknown[btapb.Type_Bytes_Encoding]); !ok {
		t.Errorf("got: %T, wanted unknown[btapb.Type_Bytes_Encoding]", val)
	}
	if val, ok := bytesEncodingProtoToType(&btapb.Type_Bytes_Encoding{}).(unknown[btapb.Type_Bytes_Encoding]); !ok {
		t.Errorf("got: %T, wanted unknown[btapb.Type_Bytes_Encoding]", val)
	}

	// int64EncodingProtoToEncoding
	if val, ok := int64EncodingProtoToEncoding(nil).(unknown[btapb.Type_Int64_Encoding]); !ok {
		t.Errorf("got: %T, wanted unknown[btapb.Type_Int64_Encoding]", val)
	}
	if val, ok := int64EncodingProtoToEncoding(&btapb.Type_Int64_Encoding{}).(unknown[btapb.Type_Int64_Encoding]); !ok {
		t.Errorf("got: %T, wanted unknown[btapb.Type_Int64_Encoding]", val)
	}

	// aggregateProtoToType
	aggType1 := aggregateProtoToType(nil)
	if val, ok := aggType1.Aggregator.(unknownAggregator); !ok {
		t.Errorf("got: %T, wanted unknownAggregator", val)
	}
	if aggType1.Input != nil {
		t.Errorf("got: %v, wanted nil", aggType1.Input)
	}

	aggType2 := aggregateProtoToType(&btapb.Type_Aggregate{})
	if val, ok := aggType2.Aggregator.(unknownAggregator); !ok {
		t.Errorf("got: %T, wanted unknownAggregator", val)
	}
	if val, ok := aggType2.Input.(unknown[btapb.Type]); !ok {
		t.Errorf("got: %T, wanted unknown[btapb.Type]", val)
	}
}
