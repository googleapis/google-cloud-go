// Copyright 2016 Google Inc. All Rights Reserved.
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

package bigquery

import (
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"time"

	"cloud.google.com/go/internal/fields"

	bq "google.golang.org/api/bigquery/v2"
)

// See https://cloud.google.com/bigquery/docs/reference/standard-sql/data-types#timestamp-type.
var timestampFormat = "2006-01-02 15:04:05.999999-07:00"

var fieldCache = fields.NewCache(nil)

var (
	int64ParamType     = &bq.QueryParameterType{Type: "INT64"}
	float64ParamType   = &bq.QueryParameterType{Type: "FLOAT64"}
	boolParamType      = &bq.QueryParameterType{Type: "BOOL"}
	stringParamType    = &bq.QueryParameterType{Type: "STRING"}
	bytesParamType     = &bq.QueryParameterType{Type: "BYTES"}
	timestampParamType = &bq.QueryParameterType{Type: "TIMESTAMP"}
)

var timeType = reflect.TypeOf(time.Time{})

func paramType(t reflect.Type) (*bq.QueryParameterType, error) {
	if t == nil {
		return nil, errors.New("bigquery: nil parameter")
	}
	if t == timeType {
		return timestampParamType, nil
	}
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return int64ParamType, nil

	case reflect.Float32, reflect.Float64:
		return float64ParamType, nil

	case reflect.Bool:
		return boolParamType, nil

	case reflect.String:
		return stringParamType, nil

	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return bytesParamType, nil
		}
		fallthrough

	case reflect.Array:
		et, err := paramType(t.Elem())
		if err != nil {
			return nil, err
		}
		return &bq.QueryParameterType{Type: "ARRAY", ArrayType: et}, nil

	case reflect.Ptr:
		if t.Elem().Kind() != reflect.Struct {
			break
		}
		t = t.Elem()
		fallthrough

	case reflect.Struct:
		var fts []*bq.QueryParameterTypeStructTypes
		for _, f := range fieldCache.Fields(t) {
			pt, err := paramType(f.Type)
			if err != nil {
				return nil, err
			}
			fts = append(fts, &bq.QueryParameterTypeStructTypes{
				Name: f.Name,
				Type: pt,
			})
		}
		return &bq.QueryParameterType{Type: "STRUCT", StructTypes: fts}, nil
	}
	return nil, fmt.Errorf("bigquery: Go type %s cannot be represented as a parameter type", t)
}

func paramValue(v reflect.Value) (bq.QueryParameterValue, error) {
	var res bq.QueryParameterValue
	if !v.IsValid() {
		return res, errors.New("bigquery: nil parameter")
	}
	t := v.Type()
	if t == timeType {
		res.Value = v.Interface().(time.Time).Format(timestampFormat)
		return res, nil
	}
	switch t.Kind() {
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			res.Value = base64.StdEncoding.EncodeToString(v.Interface().([]byte))
			return res, nil
		}
		fallthrough

	case reflect.Array:
		var vals []*bq.QueryParameterValue
		for i := 0; i < v.Len(); i++ {
			val, err := paramValue(v.Index(i))
			if err != nil {
				return bq.QueryParameterValue{}, err
			}
			vals = append(vals, &val)
		}
		return bq.QueryParameterValue{ArrayValues: vals}, nil

	case reflect.Ptr:
		if t.Elem().Kind() != reflect.Struct {
			return res, fmt.Errorf("bigquery: Go type %s cannot be represented as a parameter value", t)
		}
		t = t.Elem()
		v = v.Elem()
		if !v.IsValid() {
			// nil pointer becomes empty value
			return res, nil
		}
		fallthrough

	case reflect.Struct:
		fields := fieldCache.Fields(t)
		res.StructValues = map[string]bq.QueryParameterValue{}
		for _, f := range fields {
			fv := v.FieldByIndex(f.Index)
			fp, err := paramValue(fv)
			if err != nil {
				return bq.QueryParameterValue{}, err
			}
			res.StructValues[f.Name] = fp
		}
		return res, nil
	}
	// None of the above: assume a scalar type. (If it's not a valid type,
	// paramType will catch the error.)
	res.Value = fmt.Sprint(v.Interface())
	return res, nil
}
