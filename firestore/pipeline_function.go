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

// Function represents Firestore [Pipeline] functions, which can be evaluated within pipeline
// execution.
type Function interface {
	Expr
	isFunction()
}

type baseFunction struct {
	*baseExpr
}

func (b *baseFunction) isFunction() {}

// Ensure that *baseFunction implements the Function interface.
var _ Function = (*baseFunction)(nil)

func newBaseFunction(name string, params []Expr) *baseFunction {
	argsPbVals := make([]*pb.Value, 0, len(params))
	for i, param := range params {

		paramExpr := toExprOrField(param)
		pbVal, err := paramExpr.toProto()
		if err != nil {
			return &baseFunction{baseExpr: &baseExpr{err: fmt.Errorf("error converting arg %d for function %q: %w", i, name, err)}}
		}
		argsPbVals = append(argsPbVals, pbVal)
	}
	pbVal := &pb.Value{ValueType: &pb.Value_FunctionValue{
		FunctionValue: &pb.Function{
			Name: name,
			Args: argsPbVals,
		},
	}}

	return &baseFunction{baseExpr: &baseExpr{pbVal: pbVal}}
}

// Add creates an expression that adds two expressions together, returning it as an Expr.
// - left can be a field path string, [FieldPath] or [Expr].
// - right can be a constant or an [Expr].
//
// Example:
//
//	// Add 5 to the value of the 'age' field.
//	Add("age", 5)
//
//	// Add 'height' to 'weight' field.
//	Add(FieldOf("height"), FieldOf("weight"))
func Add(left, right any) Expr {
	return leftRightToBaseFunction("add", left, right)
}

// Subtract creates an expression that subtracts the right expression from the left expression, returning it as an Expr.
// - left can be a field path string, [FieldPath] or [Expr].
// - right can be a constant or an [Expr].
//
// Example:
//
//	// Subtract 5 from the value of the 'age' field.
//	Subtract("age", 5)
//
//	// Subtract 'discount' from 'price' field.
//	Subtract(FieldOf("price"), FieldOf("discount"))
func Subtract(left, right any) Expr {
	return leftRightToBaseFunction("subtract", left, right)
}

// Multiply creates an expression that multiplies the left and right expressions, returning it as an Expr.
// - left can be a field path string, [FieldPath] or [Expr].
// - right can be a constant or an [Expr].
//
// Example:
//
//	// Multiply 5 to the value of the 'age' field.
//	Multiply("age", 5)
//
//	// Multiply 'discount' and 'price' fields.
//	Multiply(FieldOf("price"), FieldOf("discount"))
func Multiply(left, right any) Expr {
	return leftRightToBaseFunction("multiply", left, right)
}

// Divide creates an expression that divides the left expression by the right expression, returning it as an Expr.
// - left can be a field path string, [FieldPath] or [Expr].
// - right can be a constant or an [Expr].
//
// Example:
//
//	// Divide the value of the 'age' field by 5.
//	Divide("age", 5)
//
//	// Divide 'discount' field by 'price' field.
//	Divide(FieldOf("price"), FieldOf("discount"))
func Divide(left, right any) Expr {
	return leftRightToBaseFunction("divide", left, right)
}

// Abs creates an expression that is the absolute value of the input field or expression.
// - numericExprOrField can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
//
// Example:
//
//	// Absolute value of the 'age' field.
//	Abs("age")
func Abs(numericExprOrField any) Expr {
	return newBaseFunction("abs", []Expr{toExprOrField(numericExprOrField)})
}

// Floor creates an expression that is the largest integer that isn't less than the input field or expression.
// - numericExprOrField can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
//
// Example:
//
//	// Floor value of the 'age' field.
//	Floor("age")
func Floor(numericExprOrField any) Expr {
	return newBaseFunction("floor", []Expr{toExprOrField(numericExprOrField)})
}

// Ceil creates an expression that is the smallest integer that isn't less than the input field or expression.
// - numericExprOrField can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
//
// Example:
//
//	// Ceiling value of the 'age' field.
//	Ceil("age")
func Ceil(numericExprOrField any) Expr {
	return newBaseFunction("ceil", []Expr{toExprOrField(numericExprOrField)})
}

// Exp creates an expression that is the Euler's number e raised to the power of the input field or expression.
// - numericExprOrField can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
//
// Example:
//
//	// e to the power of the value of the 'age' field.
//	Exp("age")
func Exp(numericExprOrField any) Expr {
	return newBaseFunction("exp", []Expr{toExprOrField(numericExprOrField)})
}

// Log creates an expression that is logarithm of the left expression to base as the right expression, returning it as an Expr.
// - left can be a field path string, [FieldPath] or [Expr].
// - right can be a constant or an [Expr].
//
// Example:
//
//	// Logarithm of 'age' field to base 5.
//	Log("age", 5)
//
//	// Log 'height' to base 'weight' field.
//	Log(FieldOf("height"), FieldOf("weight"))
func Log(left, right any) Expr {
	return leftRightToBaseFunction("log", left, right)
}

// Log10 creates an expression that is the base 10 logarithm of the input field or expression.
// - numericExprOrField can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
//
// Example:
//
//	// Base 10 logarithmic value of the 'age' field.
//	Log10("age")
func Log10(numericExprOrField any) Expr {
	return newBaseFunction("log10", []Expr{toExprOrField(numericExprOrField)})
}

// Ln creates an expression that is the natural logarithm (base e) of the input field or expression.
// - numericExprOrField can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
//
// Example:
//
//	// Natural logarithmic value of the 'age' field.
//	Ln("age")
func Ln(numericExprOrField any) Expr {
	return newBaseFunction("ln", []Expr{toExprOrField(numericExprOrField)})
}

// Mod creates an expression that computes the modulo of the left expression by the right expression, returning it as an Expr.
// - left can be a field path string, [FieldPath] or [Expr].
// - right can be a constant or an [Expr].
//
// Example:
//
//	// Modulo of 'age' field by 5.
//	Mod("age", 5)
//
//	// Modulo of 'price' field by 'discount' field.
//	Mod(FieldOf("price"), FieldOf("discount"))
func Mod(left, right any) Expr {
	return leftRightToBaseFunction("mod", left, right)
}

// Pow creates an expression that computes the left expression raised to the power of the right expression, returning it as an Expr.
// - left can be a field path string, [FieldPath] or [Expr].
// - right can be a constant or an [Expr].
//
// Example:
//
//	// 'age' field raised to the power of 5.
//	Pow("age", 5)
//
//	// 'price' field raised to the power of 'discount' field.
//	Pow(FieldOf("price"), FieldOf("discount"))
func Pow(left, right any) Expr {
	return leftRightToBaseFunction("pow", left, right)
}

// Rand creates an expression that return a pseudo-random number of type double in the range of [0, 1),
// inclusive of 0 and exclusive of 1.
func Rand() Expr {
	return newBaseFunction("rand", []Expr{})
}

// Round creates an expression that rounds the input field or expression to nearest integer.
// - numericExprOrField can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
//
// Example:
//
//	// Round the value of the 'age' field.
//	Round("age")
func Round(numericExprOrField any) Expr {
	return newBaseFunction("round", []Expr{toExprOrField(numericExprOrField)})
}

// Sqrt creates an expression that is the square root of the input field or expression.
// - numericExprOrField can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
//
// Example:
//
//	// Square root of the value of the 'age' field.
//	Sqrt("age")
func Sqrt(numericExprOrField any) Expr {
	return newBaseFunction("sqrt", []Expr{toExprOrField(numericExprOrField)})
}
