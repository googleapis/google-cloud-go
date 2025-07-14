// Copyright 2025 Google LLC
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

package query

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"cloud.google.com/go/civil"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"google.golang.org/protobuf/types/known/structpb"
)

type arrowDecoder struct {
	allocator             memory.Allocator
	tableSchema           *schema
	arrowSchema           *arrow.Schema
	arrowSerializedSchema []byte
}

func newArrowDecoder(arrowSerializedSchema []byte, schema *schema) (*arrowDecoder, error) {
	buf := bytes.NewBuffer(arrowSerializedSchema)
	r, err := ipc.NewReader(buf)
	if err != nil {
		return nil, err
	}
	defer r.Release()
	p := &arrowDecoder{
		tableSchema:           schema,
		arrowSchema:           r.Schema(),
		allocator:             memory.DefaultAllocator,
		arrowSerializedSchema: arrowSerializedSchema,
	}
	return p, nil
}

func (ap *arrowDecoder) createIPCReaderForBatch(r *storagepb.ArrowRecordBatch) (*ipc.Reader, error) {
	buf := bytes.NewBuffer(ap.arrowSerializedSchema)
	buf.Write(r.SerializedRecordBatch)
	return ipc.NewReader(
		buf,
		ipc.WithSchema(ap.arrowSchema),
		ipc.WithAllocator(ap.allocator),
	)
}

// decodeArrowRecords decodes BQ ArrowRecordBatch into rows of *Row.
func (ap *arrowDecoder) decodeArrowRecords(arrowRecordBatch *storagepb.ArrowRecordBatch) ([]*Row, error) {
	r, err := ap.createIPCReaderForBatch(arrowRecordBatch)
	if err != nil {
		return nil, err
	}
	defer r.Release()
	rs := make([]*Row, 0)
	for r.Next() {
		rec := r.Record()
		values, err := ap.convertArrowRecordValue(rec)
		if err != nil {
			return nil, err
		}
		rs = append(rs, values...)
	}
	return rs, nil
}

// convertArrowRows converts an arrow.Record into a series of Value slices.
func (ap *arrowDecoder) convertArrowRecordValue(record arrow.Record) ([]*Row, error) {
	rs := make([]*Row, record.NumRows())
	for i := range record.NumRows() {
		rs[i] = newRow(ap.tableSchema)
	}
	for j, col := range record.Columns() {
		fs := ap.tableSchema.pb.Fields[j]
		ft := ap.arrowSchema.Field(j).Type
		for i := 0; i < col.Len(); i++ {
			v, err := convertArrowValue(col, i, ft, fs)
			if err != nil {
				return nil, fmt.Errorf("found arrow type %s, but could not convert value: %v", ap.arrowSchema.Field(j).Type, err)
			}
			rs[i].setValue(fs.Name, v)
		}
	}
	return rs, nil
}

