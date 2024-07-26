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
	"fmt"
	"log"
	"math/big"
	"reflect"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/executor/apiv1/executorpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// BuildQuery constructs a spanner.Statement query and bind the params from the input executorpb.QueryAction.
func BuildQuery(queryAction *executorpb.QueryAction) (spanner.Statement, error) {
	stmt := spanner.Statement{SQL: queryAction.GetSql(), Params: make(map[string]interface{})}
	for _, param := range queryAction.GetParams() {
		value, err := ExecutorValueToSpannerValue(param.GetType(), param.GetValue(), param.GetValue().GetIsNull())
		if err != nil {
			return spanner.Statement{}, err
		}
		stmt.Params[param.GetName()] = value
	}
	return stmt, nil
}

// encodedJSON is a pre-encoded JSON value, so when marshaled the underlying
// bytes are returned as-is.
type encodedJSON []byte

func (v encodedJSON) MarshalJSON() ([]byte, error) {
	return []byte(v), nil
}

// ExecutorValueToSpannerValue converts executorpb.Value with given spannerpb.Type into a cloud spanner interface.
// Parameter null indicates whether this value is NULL.
func ExecutorValueToSpannerValue(t *spannerpb.Type, v *executorpb.Value, null bool) (any, error) {
	if v.GetIsCommitTimestamp() {
		return spanner.NullTime{Time: spanner.CommitTimestamp, Valid: true}, nil
	}
	switch t.GetCode() {
	case spannerpb.TypeCode_INT64:
		return spanner.NullInt64{Int64: v.GetIntValue(), Valid: !null}, nil
	case spannerpb.TypeCode_FLOAT64:
		return spanner.NullFloat64{Float64: v.GetDoubleValue(), Valid: !null}, nil
	case spannerpb.TypeCode_STRING:
		return spanner.NullString{StringVal: v.GetStringValue(), Valid: !null}, nil
	case spannerpb.TypeCode_BYTES:
		if null {
			return []byte(nil), nil
		}
		out := v.GetBytesValue()
		if out == nil {
			out = make([]byte, 0)
		}
		return out, nil
	case spannerpb.TypeCode_BOOL:
		if null {
			return spanner.NullBool{}, nil
		}
		return spanner.NullBool{Bool: v.GetBoolValue(), Valid: !null}, nil
	case spannerpb.TypeCode_TIMESTAMP:
		if null {
			return spanner.NullTime{Time: time.Unix(0, 0), Valid: false}, nil
		}
		if v.GetIsCommitTimestamp() || v.GetBytesValue() != nil {
			return spanner.NullTime{Time: spanner.CommitTimestamp, Valid: true}, nil
		}
		return spanner.NullTime{Time: time.Unix(v.GetTimestampValue().Seconds, int64(v.GetTimestampValue().Nanos)), Valid: true}, nil
	case spannerpb.TypeCode_DATE:
		epoch := civil.DateOf(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC))
		y := epoch.AddDays(int(v.GetDateDaysValue()))
		return spanner.NullDate{Date: y, Valid: !null}, nil
	case spannerpb.TypeCode_NUMERIC:
		if null {
			return spanner.NullNumeric{Numeric: big.Rat{}, Valid: false}, nil
		}
		x := v.GetStringValue()
		y, ok := (&big.Rat{}).SetString(x)
		if !ok {
			return nil, spanner.ToSpannerError(status.Error(codes.InvalidArgument, fmt.Sprintf("unexpected string value %q for numeric number", x)))
		}
		return spanner.NullNumeric{Numeric: *y, Valid: true}, nil
	case spannerpb.TypeCode_JSON:
		if null {
			return spanner.NullJSON{}, nil
		}
		x := v.GetStringValue()
		return spanner.NullJSON{Value: encodedJSON(x), Valid: true}, nil
	case spannerpb.TypeCode_STRUCT:
		return executorStructValueToSpannerValue(t, v.GetStructValue(), null)
	case spannerpb.TypeCode_ARRAY:
		return executorArrayValueToSpannerValue(t, v, null)
	default:
		return nil, status.Errorf(codes.InvalidArgument, "executorValueToSpannerValue: unsupported type %s while converting from value proto.", t.GetCode().String())
	}
}

