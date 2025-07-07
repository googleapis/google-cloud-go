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
	"fmt"
)

// Row represents a single row in the query results.
type Row struct {
	schema *schema
	value  map[string]Value
}

// A Record behaves like Row, but it's embedded within a row
type Record = Row

func newRow(schema *schema) *Row {
	return &Row{
		schema: schema,
		value:  map[string]Value{},
	}
}

func (r *Row) setValue(columnIndex int, columnName string, value Value) {
	r.value[columnName] = value
}

// GetColumnAtIndex get a FieldValue by column index.
func (r *Row) GetColumnAtIndex(idx int) *FieldValue {
	if idx >= r.schema.len() {
		return nil
	}
	f := r.schema.pb.Fields[idx]
	return &FieldValue{
		Type:  FieldType(f.Type),
		Value: r.value[f.Name],
	}
}

// GetColumnName get a FieldValue by column name.
func (r *Row) GetColumnName(name string) *FieldValue {
	for _, f := range r.schema.pb.Fields {
		if f.Name == name {
			return &FieldValue{
				Type:  FieldType(f.Type),
				Value: r.value[f.Name],
			}
		}
	}
	return nil
}

// AsMap returns the row as a JSON object.
func (r *Row) AsMap() map[string]Value {
	values := map[string]Value{}
	for _, f := range r.schema.pb.Fields {
		fval := r.value[f.Name]
		if input, ok := fval.(*Row); ok {
			fval = input.AsMap()
		}
		if input, ok := fval.([]Value); ok {
			arr := []Value{}
			for _, row := range input {
				if input, ok := row.(*Row); ok {
					arr = append(arr, input.AsMap())
				} else {
					arr = append(arr, row)
				}
			}
			fval = arr
		}
		values[f.Name] = fval
	}
	return values
}

// AsValues decodes the row into an array of Value.
func (r *Row) AsValues() []Value {
	values := []Value{}
	for _, f := range r.schema.pb.Fields {
		v := r.value[f.Name]
		if input, ok := v.(*Row); ok {
			v = input.AsValues()
		}
		if input, ok := v.([]Value); ok {
			arr := []Value{}
			for _, row := range input {
				if input, ok := row.(*Row); ok {
					arr = append(arr, input.AsValues())
				} else {
					arr = append(arr, row)
				}
			}
			v = arr
		}
		values = append(values, v)
	}
	return values
}

// AsStruct decodes the row into a struct.
func (r *Row) AsStruct(v any) error {
	return fmt.Errorf("not implemented yet")
}
