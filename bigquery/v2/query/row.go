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

	"google.golang.org/protobuf/types/known/structpb"
)

// Row represents a single row in the query results.
type Row struct {
	raw   *structpb.Struct // TODO use arrow.Record internally ?
	value []Value
	json  []byte
}

func newRowFromValues(values []Value) *Row {
	r := &Row{
		value: values,
	}
	return r
}

// AsJSON returns the row as a JSON object.
func (r *Row) AsJSON() (map[string]interface{}, error) {
	return r.raw.AsMap(), nil
}

// AsValue decodes the row into an array of Value.
func (r *Row) AsValue() ([]Value, error) {
	return r.value, nil
}

// AsStruct decodes the row into a struct.
func (r *Row) AsStruct(v interface{}) error {
	return json.Unmarshal(r.json, &v)
}
