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
	"fmt"
	"reflect"
	"time"

	bq "google.golang.org/api/bigquery/v2"
)

// See https://cloud.google.com/bigquery/docs/reference/standard-sql/data-types#timestamp-type.
var timestampFormat = "2006-01-02 15:04:05.999999-07:00"

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
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return int64ParamType, nil
	case reflect.Float32, reflect.Float64:
		return float64ParamType, nil
	case reflect.Bool:
		return boolParamType, nil
	case reflect.String:
		return stringParamType, nil
	case reflect.Slice, reflect.Array:
		if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
			return bytesParamType, nil
		}
		et, err := paramType(t.Elem())
		if err != nil {
			return nil, err
		}
		return &bq.QueryParameterType{Type: "ARRAY", ArrayType: et}, nil
	}
	if t == timeType {
		return timestampParamType, nil
	}
	return nil, fmt.Errorf("Go type %s cannot be represented as a parameter type", t)
}

func paramValue(x interface{}) (bq.QueryParameterValue, error) {
	// convenience function for scalar value
	sval := func(s string) bq.QueryParameterValue {
		return bq.QueryParameterValue{Value: s}
	}
	switch x := x.(type) {
	case []byte:
		return sval(base64.StdEncoding.EncodeToString(x)), nil
	case time.Time:
		return sval(x.Format(timestampFormat)), nil
	}
	t := reflect.TypeOf(x)
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		var vals []*bq.QueryParameterValue
		v := reflect.ValueOf(x)
		for i := 0; i < v.Len(); i++ {
			val, err := paramValue(v.Index(i).Interface())
			if err != nil {
				return bq.QueryParameterValue{}, err
			}
			vals = append(vals, &val)
		}
		return bq.QueryParameterValue{ArrayValues: vals}, nil
	}
	return sval(fmt.Sprint(x)), nil
}