// executorStructValueToSpannerValue converts executorpb.Value with spannerpb.Type of TypeCode_STRUCT to a dynamically
// created pointer to a Go struct value with a type derived from t. If null is set, returns a nil pointer
// of the Go struct type for NULL struct values.
func executorStructValueToSpannerValue(t *spannerpb.Type, v *executorpb.ValueList, null bool) (any, error) {
	var fieldValues []*executorpb.Value
	fieldTypes := t.GetStructType().GetFields()
	if !null {
		fieldValues = v.GetValue()
		if len(fieldValues) != len(fieldTypes) {
			return nil, spanner.ToSpannerError(status.Error(codes.InvalidArgument, "Mismatch between number of expected fields and specified values for struct type"))
		}
	}

	cloudFields := make([]reflect.StructField, 0, len(fieldTypes))
	cloudFieldVals := make([]any, 0, len(fieldTypes))

	// Convert the fields to Go types and build the struct's dynamic type.
	for i := 0; i < len(fieldTypes); i++ {
		var techValue *executorpb.Value
		var isnull bool

		if null {
			isnull = true
			techValue = nil
		} else {
			isnull = isNullTechValue(fieldValues[i])
			techValue = fieldValues[i]
		}

		// Go structs do not allow empty and duplicate field names and lowercase field names
		// make the field unexported. We use struct tags for specifying field names.
		cloudFieldVal, err := ExecutorValueToSpannerValue(fieldTypes[i].Type, techValue, isnull)
		if err != nil {
			return nil, err
		}
		if cloudFieldVal == nil {
			return nil, status.Errorf(codes.Internal, "Was not able to calculate the type for %s", fieldTypes[i].Type)
		}

		cloudFields = append(cloudFields,
			reflect.StructField{
				Name: fmt.Sprintf("Field_%d", i),
				Type: reflect.TypeOf(cloudFieldVal),
				Tag:  reflect.StructTag(fmt.Sprintf(`spanner:"%s"`, fieldTypes[i].Name)),
			})
		cloudFieldVals = append(cloudFieldVals, cloudFieldVal)
	}

	cloudStructType := reflect.StructOf(cloudFields)
	if null {
		// Return a nil pointer to Go struct with the built struct type.
		return reflect.Zero(reflect.PtrTo(cloudStructType)).Interface(), nil
	}
	// For a non-null struct, set the field values.
	cloudStruct := reflect.New(cloudStructType)
	for i, fieldVal := range cloudFieldVals {
		cloudStruct.Elem().Field(i).Set(reflect.ValueOf(fieldVal))
	}
	// Returns a pointer to the Go struct.
	return cloudStruct.Interface(), nil
}

// isNullTechValue returns whether an executorpb.Value is Value_IsNull or not.
func isNullTechValue(tv *executorpb.Value) bool {
	switch tv.GetValueType().(type) {
	case *executorpb.Value_IsNull:
		return true
	default:
		return false
	}
}

