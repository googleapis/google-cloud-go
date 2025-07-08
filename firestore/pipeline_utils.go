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

// leftRightToBaseFunction creates a baseFunction from a function name and two operands.
// The left operand must be a field path string, FieldPath, or Expr.
// The right operand can be a constant value or an Expr.
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

// selectablesToPbVal converts a slice of Selectable or AccumulatorTarget to a single protobuf Value.
// The returned Value will be a MapValue where keys are aliases and values are the expression protos.
func selectablesToPbVal[T []Selectable | []*AccumulatorTarget](selectables T) (*pb.Value, error) {
	if selectables == nil {
		return &pb.Value{ValueType: &pb.Value_MapValue{}}, nil
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

// selectableToPbVal converts a single Selectable to its alias and protobuf Value.
func selectableToPbVal(s Selectable) (string, *pb.Value, error) {
	alias, expr := s.getSelectionDetails()
	protoVal, err := expr.toProto()
	if err != nil {
		return alias, nil, fmt.Errorf("error processing expression for alias %q: %w", alias, err)
	}
	return alias, protoVal, nil
}

// fieldsOrSelectablesToSelectables converts a variadic list of field paths (string), FieldPath, or Selectable
// to a slice of Selectable.
func fieldsOrSelectablesToSelectables(fieldsOrSelectables ...any) ([]Selectable, error) {
	selectables := make([]Selectable, 0, len(fieldsOrSelectables))
	for _, f := range fieldsOrSelectables {
		var s Selectable
		switch v := f.(type) {
		case string:
			if v == "" {
				return nil, errors.New("firestore: path cannot be empty")
			}
			s = FieldOf(v)
		case FieldPath:
			s = FieldOfPath(v)
		case Selectable:
			s = v
		default:
			return nil, fmt.Errorf("firestore: %v is not a string, FieldPath, or Selectable", f)
		}
		selectables = append(selectables, s)
	}
	return selectables, nil
}

// exprToProtoValue converts an Expr to a protobuf Value.
// If the expression is nil, it returns a Null Value.
func exprToProtoValue(expr Expr) (*pb.Value, error) {
	if expr == nil {
		return ConstantOfNull().pbVal, nil
	}
	return expr.toProto()
}
