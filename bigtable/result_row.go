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

package bigtable // import "cloud.google.com/go/bigtable"

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/civil"
)

// ResultRow represents a single row in the result set returned on executing a GoogleSQL query in Cloud Bigtable
type ResultRow struct {
	pbValues   []*btpb.Value
	pbMetadata *btpb.ResultSetMetadata
	// map from column name to list of indices {name -> [idx1, idx2, ...]}
	colIndexMap map[string][]int

	Metadata ResultRowMetadata
}

// ColumnMetadata describes a single column in a ResultRowMetadata.
type ColumnMetadata struct {
	// Name is the name of the column as returned by the query (e.g., alias or derived name).
	Name string
	// SQLType provides the original Bigtable SQL type information. This can be useful
	// for understanding the underlying storage or type details.
	SQLType SQLType
}

// ResultRowMetadata provides information about the schema of the ResultRow
type ResultRowMetadata struct {
	// the order of values returned by [ResultRow.Scan].
	Columns []ColumnMetadata
}

func newResultRow(values []*btpb.Value, metadata *btpb.ResultSetMetadata) (*ResultRow, error) {
	rrMetadata, err := newResultRowMetadata(metadata)
	if err != nil {
		return nil, err
	}

	colIndexMap := make(map[string][]int)
	for i, colInfo := range rrMetadata.Columns {
		name := colInfo.Name
		colIndexMap[name] = append(colIndexMap[name], i)
	}

	return &ResultRow{
		pbValues:   values,
		pbMetadata: metadata,
		Metadata:   *rrMetadata,
	}, nil
}

// newResultRowMetadata returns the schema of the result row, describing the name and type of each column.
// The order of columns matches the order of values returned by [ResultRow.Scan].
func newResultRowMetadata(metadata *btpb.ResultSetMetadata) (*ResultRowMetadata, error) {
	protoSchema := metadata.GetProtoSchema()
	cols := protoSchema.GetColumns()
	md := make([]ColumnMetadata, len(cols))
	for i, colMeta := range cols {
		pbType := colMeta.GetType()
		sqlType, err := pbTypeToSQLType(pbType)
		if err != nil {
			return nil, fmt.Errorf("error parsing metadata type for column %q (index %d): %w", colMeta.GetName(), i, err)
		}
		md[i] = ColumnMetadata{
			Name:    colMeta.GetName(),
			SQLType: sqlType,
		}
	}
	return &ResultRowMetadata{
		Columns: md,
	}, nil
}

// GetByIndex returns the value of the column at the specified index.
// The returned value will be of the corresponding Go type (e.g., int64, string,
// time.Time, []any, map[string]any).
func (rr *ResultRow) GetByIndex(index int) (any, error) {
	if index < 0 || index >= len(rr.pbValues) {
		return nil, fmt.Errorf("bigtable: index %d out of bounds for row with %d columns", index, len(rr.pbValues))
	}

	pbVal := rr.pbValues[index]
	sqlType := rr.Metadata.Columns[index].SQLType
	pbType, err := sqlType.typeProto()
	if err != nil {
		return nil, fmt.Errorf("bigtable: internal error - failed to get protobuf type for column index %d: %w", index, err)
	}

	goVal, err := pbValueToGoValue(pbVal, pbType)
	if err != nil {
		return nil, fmt.Errorf("bigtable: error converting column %d (%q): %w", index, rr.Metadata.Columns[index].Name, err)
	}
	return goVal, nil
}

// GetByName returns the value of the first column found with the specified name.
// The returned value will be of the corresponding Go type.
// Returns an error if no column with the specified name is found.
// Column name matching is case-sensitive.
func (rr *ResultRow) GetByName(name string) (any, error) {
	indices, found := rr.colIndexMap[name]
	if !found || len(indices) == 0 {
		return nil, fmt.Errorf("bigtable: column %q not found in result row", name)
	}

	// Return the value at the first index found for this name
	return rr.GetByIndex(indices[0])
}