// executorArrayValueToSpannerValue converts executorpb.Value with spannerpb.Type of TypeCode_ARRAY into a cloud Spanner's interface.
func executorArrayValueToSpannerValue(t *spannerpb.Type, v *executorpb.Value, null bool) (any, error) {
	switch t.GetArrayElementType().GetCode() {
	case spannerpb.TypeCode_INT64:
		if null {
			return ([]spanner.NullInt64)(nil), nil
		}
		out := make([]spanner.NullInt64, 0)
		for _, value := range v.GetArrayValue().GetValue() {
			out = append(out, spanner.NullInt64{Int64: value.GetIntValue(), Valid: !value.GetIsNull()})
		}
		return out, nil
	case spannerpb.TypeCode_FLOAT64:
		if null {
			return ([]spanner.NullFloat64)(nil), nil
		}
		out := make([]spanner.NullFloat64, 0)
		for _, value := range v.GetArrayValue().GetValue() {
			out = append(out, spanner.NullFloat64{Float64: value.GetDoubleValue(), Valid: !value.GetIsNull()})
		}
		return out, nil
	case spannerpb.TypeCode_STRING:
		if null {
			return ([]spanner.NullString)(nil), nil
		}
		out := make([]spanner.NullString, 0)
		for _, value := range v.GetArrayValue().GetValue() {
			out = append(out, spanner.NullString{StringVal: value.GetStringValue(), Valid: !value.GetIsNull()})
		}
		return out, nil
	case spannerpb.TypeCode_BYTES:
		if null {
			return ([][]byte)(nil), nil
		}
		out := make([][]byte, 0)
		for _, value := range v.GetArrayValue().GetValue() {
			if !value.GetIsNull() {
				out = append(out, value.GetBytesValue())
			}
		}
		return out, nil
	case spannerpb.TypeCode_BOOL:
		if null {
			return ([]spanner.NullBool)(nil), nil
		}
		out := make([]spanner.NullBool, 0)
		for _, value := range v.GetArrayValue().GetValue() {
			out = append(out, spanner.NullBool{Bool: value.GetBoolValue(), Valid: !value.GetIsNull()})
		}
		return out, nil
	case spannerpb.TypeCode_TIMESTAMP:
		if null {
			return ([]spanner.NullTime)(nil), nil
		}
		out := make([]spanner.NullTime, 0)
		for _, value := range v.GetArrayValue().GetValue() {
			spannerValue, err := ExecutorValueToSpannerValue(t.GetArrayElementType(), value, value.GetIsNull())
			if err != nil {
				return nil, err
			}
			if v, ok := spannerValue.(spanner.NullTime); ok {
				out = append(out, v)
			}
		}
		return out, nil
	case spannerpb.TypeCode_DATE:
		if null {
			return ([]spanner.NullDate)(nil), nil
		}
		out := make([]spanner.NullDate, 0)
		for _, value := range v.GetArrayValue().GetValue() {
			spannerValue, err := ExecutorValueToSpannerValue(t.GetArrayElementType(), value, value.GetIsNull())
			if err != nil {
				return nil, err
			}
			if v, ok := spannerValue.(spanner.NullDate); ok {
				out = append(out, v)
			}
		}
		return out, nil
	case spannerpb.TypeCode_NUMERIC:
		if null {
			return ([]spanner.NullNumeric)(nil), nil
		}
		out := make([]spanner.NullNumeric, 0)
		for _, value := range v.GetArrayValue().GetValue() {
			if value.GetIsNull() {
				out = append(out, spanner.NullNumeric{Numeric: big.Rat{}, Valid: false})
			} else {
				y, ok := (&big.Rat{}).SetString(value.GetStringValue())
				if !ok {
					return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "unexpected string value %q for numeric number", value.GetStringValue()))
				}
				out = append(out, spanner.NullNumeric{Numeric: *y, Valid: true})
			}
		}
		return out, nil
	case spannerpb.TypeCode_STRUCT:
		if null {
			// TODO(sriharshach): will remove this after few successful systest runs. Need this to debug logs.
			log.Println("Failing again due to passing untyped nil value for array of structs. Might need to change to typed nil similar to other types (made a fix below)")
		}
		// Non-NULL array of structs
		structElemType := t.GetArrayElementType()
		in := v.GetArrayValue()

		// Create a dummy struct value to get the element type.
		dummyStructPtr, err := executorStructValueToSpannerValue(structElemType, nil, true)
		if err != nil {
			return nil, err
		}
		goStructType := reflect.TypeOf(dummyStructPtr)
		if null {
			log.Printf("returning nil slice of struct : %q", reflect.Zero(reflect.SliceOf(goStructType)).Interface())
			return reflect.Zero(reflect.SliceOf(goStructType)).Interface(), nil
		}
		out := reflect.MakeSlice(reflect.SliceOf(goStructType), 0, len(in.GetValue()))
		for _, value := range in.GetValue() {
			cv, err := executorStructValueToSpannerValue(structElemType, value.GetStructValue(), false)
			if err != nil {
				return nil, err
			}
			et := reflect.TypeOf(cv)
			if !reflect.DeepEqual(et, goStructType) {
				return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "Mismatch between computed struct array element type %v and received element type %v", goStructType, et))
			}
			out = reflect.Append(out, reflect.ValueOf(cv))
		}
		return out.Interface(), nil
	case spannerpb.TypeCode_JSON:
		if null {
			return ([]spanner.NullJSON)(nil), nil
		}
		out := make([]spanner.NullJSON, 0)
		for _, value := range v.GetArrayValue().GetValue() {
			spannerValue, err := ExecutorValueToSpannerValue(t.GetArrayElementType(), value, value.GetIsNull())
			if err != nil {
				return nil, err
			}
			if v, ok := spannerValue.(spanner.NullJSON); ok {
				out = append(out, v)
			}
		}
		return out, nil
	default:
		return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "executorArrayValueToSpannerValue: unsupported array element type while converting from executor proto of type: %s", t.GetArrayElementType().GetCode().String()))
	}
}
