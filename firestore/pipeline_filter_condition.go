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

// ArrayContains creates an expression that checks if an array contains a specified element.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to an array.
// - value is the element to check for.
//
// Example:
//
//	// Check if the 'tags' array contains "Go".
//	ArrayContains("tags", "Go")
func ArrayContains(exprOrFieldPath any, value any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: newBaseFunction("array_contains", []Expr{asFieldExpr(exprOrFieldPath), toExprOrConstant(value)})}
}

// ArrayContainsAll creates an expression that checks if an array contains all of the provided values.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to an array.
// - values can be an array of values or an expression that evaluates to an array.
//
// Example:
//
//	// Check if the 'tags' array contains both "Go" and "Firestore".
//	ArrayContainsAll("tags", []string{"Go", "Firestore"})
func ArrayContainsAll(exprOrFieldPath any, values any) BooleanExpr {
	return newFieldAndArrayBooleanExpr("array_contains_all", exprOrFieldPath, values)
}

// ArrayContainsAny creates an expression that checks if an array contains any of the provided values.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to an array.
// - values can be an array of values or an expression that evaluates to an array.
//
// Example:
//
//	// Check if the 'tags' array contains either "Go" or "Firestore".
//	ArrayContainsAny("tags", []string{"Go", "Firestore"})
func ArrayContainsAny(exprOrFieldPath any, values any) BooleanExpr {
	return newFieldAndArrayBooleanExpr("array_contains_any", exprOrFieldPath, values)
}

// EqualAny creates an expression that checks if a field or expression is equal to any of the provided values.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr].
// - values can be an array of values or an expression that evaluates to an array.
//
// Example:
//
//	// Check if the 'status' field is either "active" or "pending".
//	EqualAny("status", []string{"active", "pending"})
func EqualAny(exprOrFieldPath any, values any) BooleanExpr {
	return newFieldAndArrayBooleanExpr("equal_any", exprOrFieldPath, values)
}

// NotEqualAny creates an expression that checks if a field or expression is not equal to any of the provided values.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr].
// - values can be an array of values or an expression that evaluates to an array.
//
// Example:
//
//	// Check if the 'status' field is not "archived" or "deleted".
//	NotEqualAny("status", []string{"archived", "deleted"})
func NotEqualAny(exprOrFieldPath any, values any) BooleanExpr {
	return newFieldAndArrayBooleanExpr("not_equal_any", exprOrFieldPath, values)
}

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
	return &baseBooleanExpr{baseFunction: leftRightToBaseFunction("equal", left, right)}
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
	return &baseBooleanExpr{baseFunction: leftRightToBaseFunction("not_equal", left, right)}
}

// GreaterThan creates an expression that checks if field's value or an expression is greater than an expression or a constant value,
// returning it as a BooleanExpr.
//   - left: The field path string, [FieldPath] or [Expr] to compare.
//   - right: The constant value or [Expr] to compare to.
//
// Example:
//
//		// Check if the 'age' field is greater than 21
//		GreaterThan(FieldOf("age"), 21)
//
//		// Check if the 'age' field is greater than an expression
//	 	GreaterThan(FieldOf("age"), FieldOf("minAge").Add(10))
//
//		// Check if the 'age' field is greater than the 'limit' field
//		GreaterThan("age", FieldOf("limit"))
func GreaterThan(left, right any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: leftRightToBaseFunction("greater_than", left, right)}
}

// GreaterThanOrEqual creates an expression that checks if field's value or an expression is greater than or equal to an expression or a constant value,
// returning it as a BooleanExpr.
//   - left: The field path string, [FieldPath] or [Expr] to compare.
//   - right: The constant value or [Expr] to compare to.
//
// Example:
//
//		// Check if the 'age' field is greater than or equal to 21
//		GreaterThanOrEqual(FieldOf("age"), 21)
//
//		// Check if the 'age' field is greater than or equal to an expression
//	 	GreaterThanOrEqual(FieldOf("age"), FieldOf("minAge").Add(10))
//
//		// Check if the 'age' field is greater than or equal to the 'limit' field
//		GreaterThanOrEqual("age", FieldOf("limit"))
func GreaterThanOrEqual(left, right any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: leftRightToBaseFunction("greater_than_or_equal", left, right)}
}

// LessThan creates an expression that checks if field's value or an expression is less than an expression or a constant value,
// returning it as a BooleanExpr.
//   - left: The field path string, [FieldPath] or [Expr] to compare.
//   - right: The constant value or [Expr] to compare to.
//
// Example:
//
//		// Check if the 'age' field is less than 21
//		LessThan(FieldOf("age"), 21)
//
//		// Check if the 'age' field is less than an expression
//	 	LessThan(FieldOf("age"), FieldOf("minAge").Add(10))
//
//		// Check if the 'age' field is less than the 'limit' field
//		LessThan("age", FieldOf("limit"))
func LessThan(left, right any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: leftRightToBaseFunction("less_than", left, right)}
}

// LessThanOrEqual creates an expression that checks if field's value or an expression is less than or equal to an expression or a constant value,
// returning it as a BooleanExpr.
//   - left: The field path string, [FieldPath] or [Expr] to compare.
//   - right: The constant value or [Expr] to compare to.
//
// Example:
//
//		// Check if the 'age' field is less than or equal to 21
//		LessThanOrEqual(FieldOf("age"), 21)
//
//		// Check if the 'age' field is less than or equal to an expression
//	 	LessThanOrEqual(FieldOf("age"), FieldOf("minAge").Add(10))
//
//		// Check if the 'age' field is less than or equal to the 'limit' field
//		LessThanOrEqual("age", FieldOf("limit"))
func LessThanOrEqual(left, right any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: leftRightToBaseFunction("less_than_or_equal", left, right)}
}

