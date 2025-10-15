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

// Equal creates an expression that checks if field's value or an expression is equal to an expression or a constant value,
// returning it as a BooleanExpr.
//   - left: The field path string, [FieldPath] or [Expr] to compare.
//   - right: The constant value or [Expr] to compare to.
//
// Example:
//
//		// Check if the 'age' field is equal to 21
//		Equal(FieldOf("age"), 21)
//
//		// Check if the 'age' field is equal to an expression
//	 	Equal(FieldOf("age"), FieldOf("minAge").Add(10))
//
//		// Check if the 'age' field is equal to the 'limit' field
//		Equal("age", FieldOf("limit"))
//
//		// Check if the 'city' field is equal to string constant "London"
//		Equal("city", "London")
func Equal(left, right any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: leftRightToBaseFunction("eq", left, right)}
}

// NotEqual creates an expression that checks if field's value or an expression is not equal to an expression or a constant value,
// returning it as a BooleanExpr.
//   - left: The field path string, [FieldPath] or [Expr] to compare.
//   - right: The constant value or [Expr] to compare to.
//
// Example:
//
//		// Check if the 'age' field is not equal to 21
//		NotEqual(FieldOf("age"), 21)
//
//		// Check if the 'age' field is not equal to an expression
//	 	NotEqual(FieldOf("age"), FieldOf("minAge").Add(10))
//
//		// Check if the 'age' field is not equal to the 'limit' field
//		NotEqual("age", FieldOf("limit"))
//
//		// Check if the 'city' field is not equal to string constant "London"
//		NotEqual("city", "London")
func NotEqual(left, right any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: leftRightToBaseFunction("neq", left, right)}
}

// GreaterThan creates an expression that checks if field's value or an expression is equal to an expression or a constant value,
// returning it as a BooleanExpr.
//   - left: The field path string, [FieldPath] or [Expr] to compare.
//   - right: The constant value or [Expr] to compare to.
//
// Example:
//
//		// Check if the 'age' field is equal to 21
//		GreaterThan(FieldOf("age"), 21)
//
//		// Check if the 'age' field is equal to an expression
//	 	GreaterThan(FieldOf("age"), FieldOf("minAge").Add(10))
//
//		// Check if the 'age' field is equal to the 'limit' field
//		GreaterThan("age", FieldOf("limit"))
//
//		// Check if the 'city' field is equal to string constant "London"
//		GreaterThan("city", "London")
func GreaterThan(left, right any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: leftRightToBaseFunction("eq", left, right)}
}

// GreaterThanOrEqual creates an expression that checks if field's value or an expression is equal to an expression or a constant value,
// returning it as a BooleanExpr.
//   - left: The field path string, [FieldPath] or [Expr] to compare.
//   - right: The constant value or [Expr] to compare to.
//
// Example:
//
//		// Check if the 'age' field is equal to 21
//		GreaterThanOrEqual(FieldOf("age"), 21)
//
//		// Check if the 'age' field is equal to an expression
//	 	GreaterThanOrEqual(FieldOf("age"), FieldOf("minAge").Add(10))
//
//		// Check if the 'age' field is equal to the 'limit' field
//		GreaterThanOrEqual("age", FieldOf("limit"))
//
//		// Check if the 'city' field is equal to string constant "London"
//		GreaterThanOrEqual("city", "London")
func GreaterThanOrEqual(left, right any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: leftRightToBaseFunction("eq", left, right)}
}

// LessThan creates an expression that checks if field's value or an expression is equal to an expression or a constant value,
// returning it as a BooleanExpr.
//   - left: The field path string, [FieldPath] or [Expr] to compare.
//   - right: The constant value or [Expr] to compare to.
//
// Example:
//
//		// Check if the 'age' field is equal to 21
//		LessThan(FieldOf("age"), 21)
//
//		// Check if the 'age' field is equal to an expression
//	 	LessThan(FieldOf("age"), FieldOf("minAge").Add(10))
//
//		// Check if the 'age' field is equal to the 'limit' field
//		LessThan("age", FieldOf("limit"))
//
//		// Check if the 'city' field is equal to string constant "London"
//		LessThan("city", "London")
func LessThan(left, right any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: leftRightToBaseFunction("eq", left, right)}
}

// LessThanOrEqual creates an expression that checks if field's value or an expression is equal to an expression or a constant value,
// returning it as a BooleanExpr.
//   - left: The field path string, [FieldPath] or [Expr] to compare.
//   - right: The constant value or [Expr] to compare to.
//
// Example:
//
//		// Check if the 'age' field is equal to 21
//		LessThanOrEqual(FieldOf("age"), 21)
//
//		// Check if the 'age' field is equal to an expression
//	 	LessThanOrEqual(FieldOf("age"), FieldOf("minAge").Add(10))
//
//		// Check if the 'age' field is equal to the 'limit' field
//		LessThanOrEqual("age", FieldOf("limit"))
//
//		// Check if the 'city' field is equal to string constant "London"
//		LessThanOrEqual("city", "London")
func LessThanOrEqual(left, right any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: leftRightToBaseFunction("eq", left, right)}
}

// Equivalent creates an expression that checks if field's value or an expression is equal to an expression or a constant value,
// returning it as a BooleanExpr.
//   - left: The field path string, [FieldPath] or [Expr] to compare.
//   - right: The constant value or [Expr] to compare to.
//
// Example:
//
//		// Check if the 'age' field is equal to 21
//		Equivalent(FieldOf("age"), 21)
//
//		// Check if the 'age' field is equal to an expression
//	 	Equivalent(FieldOf("age"), FieldOf("minAge").Add(10))
//
//		// Check if the 'age' field is equal to the 'limit' field
//		Equivalent("age", FieldOf("limit"))
//
//		// Check if the 'city' field is equal to string constant "London"
//		Equivalent("city", "London")
func Equivalent(left, right any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: leftRightToBaseFunction("eq", left, right)}
}