// GetAllByName returns a slice containing the values of *all* columns matching the
// specified name, in the order they appear in the result set.
// If no columns match the name, it returns (nil, nil).
// The values in the returned slice will be of their corresponding Go types.
// Returns an error if any value conversion fails.
// Column name matching is case-sensitive.
func (rr *ResultRow) GetAllByName(name string) ([]any, error) {
	indices, found := rr.colIndexMap[name]
	if !found || len(indices) == 0 {
		return nil, nil
	}

	results := make([]any, len(indices))
	for i, index := range indices {
		val, err := rr.GetByIndex(index)
		if err != nil {
			return nil, fmt.Errorf("bigtable: error getting value for column %q at index %d: %w", name, index, err)
		}
		results[i] = val
	}

	return results, nil
}

// Scan copies the columns in the current row into the values pointed at by dest.
// The number of values in dest must equal the number of columns in the ResultRow.
//
// Scan converts columns to the following Go types where possible:
//
//   - string
//   - []byte
//   - int64 (and other integer types like int, int32, uint64 etc.)
//   - float32, float64
//   - bool
//   - time.Time (for TIMESTAMP)
//   - civil.Date (for DATE)
//   - Slice types (e.g., []string, []int64) for ARRAY
//   - Map types (e.g., map[string]any) for MAP or STRUCT
//   - any (interface{})
//   - Pointers to the above types
//
// It performs basic type conversions. For example,
// an int64 column value can be scanned into an *int, and a []byte column
// can be scanned into a *string. If a column value is NULL, Scan assigns
// the zero value to the destination, unless the destination is a pointer,
// interface, slice, or map, in which case it assigns nil.
// An error is returned if a destination argument is not a pointer or if
// a type conversion is not supported.
func (rr *ResultRow) Scan(dest ...any) error {
	cm := rr.Metadata.Columns
	if len(dest) != len(rr.pbValues) {
		return fmt.Errorf("bigtable: destination argument count mismatch: expected %d, got %d", len(rr.pbValues), len(dest))
	}

	for i, d := range dest {
		if d == nil {
			return fmt.Errorf("bigtable: destination argument %d is nil", i)
		}
		destPtr := reflect.ValueOf(d)
		if destPtr.Kind() != reflect.Ptr {
			return fmt.Errorf("bigtable: destination argument %d is not a pointer (got %T)", i, d)
		}
		if destPtr.IsNil() {
			return fmt.Errorf("bigtable: destination argument %d is a nil pointer", i)
		}

		destVal := destPtr.Elem() // The value the pointer points to
		if !destVal.CanSet() {
			return fmt.Errorf("bigtable: destination argument %d cannot be set (unexported field)", i)
		}

		pbVal := rr.pbValues[i]
		pbType, err := cm[i].SQLType.typeProto()
		if err != nil {
			return fmt.Errorf("bigtable: error converting column %d (%q): %w", i, cm[i].Name, err)
		}

		// Convert protobuf value to corresponding Go value
		goVal, err := pbValueToGoValue(pbVal, pbType)
		if err != nil {
			return fmt.Errorf("bigtable: error converting column %d (%q): %w", i, cm[i].Name, err)
		}

		// Assign the Go value to the destination pointer, handling conversions
		err = assignValue(destVal, goVal)
		if err != nil {
			return fmt.Errorf("bigtable: error assigning column %d (%q) (value type %T) to destination (type %s): %w", i, cm[i].Name, goVal, destVal.Type(), err)
		}
	}
	return nil
}

const tagName = "bigtable" // Struct tag name

