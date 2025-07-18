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
	"encoding/json"

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"google.golang.org/protobuf/types/known/structpb"
)

// Row represents a single row in the query results.
type Row struct {
	schema *schema
	pb     *structpb.Struct
}

// A Record behaves like Row, but it's embedded within a row
type Record = Row

func newRow(schema *schema) *Row {
	return &Row{
		schema: schema,
		pb:     schema.newStruct(),
	}
}

func (r *Row) setValue(columnName string, value *structpb.Value) {
	r.pb.Fields[columnName] = value
}

// AsMap returns the row as a JSON object.
func (r *Row) AsMap() map[string]any {
	return r.pb.AsMap()
}

// AsValues decodes the row into an array of Value.
func (r *Row) AsValues() []any {
	values := []any{}
	for _, f := range r.schema.pb.Fields {
		fv := r.pb.Fields[f.Name]
		v := fvToValues(f, fv)
		values = append(values, v)
	}
	return values
}

func fvToValues(f *bigquerypb.TableFieldSchema, v *structpb.Value) any {
	if s := v.GetStructValue(); s != nil {
		subrow := &Row{pb: s, schema: newSchemaFromField(f)}
		return subrow.AsValues()
	} else if l := v.GetListValue(); l != nil {
		arr := []any{}
		for _, row := range l.Values {
			subv := fvToValues(f, row)
			arr = append(arr, subv)
		}
		return arr
	}
	return v.AsInterface()
}

// AsStruct decodes the row into a structpb.Struct.
func (r *Row) AsStruct() *structpb.Struct {
	return r.pb
}

// Decode decodes the row into an user provided struct.
func (r *Row) Decode(v any) error {
	encoded, err := r.pb.MarshalJSON()
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, v)
}
