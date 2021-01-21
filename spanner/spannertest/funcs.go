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

	"cloud.google.com/go/spanner/spansql"
)

// This file contains implementations of query functions.

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