// DataTo scans the current row's columns into the struct or map pointed to by dest.
//
// If dest is a pointer to a struct, DataTo matches column names to struct field names
// or the names specified in the `bigtable:"..."` struct tag. Unexported fields or fields
// tagged with `bigtable:"-"` are ignored. If multiple columns match a struct field and
// the field is a slice type (excluding []byte), DataTo collects all matching values
// into the slice; otherwise, it uses the value from the first matching column.
//
// If dest is a pointer to a map[string]any, DataTo populates the map with column names
// as keys. If multiple columns have the same name, the map value will be a slice []any
// containing all corresponding column values; otherwise, the value will be the single
// column value. If the map is nil, it will be initialized.
//
// Basic type conversions are performed similar to Scan.
// It returns an error if dest is not a non-nil pointer to a struct or map[string]any,
// or if a type conversion fails.
func (rr *ResultRow) DataTo(dest any) error {
	if dest == nil {
		return errors.New("bigtable: unmarshal destination cannot be nil")
	}
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return errors.New("bigtable: unmarshal destination must be a non-nil pointer")
	}
	v = v.Elem() // Get the value the pointer points to

	isStruct := v.Kind() == reflect.Struct
	isMap := v.Kind() == reflect.Map && v.Type().Key().Kind() == reflect.String && v.Type().Elem().Kind() == reflect.Interface

	if !isStruct && !isMap {
		return fmt.Errorf("bigtable: unmarshal destination must be a pointer to a struct or map[string]any (got pointer to %s)", v.Kind())
	}
	if isMap && v.IsNil() {
		// Initialize map if it's nil
		v.Set(reflect.MakeMap(v.Type()))
	}

	cm := rr.Metadata.Columns

	//  Unmarshal into Struct
	if isStruct {
		t := v.Type()
		processedFields := make(map[string]bool) // Handle potential duplicate tags/names mapping to same column

		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			fieldV := v.Field(i)

			// Skip unexported fields
			if !fieldV.CanSet() {
				continue
			}

			// Determine column name: tag or field name
			colName := field.Name
			tag := field.Tag.Get(tagName)
			tagParts := strings.Split(tag, ",")
			if len(tagParts) > 0 && tagParts[0] != "" {
				colName = tagParts[0]
			}
			if colName == "-" { // Allow skipping fields via tag
				continue
			}
			if processedFields[colName] { // Ensure we handle each column name only once per struct
				continue
			}

			indices, found := rr.colIndexMap[colName]
			if !found {
				// Field in struct has no matching column in result set. Skip.
				continue
			}
			processedFields[colName] = true // Mark as processed

			// Handle assignment based on field type and number of matches
			if field.Type.Kind() == reflect.Slice && field.Type.Elem().Kind() != reflect.Uint8 /* Don't treat []byte as multi-value */ {
				// Target is a slice: collect all matching column values
				elemType := field.Type.Elem()
				newSlice := reflect.MakeSlice(field.Type, 0, len(indices))

				for _, index := range indices {
					pbVal := rr.pbValues[index]
					pbType, err := cm[index].SQLType.typeProto() // Get proto type
					if err != nil {
						return fmt.Errorf("bigtable: error converting column %d (%q) for slice field %q: %w", index, colName, field.Name, err)
					}

					goVal, err := pbValueToGoValue(pbVal, pbType)
					if err != nil {
						return fmt.Errorf("bigtable: error converting column %d (%q) for slice field %q: %w", index, colName, field.Name, err)
					}

					// Create element of correct type and assign
					elemVal := reflect.New(elemType).Elem()
					errAssign := assignValue(elemVal, goVal)
					if errAssign != nil {
						return fmt.Errorf("bigtable: error assigning column %d (%q) (value type %T) to slice element (type %s) for field %q: %w", index, colName, goVal, elemType, field.Name, errAssign)
					}
					newSlice = reflect.Append(newSlice, elemVal)
				}
				fieldV.Set(newSlice)

			} else {
				// Target is not a slice: use the first matching column value
				index := indices[0]
				pbVal := rr.pbValues[index]
				pbType, err := cm[index].SQLType.typeProto()
				if err != nil {
					return fmt.Errorf("bigtable: error converting column %d (%q) for field %q: %w", index, colName, field.Name, err)
				}
				goVal, err := pbValueToGoValue(pbVal, pbType)
				if err != nil {
					return fmt.Errorf("bigtable: error converting column %d (%q) for field %q: %w", index, colName, field.Name, err)
				}

				errAssign := assignValue(fieldV, goVal)
				if errAssign != nil {
					return fmt.Errorf("bigtable: error assigning column %d (%q) (value type %T) to field %q (type %s): %w", index, colName, goVal, field.Name, fieldV.Type(), errAssign)
				}
			}
		}
	}

	// Unmarshal into Map[string]any
	if isMap {
		mapValueType := v.Type().Elem() // Should be reflect.Interface for map[string]any

		for colName, indices := range rr.colIndexMap {
			var finalGoVal any // This will be assigned to the map

			if len(indices) == 1 {
				// Single column match -> assign the single value
				index := indices[0]
				pbVal := rr.pbValues[index]
				pbType, err := cm[index].SQLType.typeProto()
				if err != nil {
					return fmt.Errorf("bigtable: error converting column %d (%q) for map key %q: %w", index, colName, colName, err)
				}
				goVal, err := pbValueToGoValue(pbVal, pbType)
				if err != nil {
					return fmt.Errorf("bigtable: error converting column %d (%q) for map key %q: %w", index, colName, colName, err)
				}
				finalGoVal = goVal
			} else {
				// Multiple columns match -> create a slice []any
				multiValueSlice := make([]any, len(indices))
				for i, index := range indices {
					pbVal := rr.pbValues[index]
					pbType, err := cm[index].SQLType.typeProto()
					if err != nil {
						return fmt.Errorf("bigtable: error converting column %d (%q) for map key %q: %w", index, colName, colName, err)
					}
					goVal, err := pbValueToGoValue(pbVal, pbType)
					if err != nil {
						return fmt.Errorf("bigtable: error converting column %d (%q) for map key %q slice element %d: %w", index, colName, colName, i, err)
					}
					multiValueSlice[i] = goVal
				}
				finalGoVal = multiValueSlice // Assign the slice to the map value
			}

			// Assign finalGoVal to map[colName] using reflection
			mapValue := reflect.New(mapValueType).Elem() // Create element of interface{} type
			errAssign := assignValue(mapValue, finalGoVal)
			if errAssign != nil {
				return fmt.Errorf("bigtable: error assigning value (type %T) for map key %q: %w", finalGoVal, colName, errAssign)
			}
			v.SetMapIndex(reflect.ValueOf(colName), mapValue) // Set map["colName"] = finalGoVal
		}
	}

	return nil
}

