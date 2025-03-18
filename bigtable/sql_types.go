/*
Copyright 2025 Google LLC

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
	"errors"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SQLType represents the type of data that can be used to query Cloud Bigtable.
// It is based on the GoogleSQL standard.
type SQLType interface {
	// Used while preparing the query
	typeProto() (*btpb.Type, error)

	// Used while binding parameters to prepared query
	dataProto() (*btpb.Value, error)
}

// BytesSQLType represents a slice of bytes.
type BytesSQLType struct {
	value *[]byte
}

func (s BytesSQLType) dataProto() (*btpb.Value, error) {
	if s.value == nil {
		return &btpb.Value{}, nil
	}

	return &btpb.Value{
		Kind: &btpb.Value_BytesValue{
			BytesValue: *s.value,
		},
	}, nil
}

func (s BytesSQLType) typeProto() (*btpb.Type, error) {
	return &btpb.Type{
		Kind: &btpb.Type_BytesType{
			BytesType: &btpb.Type_Bytes{},
		},
	}, nil
}

// StringSQLType represents a string.
type StringSQLType struct {
	value *string
}

func (s StringSQLType) dataProto() (*btpb.Value, error) {
	if s.value == nil {
		return &btpb.Value{}, nil
	}
	return &btpb.Value{
		Kind: &btpb.Value_StringValue{
			StringValue: *s.value,
		},
	}, nil
}

func (s StringSQLType) typeProto() (*btpb.Type, error) {
	return &btpb.Type{
		Kind: &btpb.Type_StringType{
			StringType: &btpb.Type_String{},
		},
	}, nil
}

// Int64SQLType represents an 8-byte integer.
type Int64SQLType struct {
	value *int64
}

func (s Int64SQLType) dataProto() (*btpb.Value, error) {
	if s.value == nil {
		return &btpb.Value{}, nil
	}
	return &btpb.Value{
		Kind: &btpb.Value_IntValue{
			IntValue: *s.value,
		},
	}, nil
}

func (s Int64SQLType) typeProto() (*btpb.Type, error) {
	return &btpb.Type{
		Kind: &btpb.Type_Int64Type{
			Int64Type: &btpb.Type_Int64{},
		},
	}, nil
}

// Float32SQLType represents a 32-bit floating-point number.
type Float32SQLType struct {
	value *float32
}

func (s Float32SQLType) dataProto() (*btpb.Value, error) {
	if s.value == nil {
		return &btpb.Value{}, nil
	}
	return &btpb.Value{
		Kind: &btpb.Value_FloatValue{
			FloatValue: float64(*s.value),
		},
	}, nil
}

func (s Float32SQLType) typeProto() (*btpb.Type, error) {
	return &btpb.Type{
		Kind: &btpb.Type_Float32Type{
			Float32Type: &btpb.Type_Float32{},
		},
	}, nil
}

// Float64SQLType represents a 64-bit floating-point number.
type Float64SQLType struct {
	value *float64
}

func (s Float64SQLType) dataProto() (*btpb.Value, error) {
	if s.value == nil {
		return &btpb.Value{}, nil
	}
	return &btpb.Value{
		Kind: &btpb.Value_FloatValue{
			FloatValue: float64(*s.value),
		},
	}, nil
}

func (s Float64SQLType) typeProto() (*btpb.Type, error) {
	return &btpb.Type{
		Kind: &btpb.Type_Float64Type{
			Float64Type: &btpb.Type_Float64{},
		},
	}, nil
}

// BoolSQLType represents a boolean.
type BoolSQLType struct {
	value *bool
}

func (s BoolSQLType) dataProto() (*btpb.Value, error) {
	if s.value == nil {
		return &btpb.Value{}, nil
	}
	return &btpb.Value{
		Kind: &btpb.Value_BoolValue{
			BoolValue: *s.value,
		},
	}, nil
}

func (s BoolSQLType) typeProto() (*btpb.Type, error) {
	return &btpb.Type{
		Kind: &btpb.Type_BoolType{
			BoolType: &btpb.Type_Bool{},
		},
	}, nil
}

// TimestampSQLType represents a point in time.
type TimestampSQLType struct {
	value *time.Time
}

func (s TimestampSQLType) dataProto() (*btpb.Value, error) {
	if s.value == nil {
		return &btpb.Value{}, nil
	}
	return &btpb.Value{
		Kind: &btpb.Value_TimestampValue{
			TimestampValue: timestamppb.New(*s.value),
		},
	}, nil
}

func (s TimestampSQLType) typeProto() (*btpb.Type, error) {
	return &btpb.Type{
		Kind: &btpb.Type_TimestampType{
			TimestampType: &btpb.Type_Timestamp{},
		},
	}, nil
}

// DateSQLType represents a calendar date.
type DateSQLType struct {
	value *date.Date
}

func (s DateSQLType) dataProto() (*btpb.Value, error) {
	if s.value == nil {
		return &btpb.Value{}, nil
	}
	return &btpb.Value{
		Kind: &btpb.Value_DateValue{
			DateValue: s.value,
		},
	}, nil
}
func (s DateSQLType) typeProto() (*btpb.Type, error) {
	return &btpb.Type{
		Kind: &btpb.Type_DateType{
			DateType: &btpb.Type_Date{},
		},
	}, nil
}

// ArraySQLType represents an ordered list of elements of a given type.
type ArraySQLType struct {
	ElemType SQLType
	value    []SQLType
}

func (s ArraySQLType) dataProto() (*btpb.Value, error) {
	if s.value == nil {
		return &btpb.Value{}, nil
	}
	var values []*btpb.Value
	for _, v := range s.value {
		btpbVal, err := v.dataProto()
		if err != nil {
			return nil, err
		}
		values = append(values, btpbVal)
	}
	return &btpb.Value{
		Kind: &btpb.Value_ArrayValue{
			ArrayValue: &btpb.ArrayValue{
				Values: values,
			},
		},
	}, nil
}

func (s ArraySQLType) typeProto() (*btpb.Type, error) {
	if s.ElemType == nil {
		return nil, errors.New("must specify an explicit element type")
	}

	tp, err := s.ElemType.typeProto()
	if err != nil {
		return nil, err
	}

	return &btpb.Type{
		Kind: &btpb.Type_ArrayType{
			ArrayType: &btpb.Type_Array{
				ElementType: tp,
			},
		},
	}, nil
}
