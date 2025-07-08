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

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
)

// Function represents Firestore [Pipeline] functions, which can be evaluated within pipeline
// execution.
type Function interface {
	Expr
}

type baseFunction struct {
	*baseExpr
}

// Ensure that baseFunction implements the Function interface.
var _ Function = (*baseFunction)(nil)

func newBaseFunction(name string, params []Expr, err error) *baseFunction {
	if err != nil {
		return &baseFunction{baseExpr: &baseExpr{err: err}}
	}
	argsPbVals := make([]*pb.Value, len(params))
	for i, param := range params {
		var err error
		argsPbVals[i], err = param.toProto()
		if err != nil {
			return &baseFunction{baseExpr: &baseExpr{err: fmt.Errorf("error converting arg %d for function %q: %w", i, name, err)}}
		}
	}
	pbVal := &pb.Value{ValueType: &pb.Value_FunctionValue{
		FunctionValue: &pb.Function{
			Name: name,
			Args: argsPbVals,
		},
	}}

	return &baseFunction{baseExpr: &baseExpr{pbVal: pbVal}}
}

// As assigns an alias to Function.
// Aliases are useful for renaming fields in the output of a stage or for giving meaningful
// names to calculated values.
func (f *baseFunction) As(alias string) Selectable {
	return newExprWithAlias(f, alias)
}

type AddFunc struct{ *baseFunction }

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
func Add(left, right any) *AddFunc {
	return &AddFunc{baseFunction: leftRightToBaseFunction("add", left, right)}
}

type SubtractFunc struct{ *baseFunction }

func Subtract(left, right any) *SubtractFunc {
	return &SubtractFunc{baseFunction: leftRightToBaseFunction("subtract", left, right)}
}

type DivideFunc struct{ *baseFunction }

func Divide(left, right any) *DivideFunc {
	return &DivideFunc{baseFunction: leftRightToBaseFunction("subtract", left, right)}
}

type MultiplyFunc struct{ *baseFunction }

func Multiply(left, right any) *MultiplyFunc {
	return &MultiplyFunc{baseFunction: leftRightToBaseFunction("subtract", left, right)}
}

type ModFunc struct{ *baseFunction }

func Mod(left, right any) *ModFunc {
	return &ModFunc{baseFunction: leftRightToBaseFunction("subtract", left, right)}
}

type ArrayConcatFunc struct{ *baseFunction }

func ArrayConcat(fieldOrExpr any, elements ...any) *ArrayConcatFunc {
	exprs, err := toExprList(fieldOrExpr, elements...)
	return &ArrayConcatFunc{baseFunction: newBaseFunction("array_concat", exprs, err)}
}

type ArrayLengthFunc struct{ *baseFunction }

func ArrayLength(fieldOrExpr any) *ArrayLengthFunc {
	expr, err := toExprOrField(fieldOrExpr)
	return &ArrayLengthFunc{baseFunction: newBaseFunction("array_length", []Expr{expr}, err)}
}

type ArrayReverseFunc struct{ *baseFunction }

func ArrayReverse(fieldOrExpr any) *ArrayReverseFunc {
	expr, err := toExprOrField(fieldOrExpr)
	return &ArrayReverseFunc{baseFunction: newBaseFunction("array_reverse", []Expr{expr}, err)}
}

type ByteLengthFunc struct{ *baseFunction }

func ByteLength(fieldOrExpr any) *ByteLengthFunc {
	expr, err := toExprOrField(fieldOrExpr)
	return &ByteLengthFunc{baseFunction: newBaseFunction("byte_length", []Expr{expr}, err)}
}

type CharLengthFunc struct{ *baseFunction }

func CharLength(fieldOrExpr any) *CharLengthFunc {
	expr, err := toExprOrField(fieldOrExpr)
	return &CharLengthFunc{baseFunction: newBaseFunction("char_length", []Expr{expr}, err)}
}

type CosineDistanceFunc struct{ *baseFunction }

func CosineDistance(fieldOrExpr, other any) *CosineDistanceFunc {
	var exprs []Expr
	var err error

	// other can be []float32, []float64, Expr, Vector32 or Vector64
	switch other.(type) {
	case []float32, []float64, Expr, Vector32, Vector64:
		exprs, err = toExprList(fieldOrExpr, other)
	default:
		return &CosineDistanceFunc{
			baseFunction: newBaseFunction("cosine_distance", nil,
				fmt.Errorf("firestore: invalid type for parameter 'other': expected []float32, []float64, Expr, Vector32 or Vector64, but got %T", other)),
		}
	}

	return &CosineDistanceFunc{baseFunction: newBaseFunction("cosine_distance", exprs, err)}
}

type DotDistanceFunc struct{ *baseFunction }

func DotDistance(fieldOrExpr, other any) *DotDistanceFunc {
	var exprs []Expr
	var err error

	// other can be []float32, []float64, Expr, Vector32 or Vector64
	switch other.(type) {
	case []float32, []float64, Expr, Vector32, Vector64:
		exprs, err = toExprList(fieldOrExpr, other)
	default:
		return &DotDistanceFunc{
			baseFunction: newBaseFunction("cosine_distance", nil,
				fmt.Errorf("firestore: invalid type for parameter 'other': expected []float32, []float64, Expr, Vector32 or Vector64, but got %T", other)),
		}
	}

	return &DotDistanceFunc{baseFunction: newBaseFunction("dot_product", exprs, err)}
}