// pbTypeToSQLType converts a protobuf Type to its corresponding SQLType interface implementation.
// errors returned should be wrapped before returning to the user.
func pbTypeToSQLType(pbType *btpb.Type) (SQLType, error) {
	if pbType == nil {
		return nil, errors.New("protobuf type is nil")
	}
	switch k := pbType.Kind.(type) {
	case *btpb.Type_BytesType:
		return BytesSQLType{}, nil
	case *btpb.Type_StringType:
		return StringSQLType{}, nil
	case *btpb.Type_Int64Type:
		return Int64SQLType{}, nil
	case *btpb.Type_Float32Type:
		return Float32SQLType{}, nil
	case *btpb.Type_Float64Type:
		return Float64SQLType{}, nil
	case *btpb.Type_BoolType:
		return BoolSQLType{}, nil
	case *btpb.Type_TimestampType:
		return TimestampSQLType{}, nil
	case *btpb.Type_DateType:
		return DateSQLType{}, nil
	case *btpb.Type_ArrayType:
		elemPbType := k.ArrayType.GetElementType()
		if elemPbType == nil {
			return nil, errors.New("array element type is nil")
		}
		elemSQLType, err := pbTypeToSQLType(elemPbType)
		if err != nil {
			return nil, fmt.Errorf("invalid array element type: %w", err)
		}
		return ArraySQLType{ElemType: elemSQLType}, nil
	case *btpb.Type_MapType:
		keyPbType := k.MapType.GetKeyType()
		valPbType := k.MapType.GetValueType()
		if keyPbType == nil || valPbType == nil {
			return nil, errors.New("map key or value type is nil")
		}
		keySQLType, err := pbTypeToSQLType(keyPbType)
		if err != nil {
			return nil, fmt.Errorf("invalid map key type: %w", err)
		}
		valueSQLType, err := pbTypeToSQLType(valPbType)
		if err != nil {
			return nil, fmt.Errorf("invalid map value type: %w", err)
		}
		return MapSQLType{KeyType: keySQLType, ValueType: valueSQLType}, nil
	case *btpb.Type_StructType:
		fields := k.StructType.GetFields()
		structFields := make([]StructSQLField, len(fields))
		for i, f := range fields {
			fieldPbType := f.GetType()
			if fieldPbType == nil {
				return nil, fmt.Errorf("struct field %q type is nil", f.GetFieldName())
			}
			fieldSQLType, err := pbTypeToSQLType(fieldPbType)
			if err != nil {
				return nil, fmt.Errorf("invalid struct field %q type: %w", f.GetFieldName(), err)
			}
			structFields[i] = StructSQLField{Name: f.GetFieldName(), Type: fieldSQLType}
		}
		return StructSQLType{Fields: structFields}, nil
	default:
		return nil, fmt.Errorf("unsupported protobuf type kind: %T", k)
	}
}

