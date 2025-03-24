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
	"reflect"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/civil"
	"google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var int64ReflectType = reflect.TypeOf(int64(0))

// SQLType represents the type of data that can be used to query Cloud Bigtable.
// It is based on the GoogleSQL standard.
type SQLType interface {
	// Used while preparing the query
	typeProto() (*btpb.Type, error)

	// Used while binding parameters to prepared query
	valueProto(value any, optElemType SQLType) (*btpb.Value, error)

	isValidArrayElemType() bool
}

// BytesSQLType represents a slice of bytes.
type BytesSQLType struct{}

func (s BytesSQLType) isValidArrayElemType() bool {
	return true
}

// valid value can be of type []byte or nil.
func (s BytesSQLType) valueProto(value any, _ SQLType) (*btpb.Value, error) {
	pbType, err := BytesSQLType{}.typeProto()
	if err != nil {
		return nil, err
	}

	if value == nil {
		return &btpb.Value{
			Type: pbType,
		}, nil
	}

	typedVal, ok := value.([]byte)
	if !ok {
		return nil, &errTypeMismatch{value: value, psType: BytesSQLType{}}
	}
	return &btpb.Value{
		Type: pbType,
		Kind: &btpb.Value_BytesValue{
			BytesValue: typedVal,
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
type StringSQLType struct{}

func (s StringSQLType) isValidArrayElemType() bool {
	return true
}

// valid value can be of type string or nil.
func (s StringSQLType) valueProto(value any, _ SQLType) (*btpb.Value, error) {
	pbType, err := StringSQLType{}.typeProto()
	if err != nil {
		return nil, err
	}
	if value == nil {
		return &btpb.Value{
			Type: pbType,
		}, nil
	}

	typedVal, ok := value.(string)
	if !ok {
		return nil, &errTypeMismatch{value: value, psType: StringSQLType{}}
	}
	return &btpb.Value{
		Type: pbType,
		Kind: &btpb.Value_StringValue{
			StringValue: typedVal,
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
type Int64SQLType struct{}

func (s Int64SQLType) isValidArrayElemType() bool {
	return true
}

// valid value can be of type int64 or nil.
func (s Int64SQLType) valueProto(value any, _ SQLType) (*btpb.Value, error) {
	pbType, err := Int64SQLType{}.typeProto()
	if err != nil {
		return nil, err
	}
	if value == nil {
		return &btpb.Value{
			Type: pbType,
		}, nil
	}

	reflectVal := reflect.ValueOf(value)
	if reflectVal.CanConvert(int64ReflectType) {
		typedVal := reflectVal.Convert(int64ReflectType).Int()
		return &btpb.Value{
			Type: pbType,
			Kind: &btpb.Value_IntValue{
				IntValue: typedVal,
			},
		}, nil
	}

	return nil, &errTypeMismatch{value: value, psType: Int64SQLType{}}
}
func (s Int64SQLType) typeProto() (*btpb.Type, error) {
	return &btpb.Type{
		Kind: &btpb.Type_Int64Type{
			Int64Type: &btpb.Type_Int64{},
		},
	}, nil
}

// Float32SQLType represents a 32-bit floating-point number.
type Float32SQLType struct{}

func (s Float32SQLType) isValidArrayElemType() bool {
	return true
}

// valid value can be of type float32 or nil.
func (s Float32SQLType) valueProto(value any, _ SQLType) (*btpb.Value, error) {
	pbType, err := Float32SQLType{}.typeProto()
	if err != nil {
		return nil, err
	}
	if value == nil {
		return &btpb.Value{
			Type: pbType,
		}, nil
	}
	typedVal, ok := value.(float32)
	if !ok {
		return nil, &errTypeMismatch{value: value, psType: Float32SQLType{}}
	}
	return &btpb.Value{
		Type: pbType,
		Kind: &btpb.Value_FloatValue{
			FloatValue: float64(typedVal),
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
type Float64SQLType struct{}

func (s Float64SQLType) isValidArrayElemType() bool {
	return true
}

// valid value can be of type float64 or nil
func (s Float64SQLType) valueProto(value any, _ SQLType) (*btpb.Value, error) {
	pbType, err := Float64SQLType{}.typeProto()
	if err != nil {
		return nil, err
	}

	if value == nil {
		return &btpb.Value{
			Type: pbType,
		}, nil
	}
	typedVal, ok := value.(float64)
	if !ok {
		return nil, &errTypeMismatch{value: value, psType: Float64SQLType{}}
	}
	return &btpb.Value{
		Type: pbType,
		Kind: &btpb.Value_FloatValue{
			FloatValue: typedVal,
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
type BoolSQLType struct{}

func (s BoolSQLType) isValidArrayElemType() bool {
	return true
}

// valid value can be of type bool or nil
func (s BoolSQLType) valueProto(value any, _ SQLType) (*btpb.Value, error) {
	pbType, err := BoolSQLType{}.typeProto()
	if err != nil {
		return nil, err
	}

	if value == nil {
		return &btpb.Value{
			Type: pbType,
		}, nil
	}
	typedVal, ok := value.(bool)
	if !ok {
		return nil, &errTypeMismatch{value: value, psType: BoolSQLType{}}
	}
	return &btpb.Value{
		Type: pbType,
		Kind: &btpb.Value_BoolValue{
			BoolValue: typedVal,
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
type TimestampSQLType struct{}

func (s TimestampSQLType) isValidArrayElemType() bool {
	return true
}

// valid value can be of type time.Time or nil
func (s TimestampSQLType) valueProto(value any, _ SQLType) (*btpb.Value, error) {
	pbType, err := TimestampSQLType{}.typeProto()
	if err != nil {
		return nil, err
	}

	if value == nil {
		return &btpb.Value{
			Type: pbType,
		}, nil
	}
	typedVal, ok := value.(time.Time)
	if !ok {
		return nil, &errTypeMismatch{value: value, psType: TimestampSQLType{}}
	}
	return &btpb.Value{
		Type: pbType,
		Kind: &btpb.Value_TimestampValue{
			TimestampValue: timestamppb.New(typedVal),
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
type DateSQLType struct{}

func (s DateSQLType) isValidArrayElemType() bool {
	return true
}

// valid value can be of type civil.Date or nil
func (s DateSQLType) valueProto(value any, _ SQLType) (*btpb.Value, error) {
	pbType, err := DateSQLType{}.typeProto()
	if err != nil {
		return nil, err
	}

	if value == nil {
		return &btpb.Value{
			Type: pbType,
		}, nil
	}
	typedVal, ok := value.(civil.Date)
	if !ok {
		return nil, &errTypeMismatch{value: value, psType: DateSQLType{}}
	}
	return &btpb.Value{
		Type: pbType,
		Kind: &btpb.Value_DateValue{
			DateValue: &date.Date{Year: int32(typedVal.Year), Month: int32(typedVal.Month), Day: int32(typedVal.Day)},
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
}

func (s ArraySQLType) isValidArrayElemType() bool {
	return false
}

// valid value can be of type slice, array or nil
func (s ArraySQLType) valueProto(value any, elemType SQLType) (*btpb.Value, error) {
	pbType, err := ArraySQLType{ElemType: elemType}.typeProto()
	if err != nil {
		return nil, err
	}

	if value == nil {
		return &btpb.Value{
			Type: pbType,
		}, nil
	}

	// Use reflect to check if val is an array.
	valType := reflect.TypeOf(value)
	if valType.Kind() != reflect.Slice && valType.Kind() != reflect.Array {
		return nil, &errTypeMismatch{value: value, psType: ArraySQLType{}}
	}

	valReflectValue := reflect.ValueOf(value)
	var pbValues []*btpb.Value
	// Convert each element to SQLType.
	for i := 0; i < valReflectValue.Len(); i++ {
		elem := valReflectValue.Index(i).Interface()
		elemPbVal, err := anySQLTypeToPbVal(elem, elemType)
		if err != nil {
			return nil, err
		}

		// Type shouldn't be set in nested Values. It should only be at the top level
		elemPbVal.Type = nil
		pbValues = append(pbValues, elemPbVal)
	}

	return &btpb.Value{
		Type: pbType,
		Kind: &btpb.Value_ArrayValue{
			ArrayValue: &btpb.ArrayValue{
				Values: pbValues,
			},
		},
	}, nil
}
func (s ArraySQLType) typeProto() (*btpb.Type, error) {
	if s.ElemType == nil {
		return nil, errors.New("bigtable: ArraySQLType must specify an explicit ElemType")
	}
	if !s.ElemType.isValidArrayElemType() {
		return nil, errors.New("bigtable: unsupported ElemType: " + reflect.TypeOf(s.ElemType).String())
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

func anySQLTypeToPbVal(value any, sqlType SQLType) (*btpb.Value, error) {
	switch t := sqlType.(type) {
	case BytesSQLType:
		return BytesSQLType{}.valueProto(value, nil)
	case StringSQLType:
		return StringSQLType{}.valueProto(value, nil)
	case Int64SQLType:
		return Int64SQLType{}.valueProto(value, nil)
	case Float32SQLType:
		return Float32SQLType{}.valueProto(value, nil)
	case Float64SQLType:
		return Float64SQLType{}.valueProto(value, nil)
	case BoolSQLType:
		return BoolSQLType{}.valueProto(value, nil)
	case TimestampSQLType:
		return TimestampSQLType{}.valueProto(value, nil)
	case DateSQLType:
		return DateSQLType{}.valueProto(value, nil)
	case ArraySQLType:
		return ArraySQLType{}.valueProto(value, t.ElemType)
	default:
		return nil, errors.New("bigtable: unsupported SQLType: " + reflect.TypeOf(t).String())
	}
}

type errTypeMismatch struct {
	value  any
	psType SQLType
}

func (e *errTypeMismatch) Error() string {
	if e == nil {
		return ""
	}
	return "bigtable: Expected %v " + " to be of type " + reflect.TypeOf(e.psType).Name()
}
