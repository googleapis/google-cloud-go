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

	btapb "google.golang.org/genproto/googleapis/bigtable/admin/v2"
	"google.golang.org/protobuf/proto"
)

func TestInt64Proto(t *testing.T) {
	want := &btapb.Type{
		Kind: &btapb.Type_Int64Type{
			Int64Type: &btapb.Type_Int64{
				Encoding: &btapb.Type_Int64_Encoding{
					Encoding: &btapb.Type_Int64_Encoding_BigEndianBytes_{
						BigEndianBytes: &btapb.Type_Int64_Encoding_BigEndianBytes{
							BytesType: &btapb.Type_Bytes{
								Encoding: &btapb.Type_Bytes_Encoding{
									Encoding: &btapb.Type_Bytes_Encoding_Raw_{
										Raw: &btapb.Type_Bytes_Encoding_Raw{},
									},
								},
							},
						},
					},
				},
			},
		},
	}

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

func TestAggregateProto(t *testing.T) {
	want := &btapb.Type{
		Kind: &btapb.Type_AggregateType{
			AggregateType: &btapb.Type_Aggregate{
				InputType: &btapb.Type{
					Kind: &btapb.Type_Int64Type{
						Int64Type: &btapb.Type_Int64{
							Encoding: &btapb.Type_Int64_Encoding{
								Encoding: &btapb.Type_Int64_Encoding_BigEndianBytes_{
									BigEndianBytes: &btapb.Type_Int64_Encoding_BigEndianBytes{
										BytesType: &btapb.Type_Bytes{
											Encoding: &btapb.Type_Bytes_Encoding{
												Encoding: &btapb.Type_Bytes_Encoding_Raw_{
													Raw: &btapb.Type_Bytes_Encoding_Raw{},
												},
											},
										},
									},
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