// pbTypeToGoReflectType converts a protobuf Type to the corresponding Go reflect.Type.
var (
	bytesType   = reflect.TypeOf([]byte(nil))
	stringType  = reflect.TypeOf("")
	int64Type   = reflect.TypeOf(int64(0))
	float32Type = reflect.TypeOf(float32(0))
	float64Type = reflect.TypeOf(float64(0))
	boolType    = reflect.TypeOf(false)
	timeType    = reflect.TypeOf(time.Time{})
	dateType    = reflect.TypeOf(civil.Date{})
	anyMapType  = reflect.TypeOf(map[string]any{}) // Default for Struct/Map
)

// errors returned should be wrapped before returning to the end user.
func pbTypeToGoReflectType(pbType *btpb.Type) (reflect.Type, error) {
	if pbType == nil {
		return nil, errors.New("protobuf type is nil")
	}
	switch k := pbType.Kind.(type) {
	case *btpb.Type_BytesType:
		return bytesType, nil
	case *btpb.Type_StringType:
		return stringType, nil
	case *btpb.Type_Int64Type:
		return int64Type, nil
	case *btpb.Type_Float32Type:
		return float32Type, nil
	case *btpb.Type_Float64Type:
		return float64Type, nil
	case *btpb.Type_BoolType:
		return boolType, nil
	case *btpb.Type_TimestampType:
		return timeType, nil
	case *btpb.Type_DateType:
		return dateType, nil
	case *btpb.Type_ArrayType:
		elemPbType := k.ArrayType.GetElementType()
		if elemPbType == nil {
			return nil, errors.New("array element type is nil")
		}
		elemGoType, err := pbTypeToGoReflectType(elemPbType)
		if err != nil {
			return nil, fmt.Errorf("invalid array element type: %w", err)
		}
		return reflect.SliceOf(elemGoType), nil
	case *btpb.Type_MapType:
		keyPbType := k.MapType.GetKeyType()
		valPbType := k.MapType.GetValueType()
		if keyPbType == nil || valPbType == nil {
			return nil, errors.New("map key or value type is nil")
		}
		keyGoType, errK := pbTypeToGoReflectType(keyPbType)
		valGoType, errV := pbTypeToGoReflectType(valPbType)
		if errK != nil || errV != nil {
			return nil, fmt.Errorf("invalid map key/value type: %v / %v", errK, errV)
		}
		return reflect.MapOf(keyGoType, valGoType), nil
	case *btpb.Type_StructType:
		// Represent struct results as map[string]any
		return anyMapType, nil
	default:
		return nil, fmt.Errorf("unsupported protobuf type kind for Go type mapping: %T", k)
	}
}

