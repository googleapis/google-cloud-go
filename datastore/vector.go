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

package datastore

import (
	"fmt"

	pb "cloud.google.com/go/datastore/apiv1/datastorepb"
)

const (
	meaningVector = 31
)

var nullValue = &pb.Value{ValueType: &pb.Value_NullValue{}}

// Vector64 is an embedding vector of float64s.
type Vector64 []float64

// Vector32 is an embedding vector of float32s.
type Vector32 []float32

// vectorToProtoValue returns a Datastore [pb.Value] representing the Vector.
func vectorToProtoValue[T float32 | float64](v []T) *pb.Value {
	if v == nil {
		return nullValue
	}
	pbVals := make([]*pb.Value, len(v))
	for i, val := range v {
		pbVals[i] = floatToProtoValue(float64(val))
	}

	return &pb.Value{
		ValueType: &pb.Value_ArrayValue{
			ArrayValue: &pb.ArrayValue{Values: pbVals},
		},
		Meaning:            meaningVector,
		ExcludeFromIndexes: true,
	}
}

func pbValToVectorVals(v *pb.Value) ([]*pb.Value, error) {
	/*
		Vector is stored as:
		{
			"value": []float64{},
			"meaning": 31
		}
	*/
	if v == nil {
		return nil, nil
	}
	if v.Meaning != meaningVector {
		return nil, fmt.Errorf("datastore: Meaning field in %+v is not %v", v, meaningVector)
	}
	pbArr, ok := v.ValueType.(*pb.Value_ArrayValue)
	if !ok {
		return nil, fmt.Errorf("datastore: failed to convert %v to *pb.Value_ArrayValue", v.ValueType)
	}

	return pbArr.ArrayValue.Values, nil
}

func floatToProtoValue(f float64) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: f}}
}
