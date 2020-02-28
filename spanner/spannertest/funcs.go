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

	"cloud.google.com/go/spanner/spansql"
)

// This file contains implementations of query functions.

type aggregateFunc struct {
	// Whether the function can take a * arg (only COUNT).
	AcceptStar bool

	// Every aggregate func takes one expression.
	Eval func(values []interface{}) (interface{}, spansql.Type, error)

	// TODO: Handle qualifiers such as DISTINCT.
}

// TODO: more aggregate funcs.
var aggregateFuncs = map[string]aggregateFunc{
	"COUNT": {
		AcceptStar: true,
		Eval: func(values []interface{}) (interface{}, spansql.Type, error) {
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
	"SUM": {
		Eval: func(values []interface{}) (interface{}, spansql.Type, error) {
			// Ignoring NULL values, there may only be one type, either INT64 or FLOAT64.
			var seenInt, seenFloat bool
			var sumInt int64
			var sumFloat float64
			for _, v := range values {
				switch v := v.(type) {
				default:
					return nil, spansql.Type{}, fmt.Errorf("SUM only supports arguments of INT64 or FLOAT64 type, not %T", v)
				case nil:
					continue
				case int64:
					seenInt = true
					sumInt += v
				case float64:
					seenFloat = true
					sumFloat += v
				}
			}
			if !seenInt && !seenFloat {
				// "Returns NULL if the input contains only NULLs".
				return nil, int64Type, nil
			} else if seenInt && seenFloat {
				// This shouldn't happen.
				return nil, spansql.Type{}, fmt.Errorf("internal error: SUM saw mix of INT64 and FLOAT64")
			} else if seenInt {
				return sumInt, int64Type, nil
			}
			return sumFloat, float64Type, nil
		},
	},
}