// pbValueToGoValue converts a protobuf Value (and its Type) to a standard Go value (any).
// It handles scalar types, nulls, arrays, maps, and structs recursively.
// Structs are converted to map[string]any.
// errors returned should be wrapped before returning to the end user.
func pbValueToGoValue(pbVal *btpb.Value, pbType *btpb.Type) (any, error) {
	// Handle NULL value (protobuf Kind is nil)
	if pbVal == nil || pbVal.Kind == nil {
		// Represent SQL NULL as Go's nil interface value.
		return nil, nil
	}

	if pbType == nil {
		return nil, errors.New("internal error - pbType is nil during value conversion")
	}

	switch k := pbType.Kind.(type) {
	case *btpb.Type_BytesType:
		if val, ok := pbVal.Kind.(*btpb.Value_BytesValue); ok {
			return val.BytesValue, nil
		}
		return nil, fmt.Errorf("type mismatch: expected BytesValue for BytesType, got %T", pbVal.Kind)

	case *btpb.Type_StringType:
		if val, ok := pbVal.Kind.(*btpb.Value_StringValue); ok {
			return val.StringValue, nil
		}
		return nil, fmt.Errorf("type mismatch: expected StringValue for StringType, got %T", pbVal.Kind)

	case *btpb.Type_Int64Type:
		if val, ok := pbVal.Kind.(*btpb.Value_IntValue); ok {
			return val.IntValue, nil
		}
		return nil, fmt.Errorf("type mismatch: expected IntValue for Int64Type, got %T", pbVal.Kind)

	case *btpb.Type_Float32Type:
		if val, ok := pbVal.Kind.(*btpb.Value_FloatValue); ok {
			// Proto uses float64 for transport
			return float32(val.FloatValue), nil
		}
		return nil, fmt.Errorf("type mismatch: expected FloatValue for Float32Type, got %T", pbVal.Kind)

	case *btpb.Type_Float64Type:
		if val, ok := pbVal.Kind.(*btpb.Value_FloatValue); ok {
			return val.FloatValue, nil
		}
		return nil, fmt.Errorf("type mismatch: expected FloatValue for Float64Type, got %T", pbVal.Kind)

	case *btpb.Type_BoolType:
		if val, ok := pbVal.Kind.(*btpb.Value_BoolValue); ok {
			return val.BoolValue, nil
		}
		return nil, fmt.Errorf("type mismatch: expected BoolValue for BoolType, got %T", pbVal.Kind)

	case *btpb.Type_TimestampType:
		if val, ok := pbVal.Kind.(*btpb.Value_TimestampValue); ok {
			ts := val.TimestampValue
			if ts == nil {
				return nil, nil
			}
			if err := ts.CheckValid(); err != nil {
				return nil, fmt.Errorf("invalid timestamp value: %w", err)
			}
			return ts.AsTime(), nil
		}
		return nil, fmt.Errorf("type mismatch: expected TimestampValue for TimestampType, got %T", pbVal.Kind)

	case *btpb.Type_DateType:
		if val, ok := pbVal.Kind.(*btpb.Value_DateValue); ok {
			d := val.DateValue
			if d == nil {
				return nil, nil
			}
			return civil.Date{Year: int(d.Year), Month: time.Month(d.Month), Day: int(d.Day)}, nil
		}
		return nil, fmt.Errorf("type mismatch: expected DateValue for DateType, got %T", pbVal.Kind)

	case *btpb.Type_ArrayType:
		arrValProto, ok := pbVal.Kind.(*btpb.Value_ArrayValue)
		if !ok {
			return nil, fmt.Errorf("type mismatch: expected ArrayValue for ArrayType, got %T", pbVal.Kind)
		}
		elemPbType := k.ArrayType.GetElementType()
		if elemPbType == nil {
			return nil, errors.New("array element type is nil")
		}

		if arrValProto.ArrayValue == nil || len(arrValProto.ArrayValue.Values) == 0 {
			// Return empty slice of the correct Go type.
			elemGoType, err := pbTypeToGoReflectType(elemPbType)
			if err != nil {
				return nil, fmt.Errorf("internal error getting array element Go type: %w", err)
			}
			return reflect.MakeSlice(reflect.SliceOf(elemGoType), 0, 0).Interface(), nil
		}

		pbElements := arrValProto.ArrayValue.Values
		elemGoType, _ := pbTypeToGoReflectType(elemPbType)
		goSlice := reflect.MakeSlice(reflect.SliceOf(elemGoType), len(pbElements), len(pbElements))

		for i, pbElem := range pbElements {
			goElem, err := pbValueToGoValue(pbElem, elemPbType)
			if err != nil {
				return nil, fmt.Errorf("error converting array element at index %d: %w", i, err)
			}
			// Assign goElem to the slice element using assignValue helper logic.
			// Need temporary element value for assignValue.
			elemValDest := goSlice.Index(i)
			if err := assignValue(elemValDest, goElem); err != nil {
				return nil, fmt.Errorf("error assigning array element at index %d: %w", i, err)
			}
		}
		return goSlice.Interface(), nil

	case *btpb.Type_MapType:
		mapArrProto, ok := pbVal.Kind.(*btpb.Value_ArrayValue)
		if !ok {
			return nil, fmt.Errorf(" type mismatch: expected ArrayValue for MapType, got %T", pbVal.Kind)
		}
		keyPbType := k.MapType.GetKeyType()
		valPbType := k.MapType.GetValueType()
		if keyPbType == nil || valPbType == nil {
			return nil, errors.New("map key or value type is nil")
		}

		keyGoType, _ := pbTypeToGoReflectType(keyPbType)
		valGoType, _ := pbTypeToGoReflectType(valPbType)
		goMap := reflect.MakeMap(reflect.MapOf(keyGoType, valGoType))

		if mapArrProto.ArrayValue == nil || len(mapArrProto.ArrayValue.Values) == 0 {
			return goMap.Interface(), nil // Return empty map
		}

		pbEntries := mapArrProto.ArrayValue.Values
		for i, pbEntry := range pbEntries {
			kvPairProto, ok := pbEntry.Kind.(*btpb.Value_ArrayValue)
			if !ok || kvPairProto.ArrayValue == nil || len(kvPairProto.ArrayValue.Values) != 2 {
				// The underlying protobuf representation for a map value is an array of 2-element arrays [key, value].
				// Map {"foo": "bar", "baz": "qux"} is protobuf []{[]{"foo": "bar"}, []{"baz": "qux"}}
				return nil, fmt.Errorf("invalid map entry format at index %d: expected 2-element array", i)
			}
			pbKey := kvPairProto.ArrayValue.Values[0]
			pbValue := kvPairProto.ArrayValue.Values[1]

			goKey, err := pbValueToGoValue(pbKey, keyPbType)
			if err != nil {
				return nil, fmt.Errorf("error converting map key at entry index %d: %w", i, err)
			}
			goValue, err := pbValueToGoValue(pbValue, valPbType)
			if err != nil {
				return nil, fmt.Errorf("error converting map value at entry index %d: %w", i, err)
			}

			// Use reflection + assignValue logic to set map entry
			keyReflect := reflect.New(keyGoType).Elem()
			valReflect := reflect.New(valGoType).Elem()
			if errK := assignValue(keyReflect, goKey); errK != nil {
				return nil, fmt.Errorf("error assigning map key at entry index %d: %w", i, errK)
			}
			if errV := assignValue(valReflect, goValue); errV != nil {
				return nil, fmt.Errorf("error assigning map value at entry index %d: %w", i, errV)
			}
			goMap.SetMapIndex(keyReflect, valReflect)
		}
		return goMap.Interface(), nil

	case *btpb.Type_StructType:
		structArrProto, ok := pbVal.Kind.(*btpb.Value_ArrayValue)
		if !ok {
			return nil, fmt.Errorf("type mismatch: expected ArrayValue for StructType, got %T", pbVal.Kind)
		}
		pbFields := k.StructType.GetFields()
		if structArrProto.ArrayValue == nil {
			return nil, nil // Return nil for null struct
		}

		pbFieldValues := structArrProto.ArrayValue.Values
		if len(pbFieldValues) != len(pbFields) {
			return nil, fmt.Errorf("struct data/schema mismatch: expected %d fields, got %d values", len(pbFields), len(pbFieldValues))
		}

		// Represent struct as map[string]any
		goStructMap := make(map[string]any, len(pbFields))
		for i, pbFieldInfo := range pbFields {
			fieldName := pbFieldInfo.GetFieldName()
			fieldPbType := pbFieldInfo.GetType()
			if fieldPbType == nil {
				return nil, fmt.Errorf("struct field %q type is nil", fieldName)
			}
			fieldPbValue := pbFieldValues[i]

			goFieldValue, err := pbValueToGoValue(fieldPbValue, fieldPbType) // Recursive call
			if err != nil {
				return nil, fmt.Errorf("error converting struct field %q: %w", fieldName, err)
			}
			goStructMap[fieldName] = goFieldValue // Store as any in the map
		}
		return goStructMap, nil

	default:
		return nil, fmt.Errorf("unsupported protobuf type kind for Go value conversion: %T", k)
	}
}

