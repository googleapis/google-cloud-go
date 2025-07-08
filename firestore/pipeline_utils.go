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
	"errors"
	"fmt"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
)

// toExprOrConstant converts a plain Go value or an existing Expr into an Expr.
// Plain values are wrapped in a Constant.
func toExprOrConstant(val any) Expr {
	if expr, ok := val.(Expr); ok {
		return expr
	}
	return ConstantOf(val)
}

// toExprOrField converts a plain Go value or an existing Expr into an Expr.
// Plain strings are wrapped in a Field.
func toExprOrField(val any) (Expr, error) {
	switch v := val.(type) {
	case Expr:
		return v, nil
	case FieldPath:
		return FieldOfPath(v), nil
	case string:
		return FieldOf(v), nil
	default:
		return nil, fmt.Errorf("firestore: %v is not a string or an Expr", val)
	}
}

func leftRightToBaseFunction(name string, left, right any) *baseFunction {
	leftExpr, err := toExprOrField(left)
	if err != nil {
		return &baseFunction{baseExpr: &baseExpr{err: err}}
	}
	return newBaseFunction(name, []Expr{leftExpr, toExprOrConstant(right)}, nil)
}

func selectablesToPbVals[T []Selectable | []*AccumulatorTarget](selectables T) ([]*pb.Value, error) {
	pbVal, err := selectablesToPbVal(selectables)
	if err != nil {
		return nil, err
	}

	return []*pb.Value{pbVal}, nil
}

func selectablesToPbVal[T []Selectable | []*AccumulatorTarget](selectables T) (*pb.Value, error) {
	if selectables == nil {
		// TODO: THink more on this
		return &pb.Value{ValueType: &pb.Value_MapValue{}}, nil // An empty slice is valid, results in an empty map. TODO: Validate
	}
	fields := make(map[string]bool, len(selectables))
	fieldsProto := make(map[string]*pb.Value, len(selectables))

	process := func(alias string, protoVal *pb.Value, err error) error {
		if err != nil {
			return err
		}
		if _, exists := fields[alias]; exists {
			return fmt.Errorf("firestore: duplicate alias or field name %q in selectables", alias)
		}
		fields[alias] = true
		fieldsProto[alias] = protoVal
		return nil
	}

	switch v := any(selectables).(type) {
	case []*AccumulatorTarget:
		for _, s := range v {
			alias, protoVal, err := selectableToPbVal(s)
			if err := process(alias, protoVal, err); err != nil {
				return nil, err
			}
		}
	case []Selectable:
		for _, s := range v {
			alias, protoVal, err := selectableToPbVal(s)
			if err := process(alias, protoVal, err); err != nil {
				return nil, err
			}
		}
	}

	return &pb.Value{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: fieldsProto}}}, nil
}

func selectableToPbVal(s Selectable) (string, *pb.Value, error) {
	alias, expr := s.getSelectionDetails()
	protoVal, err := expr.toProto()
	if err != nil {
		return alias, nil, fmt.Errorf("error processing expression for alias %q: %w", alias, err)
	}
	return alias, protoVal, nil
}

func stringPathsToSelectables(paths ...string) ([]Selectable, error) {
	selectables := make([]Selectable, len(paths))
	for i, path := range paths {
		if path == "" {
			return nil, errors.New("firestore: path in paths cannot be empty")
		}
		selectables[i] = FieldOf(path)
	}
	return selectables, nil
}

func fieldPathsToSelectables(fieldpaths ...FieldPath) []Selectable {
	selectables := make([]Selectable, len(fieldpaths))
	for i, name := range fieldpaths {
		selectables[i] = FieldOfPath(name)
	}
	return selectables
}

func toExprList(fieldOrExpr any, vals ...any) ([]Expr, error) {
	expr, err := toExprOrField(fieldOrExpr)
	if err != nil {
		return nil, err
	}

	exprs := make([]Expr, len(vals)+1)
	exprs[0] = expr
	for i, val := range vals {
		exprs[i+1] = toExprOrConstant(val)
	}
	return exprs, nil
}

func exprToProtoValue(expr Expr) (*pb.Value, error) {
	if expr == nil {
		return ConstantOfNull().pbVal, nil
	}
	return expr.toProto()
}
