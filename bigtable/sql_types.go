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

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

// SQLType represents the type of data that can be used to query Cloud Bigtable.
// It is based on the GoogleSQL standard.
type SQLType interface {
	// Used while preparing the query
	typeProto() (*btpb.Type, error)
}

// BytesSQLType represents a slice of bytes.
type BytesSQLType struct{}

func (s BytesSQLType) typeProto() (*btpb.Type, error) {
	return &btpb.Type{
		Kind: &btpb.Type_BytesType{
			BytesType: &btpb.Type_Bytes{},
		},
	}, nil
}

// StringSQLType represents a string.
type StringSQLType struct {
}

func (s StringSQLType) typeProto() (*btpb.Type, error) {
	return &btpb.Type{
		Kind: &btpb.Type_StringType{
			StringType: &btpb.Type_String{},
		},
	}, nil
}

// Int64SQLType represents an 8-byte integer.
type Int64SQLType struct{}

func (s Int64SQLType) typeProto() (*btpb.Type, error) {
	return &btpb.Type{
		Kind: &btpb.Type_Int64Type{
			Int64Type: &btpb.Type_Int64{},
		},
	}, nil
}

// Float32SQLType represents a 32-bit floating-point number.
type Float32SQLType struct{}

func (s Float32SQLType) typeProto() (*btpb.Type, error) {
	return &btpb.Type{
		Kind: &btpb.Type_Float32Type{
			Float32Type: &btpb.Type_Float32{},
		},
	}, nil
}

// Float64SQLType represents a 64-bit floating-point number.
type Float64SQLType struct{}

func (s Float64SQLType) typeProto() (*btpb.Type, error) {
	return &btpb.Type{
		Kind: &btpb.Type_Float64Type{
			Float64Type: &btpb.Type_Float64{},
		},
	}, nil
}

// BoolSQLType represents a boolean.
type BoolSQLType struct{}

func (s BoolSQLType) typeProto() (*btpb.Type, error) {
	return &btpb.Type{
		Kind: &btpb.Type_BoolType{
			BoolType: &btpb.Type_Bool{},
		},
	}, nil
}

// TimestampSQLType represents a point in time.
type TimestampSQLType struct{}

func (s TimestampSQLType) typeProto() (*btpb.Type, error) {
	return &btpb.Type{
		Kind: &btpb.Type_TimestampType{
			TimestampType: &btpb.Type_Timestamp{},
		},
	}, nil
}

// DateSQLType represents a calendar date.
type DateSQLType struct{}

func (s DateSQLType) typeProto() (*btpb.Type, error) {
	return &btpb.Type{
		Kind: &btpb.Type_DateType{
			DateType: &btpb.Type_Date{},
		},
	}, nil
}

// ArraySQLType represents an ordered list of elements of a given type.
type ArraySQLType struct {
	ElementType SQLType
}

func (s ArraySQLType) typeProto() (*btpb.Type, error) {
	if s.ElementType == nil {
		return nil, errors.New("must specify an explicit element type")
	}

	tp, err := s.ElementType.typeProto()
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
