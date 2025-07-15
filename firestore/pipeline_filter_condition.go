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

// BooleanExpr is an interface that represents a boolean expression in a pipeline.
type BooleanExpr interface {
	Expr // Embed Expr interface
	isBooleanExpr()
}

// baseBooleanExpr provides common methods for all BooleanExpr implementations.
type baseBooleanExpr struct {
	*baseFunction // Embed Function to get Expr methods and toProto
}

func (b *baseBooleanExpr) isBooleanExpr() {}

// Ensure that baseBooleanExpr implements the BooleanExpr interface.
var _ BooleanExpr = (*baseBooleanExpr)(nil)

// Eq creates an expression that checks if field's value or an expression is equal to an expression or a constant value,
// returning it as a BooleanExpr.
//   - left: The field path string, [FieldPath] or [Expr] to compare.
//   - right: The constant value or [Expr] to compare to.
//
// Example:
//
//		// Check if the 'age' field is equal to 21
//		Eq(FieldOf("age"), 21)
//
//		// Check if the 'age' field is equal to an expression
//	 	Eq(FieldOf("age"), FieldOf("minAge").Add(10))
//
//		// Check if the 'age' field is equal to the 'limit' field
//		Eq("age", FieldOf("limit"))
//
//		// Check if the 'city' field is equal to string constant "London"
//		Eq("city", "London")
func Eq(left, right any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: leftRightToBaseFunction("eq", left, right)}
}

// Neq creates an expression that checks if field's value or an expression is not equal to an expression or a constant value,
// returning it as a BooleanExpr.
//   - left: The field path string, [FieldPath] or [Expr] to compare.
//   - right: The constant value or [Expr] to compare to.
//
// Example:
//
//		// Check if the 'age' field is not equal to 21
//		Neq(FieldOf("age"), 21)
//
//		// Check if the 'age' field is not equal to an expression
//	 	Neq(FieldOf("age"), FieldOf("minAge").Add(10))
//
//		// Check if the 'age' field is not equal to the 'limit' field
//		Neq("age", FieldOf("limit"))
//
//		// Check if the 'city' field is not equal to string constant "London"
//		Neq("city", "London")
func Neq(left, right any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: leftRightToBaseFunction("neq", left, right)}
}
