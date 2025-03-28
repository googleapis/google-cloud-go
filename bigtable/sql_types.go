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
	"fmt"
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
	valueProto(value any) (*btpb.Value, error)

	isValidArrayElemType() bool

	isValidBindParamType() bool
}

// BytesSQLType represents a slice of bytes.
type BytesSQLType struct{}

func (s BytesSQLType) isValidArrayElemType() bool {
	return true
}

func (s BytesSQLType) isValidBindParamType() bool { return true }

// valid value can be of type []byte or nil.
func (s BytesSQLType) valueProto(value any) (*btpb.Value, error) {
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

func (s StringSQLType) isValidBindParamType() bool { return true }

// valid value can be of type string or nil.
func (s StringSQLType) valueProto(value any) (*btpb.Value, error) {
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

func (s Int64SQLType) isValidBindParamType() bool { return true }

// valid value can be of type int64 or nil.
func (s Int64SQLType) valueProto(value any) (*btpb.Value, error) {
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

func (s Float32SQLType) isValidBindParamType() bool { return true }

// valid value can be of type float32 or nil.
func (s Float32SQLType) valueProto(value any) (*btpb.Value, error) {
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

func (s Float64SQLType) isValidBindParamType() bool { return true }

// valid value can be of type float64 or nil
func (s Float64SQLType) valueProto(value any) (*btpb.Value, error) {
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

func (s BoolSQLType) isValidBindParamType() bool { return true }

// valid value can be of type bool or nil
func (s BoolSQLType) valueProto(value any) (*btpb.Value, error) {
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

func (s TimestampSQLType) isValidBindParamType() bool { return true }

// valid value can be of type time.Time or nil
func (s TimestampSQLType) valueProto(value any) (*btpb.Value, error) {
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

func (s DateSQLType) isValidBindParamType() bool { return true }

// valid value can be of type civil.Date or nil
func (s DateSQLType) valueProto(value any) (*btpb.Value, error) {
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

func (s ArraySQLType) isValidBindParamType() bool { return true }

// valid value can be of type slice, array or nil
func (s ArraySQLType) valueProto(value any) (*btpb.Value, error) {
	pbType, err := s.typeProto()
	if err != nil {
		return nil, err
	}

	if value == nil {
		return &btpb.Value{Type: pbType}, nil
	}

	// Use reflect to check if val is an array.
	valType := reflect.TypeOf(value)
	if valType.Kind() != reflect.Slice && valType.Kind() != reflect.Array {
		return nil, &errTypeMismatch{value: value, psType: s}
	}

	valReflectValue := reflect.ValueOf(value)
	pbValues := make([]*btpb.Value, 0, valReflectValue.Len())
	for i := 0; i < valReflectValue.Len(); i++ {
		elem := valReflectValue.Index(i).Interface()
		elemPbVal, err := s.ElemType.valueProto(elem)
		if err != nil {
			// Wrap error for context
			return nil, fmt.Errorf("bigtable: error converting array element at index %d: %w", i, err)
		}
		// Type shouldn't be set in nested Values. It should only be at the top level.
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

// MapSQLType represents a map from a key type to a value type for query parameters.
type MapSQLType struct {
	KeyType   SQLType
	ValueType SQLType
}

func (s MapSQLType) isValidArrayElemType() bool { return true }

func (s MapSQLType) isValidBindParamType() bool { return false }

func (s MapSQLType) typeProto() (*btpb.Type, error) {
	if s.KeyType == nil || s.ValueType == nil {
		return nil, errors.New("bigtable: MapSQLType must specify non-nil KeyType and ValueType")
	}

	keyTp, err := s.KeyType.typeProto()
	if err != nil {
		return nil, err
	}
	valueTp, err := s.ValueType.typeProto()
	if err != nil {
		return nil, err
	}

	return &btpb.Type{
		Kind: &btpb.Type_MapType{
			MapType: &btpb.Type_Map{
				KeyType:   keyTp,
				ValueType: valueTp,
			},
		},
	}, nil
}

// valueProto converts a Go map (map[K]V) to its protobuf representation for query parameters.
func (s MapSQLType) valueProto(value any) (*btpb.Value, error) {
	pbType, err := s.typeProto() // Contains KeyType/ValueType info needed for validation
	if err != nil {
		return nil, err
	}

	if value == nil {
		return &btpb.Value{Type: pbType}, nil
	}

	valReflectType := reflect.TypeOf(value)
	if valReflectType.Kind() != reflect.Map {
		return nil, &errTypeMismatch{value: value, psType: s}
	}

	// TODO: Runtime check if map key/value Go types are compatible with SQLTypes.
	// This requires SQLType to expose its expected Go type, which adds complexity.
	// Relying on the user providing the correct Go map type matching the MapSQLType definition.

	valReflectValue := reflect.ValueOf(value)
	mapIter := valReflectValue.MapRange()
	pbMapEntries := make([]*btpb.Value, 0, valReflectValue.Len())

	for mapIter.Next() {
		key := mapIter.Key().Interface()
		val := mapIter.Value().Interface()

		keyPbVal, err := s.KeyType.valueProto(key)
		if err != nil {
			return nil, fmt.Errorf("error converting map key %v (type %T): %w", key, key, err)
		}
		keyPbVal.Type = nil // Nested values shouldn't have type

		// Convert value using the specified ValueType's valueProto
		valPbVal, err := s.ValueType.valueProto(val)
		if err != nil {
			return nil, fmt.Errorf("error converting map value for key %v (value type %T): %w", key, val, err)
		}
		valPbVal.Type = nil // Nested values shouldn't have type

		// Map entry is represented as a 2-element array [key, value]
		entryPbVal := &btpb.Value{
			Kind: &btpb.Value_ArrayValue{
				ArrayValue: &btpb.ArrayValue{
					Values: []*btpb.Value{keyPbVal, valPbVal},
				},
			},
			// Type: No type needed for nested map entry array itself.
		}
		pbMapEntries = append(pbMapEntries, entryPbVal)
	}

	return &btpb.Value{
		Type: pbType,
		Kind: &btpb.Value_ArrayValue{ // Map is stored as ArrayValue
			ArrayValue: &btpb.ArrayValue{
				Values: pbMapEntries,
			},
		},
	}, nil
}

// StructSQLType represents a struct with named fields for query parameters.
// Field order specified in `Fields` is significant for the protobuf representation.
// Struct values in result rows are typically represented as map[string]any.
type StructSQLType struct {
	// Fields defines the ordered sequence of fields within the struct parameter.
	Fields []StructSQLField
}

// StructSQLField defines a single named and typed field within a StructSQLType.
type StructSQLField struct {
	Name string
	Type SQLType
}

// isValidArrayElemType reports whether StructSQLType can be used as an element in an ArraySQLType.
func (s StructSQLType) isValidArrayElemType() bool { return true } // Structs can be elements of arrays.

func (s StructSQLType) isValidBindParamType() bool { return false }

// typeProto generates the protobuf Type message for a Struct.
func (s StructSQLType) typeProto() (*btpb.Type, error) {
	pbFields := make([]*btpb.Type_Struct_Field, len(s.Fields))
	seenNames := make(map[string]struct{})

	if len(s.Fields) == 0 {
		// Representing an empty struct. Is this valid in SQL? Assume yes for now.
	}

	for i, field := range s.Fields {
		if field.Name == "" {
			return nil, fmt.Errorf("bigtable: StructSQLType field at index %d must have a name", i)
		}
		if _, exists := seenNames[field.Name]; exists {
			// GoogleSQL structs allow duplicate field names, but it's unusual for parameters. Error for now.
			return nil, fmt.Errorf("bigtable: StructSQLType duplicate field name %q specified", field.Name)
		}
		seenNames[field.Name] = struct{}{}
		if field.Type == nil {
			return nil, fmt.Errorf("bigtable: StructSQLType field %q must have a non-nil type", field.Name)
		}
		fieldTypeProto, err := field.Type.typeProto()
		if err != nil {
			return nil, fmt.Errorf("invalid type for struct field %q: %w", field.Name, err)
		}
		pbFields[i] = &btpb.Type_Struct_Field{
			FieldName: field.Name,
			Type:      fieldTypeProto,
		}
	}

	return &btpb.Type{
		Kind: &btpb.Type_StructType{
			StructType: &btpb.Type_Struct{
				Fields: pbFields,
			},
		},
	}, nil
}

// valueProto converts a Go struct or map[string]any to its protobuf representation for query parameters.
// The underlying protobuf representation for a struct is an array of its field values,
// in the order defined by StructSQLType.Fields.
func (s StructSQLType) valueProto(value any) (*btpb.Value, error) {
	pbType, err := s.typeProto() // Contains field name/type/order info
	if err != nil {
		return nil, err
	}

	if value == nil {
		// Return protobuf NULL value with type information
		return &btpb.Value{Type: pbType}, nil
	}

	valReflectValue := reflect.ValueOf(value)
	valReflectType := valReflectValue.Type()

	pbFieldValues := make([]*btpb.Value, len(s.Fields))

	// Determine how to access field values: by struct field name or map key
	var getValue func(fieldName string) (any, bool, error)

	switch valReflectType.Kind() {
	case reflect.Map:
		if valReflectType.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("bigtable: expected map[string]any for StructSQLType parameter, got %v", valReflectType)
		}
		if valReflectValue.IsNil() { // Handle nil map as all fields missing.
			getValue = func(fieldName string) (any, bool, error) { return nil, false, nil }
		} else {
			getValue = func(fieldName string) (any, bool, error) {
				mapValue := valReflectValue.MapIndex(reflect.ValueOf(fieldName))
				if !mapValue.IsValid() {
					return nil, false, nil
				}
				return mapValue.Interface(), true, nil
			}
		}
	case reflect.Struct:
		getValue = func(fieldName string) (any, bool, error) {
			StructSQLField := valReflectValue.FieldByName(fieldName)
			if !StructSQLField.IsValid() {
				return nil, false, nil // Not found
			}
			// TODO: Handle unexported fields
			if !StructSQLField.CanInterface() {
				return nil, false, fmt.Errorf("cannot access unexported struct field %q", fieldName)
			}
			return StructSQLField.Interface(), true, nil
		}
	default:
		return nil, &errTypeMismatch{value: value, psType: s}
	}

	// Iterate through the defined fields IN ORDER and get/convert values
	for i, fieldInfo := range s.Fields {
		fieldValue, found, accessErr := getValue(fieldInfo.Name)
		if accessErr != nil {
			return nil, fmt.Errorf("error accessing field %q for StructSQLType parameter: %w", fieldInfo.Name, accessErr)
		}

		if !found {
			// Field defined in StructSQLType is missing in the provided Go value.
			// GoogleSQL requires all struct fields to be present. Error out.
			return nil, fmt.Errorf("bigtable: struct field %q defined in StructSQLType not found in provided value (type %T)", fieldInfo.Name, value)
		}

		// Convert the Go field value using the specified SQLType for the field
		fieldPbValue, err := fieldInfo.Type.valueProto(fieldValue)
		if err != nil {
			return nil, fmt.Errorf("error converting struct field %q (value type %T): %w", fieldInfo.Name, fieldValue, err)
		}
		fieldPbValue.Type = nil // Nested values shouldn't have type
		pbFieldValues[i] = fieldPbValue
	}

	return &btpb.Value{
		Type: pbType,
		Kind: &btpb.Value_ArrayValue{ // Struct is stored as ArrayValue
			ArrayValue: &btpb.ArrayValue{
				Values: pbFieldValues,
			},
		},
	}, nil
}

// anySQLTypeToPbVal converts a Go value to a protobuf Value based on the provided SQLType.
func anySQLTypeToPbVal(value any, sqlType SQLType) (*btpb.Value, error) {
	if sqlType == nil {
		return nil, errors.New("bigtable: invalid SQLType: nil")
	}
	// Use the valueProto method directly from the SQLType instance.
	// This automatically handles simple types, arrays, maps, and structs correctly.
	return sqlType.valueProto(value)
}

type errTypeMismatch struct {
	value  any
	psType SQLType
}

func (e *errTypeMismatch) Error() string {
	if e == nil {
		return ""
	}
	// Provide more specific type information if possible
	expectedTypeName := reflect.TypeOf(e.psType).Name()
	// Add details for composite types
	switch t := e.psType.(type) {
	case ArraySQLType:
		expectedTypeName = fmt.Sprintf("ArraySQLType (elements: %T)", t.ElemType)
	case MapSQLType:
		expectedTypeName = fmt.Sprintf("MapSQLType (key: %T, value: %T)", t.KeyType, t.ValueType)
	case StructSQLType:
		expectedTypeName = fmt.Sprintf("StructSQLType (with %d fields)", len(t.Fields))
	}

	return fmt.Sprintf("bigtable: parameter type mismatch: expected Go type compatible with %s, but got %T", expectedTypeName, e.value)
}
