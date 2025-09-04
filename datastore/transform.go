// Copyright 2025 Google LLC
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
	"reflect"

	pb "cloud.google.com/go/datastore/apiv1/datastorepb"
)

// PropertyTransform represents a server-side transformation to be applied to a property.
// Instances should be created using the specific transform functions (e.g., Increment,
// SetToServerTime, AppendMissingElements).
type PropertyTransform struct {
	pb  *pb.PropertyTransform
	err error
}

// Increment returns a PropertyTransform that atomically adds the given numeric value
// to the property's current numeric value.
// Accepted types for value: int, int8, int16, int32, int64, uint8, uint16, uint32, float32, float64.
// fieldName is the name of the property to transform.
func Increment(fieldName string, value interface{}) PropertyTransform {
	pbVal, err := toNumericValue(value)
	if err != nil {
		return PropertyTransform{err: fmt.Errorf("datastore: Increment: %w", err)}
	}
	return PropertyTransform{
		pb: &pb.PropertyTransform{
			Property: fieldName,
			TransformType: &pb.PropertyTransform_Increment{
				Increment: pbVal,
			},
		},
	}
}

// SetToServerTime returns a PropertyTransform that sets the property
// to the time at which the server processed the request (REQUEST_TIME).
// fieldName is the name of the property to transform.
func SetToServerTime(fieldName string) PropertyTransform {
	return PropertyTransform{
		pb: &pb.PropertyTransform{
			Property: fieldName,
			TransformType: &pb.PropertyTransform_SetToServerValue{
				SetToServerValue: pb.PropertyTransform_REQUEST_TIME,
			},
		},
	}
}

// Maximum returns a PropertyTransform that atomically sets the property to the maximum
// of its current numeric value and the given numeric value.
// Accepted types for value: int, int8, int16, int32, int64, uint8, uint16, uint32, float32, float64.
// fieldName is the name of the property to transform.
func Maximum(fieldName string, value interface{}) PropertyTransform {
	pbVal, err := toNumericValue(value)
	if err != nil {
		return PropertyTransform{err: fmt.Errorf("datastore: Maximum: %w", err)}
	}
	return PropertyTransform{
		pb: &pb.PropertyTransform{
			Property: fieldName,
			TransformType: &pb.PropertyTransform_Maximum{
				Maximum: pbVal,
			},
		},
	}
}

// Minimum returns a PropertyTransform that atomically sets the property to the minimum
// of its current numeric value and the given numeric value.
// Accepted types for value: int, int8, int16, int32, int64, uint8, uint16, uint32, float32, float64.
// fieldName is the name of the property to transform.
func Minimum(fieldName string, value interface{}) PropertyTransform {
	pbVal, err := toNumericValue(value)
	if err != nil {
		return PropertyTransform{err: fmt.Errorf("datastore: Minimum: %w", err)}
	}
	return PropertyTransform{
		pb: &pb.PropertyTransform{
			Property: fieldName,
			TransformType: &pb.PropertyTransform_Minimum{
				Minimum: pbVal,
			},
		},
	}
}

// AppendMissingElements returns a PropertyTransform that atomically appends the given elements
// to an array property, only if they are not already present.
// If the property is not an array, or if it does not yet exist,
// it is first set to the empty array.
// fieldName is the name of the property to transform.
// Returns an error if any element cannot be converted to a Datastore Value.
func AppendMissingElements(fieldName string, elements ...interface{}) PropertyTransform {
	pbValues := make([]*pb.Value, len(elements))
	for i, el := range elements {
		vProto, err := interfaceToProto(el, false) // NoIndex is false for elements in array for matching
		if err != nil {
			return PropertyTransform{err: fmt.Errorf("datastore: AppendMissingElements: cannot convert element at index %d for field '%s': %w", i, fieldName, err)}
		}
		pbValues[i] = vProto
	}

	return PropertyTransform{
		pb: &pb.PropertyTransform{
			Property: fieldName,
			TransformType: &pb.PropertyTransform_AppendMissingElements{
				AppendMissingElements: &pb.ArrayValue{Values: pbValues},
			},
		},
	}
}

// RemoveAllFromArray returns a PropertyTransform that atomically removes all occurrences
// of the given elements from an array in the property.
// If the property is not an array, or if it does not yet exist,
// it is first set to the empty array.
// fieldName is the name of the property to transform.
// Returns an error if any element cannot be converted to a Datastore Value.
func RemoveAllFromArray(fieldName string, elements ...interface{}) PropertyTransform {
	pbValues := make([]*pb.Value, len(elements))
	for i, el := range elements {
		vProto, err := interfaceToProto(el, false) // NoIndex is false for elements in array for matching
		if err != nil {
			return PropertyTransform{err: fmt.Errorf("datastore: RemoveAllFromArray: cannot convert element at index %d for field '%s': %w", i, fieldName, err)}
		}
		pbValues[i] = vProto
	}
	return PropertyTransform{
		pb: &pb.PropertyTransform{
			Property: fieldName,
			TransformType: &pb.PropertyTransform_RemoveAllFromArray{
				RemoveAllFromArray: &pb.ArrayValue{Values: pbValues},
			},
		},
	}
}

// toNumericValue converts an interface{} to a protobuf Value with either
// IntegerValue or DoubleValue.
// It accepts int, int8, int16, int32, int64, uint8, uint16, uint32, float32, float64.
func toNumericValue(n interface{}) (*pb.Value, error) {
	v := reflect.ValueOf(n)
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: v.Int()}}, nil
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: int64(v.Uint())}}, nil
	case reflect.Float32, reflect.Float64:
		return &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: v.Float()}}, nil
	// uint, uint64, uintptr are excluded as they can overflow int64
	default:
		return nil, fmt.Errorf("datastore: unsupported type for numeric transform: %T", n)
	}
}
