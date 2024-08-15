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
	"encoding/json"
	"fmt"
	"testing"

	btapb "cloud.google.com/go/bigtable/admin/apiv2/adminpb"
	"github.com/google/go-cmp/cmp"
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
	it := Int64Type{}
	got := it.proto()
	if !proto.Equal(got, want) {
		t.Errorf("got type %v, want: %v", got, want)
	}

	gotJSON, err := json.Marshal(it)
	if err != nil {
		t.Fatalf("Error calling json.Marshal: %v", err)
	}
	wantJSON := "{\"int64Type\":{\"encoding\":{\"bigEndianBytes\":{}}}}"
	if string(gotJSON) != wantJSON {
		t.Errorf("got %q, want %q", string(gotJSON), wantJSON)
	}
	var result Int64Type
	if err := json.Unmarshal(gotJSON, &result); err != nil {
		t.Fatalf("Error calling json.Unmarshal: %v", err)
	}
	if diff := cmp.Diff(result, it); diff != "" {
		t.Errorf("Unexpected diff: \n%s", diff)
	}
	if !result.Equal(it) {
		t.Errorf("Unexpected result. Got %#v, want %#v", result, it)
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
	st := StringType{}
	got := st.proto()
	if !proto.Equal(got, want) {
		t.Errorf("got type %v, want: %v", got, want)
	}

	gotJSON, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("Error calling ToJSON: %v", err)
	}
	wantJSON := "{\"stringType\":{\"encoding\":{\"utf8Raw\":{}}}}"
	if string(gotJSON) != wantJSON {
		t.Errorf("got %q, want %q", string(gotJSON), wantJSON)
	}
	var result StringType
	if err := json.Unmarshal(gotJSON, &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}
	if diff := cmp.Diff(result, st); diff != "" {
		t.Errorf("Unexpected diff: \n%s", diff)
	}
	if !result.Equal(st) {
		t.Errorf("Unexpected result. Got %#v, want %#v", result, st)
	}
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
		fn       string
		agg      Aggregator
		protoAgg btapb.Type_Aggregate
	}{{
		name: "hll",
		fn:   "hllppUniqueCount",
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
			fn:   "min",
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
			fn:   "max",
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
			fn:   "sum",
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

			got := at.proto()
			if !proto.Equal(got, want) {
				t.Errorf("got type %v, want: %v", got, want)
			}

			gotJSON, err := json.Marshal(at)
			if err != nil {
				t.Fatalf("Error calling ToJSON: %v", err)
			}
			wantJSON := fmt.Sprintf("{\"aggregateType\":{\"inputType\":{\"int64Type\":{\"encoding\":{\"bigEndianBytes\":{}}}},\"%s\":{}}}", tc.fn)
			if string(gotJSON) != wantJSON {
				t.Errorf("unexpected different JSON got %q, want %q", string(gotJSON), wantJSON)
			}
			var result AggregateType
			if err := json.Unmarshal(gotJSON, &result); err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}
			if diff := cmp.Diff(result, at); diff != "" {
				t.Errorf("Unexpected diff: \n%s", diff)
			}
			if !result.Equal(at) {
				t.Errorf("Unexpected result. Got %#v, want %#v", result, at)
			}
		})
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
