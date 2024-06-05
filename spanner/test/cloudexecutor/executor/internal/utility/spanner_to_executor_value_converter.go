// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utility

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/executor/apiv1/executorpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	proto3 "google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ConvertSpannerRow takes a Cloud Spanner Row and translates it to executorpb.ValueList and sppb.StructType.
// The result is always a struct, in which each value corresponds to a column of the Row.
func ConvertSpannerRow(row *spanner.Row) (*executorpb.ValueList, *sppb.StructType, error) {
	rowBuilder := &executorpb.ValueList{}
	rowTypeBuilder := &sppb.StructType{}
	for i := 0; i < row.Size(); i++ {
		rowTypeBuilderField := &sppb.StructType_Field{Name: row.ColumnName(i), Type: row.ColumnType(i)}
		rowTypeBuilder.Fields = append(rowTypeBuilder.Fields, rowTypeBuilderField)
		v, err := extractRowValue(row, i, row.ColumnType(i))
		if err != nil {
			return nil, nil, err
		}
		rowBuilder.Value = append(rowBuilder.Value, v)
	}
	return rowBuilder, rowTypeBuilder, nil
}

// extractRowValue extracts a single column's value at given index i from result row.
func extractRowValue(row *spanner.Row, i int, t *sppb.Type) (*executorpb.Value, error) {
	val := &executorpb.Value{}
	_, isNull := row.ColumnValue(i).Kind.(*proto3.Value_NullValue)
	if isNull {
		val.ValueType = &executorpb.Value_IsNull{IsNull: true}
		return val, nil
	}
	var err error
	// nested row
	if t.GetCode() == sppb.TypeCode_ARRAY && t.GetArrayElementType().GetCode() == sppb.TypeCode_STRUCT {
		log.Println("with in array<struct> that is unimplemented")
	}
	switch t.GetCode() {
	case sppb.TypeCode_BOOL:
		var v bool
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		val.ValueType = &executorpb.Value_BoolValue{BoolValue: v}
	case sppb.TypeCode_FLOAT64:
		var v float64
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		val.ValueType = &executorpb.Value_DoubleValue{DoubleValue: v}
	case sppb.TypeCode_INT64:
		var v int64
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		val.ValueType = &executorpb.Value_IntValue{IntValue: v}
	case sppb.TypeCode_STRING:
		var v string
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		val.ValueType = &executorpb.Value_StringValue{StringValue: v}
	case sppb.TypeCode_BYTES:
		var v []byte
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		val.ValueType = &executorpb.Value_BytesValue{BytesValue: v}
	case sppb.TypeCode_TIMESTAMP:
		var v time.Time
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		val.ValueType = &executorpb.Value_TimestampValue{TimestampValue: &timestamppb.Timestamp{Seconds: v.Unix(), Nanos: int32(v.Nanosecond())}}
	case sppb.TypeCode_DATE:
		var v civil.Date
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		epoch := civil.DateOf(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC))
		val.ValueType = &executorpb.Value_DateDaysValue{DateDaysValue: int32(v.DaysSince(epoch))}
	case sppb.TypeCode_NUMERIC:
		var numeric big.Rat
		err = row.Column(i, &numeric)
		if err != nil {
			return nil, err
		}
		v := spanner.NumericString(&numeric)
		val.ValueType = &executorpb.Value_StringValue{StringValue: v}
	case sppb.TypeCode_JSON:
		var v spanner.NullJSON
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		val.ValueType = &executorpb.Value_StringValue{StringValue: encodeJSON(v)}
	case sppb.TypeCode_ARRAY:
		val, err = extractRowArrayValue(row, i, t.GetArrayElementType())
		if err != nil {
			return nil, err
		}
	default:
		return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "extractRowValue: unable to extract value: type %s not supported", t.GetCode()))
	}
	return val, nil
}

func encodeJSON(n spanner.NullJSON) string {
	if !n.Valid {
		return "<null>"
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)

	err := encoder.Encode(n.Value)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	resultString := buf.String()

	// Trim the new line since json.Encoder.Encode adds a new line character at the end.
	resultString = strings.TrimSuffix(resultString, "\n")
	return resultString
}

