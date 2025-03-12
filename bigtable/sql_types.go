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
	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

// SqlType represents the type of data that can be used to query Cloud Bigtable.
// It is heavily based on the GoogleSQL standard to help maintain
// familiarity and consistency across products and features.
type SqlType interface {
	// Used while preparing the query
	typeProto() *btpb.Type
}

// BytesSqlType represents a slice of bytes
type BytesSqlType struct{}

func (s BytesSqlType) typeProto() *btpb.Type {
	return &btpb.Type{
		Kind: &btpb.Type_BytesType{
			BytesType: &btpb.Type_Bytes{},
		},
	}
}

// StringSqlType represents a string
type StringSqlType struct {
}

func (s StringSqlType) typeProto() *btpb.Type {
	return &btpb.Type{
		Kind: &btpb.Type_StringType{
			StringType: &btpb.Type_String{},
		},
	}
}

// Int64SqlType represents an 8-byte integer.
type Int64SqlType struct{}

func (s Int64SqlType) typeProto() *btpb.Type {
	return &btpb.Type{
		Kind: &btpb.Type_Int64Type{
			Int64Type: &btpb.Type_Int64{},
		},
	}
}

// Float32SqlType represents a 32-bit floating-point number
type Float32SqlType struct{}

func (s Float32SqlType) typeProto() *btpb.Type {
	return &btpb.Type{
		Kind: &btpb.Type_Float32Type{
			Float32Type: &btpb.Type_Float32{},
		},
	}
}

// Float64SqlType represents a 64-bit floating-point number
type Float64SqlType struct{}

func (s Float64SqlType) typeProto() *btpb.Type {
	return &btpb.Type{
		Kind: &btpb.Type_Float64Type{
			Float64Type: &btpb.Type_Float64{},
		},
	}
}

// BoolSqlType represents a boolean
type BoolSqlType struct{}

func (s BoolSqlType) typeProto() *btpb.Type {
	return &btpb.Type{
		Kind: &btpb.Type_BoolType{
			BoolType: &btpb.Type_Bool{},
		},
	}
}

// TimestampSqlType represents a point in time
type TimestampSqlType struct{}

func (s TimestampSqlType) typeProto() *btpb.Type {
	return &btpb.Type{
		Kind: &btpb.Type_TimestampType{
			TimestampType: &btpb.Type_Timestamp{},
		},
	}
}

// DateSqlType represents a calendar date
type DateSqlType struct{}

func (s DateSqlType) typeProto() *btpb.Type {
	return &btpb.Type{
		Kind: &btpb.Type_DateType{
			DateType: &btpb.Type_Date{},
		},
	}
}

// ArraySqlType represents an ordered list of elements of a given type
type ArraySqlType struct {
	ElemType SqlType
}

func (s ArraySqlType) typeProto() *btpb.Type {
	return &btpb.Type{
		Kind: &btpb.Type_ArrayType{
			ArrayType: &btpb.Type_Array{
				ElementType: s.ElemType.typeProto(),
			},
		},
	}
}