// convertArrow gets row value in the given column and converts to a Value.
// Arrow is a colunar storage, so we navigate first by column and get the row value.
// More details on conversions can be seen here: https://cloud.google.com/bigquery/docs/reference/storage#arrow_schema_details
func convertArrowValue(col arrow.Array, i int, ft arrow.DataType, fs *bigquerypb.TableFieldSchema) (*structpb.Value, error) {
	if !col.IsValid(i) {
		return nil, nil
	}
	switch ft.(type) {
	case *arrow.BooleanType:
		v := col.(*array.Boolean).Value(i)
		return convertBasicType(fmt.Sprintf("%v", v), FieldType(fs.Type))
	case *arrow.Int8Type:
		v := col.(*array.Int8).Value(i)
		return convertBasicType(fmt.Sprintf("%v", v), FieldType(fs.Type))
	case *arrow.Int16Type:
		v := col.(*array.Int16).Value(i)
		return convertBasicType(fmt.Sprintf("%v", v), FieldType(fs.Type))
	case *arrow.Int32Type:
		v := col.(*array.Int32).Value(i)
		return convertBasicType(fmt.Sprintf("%v", v), FieldType(fs.Type))
	case *arrow.Int64Type:
		v := col.(*array.Int64).Value(i)
		return convertBasicType(fmt.Sprintf("%v", v), FieldType(fs.Type))
	case *arrow.Float16Type:
		v := col.(*array.Float16).Value(i)
		return convertBasicType(fmt.Sprintf("%v", v.Float32()), FieldType(fs.Type))
	case *arrow.Float32Type:
		v := col.(*array.Float32).Value(i)
		return convertBasicType(fmt.Sprintf("%v", v), FieldType(fs.Type))
	case *arrow.Float64Type:
		v := col.(*array.Float64).Value(i)
		return convertBasicType(fmt.Sprintf("%v", v), FieldType(fs.Type))
	case *arrow.BinaryType:
		v := col.(*array.Binary).Value(i)
		encoded := base64.StdEncoding.EncodeToString(v)
		return convertBasicType(encoded, FieldType(fs.Type))
	case *arrow.StringType:
		v := col.(*array.String).Value(i)
		return convertBasicType(v, FieldType(fs.Type))
	case *arrow.Date32Type:
		v := col.(*array.Date32).Value(i)
		return convertBasicType(v.FormattedString(), FieldType(fs.Type))
	case *arrow.Date64Type:
		v := col.(*array.Date64).Value(i)
		return convertBasicType(v.FormattedString(), FieldType(fs.Type))
	case *arrow.TimestampType:
		v := col.(*array.Timestamp).Value(i)
		dft := ft.(*arrow.TimestampType)
		t := v.ToTime(dft.Unit)
		if dft.TimeZone == "" { // Datetime
			dt := civil.DateTimeOf(t)
			return structpb.NewStringValue(civilDateTimeString(dt)), nil
		}
		usec := fmt.Sprintf("%d", t.UTC().UnixMicro())
		return structpb.NewStringValue(usec), nil // Timestamp
	case *arrow.Time32Type:
		v := col.(*array.Time32).Value(i)
		return convertBasicType(v.FormattedString(arrow.Microsecond), FieldType(fs.Type))
	case *arrow.Time64Type:
		v := col.(*array.Time64).Value(i)
		return convertBasicType(v.FormattedString(arrow.Microsecond), FieldType(fs.Type))
	case *arrow.Decimal128Type:
		dft := ft.(*arrow.Decimal128Type)
		v := col.(*array.Decimal128).Value(i)
		s := strings.TrimRight(v.ToString(dft.Scale), "0")
		return structpb.NewStringValue(s), nil
	case *arrow.Decimal256Type:
		dft := ft.(*arrow.Decimal256Type)
		v := col.(*array.Decimal256).Value(i)
		s := strings.TrimRight(v.ToString(dft.Scale), "0")
		return structpb.NewStringValue(s), nil
	case *arrow.ListType:
		arr := col.(*array.List)
		dft := ft.(*arrow.ListType)
		values := []*structpb.Value{}
		start, end := arr.ValueOffsets(i)
		slice := array.NewSlice(arr.ListValues(), start, end)
		for j := 0; j < slice.Len(); j++ {
			v, err := convertArrowValue(slice, j, dft.Elem(), fs)
			if err != nil {
				return nil, err
			}
			values = append(values, v)
		}
		return structpb.NewListValue(&structpb.ListValue{
			Values: values,
		}), nil
	case *arrow.StructType:
		arr := col.(*array.Struct)
		s := newSchemaFromField(fs)
		nestedValues := s.newStruct()
		fields := ft.(*arrow.StructType).Fields()
		if fs.Type == string(RangeFieldType) {
			panic("range not supported yet")
		}
		for fIndex, f := range fields {
			v, err := convertArrowValue(arr.Field(fIndex), i, f.Type, fs.Fields[fIndex])
			if err != nil {
				return nil, err
			}
			nestedValues.Fields[f.Name] = v
		}
		return structpb.NewStructValue(nestedValues), nil
	default:
		return nil, fmt.Errorf("unknown arrow type: %v", ft)
	}
}
