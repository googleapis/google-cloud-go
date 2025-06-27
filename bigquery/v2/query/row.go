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
	raw  *structpb.Struct
	json []byte
}

func newRowFromFieldValueStruct(v *structpb.Struct, schema *bigquerypb.TableSchema) *Row {
	r := &Row{
		raw: v,
		// TODO: convert from field Value format to a json value
		json: []byte{},
	}
	return r
}

// AsJSON returns the row as a JSON object.
func (r *Row) AsJSON() (map[string]interface{}, error) {
	return r.raw.AsMap(), nil
}

// AsStruct decodes the row into a struct.
func (r *Row) AsStruct(v interface{}) error {
	return json.Unmarshal(r.json, &v)
}
