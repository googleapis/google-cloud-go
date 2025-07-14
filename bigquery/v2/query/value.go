// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package query

import (
	"errors"
	"fmt"
	"strconv"

	"google.golang.org/protobuf/types/known/structpb"
)

// convertRows converts a series of TableRows into a series of Value slices.
// schema is used to interpret the data from rows; its length must match the
// length of each row.
func convertRows(rows []*structpb.Struct, schema *schema) ([]*Row, error) {
	var rs []*Row
	for _, r := range rows {
		row, err := convertRow(r, schema)
		if err != nil {
			return nil, err
		}
		rs = append(rs, row)
	}
	return rs, nil
}

func convertRow(r *structpb.Struct, schema *schema) (*Row, error) {
	fields, err := getFieldList(r)
	if err != nil {
		return nil, err
	}
	if schema.len() != len(fields) {
		return nil, errors.New("schema length does not match row length")
	}
	row := newRow(schema)
	for i, cell := range fields {
		cellValue, err := getFieldValue(cell)
		if err != nil {
			return nil, err
		}
		fs := schema.pb.Fields[i]
		var v *structpb.Value
		if fs.Type == string(RangeFieldType) {
			panic("range not supported yet")
		} else {
			v, err = convertValue(cellValue, FieldType(fs.Type), newSchemaFromField(fs))
		}
		if err != nil {
			return nil, err
		}
		row.setValue(fs.Name, v)
	}
	return row, nil
}

func convertValue(val *structpb.Value, typ FieldType, schema *schema) (*structpb.Value, error) {
	switch val.Kind.(type) {
	case *structpb.Value_NullValue:
		return nil, nil
	case *structpb.Value_ListValue:
		return convertRepeatedRecord(val.GetListValue(), typ, schema)
	case *structpb.Value_StructValue:
		return convertNestedRecord(val.GetStructValue(), schema)
	case *structpb.Value_StringValue:
		return convertBasicType(val.GetStringValue(), typ)
	default:
		return nil, fmt.Errorf("got value %v; expected a value of type %s", val, typ)
	}
}

func convertRepeatedRecord(vals *structpb.ListValue, typ FieldType, schema *schema) (*structpb.Value, error) {
	var values []*structpb.Value
	for _, cell := range vals.Values {
		// each cell contains a single entry, keyed by "v"
		val, err := getFieldValue(cell)
		if err != nil {
			return nil, err
		}
		v, err := convertValue(val, typ, schema)
		if err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return structpb.NewListValue(&structpb.ListValue{
		Values: values,
	}), nil
}

func convertNestedRecord(val *structpb.Struct, schema *schema) (*structpb.Value, error) {
	// convertNestedRecord is similar to convertRow, as a record has the same structure as a row.
	// Nested records are wrapped in a map with a single key, "f".
	record, err := getFieldList(val)
	if err != nil {
		return nil, err
	}
	if schema.len() != len(record) {
		return nil, errors.New("schema length does not match row length")
	}

	r := newRow(schema)
	for i, cell := range record {
		// each cell contains a single entry, keyed by "v"
		val, err := getFieldValue(cell)
		if err != nil {
			return nil, err
		}
		fs := schema.pb.Fields[i]
		v, err := convertValue(val, FieldType(fs.Type), newSchemaFromField(fs))
		if err != nil {
			return nil, err
		}
		r.setValue(fs.Name, v)
	}
	s, err := r.AsStruct()
	if err != nil {
		return nil, err
	}
	return structpb.NewStructValue(s), nil
}

// convertBasicType returns val as an interface with a concrete type specified by typ.
func convertBasicType(val string, typ FieldType) (*structpb.Value, error) {
	switch typ {
	case StringFieldType, BytesFieldType, TimestampFieldType, DateFieldType, TimeFieldType,
		DateTimeFieldType, NumericFieldType, BigNumericFieldType, GeographyFieldType,
		JSONFieldType:
		return structpb.NewStringValue(val), nil
	case IntegerFieldType:
		v, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return nil, err
		}
		return structpb.NewNumberValue(float64(v)), nil
	case FloatFieldType:
		v, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return nil, err
		}
		return structpb.NewNumberValue(float64(v)), nil
	case BooleanFieldType:
		v, err := strconv.ParseBool(val)
		if err != nil {
			return nil, err
		}
		return structpb.NewBoolValue(v), nil
	case IntervalFieldType:
		panic("interval not supported yet")
	default:
		return nil, fmt.Errorf("unrecognized type: %s", typ)
	}
}

func fieldValueRowsToRowList(rows []*structpb.Struct, schema *schema) ([]*Row, error) {
	return convertRows(rows, schema)
}

func getFieldList(r *structpb.Struct) ([]*structpb.Value, error) {
	fieldValue := r.Fields["f"]
	if fieldValue == nil {
		return nil, errors.New("missing fields in row")
	}
	fields := fieldValue.GetListValue()
	if fields == nil {
		return nil, errors.New("missing fields in row")
	}
	return fields.GetValues(), nil
}

func getFieldValue(v *structpb.Value) (*structpb.Value, error) {
	s := v.GetStructValue()
	if s == nil {
		return nil, errors.New("missing value in a field row")
	}
	fieldValue := s.Fields["v"]
	if fieldValue == nil {
		return nil, errors.New("missing value in a field row")
	}
	return fieldValue, nil
}
