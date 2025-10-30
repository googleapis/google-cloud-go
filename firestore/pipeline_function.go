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

		paramExpr := asFieldExpr(param)
		pbVal, err := paramExpr.toProto()
		if err != nil {
			return &baseFunction{baseExpr: &baseExpr{err: fmt.Errorf("firestore: error converting arg %d for function %q: %w", i, name, err)}}
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
// - right can be a numeric constant or a numeric [Expr].
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
// - numericExprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
//
// Example:
//
//	// Absolute value of the 'age' field.
//	Abs("age")
func Abs(numericExprOrFieldPath any) Expr {
	return newBaseFunction("abs", []Expr{asFieldExpr(numericExprOrFieldPath)})
}

// Floor creates an expression that is the largest integer that isn't less than the input field or expression.
// - numericExprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
//
// Example:
//
//	// Floor value of the 'age' field.
//	Floor("age")
func Floor(numericExprOrFieldPath any) Expr {
	return newBaseFunction("floor", []Expr{asFieldExpr(numericExprOrFieldPath)})
}

// Ceil creates an expression that is the smallest integer that isn't less than the input field or expression.
// - numericExprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
//
// Example:
//
//	// Ceiling value of the 'age' field.
//	Ceil("age")
func Ceil(numericExprOrFieldPath any) Expr {
	return newBaseFunction("ceil", []Expr{asFieldExpr(numericExprOrFieldPath)})
}

// Exp creates an expression that is the Euler's number e raised to the power of the input field or expression.
// - numericExprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
//
// Example:
//
//	// e to the power of the value of the 'age' field.
//	Exp("age")
func Exp(numericExprOrFieldPath any) Expr {
	return newBaseFunction("exp", []Expr{asFieldExpr(numericExprOrFieldPath)})
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
// - numericExprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
//
// Example:
//
//	// Base 10 logarithmic value of the 'age' field.
//	Log10("age")
func Log10(numericExprOrFieldPath any) Expr {
	return newBaseFunction("log10", []Expr{asFieldExpr(numericExprOrFieldPath)})
}

// Ln creates an expression that is the natural logarithm (base e) of the input field or expression.
// - numericExprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
//
// Example:
//
//	// Natural logarithmic value of the 'age' field.
//	Ln("age")
func Ln(numericExprOrFieldPath any) Expr {
	return newBaseFunction("ln", []Expr{asFieldExpr(numericExprOrFieldPath)})
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

// Round creates an expression that rounds the input field or expression to nearest integer.
// - numericExprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
//
// Example:
//
//	// Round the value of the 'age' field.
//	Round("age")
func Round(numericExprOrFieldPath any) Expr {
	return newBaseFunction("round", []Expr{asFieldExpr(numericExprOrFieldPath)})
}

// Sqrt creates an expression that is the square root of the input field or expression.
// - numericExprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
//
// Example:
//
//	// Square root of the value of the 'age' field.
//	Sqrt("age")
func Sqrt(numericExprOrFieldPath any) Expr {
	return newBaseFunction("sqrt", []Expr{asFieldExpr(numericExprOrFieldPath)})
}

// TimestampAdd creates an expression that adds a specified amount of time to a timestamp.
// - timestamp can be a field path string, [FieldPath] or [Expr].
// - unit can be a string or an [Expr]. Valid units include "microsecond", "millisecond", "second", "minute", "hour" and "day".
// - amount can be an int, int32, int64 or [Expr].
//
// Example:
//
//	// Add 5 hours to the value of the 'last_updated' field.
//	TimestampAdd("last_updated", "hour", 5)
func TimestampAdd(timestamp, unit, amount any) Expr {
	return newBaseFunction("timestamp_add", []Expr{asFieldExpr(timestamp), asStringExpr(unit), asInt64Expr(amount)})
}

// TimestampSubtract creates an expression that subtracts a specified amount of time from a timestamp.
// - timestamp can be a field path string, [FieldPath] or [Expr].
// - unit can be a string or an [Expr]. Valid units include "microsecond", "millisecond", "second", "minute", "hour" and "day".
// - amount can be an int, int32, int64 or [Expr].
//
// Example:
//
//	// Subtract 10 days from the value of the 'last_updated' field.
//	TimestampSubtract("last_updated", "day", 10)
func TimestampSubtract(timestamp, unit, amount any) Expr {
	return newBaseFunction("timestamp_subtract", []Expr{asFieldExpr(timestamp), asStringExpr(unit), asInt64Expr(amount)})
}

// TimestampToUnixMicros creates an expression that converts a timestamp expression to the number of microseconds since
// the Unix epoch (1970-01-01 00:00:00 UTC).
// - timestamp can be a field path string, [FieldPath] or [Expr].
func TimestampToUnixMicros(timestamp any) Expr {
	return newBaseFunction("timestamp_to_unix_micros", []Expr{asFieldExpr(timestamp)})
}

// TimestampToUnixMillis creates an expression that converts a timestamp expression to the number of milliseconds since
// the Unix epoch (1970-01-01 00:00:00 UTC).
// - timestamp can be a field path string, [FieldPath] or [Expr].
func TimestampToUnixMillis(timestamp any) Expr {
	return newBaseFunction("timestamp_to_unix_millis", []Expr{asFieldExpr(timestamp)})
}

// TimestampToUnixSeconds creates an expression that converts a timestamp expression to the number of seconds since
// the Unix epoch (1970-01-01 00:00:00 UTC).
// - timestamp can be a field path string, [FieldPath] or [Expr].
func TimestampToUnixSeconds(timestamp any) Expr {
	return newBaseFunction("timestamp_to_unix_seconds", []Expr{asFieldExpr(timestamp)})
}

// UnixMicrosToTimestamp creates an expression that converts a Unix timestamp in microseconds to a Firestore timestamp.
// - micros can be a field path string, [FieldPath] or [Expr].
func UnixMicrosToTimestamp(micros any) Expr {
	return newBaseFunction("unix_micros_to_timestamp", []Expr{asFieldExpr(micros)})
}

// UnixMillisToTimestamp creates an expression that converts a Unix timestamp in milliseconds to a Firestore timestamp.
// - millis can be a field path string, [FieldPath] or [Expr].
func UnixMillisToTimestamp(millis any) Expr {
	return newBaseFunction("unix_millis_to_timestamp", []Expr{asFieldExpr(millis)})
}

// UnixSecondsToTimestamp creates an expression that converts a Unix timestamp in seconds to a Firestore timestamp.
// - seconds can be a field path string, [FieldPath] or [Expr].
func UnixSecondsToTimestamp(seconds any) Expr {
	return newBaseFunction("unix_seconds_to_timestamp", []Expr{asFieldExpr(seconds)})
}

// CurrentTimestamp creates an expression that returns the current timestamp.
func CurrentTimestamp() Expr {
	return newBaseFunction("current_timestamp", []Expr{})
}

// ArrayLength creates an expression that calculates the length of an array.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to an array.
//
// Example:
//
//	// Get the length of the 'tags' array field.
//	ArrayLength("tags")
func ArrayLength(exprOrFieldPath any) Expr {
	return newBaseFunction("array_length", []Expr{toExprOrField(exprOrFieldPath)})
}

// Array creates an expression that represents a Firestore array.
// - elements can be any number of values or expressions that will form the elements of the array.
//
// Example:
//
//	// Create an array of numbers.
//	Array(1, 2, 3)
func Array(elements ...any) Expr {
	return newBaseFunction("array", toExprs(elements))
}

// ArrayFromSlice creates a new array expression from a slice of elements.
// This function is necessary for creating an array from an existing typed slice (e.g., []int),
// as the [Array] function (which takes variadic arguments) cannot directly accept a typed slice
// using the spread operator (...). It handles the conversion of each element to `any` internally.
func ArrayFromSlice[T any](elements []T) Expr {
	return newBaseFunction("array", toExprsFromSlice(elements))
}

// ArrayGet creates an expression that retrieves an element from an array at a specified index.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to an array.
// - offset is the 0-based index of the element to retrieve.
//
// Example:
//
//	// Get the first element of the 'tags' array field.
//	ArrayGet("tags", 0)
func ArrayGet(exprOrFieldPath any, offset any) Expr {
	return newBaseFunction("array_get", []Expr{toExprOrField(exprOrFieldPath), asInt64Expr(offset)})
}

// ArrayReverse creates an expression that reverses the order of elements in an array.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to an array.
//
// Example:
//
//	// Reverse the 'tags' array.
//	ArrayReverse("tags")
func ArrayReverse(exprOrFieldPath any) Expr {
	return newBaseFunction("array_reverse", []Expr{toExprOrField(exprOrFieldPath)})
}

// ArrayConcat creates an expression that concatenates multiple arrays into a single array.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to an array.
// - otherArrays are the other arrays to concatenate.
//
// Example:
//
//	// Concatenate the 'tags' and 'categories' array fields.
//	ArrayConcat("tags", FieldOf("categories"))
func ArrayConcat(exprOrFieldPath any, otherArrays ...any) Expr {
	return newBaseFunction("array_concat", append([]Expr{toExprOrField(exprOrFieldPath)}, toExprs(otherArrays)...))
}

// ArraySum creates an expression that calculates the sum of all elements in a numeric array.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a numeric array.
//
// Example:
//
//	// Calculate the sum of the 'scores' array.
//	ArraySum("scores")
func ArraySum(exprOrFieldPath any) Expr {
	return newBaseFunction("sum", []Expr{toExprOrField(exprOrFieldPath)})
}

// ArrayMaximum creates an expression that finds the maximum element in a numeric array.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a numeric array.
//
// Example:
//
//	// Find the maximum value in the 'scores' array.
//	ArrayMaximum("scores")
func ArrayMaximum(exprOrFieldPath any) Expr {
	return newBaseFunction("maximum", []Expr{toExprOrField(exprOrFieldPath)})
}

// ArrayMinimum creates an expression that finds the minimum element in a numeric array.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a numeric array.
//
// Example:
//
//	// Find the minimum value in the 'scores' array.
//	ArrayMinimum("scores")
func ArrayMinimum(exprOrFieldPath any) Expr {
	return newBaseFunction("minimum", []Expr{toExprOrField(exprOrFieldPath)})
}

// ByteLength creates an expression that calculates the length of a string represented by a field or [Expr] in UTF-8
// bytes, or just the length of a Blob.
//   - exprOrFieldPath can be a field path string, [FieldPath] or [Expr].
//
// Example:
//
//	// Get the byte length of the 'name' field.
//	ByteLength("name")
func ByteLength(exprOrFieldPath any) Expr {
	return newBaseFunction("byte_length", []Expr{toExprOrField(exprOrFieldPath)})
}

// CharLength creates an expression that calculates the character length of a string field or expression in UTF8.
//   - exprOrFieldPath can be a field path string, [FieldPath] or [Expr].
//
// Example:
//
//	// Get the character length of the 'name' field.
//	CharLength("name")
func CharLength(exprOrFieldPath any) Expr {
	return newBaseFunction("char_length", []Expr{toExprOrField(exprOrFieldPath)})
}

// StringConcat creates an expression that concatenates multiple strings into a single string.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a string.
// - otherStrings are the other strings to concatenate.
//
// Example:
//
//	// Concatenate first name and last name.
//	StringConcat(FieldOf("firstName"), " ", FieldOf("lastName"))
func StringConcat(exprOrFieldPath any, otherStrings ...any) Expr {
	return newBaseFunction("string_concat", append([]Expr{toExprOrField(exprOrFieldPath)}, toExprs(otherStrings)...))
}

// StringReverse creates an expression that reverses a string.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a string.
//
// Example:
//
//	// Reverse the 'name' field.
//	StringReverse("name")
func StringReverse(exprOrFieldPath any) Expr {
	return newBaseFunction("string_reverse", []Expr{toExprOrField(exprOrFieldPath)})
}

// Join creates an expression that joins the elements of a string array into a single string.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a string array.
// - separator is the string to use as a separator between elements.
//
// Example:
//
//	// Join the 'tags' array with a comma and space.
//	Join("tags", ", ")
func Join(exprOrFieldPath any, separator any) Expr {
	return newBaseFunction("join", []Expr{toExprOrField(exprOrFieldPath), asStringExpr(separator)})
}

// Substring creates an expression that returns a substring of a string.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a string.
// - index is the starting index of the substring.
// - offset is the length of the substring.
//
// Example:
//
//	// Get the first 5 characters of the 'description' field.
//	Substring("description", 0, 5)
func Substring(exprOrFieldPath any, index any, offset any) Expr {
	return newBaseFunction("substring", []Expr{toExprOrField(exprOrFieldPath), asInt64Expr(index), asInt64Expr(offset)})
}

// ToLower creates an expression that converts a string to lowercase.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a string.
//
// Example:
//
//	// Convert the 'username' to lowercase.
//	ToLower("username")
func ToLower(exprOrFieldPath any) Expr {
	return newBaseFunction("to_lower", []Expr{toExprOrField(exprOrFieldPath)})
}

// ToUpper creates an expression that converts a string to uppercase.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a string.
//
// Example:
//
//	// Convert the 'product_code' to uppercase.
//	ToUpper("product_code")
func ToUpper(exprOrFieldPath any) Expr {
	return newBaseFunction("to_upper", []Expr{toExprOrField(exprOrFieldPath)})
}

// Trim creates an expression that removes leading and trailing whitespace from a string.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a string.
//
// Example:
//
//	// Trim the 'email' field.
//	Trim("email")
func Trim(exprOrFieldPath any) Expr {
	return newBaseFunction("trim", []Expr{toExprOrField(exprOrFieldPath)})
}

// CosineDistance creates an expression that calculates the cosine distance between two vectors.
//   - vector1 can be a field path string, [FieldPath] or [Expr].
//   - vector2 can be [Vector32], [Vector64], []float32, []float64 or [Expr].
//
// Example:
//
//	// Calculate the cosine distance between two vector fields.
//	CosineDistance("vector_field_1", FieldOf("vector_field_2"))
func CosineDistance(vector1 any, vector2 any) Expr {
	return newBaseFunction("cosine_distance", []Expr{toExprOrField(vector1), asVectorExpr(vector2)})
}

// DotProduct creates an expression that calculates the dot product of two vectors.
//   - vector1 can be a field path string, [FieldPath] or [Expr].
//   - vector2 can be [Vector32], [Vector64], []float32, []float64 or [Expr].
//
// Example:
//
//	// Calculate the dot product of two vector fields.
//	DotProduct("vector_field_1", FieldOf("vector_field_2"))
func DotProduct(vector1 any, vector2 any) Expr {
	return newBaseFunction("dot_product", []Expr{toExprOrField(vector1), asVectorExpr(vector2)})
}

// EuclideanDistance creates an expression that calculates the euclidean distance between two vectors.
//   - vector1 can be a field path string, [FieldPath] or [Expr].
//   - vector2 can be [Vector32], [Vector64], []float32, []float64 or [Expr].
//
// Example:
//
//	// Calculate the euclidean distance between two vector fields.
//	EuclideanDistance("vector_field_1", FieldOf("vector_field_2"))
func EuclideanDistance(vector1 any, vector2 any) Expr {
	return newBaseFunction("euclidean_distance", []Expr{toExprOrField(vector1), asVectorExpr(vector2)})
}

// VectorLength creates an expression that calculates the length of a vector.
//   - exprOrFieldPath can be a field path string, [FieldPath] or [Expr].
//
// Example:
//
//	// Calculate the length of a vector field.
//	VectorLength("vector_field")
func VectorLength(exprOrFieldPath any) Expr {
	return newBaseFunction("vector_length", []Expr{toExprOrField(exprOrFieldPath)})
}