// assignValue attempts to assign src Go value to dest reflect.Value, handling common conversions.
// dest must be settable.
// errors returned must be wrapped by caller.
func assignValue(dest reflect.Value, src any) error {
	if !dest.CanSet() {
		return errors.New("destination is not settable")
	}

	if src == nil {
		// Assigning nil. Check if dest is nillable.
		switch dest.Kind() {
		case reflect.Interface, reflect.Ptr, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
			// Assign typed nil (zero value of the destination type)
			dest.Set(reflect.Zero(dest.Type()))
			return nil
		default:
			// Cannot assign nil to non-nillable types like int, string, bool, struct.
			return fmt.Errorf("cannot assign <nil> to destination of type %s", dest.Type())
		}
	}

	srcVal := reflect.ValueOf(src)

	// Direct assignment check
	if srcVal.Type().AssignableTo(dest.Type()) {
		dest.Set(srcVal)
		return nil
	}

	// Pointer destination: can we assign src to *dest?
	if dest.Kind() == reflect.Ptr && srcVal.Type().AssignableTo(dest.Type().Elem()) {
		// Allocate new pointer, set its element, assign pointer to dest
		newPtr := reflect.New(dest.Type().Elem())
		newPtr.Elem().Set(srcVal)
		dest.Set(newPtr)
		return nil
	}
	// Pointer source: can we assign *src to dest?
	if srcVal.Kind() == reflect.Ptr && !srcVal.IsNil() && srcVal.Elem().Type().AssignableTo(dest.Type()) {
		dest.Set(srcVal.Elem())
		return nil
	}

	// Common numeric conversions (allow assigning int64 to int, float64 to float32 etc.)
	if srcVal.CanConvert(dest.Type()) {
		dest.Set(srcVal.Convert(dest.Type()))
		return nil
	}

	// Special case: []byte <-> string
	// If dest is string, and src is []byte
	if dest.Kind() == reflect.String && srcVal.Kind() == reflect.Slice && srcVal.Type().Elem().Kind() == reflect.Uint8 {
		dest.SetString(string(srcVal.Bytes()))
		return nil
	}
	// If dest is []byte, and src is string
	if dest.Kind() == reflect.Slice && dest.Type().Elem().Kind() == reflect.Uint8 && srcVal.Kind() == reflect.String {
		dest.SetBytes([]byte(srcVal.String()))
		return nil
	}

	// TODO: Add more conversions. time.Time, int64, float32, float64 <-> string.  Requires parsing/formatting.
	return fmt.Errorf("unsupported type conversion from %s to %s", srcVal.Type(), dest.Type())
}
