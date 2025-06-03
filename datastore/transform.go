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

	pb "cloud.google.com/go/datastore/apiv1/datastorepb"
)

// PropertyTransform represents a server-side transformation to be applied to a property.
// Instances should be created using the specific transform functions (e.g., Increment,
// SetToServerTime, AppendMissingElements).
type PropertyTransform struct {
	pb *pb.PropertyTransform
}

// Increment returns a PropertyTransform that atomically adds the given integer value
// to the property's current numeric value.
// If the property does not exist or is not numeric, it's set to the given value.
// fieldName is the name of the property to transform.
func Increment(fieldName string, value int64) PropertyTransform {
	return PropertyTransform{
		pb: &pb.PropertyTransform{
			Property: fieldName,
			TransformType: &pb.PropertyTransform_Increment{
				Increment: &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: value}},
			},
		},
	}
}

// IncrementFloat returns a PropertyTransform that atomically adds the given float value
// to the property's current numeric value.
// If the property does not exist or is not numeric, it's set to the given value.
// Double arithmetic follows IEEE 754 semantics.
// fieldName is the name of the property to transform.
func IncrementFloat(fieldName string, value float64) PropertyTransform {
	return PropertyTransform{
		pb: &pb.PropertyTransform{
			Property: fieldName,
			TransformType: &pb.PropertyTransform_Increment{
				Increment: &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: value}},
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
// of its current numeric value and the given integer value.
// If the property does not exist or is not numeric, it's set to the given value.
// fieldName is the name of the property to transform.
func Maximum(fieldName string, value int64) PropertyTransform {
	return PropertyTransform{
		pb: &pb.PropertyTransform{
			Property: fieldName,
			TransformType: &pb.PropertyTransform_Maximum{
				Maximum: &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: value}},
			},
		},
	}
}

// MaximumFloat returns a PropertyTransform that atomically sets the property to the maximum
// of its current numeric value and the given float value.
// If the property does not exist or is not numeric, it's set to the given value.
// fieldName is the name of the property to transform.
func MaximumFloat(fieldName string, value float64) PropertyTransform {
	return PropertyTransform{
		pb: &pb.PropertyTransform{
			Property: fieldName,
			TransformType: &pb.PropertyTransform_Maximum{
				Maximum: &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: value}},
			},
		},
	}
}

// Minimum returns a PropertyTransform that atomically sets the property to the minimum
// of its current numeric value and the given integer value.
// fieldName is the name of the property to transform.
func Minimum(fieldName string, value int64) PropertyTransform {
	return PropertyTransform{
		pb: &pb.PropertyTransform{
			Property: fieldName,
			TransformType: &pb.PropertyTransform_Minimum{
				Minimum: &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: value}},
			},
		},
	}
}

// MinimumFloat returns a PropertyTransform that atomically sets the property to the minimum
// of its current numeric value and the given float value.
// fieldName is the name of the property to transform.
func MinimumFloat(fieldName string, value float64) PropertyTransform {
	return PropertyTransform{
		pb: &pb.PropertyTransform{
			Property: fieldName,
			TransformType: &pb.PropertyTransform_Minimum{
				Minimum: &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: value}},
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
func AppendMissingElements(fieldName string, elements ...interface{}) (PropertyTransform, error) {
	pbValues := make([]*pb.Value, len(elements))
	for i, el := range elements {
		vProto, err := interfaceToProto(el, false)
		if err != nil {
			return PropertyTransform{}, fmt.Errorf("datastore: AppendMissingElements: cannot convert element at index %d for field '%s': %w", i, fieldName, err)
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
	}, nil
}

// RemoveAllFromArray returns a PropertyTransform that atomically removes all occurrences
// of the given elements from an array in the property.
// If the property is not an array, or if it does not yet exist,
// it is first set to the empty array.
// fieldName is the name of the property to transform.
// Returns an error if any element cannot be converted to a Datastore Value.
func RemoveAllFromArray(fieldName string, elements ...interface{}) (PropertyTransform, error) {
	pbValues := make([]*pb.Value, len(elements))
	for i, el := range elements {
		vProto, err := interfaceToProto(el, false)
		if err != nil {
			return PropertyTransform{}, fmt.Errorf("datastore: RemoveAllFromArray: cannot convert element at index %d for field '%s': %w", i, fieldName, err)
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
	}, nil
}
