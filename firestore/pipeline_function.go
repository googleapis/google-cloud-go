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
	"fmt"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
)

const (
	functionNameAdd      = "add"
	functionNameSubtract = "subtract"
	functionNameMultiply = "multiply"
	functionNameDivide   = "divide"
)

type baseFunction struct {
	baseExpr
	name   string
	params []Expr
	err    error
}

func (f *baseFunction) As(alias string) Selectable { return newExprWithAlias(f, alias) }
func (f *baseFunction) toArgumentProto() (*pb.Value, error) {
	if f.err != nil {
		return nil, f.err
	}
	argProtos := make([]*pb.Value, len(f.params))
	for i, arg := range f.params {
		var err error
		argProtos[i], err = arg.toArgumentProto()
		if err != nil {
			return nil, fmt.Errorf("error converting arg %d for function %q: %w", i, f.name, err)
		}
	}
	return &pb.Value{ValueType: &pb.Value_FunctionValue{
		FunctionValue: &pb.Function{
			Name: f.name,
			Args: argProtos,
		},
	}}, nil
}

// type baseAccumulator struct{ *baseFunction }

// func (a *baseAccumulator) isAccumulator() {}

// AddExpr represents an addition operation.
type AddExpr struct {
	baseFunction
}

// Add creates an expression that adds two expressions together.
//
// left can be a fieldname or an [Expr].
// right can be a constant or an [Expr].
//
// E.g. Add 5 to the value of the 'age' field.
//
//	Add("age", 5)
//
// E.g. Add 'height' to 'weight' field.
//
//	Add(FieldOf("height"), FieldOf("weight"))
func Add(left, right any) *AddExpr {
	expr := AddExpr{
		baseFunction: baseFunction{
			name: functionNameAdd,
		},
	}

	leftExpr, err1 := toExprOrField(left)
	if err1 != nil {
		expr.baseFunction.err = err1
		return &expr
	}
	rightExpr, err2 := toExprOrConstant(right)
	if err2 != nil {
		return &expr
	}

	expr.baseFunction.self = &expr
	expr.baseFunction.params = []Expr{leftExpr, rightExpr}
	return &expr
}

// SubtractExpr represents a subtraction operation.
type SubtractExpr struct {
	baseFunction
}

// Subtract creates a subtraction expression.
func Subtract(left, right any) *SubtractExpr {
	expr := SubtractExpr{
		baseFunction: baseFunction{
			name: functionNameSubtract,
		},
	}

	leftExpr, err1 := toExprOrField(left)
	if err1 != nil {
		expr.baseFunction.err = err1
		return &expr
	}
	rightExpr, err2 := toExprOrConstant(right)
	if err2 != nil {
		return &expr
	}

	expr.baseFunction.self = &expr
	expr.baseFunction.params = []Expr{leftExpr, rightExpr}
	return &expr
}

// MultiplyExpr represents a multiplication operation.
type MultiplyExpr struct {
	baseFunction
}

// Multiply creates a multiplication expression.
func Multiply(left, right any) *MultiplyExpr {
	expr := MultiplyExpr{
		baseFunction: baseFunction{
			name: functionNameMultiply,
		},
	}

	leftExpr, err1 := toExprOrField(left)
	if err1 != nil {
		expr.baseFunction.err = err1
		return &expr
	}
	rightExpr, err2 := toExprOrConstant(right)
	if err2 != nil {
		return &expr
	}

	expr.baseFunction.self = &expr
	expr.baseFunction.params = []Expr{leftExpr, rightExpr}
	return &expr
}

// DivideExpr represents a division operation.
type DivideExpr struct {
	baseFunction
}

// Divide creates a division expression.
func Divide(left, right any) *DivideExpr {
	expr := DivideExpr{
		baseFunction: baseFunction{
			name: functionNameDivide,
		},
	}

	leftExpr, err1 := toExprOrField(left)
	if err1 != nil {
		expr.baseFunction.err = err1
		return &expr
	}
	rightExpr, err2 := toExprOrConstant(right)
	if err2 != nil {
		return &expr
	}

	expr.baseFunction.self = &expr
	expr.baseFunction.params = []Expr{leftExpr, rightExpr}
	return &expr
}

// toExprOrConstant converts a plain Go value or an existing Expr into an Expr.
// Plain values are wrapped in a Constant.
func toExprOrConstant(val any) (Expr, error) {
	if expr, ok := val.(Expr); ok {
		return expr, nil
	}
	return constantOf(val)
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

// type Accumulator interface {
// 	Expr
// 	isAccumulator()
// }

// type baseAccumulator struct{ baseFunction }

// func (a *baseAccumulator) As(alias string) ExprWithAlias[Accumulator] {
// 	return newExprWithAlias(a, alias)
// }

// func (a *baseAccumulator) isAccumulator() {}

// type SumAccum struct{ baseAccumulator }