// extractRowArrayValue extracts a single column's array value at given index i from result row.
func extractRowArrayValue(row *spanner.Row, i int, t *sppb.Type) (*executorpb.Value, error) {
	val := &executorpb.Value{}
	var err error
	switch t.GetCode() {
	case sppb.TypeCode_BOOL:
		arrayValue := &executorpb.ValueList{}
		var v []*bool
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		for _, booleanValue := range v {
			if booleanValue == nil {
				value := &executorpb.Value{ValueType: &executorpb.Value_IsNull{IsNull: true}}
				arrayValue.Value = append(arrayValue.Value, value)
			} else {
				value := &executorpb.Value{ValueType: &executorpb.Value_BoolValue{BoolValue: *booleanValue}}
				arrayValue.Value = append(arrayValue.Value, value)
			}
		}
		val.ValueType = &executorpb.Value_ArrayValue{ArrayValue: arrayValue}
		val.ArrayType = &sppb.Type{Code: sppb.TypeCode_BOOL}
	case sppb.TypeCode_FLOAT64:
		arrayValue := &executorpb.ValueList{}
		var v []*float64
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		for _, vv := range v {
			if vv == nil {
				value := &executorpb.Value{ValueType: &executorpb.Value_IsNull{IsNull: true}}
				arrayValue.Value = append(arrayValue.Value, value)
			} else {
				value := &executorpb.Value{ValueType: &executorpb.Value_DoubleValue{DoubleValue: *vv}}
				arrayValue.Value = append(arrayValue.Value, value)
			}
		}
		val.ValueType = &executorpb.Value_ArrayValue{ArrayValue: arrayValue}
		val.ArrayType = &sppb.Type{Code: sppb.TypeCode_FLOAT64}
	case sppb.TypeCode_INT64:
		arrayValue := &executorpb.ValueList{}
		var v []*int64
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		for _, vv := range v {
			if vv == nil {
				value := &executorpb.Value{ValueType: &executorpb.Value_IsNull{IsNull: true}}
				arrayValue.Value = append(arrayValue.Value, value)
			} else {
				value := &executorpb.Value{ValueType: &executorpb.Value_IntValue{IntValue: *vv}}
				arrayValue.Value = append(arrayValue.Value, value)
			}
		}
		val.ValueType = &executorpb.Value_ArrayValue{ArrayValue: arrayValue}
		val.ArrayType = &sppb.Type{Code: sppb.TypeCode_INT64}
	case sppb.TypeCode_STRING:
		arrayValue := &executorpb.ValueList{}
		var v []*string
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		for _, vv := range v {
			if vv == nil {
				value := &executorpb.Value{ValueType: &executorpb.Value_IsNull{IsNull: true}}
				arrayValue.Value = append(arrayValue.Value, value)
			} else {
				value := &executorpb.Value{ValueType: &executorpb.Value_StringValue{StringValue: *vv}}
				arrayValue.Value = append(arrayValue.Value, value)
			}
		}
		val.ValueType = &executorpb.Value_ArrayValue{ArrayValue: arrayValue}
		val.ArrayType = &sppb.Type{Code: sppb.TypeCode_STRING}
	case sppb.TypeCode_BYTES:
		arrayValue := &executorpb.ValueList{}
		var v [][]byte
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		for _, vv := range v {
			if vv == nil {
				value := &executorpb.Value{ValueType: &executorpb.Value_IsNull{IsNull: true}}
				arrayValue.Value = append(arrayValue.Value, value)
			} else {
				value := &executorpb.Value{ValueType: &executorpb.Value_BytesValue{BytesValue: vv}}
				arrayValue.Value = append(arrayValue.Value, value)
			}
		}
		val.ValueType = &executorpb.Value_ArrayValue{ArrayValue: arrayValue}
		val.ArrayType = &sppb.Type{Code: sppb.TypeCode_BYTES}
	case sppb.TypeCode_DATE:
		arrayValue := &executorpb.ValueList{}
		var v []*civil.Date
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		epoch := civil.DateOf(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC))
		for _, vv := range v {
			if vv == nil {
				value := &executorpb.Value{ValueType: &executorpb.Value_IsNull{IsNull: true}}
				arrayValue.Value = append(arrayValue.Value, value)
			} else {
				value := &executorpb.Value{ValueType: &executorpb.Value_DateDaysValue{DateDaysValue: int32(vv.DaysSince(epoch))}}
				arrayValue.Value = append(arrayValue.Value, value)
			}
		}
		val.ValueType = &executorpb.Value_ArrayValue{ArrayValue: arrayValue}
		val.ArrayType = &sppb.Type{Code: sppb.TypeCode_DATE}
	case sppb.TypeCode_TIMESTAMP:
		arrayValue := &executorpb.ValueList{}
		var v []*time.Time
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		for _, vv := range v {
			if vv == nil {
				value := &executorpb.Value{ValueType: &executorpb.Value_IsNull{IsNull: true}}
				arrayValue.Value = append(arrayValue.Value, value)
			} else {
				value := &executorpb.Value{ValueType: &executorpb.Value_TimestampValue{TimestampValue: &timestamppb.Timestamp{Seconds: vv.Unix(), Nanos: int32(vv.Nanosecond())}}}
				arrayValue.Value = append(arrayValue.Value, value)
			}
		}
		val.ValueType = &executorpb.Value_ArrayValue{ArrayValue: arrayValue}
		val.ArrayType = &sppb.Type{Code: sppb.TypeCode_TIMESTAMP}
	case sppb.TypeCode_NUMERIC:
		arrayValue := &executorpb.ValueList{}
		var v []*big.Rat
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		for _, vv := range v {
			if vv == nil {
				value := &executorpb.Value{ValueType: &executorpb.Value_IsNull{IsNull: true}}
				arrayValue.Value = append(arrayValue.Value, value)
			} else {
				value := &executorpb.Value{ValueType: &executorpb.Value_StringValue{StringValue: spanner.NumericString(vv)}}
				arrayValue.Value = append(arrayValue.Value, value)
			}
		}
		val.ValueType = &executorpb.Value_ArrayValue{ArrayValue: arrayValue}
		val.ArrayType = &sppb.Type{Code: sppb.TypeCode_NUMERIC}
	case sppb.TypeCode_JSON:
		arrayValue := &executorpb.ValueList{}
		var v []spanner.NullJSON
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		for _, vv := range v {
			if !vv.Valid {
				value := &executorpb.Value{ValueType: &executorpb.Value_IsNull{IsNull: true}}
				arrayValue.Value = append(arrayValue.Value, value)
			} else {
				value := &executorpb.Value{ValueType: &executorpb.Value_StringValue{StringValue: encodeJSON(vv)}}
				arrayValue.Value = append(arrayValue.Value, value)
			}
		}
		val.ValueType = &executorpb.Value_ArrayValue{ArrayValue: arrayValue}
		val.ArrayType = &sppb.Type{Code: sppb.TypeCode_JSON}
	default:
		return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "extractRowArrayValue: unable to extract value: type %s not supported", t.GetCode()))
	}
	return val, nil
}
