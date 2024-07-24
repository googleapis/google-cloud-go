// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package firestore

import (
	"testing"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestVectorToProtoValue(t *testing.T) {
	tests := []struct {
		name string
		v    Vector64
		want *pb.Value
	}{
		{
			name: "nil vector",
			v:    nil,
			want: nullValue,
		},
		{
			name: "empty vector",
			v:    Vector64{},
			want: &pb.Value{
				ValueType: &pb.Value_MapValue{
					MapValue: &pb.MapValue{
						Fields: map[string]*pb.Value{
							typeKey: stringToProtoValue(typeValVector),
							valueKey: {
								ValueType: &pb.Value_ArrayValue{
									ArrayValue: &pb.ArrayValue{Values: []*pb.Value{}},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "multiple element vector",
			v:    Vector64{1.0, 2.0, 3.0},
			want: &pb.Value{
				ValueType: &pb.Value_MapValue{
					MapValue: &pb.MapValue{
						Fields: map[string]*pb.Value{
							typeKey: stringToProtoValue(typeValVector),
							valueKey: {
								ValueType: &pb.Value_ArrayValue{
									ArrayValue: &pb.ArrayValue{Values: []*pb.Value{floatToProtoValue(1.0), floatToProtoValue(2.0), floatToProtoValue(3.0)}},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vectorToProtoValue(tt.v)
			if !testEqual(got, tt.want) {
				t.Errorf("vectorToProtoValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVectorFromProtoValue(t *testing.T) {
	tests := []struct {
		name    string
		v       *pb.Value
		want    Vector64
		wantErr bool
	}{
		{
			name: "nil value",
			v:    nil,
			want: nil,
		},
		{
			name: "empty vector",
			v: &pb.Value{
				ValueType: &pb.Value_MapValue{
					MapValue: &pb.MapValue{
						Fields: map[string]*pb.Value{
							typeKey: stringToProtoValue(typeValVector),
							valueKey: {
								ValueType: &pb.Value_ArrayValue{
									ArrayValue: &pb.ArrayValue{Values: []*pb.Value{}},
								},
							},
						},
					},
				},
			},
			want: Vector64{},
		},
		{
			name: "multiple element vector",
			v: &pb.Value{
				ValueType: &pb.Value_MapValue{
					MapValue: &pb.MapValue{
						Fields: map[string]*pb.Value{
							typeKey: stringToProtoValue(typeValVector),
							valueKey: {
								ValueType: &pb.Value_ArrayValue{
									ArrayValue: &pb.ArrayValue{Values: []*pb.Value{floatToProtoValue(1.0), floatToProtoValue(2.0), floatToProtoValue(3.0)}},
								},
							},
						},
					},
				},
			},
			want: Vector64{1.0, 2.0, 3.0},
		},
		{
			name: "invalid type",
			v: &pb.Value{
				ValueType: &pb.Value_MapValue{
					MapValue: &pb.MapValue{
						Fields: map[string]*pb.Value{
							typeKey: stringToProtoValue("invalid_type"),
							valueKey: {
								ValueType: &pb.Value_ArrayValue{
									ArrayValue: &pb.ArrayValue{Values: []*pb.Value{floatToProtoValue(1.0), floatToProtoValue(2.0), floatToProtoValue(3.0)}},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing type",
			v: &pb.Value{
				ValueType: &pb.Value_MapValue{
					MapValue: &pb.MapValue{
						Fields: map[string]*pb.Value{
							valueKey: {
								ValueType: &pb.Value_ArrayValue{
									ArrayValue: &pb.ArrayValue{Values: []*pb.Value{floatToProtoValue(1.0), floatToProtoValue(2.0), floatToProtoValue(3.0)}},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing value",
			v: &pb.Value{
				ValueType: &pb.Value_MapValue{
					MapValue: &pb.MapValue{
						Fields: map[string]*pb.Value{
							typeKey: stringToProtoValue(typeValVector),
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid value",
			v: &pb.Value{
				ValueType: &pb.Value_MapValue{
					MapValue: &pb.MapValue{
						Fields: map[string]*pb.Value{
							typeKey: stringToProtoValue(typeValVector),
							valueKey: {
								ValueType: &pb.Value_StringValue{
									StringValue: "invalid_value",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := vector64FromProtoValue(tt.v)
			if (err != nil) != tt.wantErr {
				t.Errorf("vectorFromProtoValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if !cmp.Equal(got, tt.want, cmpopts.EquateEmpty()) {
				t.Errorf("vectorFromProtoValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStringFromProtoValue(t *testing.T) {
	tests := []struct {
		name    string
		v       *pb.Value
		want    string
		wantErr bool
	}{
		{
			name:    "nil value",
			v:       nil,
			wantErr: true,
		},
		{
			name: "string value",
			v: &pb.Value{
				ValueType: &pb.Value_StringValue{
					StringValue: "test_string",
				},
			},
			want: "test_string",
		},
		{
			name: "invalid value",
			v: &pb.Value{
				ValueType: &pb.Value_IntegerValue{
					IntegerValue: 123,
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := stringFromProtoValue(tt.v)
			if (err != nil) != tt.wantErr {
				t.Errorf("stringFromProtoValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Errorf("stringFromProtoValue() = %v, want %v", got, tt.want)
			}
		})
	}
}
