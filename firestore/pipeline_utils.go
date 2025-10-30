// Copyright 2025 Google LLC
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

// asFieldExpr converts a plain Go string or FieldPath into a field expression.
// If the value is already an Expr, it's returned directly.
func asFieldExpr(val any) Expr {
	switch v := val.(type) {
	case Expr:
		return v
	case FieldPath:
		return FieldOfPath(v)
	case string:
		return FieldOf(v)
	default:
		return &baseExpr{err: fmt.Errorf("firestore: value must be a string, FieldPath, or Expr, but got %T", val)}
	}
}

func asInt64Expr(val any) Expr {
	switch v := val.(type) {
	case Expr:
		return v
	case int, int32, int64:
		return ConstantOf(v)
	default:
		return &baseExpr{err: fmt.Errorf("firestore: value must be a int, int32, int64 or Expr, but got %T", val)}
	}
}

func asStringExpr(val any) Expr {
	switch v := val.(type) {
	case Expr:
		return v
	case string:
		return ConstantOf(v)
	default:
		return &baseExpr{err: fmt.Errorf("firestore: value must be a string or Expr, but got %T", val)}
	}
}

// leftRightToBaseFunction is a helper for creating binary functions like Add or Eq.
// It ensures the left operand is a field-like expression and the right is a constant-like expression.
func leftRightToBaseFunction(name string, left, right any) *baseFunction {
	return newBaseFunction(name, []Expr{asFieldExpr(left), toExprOrConstant(right)})
}

// projectionsToMapValue converts a slice of Selectable items into a single
// protobuf MapValue.
func projectionsToMapValue(selectables []Selectable) (*pb.Value, error) {
	if selectables == nil {
		return &pb.Value{ValueType: &pb.Value_MapValue{}}, nil
	}
	fieldsProto := make(map[string]*pb.Value, len(selectables))
	for _, s := range selectables {
		alias, expr := s.getSelectionDetails()
		if _, exists := fieldsProto[alias]; exists {
			return nil, fmt.Errorf("firestore: duplicate alias or field name %q in selectables", alias)
		}

		protoVal, err := expr.toProto()
		if err != nil {
			return nil, fmt.Errorf("firestore: error processing expression for alias %q: %w", alias, err)
		}
		fieldsProto[alias] = protoVal
	}
	return &pb.Value{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: fieldsProto}}}, nil
}

// aliasedAggregatesToMapValue converts a slice of AliasedAggregate items into a single
// protobuf MapValue.
func aliasedAggregatesToMapValue(aggregates []*AliasedAggregate) (*pb.Value, error) {
	if aggregates == nil {
		return &pb.Value{ValueType: &pb.Value_MapValue{}}, nil
	}
	fieldsProto := make(map[string]*pb.Value, len(aggregates))
	for _, agg := range aggregates {
		if _, exists := fieldsProto[agg.alias]; exists {
			return nil, fmt.Errorf("firestore: duplicate alias %q in aggregations", agg.alias)
		}

		base := agg.getBaseAggregateFunction()
		if base.err != nil {
			return nil, fmt.Errorf("error in aggregate expression for alias %q: %w", agg.alias, base.err)
		}
		protoVal, err := base.toProto()
		if err != nil {
			return nil, fmt.Errorf("firestore: error converting aggregate for alias %q to proto: %w", agg.alias, err)
		}
		fieldsProto[agg.alias] = protoVal
	}
	return &pb.Value{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: fieldsProto}}}, nil
}

// fieldsOrSelectablesToSelectables converts a user-provided list of mixed types
// (string, FieldPath, Selectable) into a uniform []Selectable slice.
func fieldsOrSelectablesToSelectables(fieldsOrSelectables ...any) ([]Selectable, error) {
	selectables := make([]Selectable, 0, len(fieldsOrSelectables))
	for _, f := range fieldsOrSelectables {
		var s Selectable
		switch v := f.(type) {
		case string:
			if v == "" {
				return nil, errors.New("firestore: path cannot be empty")
			}
			s = FieldOf(v).(*field)
		case FieldPath:
			s = FieldOfPath(v).(*field)
		case Selectable:
			s = v
		default:
			return nil, fmt.Errorf("firestore: value must be a string, FieldPath, or Selectable, but got %T", v)
		}
		selectables = append(selectables, s)
	}
	return selectables, nil
}

// exprToProtoValue converts an Expr to a protobuf Value.
// If the expression is nil, it returns a Null Value.
func exprToProtoValue(expr Expr) (*pb.Value, error) {
	if expr == nil {
		return ConstantOfNull().getBaseExpr().pbVal, nil
	}
	return expr.toProto()
}
