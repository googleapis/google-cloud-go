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

package firestore

import (
	"fmt"
)

// toExprOrConstant converts a plain Go value or an existing Expr into an Expr.
// Plain values are wrapped in a Constant.
func toExprOrConstant(val any) (Expr, error) {
	if expr, ok := val.(Expr); ok {
		return expr, nil
	}
	return ConstantOf(val), nil
}

// toExprOrField converts a plain Go value or an existing Expr into an Expr.
// Plain strings are wrapped in a Field.
func toExprOrField(val any) (Expr, error) {
	if expr, ok := val.(Expr); ok {
		return expr, nil
	}

	if str, ok := val.(string); ok {
		return FieldOf(str), nil
	}
	return nil, fmt.Errorf("firestore: %v is not a string or an Expr", val)
}

func leftRightToBaseFunction(name string, left, right Expr) *baseFunction {
	leftExpr, err := toExprOrField(left)
	if err != nil {
		return &baseFunction{baseExpr: &baseExpr{err: err}}
	}
	rightExpr, err := toExprOrConstant(right)
	if err != nil {
		return &baseFunction{baseExpr: &baseExpr{err: err}}
	}

	return newBaseFunction(name, leftExpr, rightExpr)
}

func selectablesToMap(selectables ...Selectable) (map[string]Expr, error) {
	if len(selectables) == 0 {
		return map[string]Expr{}, nil
	}

	fields := make(map[string]Expr, len(selectables))
	for _, s := range selectables {
		alias, expr, err := s.getSelectionDetails()
		if err != nil {
			return nil, err
		}
		if _, exists := fields[alias]; exists {
			return nil, fmt.Errorf("firestore: duplicate alias or field name %q in selectables", alias)
		}
		fields[alias] = expr
	}
	return fields, nil
}