// Equivalent creates an expression that checks if field's value or an expression is equal to an expression or a constant value,
// returning it as a BooleanExpr. This is an alias for Equal.
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
	return &baseBooleanExpr{baseFunction: leftRightToBaseFunction("equivalent", left, right)}
}

// EndsWith creates an expression that checks if a string field or expression ends with a given suffix.
// - exprOrFieldPath can be a field path string, [FieldPath] or [Expr].
// - suffix string or [Expr] to check for.
//
// Example:
//
//	// Check if the 'filename' field ends with ".go".
//	EndsWith("filename", ".go")
func EndsWith(exprOrFieldPath any, suffix any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: newBaseFunction("ends_with", []Expr{asFieldExpr(exprOrFieldPath), asStringExpr(suffix)})}
}

// Like creates an expression that performs a case-sensitive wildcard string comparison.
// - exprOrFieldPath can be a field path string, [FieldPath] or [Expr].
// - pattern string or [Expr] to search for. You can use "%" as a wildcard character.
//
// Example:
//
//	// Check if the 'name' field starts with "G".
//	Like("name", "G%")
func Like(exprOrFieldPath any, pattern any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: newBaseFunction("like", []Expr{asFieldExpr(exprOrFieldPath), asStringExpr(pattern)})}
}

// RegexContains creates an expression that checks if a string contains a match for a regular expression.
// - exprOrFieldPath can be a field path string, [FieldPath] or [Expr].
// - pattern is the regular expression to search for.
//
// Example:
//
//	// Check if the 'email' field contains a gmail address.
//	RegexContains("email", "@gmail\\.com$")
func RegexContains(exprOrFieldPath any, pattern any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: newBaseFunction("regex_contains", []Expr{asFieldExpr(exprOrFieldPath), asStringExpr(pattern)})}
}

// RegexMatch creates an expression that checks if a string matches a regular expression.
// - exprOrFieldPath can be a field path string, [FieldPath] or [Expr].
// - pattern is the regular expression to match against.
//
// Example:
//
//	// Check if the 'zip_code' field is a 5-digit number.
//	RegexMatch("zip_code", "^[0-9]{5}$")
func RegexMatch(exprOrFieldPath any, pattern any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: newBaseFunction("regex_match", []Expr{asFieldExpr(exprOrFieldPath), asStringExpr(pattern)})}
}

// StartsWith creates an expression that checks if a string field or expression starts with a given prefix.
// - exprOrFieldPath can be a field path string, [FieldPath] or [Expr].
// - prefix string or [Expr] to check for.
//
// Example:
//
//	// Check if the 'name' field starts with "Mr.".
//	StartsWith("name", "Mr.")
func StartsWith(exprOrFieldPath any, prefix any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: newBaseFunction("starts_with", []Expr{asFieldExpr(exprOrFieldPath), asStringExpr(prefix)})}
}

// StringContains creates an expression that checks if a string contains a specified substring.
// - exprOrFieldPath can be a field path string, [FieldPath] or [Expr].
// - substring is the string to search for.
//
// Example:
//
//	// Check if the 'description' field contains the word "Firestore".
//	StringContains("description", "Firestore")
func StringContains(exprOrFieldPath any, substring any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: newBaseFunction("string_contains", []Expr{asFieldExpr(exprOrFieldPath), asStringExpr(substring)})}
}

// IsNaN creates a boolean expression that checks if a field or expression evaluates to NaN.
//
// exprOrFieldPath is the field path string, [FieldPath] or [Expr] that will be evaluated.
//
// Example:
//
//	// Check if the 'score' field is NaN.
//	IsNaN("score")
func IsNaN(exprOrFieldPath any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: newBaseFunction("is_nan", []Expr{asFieldExpr(exprOrFieldPath)})}
}

// IsNotNaN creates a boolean expression that checks if a field or expression does not evaluate to NaN.
//
// exprOrFieldPath is the field path string, [FieldPath] or [Expr] that will be evaluated.
//
// Example:
//
//	// Check if the 'score' field is not NaN.
//	IsNotNaN("score")
func IsNotNaN(exprOrFieldPath any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: newBaseFunction("is_not_nan", []Expr{asFieldExpr(exprOrFieldPath)})}
}

// IsNull creates a boolean expression that checks if a field or expression evaluates to null.
//
// exprOrFieldPath is the field path string, [FieldPath] or [Expr] that will be evaluated.
//
// Example:
//
//	// Check if the 'address' field is null.
//	IsNull("address")
func IsNull(exprOrFieldPath any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: newBaseFunction("is_null", []Expr{asFieldExpr(exprOrFieldPath)})}
}

// IsNotNull creates a boolean expression that checks if a field or expression does not evaluate to null.
//
// exprOrFieldPath is the field path string, [FieldPath] or [Expr] that will be evaluated.
//
// Example:
//
//	// Check if the 'address' field is not null.
//	IsNotNull("address")
func IsNotNull(exprOrFieldPath any) BooleanExpr {
	return &baseBooleanExpr{baseFunction: newBaseFunction("is_not_null", []Expr{asFieldExpr(exprOrFieldPath)})}
}
