// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package managedreader

import (
	"bytes"
	"fmt"

	"cloud.google.com/go/bigquery"
	"github.com/apache/arrow/go/v10/arrow"
	"github.com/apache/arrow/go/v10/arrow/array"
	"github.com/apache/arrow/go/v10/arrow/ipc"
	"github.com/apache/arrow/go/v10/arrow/memory"

	bqStoragepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
)

type arrowParser struct {
	mem            *memory.GoAllocator
	tableSchema    bigquery.Schema
	rawArrowSchema []byte
	arrowSchema    *arrow.Schema
}

func newArrowParserFromSession(session *bqStoragepb.ReadSession, schema bigquery.Schema) (*arrowParser, error) {
	arrowSerializedSchema := session.GetArrowSchema().GetSerializedSchema()
	mem := memory.NewGoAllocator()
	buf := bytes.NewBuffer(arrowSerializedSchema)
	r, err := ipc.NewReader(buf, ipc.WithAllocator(mem))
	if err != nil {
		return nil, err
	}

	p := &arrowParser{
		mem:            mem,
		tableSchema:    schema,
		rawArrowSchema: arrowSerializedSchema,
		arrowSchema:    r.Schema(),
	}
	return p, nil
}

// convertArrowRows converts an Arrow Record Batch into a series of Value slices.
// schema is used to interpret the data from rows
func (ap *arrowParser) convertArrowRows(recordBatch *bqStoragepb.ArrowRecordBatch) ([][]bigquery.Value, error) {
	var rs [][]bigquery.Value
	buf := bytes.NewBuffer(ap.rawArrowSchema)
	unecoded := recordBatch.GetSerializedRecordBatch()
	buf.Write(unecoded)
	r, err := ipc.NewReader(buf, ipc.WithAllocator(ap.mem), ipc.WithSchema(ap.arrowSchema))
	if err != nil {
		return nil, err
	}
	for r.Next() {
		rec := r.Record()
		tmp := make([][]bigquery.Value, rec.NumRows())
		for i := range tmp {
			tmp[i] = make([]bigquery.Value, rec.NumCols())
		}
		for j, col := range rec.Columns() {
			fs := ap.tableSchema[j]
			ft := ap.arrowSchema.Field(j).Type
			for i := 0; i < col.Len(); i++ {
				v, err := convertArrowValue(col, i, ft, fs)
				if err != nil {
					return nil, fmt.Errorf("found arrow type %s, but could not convert value: %v", ap.arrowSchema.Field(j).Type, err)
				}
				tmp[i][j] = v
			}
		}
		rs = append(rs, tmp...)
	}
	return rs, nil
}

// convertArrow gets row value in the given column and converts to a Value.
// Arrow is a colunar storage, so we navigate first by column and get the row value.
func convertArrowValue(col arrow.Array, i int, ft arrow.DataType, fs *bigquery.FieldSchema) (bigquery.Value, error) {
	if !col.IsValid(i) {
		return nil, nil
	}
	switch ft.(type) {
	case *arrow.StringType:
		arr := col.(*array.String)
		return bigquery.ConvertValue(fmt.Sprintf("%v", arr.Value(i)), fs.Type, fs.Schema)
	case *arrow.Int8Type:
		arr := col.(*array.Int8)
		return bigquery.ConvertValue(fmt.Sprintf("%v", arr.Value(i)), fs.Type, fs.Schema)
	case *arrow.Int16Type:
		arr := col.(*array.Int16)
		return bigquery.ConvertValue(fmt.Sprintf("%v", arr.Value(i)), fs.Type, fs.Schema)
	case *arrow.Int32Type:
		arr := col.(*array.Int32)
		return bigquery.ConvertValue(fmt.Sprintf("%v", arr.Value(i)), fs.Type, fs.Schema)
	case *arrow.Int64Type:
		arr := col.(*array.Int64)
		return bigquery.ConvertValue(fmt.Sprintf("%v", arr.Value(i)), fs.Type, fs.Schema)
	case *arrow.Date32Type:
		arr := col.(*array.Date32)
		return bigquery.ConvertValue(arr.Value(i).FormattedString(), fs.Type, fs.Schema)
	case *arrow.Date64Type:
		arr := col.(*array.Date64)
		return bigquery.ConvertValue(arr.Value(i).FormattedString(), fs.Type, fs.Schema)
	case *arrow.StructType:
		arr := col.(*array.Struct)
		nestedValues := []bigquery.Value{}
		fields := ft.(*arrow.StructType).Fields()
		for fIndex, f := range fields {
			v, err := convertArrowValue(arr.Field(fIndex), i, f.Type, fs.Schema[fIndex])
			if err != nil {
				return nil, err
			}
			nestedValues = append(nestedValues, v)
		}
		return nestedValues, nil
	default:
		return nil, fmt.Errorf("unknown arrow type: %v", ft)
	}
}
