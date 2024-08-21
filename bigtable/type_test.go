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

func TestInt64Proto(t *testing.T) {
	want := aggregateProto()
	got := Int64Type{}.proto()
	if !proto.Equal(got, want) {
		t.Errorf("got type %v, want: %v", got, want)
	}
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

	got := StringType{}.proto()
	if !proto.Equal(got, want) {
		t.Errorf("got type %v, want: %v", got, want)
	}
}

func TestSumAggregateProto(t *testing.T) {
	want := &btapb.Type{
		Kind: &btapb.Type_AggregateType{
			AggregateType: &btapb.Type_Aggregate{
				InputType: &btapb.Type{
					Kind: &btapb.Type_Int64Type{
						Int64Type: &btapb.Type_Int64{
							Encoding: &btapb.Type_Int64_Encoding{
								Encoding: &btapb.Type_Int64_Encoding_BigEndianBytes_{
									BigEndianBytes: &btapb.Type_Int64_Encoding_BigEndianBytes{},
								},
							},
						},
					},
				},
				Aggregator: &btapb.Type_Aggregate_Sum_{
					Sum: &btapb.Type_Aggregate_Sum{},
				},
			},
		},
	}

	got := AggregateType{Input: Int64Type{}, Aggregator: SumAggregator{}}.proto()
	if !proto.Equal(got, want) {
		t.Errorf("got type %v, want: %v", got, want)
	}
}

func TestProtoBijection(t *testing.T) {
	want := aggregateProto()
	got := ProtoToType(want).proto()
	if !proto.Equal(got, want) {
		t.Errorf("got type %v, want: %v", got, want)
	}
}

func TestMinAggregateProto(t *testing.T) {
	want := &btapb.Type{
		Kind: &btapb.Type_AggregateType{
			AggregateType: &btapb.Type_Aggregate{
				InputType: &btapb.Type{
					Kind: &btapb.Type_Int64Type{
						Int64Type: &btapb.Type_Int64{
							Encoding: &btapb.Type_Int64_Encoding{
								Encoding: &btapb.Type_Int64_Encoding_BigEndianBytes_{
									BigEndianBytes: &btapb.Type_Int64_Encoding_BigEndianBytes{},
								},
							},
						},
					},
				},
				Aggregator: &btapb.Type_Aggregate_Min_{
					Min: &btapb.Type_Aggregate_Min{},
				},
			},
		},
	}

	got := AggregateType{Input: Int64Type{}, Aggregator: MinAggregator{}}.proto()
	if !proto.Equal(got, want) {
		t.Errorf("got type %v, want: %v", got, want)
	}
}

func TestMaxAggregateProto(t *testing.T) {
	want := &btapb.Type{
		Kind: &btapb.Type_AggregateType{
			AggregateType: &btapb.Type_Aggregate{
				InputType: &btapb.Type{
					Kind: &btapb.Type_Int64Type{
						Int64Type: &btapb.Type_Int64{
							Encoding: &btapb.Type_Int64_Encoding{
								Encoding: &btapb.Type_Int64_Encoding_BigEndianBytes_{
									BigEndianBytes: &btapb.Type_Int64_Encoding_BigEndianBytes{},
								},
							},
						},
					},
				},
				Aggregator: &btapb.Type_Aggregate_Max_{
					Max: &btapb.Type_Aggregate_Max{},
				},
			},
		},
	}

	got := AggregateType{Input: Int64Type{}, Aggregator: MaxAggregator{}}.proto()
	if !proto.Equal(got, want) {
		t.Errorf("got type %v, want: %v", got, want)
	}
}

func TestHllAggregateProto(t *testing.T) {
	want := &btapb.Type{
		Kind: &btapb.Type_AggregateType{
			AggregateType: &btapb.Type_Aggregate{
				InputType: &btapb.Type{
					Kind: &btapb.Type_Int64Type{
						Int64Type: &btapb.Type_Int64{
							Encoding: &btapb.Type_Int64_Encoding{
								Encoding: &btapb.Type_Int64_Encoding_BigEndianBytes_{
									BigEndianBytes: &btapb.Type_Int64_Encoding_BigEndianBytes{},
								},
							},
						},
					},
				},
				Aggregator: &btapb.Type_Aggregate_HllppUniqueCount{
					HllppUniqueCount: &btapb.Type_Aggregate_HyperLogLogPlusPlusUniqueCount{},
				},
			},
		},
	}

	got := AggregateType{Input: Int64Type{}, Aggregator: HllppUniqueCountAggregator{}}.proto()
	if !proto.Equal(got, want) {
		t.Errorf("got type %v, want: %v", got, want)
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
	aggType1, ok := aggregateProtoToType(nil).(AggregateType)
	if !ok {
		t.Fatalf("got: %T, wanted AggregateType", aggType1)
	}
	if val, ok := aggType1.Aggregator.(unknownAggregator); !ok {
		t.Errorf("got: %T, wanted unknownAggregator", val)
	}
	if aggType1.Input != nil {
		t.Errorf("got: %v, wanted nil", aggType1.Input)
	}

	aggType2, ok := aggregateProtoToType(&btapb.Type_Aggregate{}).(AggregateType)
	if !ok {
		t.Fatalf("got: %T, wanted AggregateType", aggType2)
	}
	if val, ok := aggType2.Aggregator.(unknownAggregator); !ok {
		t.Errorf("got: %T, wanted unknownAggregator", val)
	}
	if val, ok := aggType2.Input.(unknown[btapb.Type]); !ok {
		t.Errorf("got: %T, wanted unknown[btapb.Type]", val)
	}
}
