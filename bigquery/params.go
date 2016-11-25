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

func paramType(x interface{}) (*bq.QueryParameterType, error) {
	switch x.(type) {
	case int, int8, int16, int32, int64, uint8, uint16, uint32:
		return int64ParamType, nil
	case float32, float64:
		return float64ParamType, nil
	case bool:
		return boolParamType, nil
	case string:
		return stringParamType, nil
	case time.Time:
		return timestampParamType, nil
	case []byte:
		return bytesParamType, nil
	default:
		return nil, fmt.Errorf("Go type %T cannot be represented as a parameter type", x)
	}
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
	default:
		return sval(fmt.Sprint(x)), nil
	}
}
