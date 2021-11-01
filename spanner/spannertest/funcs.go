/*
Copyright 2020 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spannertest

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner/spansql"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// This file contains implementations of query functions.

type function struct {
	// Eval evaluates the result of the function using the given input.
	Eval func(values []interface{}, types []spansql.Type) (interface{}, spansql.Type, error)
}

func firstErr(errors []error) error {
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}

var functions = map[string]function{
	"STARTS_WITH": {
		Eval: func(values []interface{}, types []spansql.Type) (interface{}, spansql.Type, error) {
			// TODO: Refine error messages to exactly match Spanner.
			// Check input values first.
			if len(values) != 2 {
				return nil, spansql.Type{}, status.Error(codes.InvalidArgument, "No matching signature for function STARTS_WITH for the given argument types")
			}
			for _, v := range values {
				// TODO: STARTS_WITH also supports BYTES as input parameters.
				if _, ok := v.(string); !ok {
					return nil, spansql.Type{}, status.Error(codes.InvalidArgument, "No matching signature for function STARTS_WITH for the given argument types")
				}
			}
			s := values[0].(string)
			prefix := values[1].(string)
			return strings.HasPrefix(s, prefix), spansql.Type{Base: spansql.Bool}, nil
		},
	},
	"LOWER": {
		Eval: func(values []interface{}, types []spansql.Type) (interface{}, spansql.Type, error) {
			if len(values) != 1 {
				return nil, spansql.Type{}, status.Error(codes.InvalidArgument, "No matching signature for function LOWER for the given argument types")
			}
			if values[0] == nil {
				return nil, spansql.Type{Base: spansql.String}, nil
			}
			if _, ok := values[0].(string); !ok {
				return nil, spansql.Type{}, status.Error(codes.InvalidArgument, "No matching signature for function LOWER for the given argument types")
			}
			return strings.ToLower(values[0].(string)), spansql.Type{Base: spansql.String}, nil
		},
	},
	"CAST": {
		Eval: func(values []interface{}, types []spansql.Type) (interface{}, spansql.Type, error) {
			return cast(values, types, false)
		},
	},
	"SAFE_CAST": {
		Eval: func(values []interface{}, types []spansql.Type) (interface{}, spansql.Type, error) {
			return cast(values, types, true)
		},
	},
}

func cast(values []interface{}, types []spansql.Type, safe bool) (interface{}, spansql.Type, error) {
	name := "CAST"
	if safe {
		name = "SAFE_CAST"
	}
	if len(types) != 1 {
		return nil, spansql.Type{}, status.Errorf(codes.InvalidArgument, "No type information for function %s for the given arguments", name)
	}
	if len(values) != 1 {
		return nil, spansql.Type{}, status.Errorf(codes.InvalidArgument, "No matching signature for function %s for the given arguments", name)
	}
	// If the input type is an error, then the conversion itself failed.
	if err, ok := values[0].(error); ok {
		if safe {
			return nil, types[0], nil
		}
		return nil, types[0], err
	}
	return values[0], types[0], nil
}

func convert(val interface{}, tp spansql.Type) (interface{}, error) {
	// TODO: Implement more conversions.
	if tp.Array {
		return nil, status.Errorf(codes.Unimplemented, "conversion to ARRAY types is not implemented")
	}
	var res interface{}
	var convertErr, err error
	switch tp.Base {
	case spansql.Int64:
		res, convertErr, err = convertToInt64(val)
	case spansql.Float64:
		res, convertErr, err = convertToFloat64(val)
	case spansql.String:
		res, convertErr, err = convertToString(val)
	case spansql.Bool:
		res, convertErr, err = convertToBool(val)
	case spansql.Date:
		res, convertErr, err = convertToDate(val)
	case spansql.Timestamp:
		res, convertErr, err = convertToTimestamp(val)
	case spansql.Numeric:
	case spansql.JSON:
	}
	if err != nil {
		return nil, err
	}
	if convertErr != nil {
		res = convertErr
	}
	if res != nil {
		return res, nil
	}

	return nil, status.Errorf(codes.Unimplemented, "unsupported conversion for %v to %v", val, tp.Base.SQL())
}

func convertToInt64(val interface{}) (res int64, convertErr error, err error) {
	switch v := val.(type) {
	case int64:
		return v, nil, nil
	case string:
		res, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, status.Errorf(codes.InvalidArgument, "invalid value for INT64: %q", v), nil
		}
		return res, nil, nil
	}
	return 0, nil, status.Errorf(codes.Unimplemented, "unsupported conversion for %v to INT64", val)
}

func convertToFloat64(val interface{}) (res float64, convertErr error, err error) {
	switch v := val.(type) {
	case int64:
		return float64(v), nil, nil
	case float64:
		return v, nil, nil
	case string:
		res, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, status.Errorf(codes.InvalidArgument, "invalid value for FLOAT64: %q", v), nil
		}
		return res, nil, nil
	}
	return 0, nil, status.Errorf(codes.Unimplemented, "unsupported conversion for %v to FLOAT64", val)
}

func convertToString(val interface{}) (res string, convertErr error, err error) {
	switch v := val.(type) {
	case string:
		return v, nil, nil
	case bool, int64, float64:
		return fmt.Sprintf("%v", v), nil, nil
	case civil.Date:
		return v.String(), nil, nil
	case time.Time:
		return v.UTC().Format(time.RFC3339Nano), nil, nil
	}
	return "", nil, status.Errorf(codes.Unimplemented, "unsupported conversion for %v to STRING", val)
}

func convertToBool(val interface{}) (res bool, convertErr error, err error) {
	switch v := val.(type) {
	case bool:
		return v, nil, nil
	case string:
		res, err := strconv.ParseBool(v)
		if err != nil {
			return false, status.Errorf(codes.InvalidArgument, "invalid value for BOOL: %q", v), nil
		}
		return res, nil, nil
	}
	return false, nil, status.Errorf(codes.Unimplemented, "unsupported conversion for %v to BOOL", val)
}

func convertToDate(val interface{}) (res civil.Date, convertErr error, err error) {
	switch v := val.(type) {
	case civil.Date:
		return v, nil, nil
	case string:
		res, err := civil.ParseDate(v)
		if err != nil {
			return civil.Date{}, status.Errorf(codes.InvalidArgument, "invalid value for DATE: %q", v), nil
		}
		return res, nil, nil
	}
	return civil.Date{}, nil, status.Errorf(codes.Unimplemented, "unsupported conversion for %v to DATE", val)
}

func convertToTimestamp(val interface{}) (res time.Time, convertErr error, err error) {
	switch v := val.(type) {
	case time.Time:
		return v, nil, nil
	case string:
		res, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return time.Time{}, status.Errorf(codes.InvalidArgument, "invalid value for TIMESTAMP: %q", v), nil
		}
		return res, nil, nil
	}
	return time.Time{}, nil, status.Errorf(codes.Unimplemented, "unsupported conversion for %v to TIMESTAMP", val)
}

type aggregateFunc struct {
	// Whether the function can take a * arg (only COUNT).
	AcceptStar bool

	// Every aggregate func takes one expression.
	Eval func(values []interface{}, typ spansql.Type) (interface{}, spansql.Type, error)

	// TODO: Handle qualifiers such as DISTINCT.
}

// TODO: more aggregate funcs.
var aggregateFuncs = map[string]aggregateFunc{
	"ANY_VALUE": {
		// https://cloud.google.com/spanner/docs/aggregate_functions#any_value
		Eval: func(values []interface{}, typ spansql.Type) (interface{}, spansql.Type, error) {
			// Return the first non-NULL value.
			for _, v := range values {
				if v != nil {
					return v, typ, nil
				}
			}
			// Either all values are NULL, or there are no values.
			return nil, typ, nil
		},
	},
	"ARRAY_AGG": {
		// https://cloud.google.com/spanner/docs/aggregate_functions#array_agg
		Eval: func(values []interface{}, typ spansql.Type) (interface{}, spansql.Type, error) {
			if typ.Array {
				return nil, spansql.Type{}, fmt.Errorf("ARRAY_AGG unsupported on values of type %v", typ.SQL())
			}
			typ.Array = true // use as return type
			if len(values) == 0 {
				// "If there are zero input rows, this function returns NULL."
				return nil, typ, nil
			}
			return values, typ, nil
		},
	},
	"COUNT": {
		AcceptStar: true,
		Eval: func(values []interface{}, typ spansql.Type) (interface{}, spansql.Type, error) {
			// Count the number of non-NULL values.
			// COUNT(*) receives a list of non-NULL placeholders rather than values,
			// so every value will be non-NULL.
			var n int64
			for _, v := range values {
				if v != nil {
					n++
				}
			}
			return n, int64Type, nil
		},
	},
	"MAX": {Eval: func(values []interface{}, typ spansql.Type) (interface{}, spansql.Type, error) {
		return evalMinMax("MAX", false, values, typ)
	}},
	"MIN": {Eval: func(values []interface{}, typ spansql.Type) (interface{}, spansql.Type, error) {
		return evalMinMax("MIN", true, values, typ)
	}},
	"SUM": {
		Eval: func(values []interface{}, typ spansql.Type) (interface{}, spansql.Type, error) {
			if typ.Array || !(typ.Base == spansql.Int64 || typ.Base == spansql.Float64) {
				return nil, spansql.Type{}, fmt.Errorf("SUM only supports arguments of INT64 or FLOAT64 type, not %s", typ.SQL())
			}
			if typ.Base == spansql.Int64 {
				var seen bool
				var sum int64
				for _, v := range values {
					if v == nil {
						continue
					}
					seen = true
					sum += v.(int64)
				}
				if !seen {
					// "Returns NULL if the input contains only NULLs".
					return nil, typ, nil
				}
				return sum, typ, nil
			}
			var seen bool
			var sum float64
			for _, v := range values {
				if v == nil {
					continue
				}
				seen = true
				sum += v.(float64)
			}
			if !seen {
				// "Returns NULL if the input contains only NULLs".
				return nil, typ, nil
			}
			return sum, typ, nil
		},
	},
	"AVG": {
		Eval: func(values []interface{}, typ spansql.Type) (interface{}, spansql.Type, error) {
			if typ.Array || !(typ.Base == spansql.Int64 || typ.Base == spansql.Float64) {
				return nil, spansql.Type{}, fmt.Errorf("AVG only supports arguments of INT64 or FLOAT64 type, not %s", typ.SQL())
			}
			if typ.Base == spansql.Int64 {
				var sum int64
				var n float64
				for _, v := range values {
					if v == nil {
						continue
					}
					sum += v.(int64)
					n++
				}
				if n == 0 {
					// "Returns NULL if the input contains only NULLs".
					return nil, typ, nil
				}
				return (float64(sum) / n), float64Type, nil
			}
			var sum float64
			var n float64
			for _, v := range values {
				if v == nil {
					continue
				}
				sum += v.(float64)
				n++
			}
			if n == 0 {
				// "Returns NULL if the input contains only NULLs".
				return nil, typ, nil
			}
			return (sum / n), typ, nil
		},
	},
}

func evalMinMax(name string, isMin bool, values []interface{}, typ spansql.Type) (interface{}, spansql.Type, error) {
	if typ.Array {
		return nil, spansql.Type{}, fmt.Errorf("%s only supports non-array arguments, not %s", name, typ.SQL())
	}
	if len(values) == 0 {
		// "Returns NULL if there are zero input rows".
		return nil, typ, nil
	}

	// Compute running MIN/MAX.
	// "Returns NULL if ... expression evaluates to NULL for all rows".
	var minMax interface{}
	for _, v := range values {
		if v == nil {
			// "Returns the {maximum|minimum} value of non-NULL expressions".
			continue
		}
		if typ.Base == spansql.Float64 && math.IsNaN(v.(float64)) {
			// "Returns NaN if the input contains a NaN".
			return v, typ, nil
		}
		if minMax == nil || compareVals(v, minMax) < 0 == isMin {
			minMax = v
		}
	}
	return minMax, typ, nil
}
