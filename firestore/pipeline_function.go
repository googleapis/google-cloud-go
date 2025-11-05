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

func newBaseFunctionFromBooleans(name string, params []BooleanExpr) *baseFunction {
	exprs := make([]Expr, len(params))
	for i, p := range params {
		exprs[i] = p
	}
	return newBaseFunction(name, exprs)
}

// Add creates an expression that adds two expressions together, returning it as an Expr.
// - left can be a field path string, [FieldPath] or [Expr].
// - right can be a numeric constant or a numeric [Expr].
func Add(left, right any) Expr {
	return leftRightToBaseFunction("add", left, right)
}

// Subtract creates an expression that subtracts the right expression from the left expression, returning it as an Expr.
// - left can be a field path string, [FieldPath] or [Expr].
// - right can be a constant or an [Expr].
func Subtract(left, right any) Expr {
	return leftRightToBaseFunction("subtract", left, right)
}

// Multiply creates an expression that multiplies the left and right expressions, returning it as an Expr.
// - left can be a field path string, [FieldPath] or [Expr].
// - right can be a constant or an [Expr].
func Multiply(left, right any) Expr {
	return leftRightToBaseFunction("multiply", left, right)
}

// Divide creates an expression that divides the left expression by the right expression, returning it as an Expr.
// - left can be a field path string, [FieldPath] or [Expr].
// - right can be a constant or an [Expr].
func Divide(left, right any) Expr {
	return leftRightToBaseFunction("divide", left, right)
}

// Abs creates an expression that is the absolute value of the input field or expression.
// - numericExprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
func Abs(numericExprOrFieldPath any) Expr {
	return newBaseFunction("abs", []Expr{asFieldExpr(numericExprOrFieldPath)})
}

// Floor creates an expression that is the largest integer that isn't less than the input field or expression.
// - numericExprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
func Floor(numericExprOrFieldPath any) Expr {
	return newBaseFunction("floor", []Expr{asFieldExpr(numericExprOrFieldPath)})
}

// Ceil creates an expression that is the smallest integer that isn't less than the input field or expression.
// - numericExprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
func Ceil(numericExprOrFieldPath any) Expr {
	return newBaseFunction("ceil", []Expr{asFieldExpr(numericExprOrFieldPath)})
}

// Exp creates an expression that is the Euler's number e raised to the power of the input field or expression.
// - numericExprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
func Exp(numericExprOrFieldPath any) Expr {
	return newBaseFunction("exp", []Expr{asFieldExpr(numericExprOrFieldPath)})
}

// Log creates an expression that is logarithm of the left expression to base as the right expression, returning it as an Expr.
// - left can be a field path string, [FieldPath] or [Expr].
// - right can be a constant or an [Expr].
func Log(left, right any) Expr {
	return leftRightToBaseFunction("log", left, right)
}

// Log10 creates an expression that is the base 10 logarithm of the input field or expression.
// - numericExprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
func Log10(numericExprOrFieldPath any) Expr {
	return newBaseFunction("log10", []Expr{asFieldExpr(numericExprOrFieldPath)})
}

// Ln creates an expression that is the natural logarithm (base e) of the input field or expression.
// - numericExprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
func Ln(numericExprOrFieldPath any) Expr {
	return newBaseFunction("ln", []Expr{asFieldExpr(numericExprOrFieldPath)})
}

// Mod creates an expression that computes the modulo of the left expression by the right expression, returning it as an Expr.
// - left can be a field path string, [FieldPath] or [Expr].
// - right can be a constant or an [Expr].
func Mod(left, right any) Expr {
	return leftRightToBaseFunction("mod", left, right)
}

// Pow creates an expression that computes the left expression raised to the power of the right expression, returning it as an Expr.
// - left can be a field path string, [FieldPath] or [Expr].
// - right can be a constant or an [Expr].
func Pow(left, right any) Expr {
	return leftRightToBaseFunction("pow", left, right)
}

// Round creates an expression that rounds the input field or expression to nearest integer.
// - numericExprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
func Round(numericExprOrFieldPath any) Expr {
	return newBaseFunction("round", []Expr{asFieldExpr(numericExprOrFieldPath)})
}

// Sqrt creates an expression that is the square root of the input field or expression.
// - numericExprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that returns a number when evaluated.
func Sqrt(numericExprOrFieldPath any) Expr {
	return newBaseFunction("sqrt", []Expr{asFieldExpr(numericExprOrFieldPath)})
}

// TimestampAdd creates an expression that adds a specified amount of time to a timestamp.
// - timestamp can be a field path string, [FieldPath] or [Expr].
// - unit can be a string or an [Expr]. Valid units include "microsecond", "millisecond", "second", "minute", "hour" and "day".
// - amount can be an int, int32, int64 or [Expr].
func TimestampAdd(timestamp, unit, amount any) Expr {
	return newBaseFunction("timestamp_add", []Expr{asFieldExpr(timestamp), asStringExpr(unit), asInt64Expr(amount)})
}

// TimestampSubtract creates an expression that subtracts a specified amount of time from a timestamp.
// - timestamp can be a field path string, [FieldPath] or [Expr].
// - unit can be a string or an [Expr]. Valid units include "microsecond", "millisecond", "second", "minute", "hour" and "day".
// - amount can be an int, int32, int64 or [Expr].
func TimestampSubtract(timestamp, unit, amount any) Expr {
	return newBaseFunction("timestamp_subtract", []Expr{asFieldExpr(timestamp), asStringExpr(unit), asInt64Expr(amount)})
}

// TimestampTruncate creates an expression that truncates a timestamp to a specified granularity.
//   - timestamp can be a field path string, [FieldPath] or [Expr].
//   - granularity can be a string or an [Expr]. Valid values are "microsecond",
//     "millisecond", "second", "minute", "hour", "day", "week", "week(monday)", "week(tuesday)",
//     "week(wednesday)", "week(thursday)", "week(friday)", "week(saturday)", "week(sunday)",
//     "isoweek", "month", "quarter", "year", and "isoyear".
func TimestampTruncate(timestamp, granularity any) Expr {
	return newBaseFunction("timestamp_trunc", []Expr{asFieldExpr(timestamp), asStringExpr(granularity)})
}

// TimestampTruncateWithTimezone creates an expression that truncates a timestamp to a specified granularity in a given timezone.
//   - timestamp can be a field path string, [FieldPath] or [Expr].
//   - granularity can be a string or an [Expr]. Valid values are "microsecond",
//     "millisecond", "second", "minute", "hour", "day", "week", "week(monday)", "week(tuesday)",
//     "week(wednesday)", "week(thursday)", "week(friday)", "week(saturday)", "week(sunday)",
//     "isoweek", "month", "quarter", "year", and "isoyear".
//   - timezone can be a string or an [Expr]. Valid values are from the TZ database
//     (e.g., "America/Los_Angeles") or in the format "Etc/GMT-1".
func TimestampTruncateWithTimezone(timestamp, granularity any, timezone string) Expr {
	return newBaseFunction("timestamp_trunc", []Expr{asFieldExpr(timestamp), asStringExpr(granularity), asStringExpr(timezone)})
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
func ArrayLength(exprOrFieldPath any) Expr {
	return newBaseFunction("array_length", []Expr{asFieldExpr(exprOrFieldPath)})
}

// Array creates an expression that represents a Firestore array.
// - elements can be any number of values or expressions that will form the elements of the array.
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
func ArrayGet(exprOrFieldPath any, offset any) Expr {
	return newBaseFunction("array_get", []Expr{asFieldExpr(exprOrFieldPath), asInt64Expr(offset)})
}

// ArrayReverse creates an expression that reverses the order of elements in an array.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to an array.
func ArrayReverse(exprOrFieldPath any) Expr {
	return newBaseFunction("array_reverse", []Expr{asFieldExpr(exprOrFieldPath)})
}

// ArrayConcat creates an expression that concatenates multiple arrays into a single array.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to an array.
// - otherArrays are the other arrays to concatenate.
func ArrayConcat(exprOrFieldPath any, otherArrays ...any) Expr {
	return newBaseFunction("array_concat", append([]Expr{asFieldExpr(exprOrFieldPath)}, toExprs(otherArrays)...))
}

// ArraySum creates an expression that calculates the sum of all elements in a numeric array.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a numeric array.
func ArraySum(exprOrFieldPath any) Expr {
	return newBaseFunction("sum", []Expr{asFieldExpr(exprOrFieldPath)})
}

// ArrayMaximum creates an expression that finds the maximum element in a numeric array.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a numeric array.
func ArrayMaximum(exprOrFieldPath any) Expr {
	return newBaseFunction("maximum", []Expr{asFieldExpr(exprOrFieldPath)})
}

// ArrayMinimum creates an expression that finds the minimum element in a numeric array.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a numeric array.
func ArrayMinimum(exprOrFieldPath any) Expr {
	return newBaseFunction("minimum", []Expr{asFieldExpr(exprOrFieldPath)})
}

// ByteLength creates an expression that calculates the length of a string represented by a field or [Expr] in UTF-8
// bytes.
//   - exprOrFieldPath can be a field path string, [FieldPath] or [Expr].
func ByteLength(exprOrFieldPath any) Expr {
	return newBaseFunction("byte_length", []Expr{asFieldExpr(exprOrFieldPath)})
}

// CharLength creates an expression that calculates the character length of a string field or expression in UTF8.
//   - exprOrFieldPath can be a field path string, [FieldPath] or [Expr].
func CharLength(exprOrFieldPath any) Expr {
	return newBaseFunction("char_length", []Expr{asFieldExpr(exprOrFieldPath)})
}

// StringConcat creates an expression that concatenates multiple strings into a single string.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a string.
// - otherStrings are the other strings to concatenate.
func StringConcat(exprOrFieldPath any, otherStrings ...any) Expr {
	return newBaseFunction("string_concat", append([]Expr{asFieldExpr(exprOrFieldPath)}, toExprs(otherStrings)...))
}

// StringReverse creates an expression that reverses a string.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a string.
func StringReverse(exprOrFieldPath any) Expr {
	return newBaseFunction("string_reverse", []Expr{asFieldExpr(exprOrFieldPath)})
}

// Join creates an expression that joins the elements of a string array into a single string.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a string array.
// - delimiter is the string to use as a separator between elements.
func Join(exprOrFieldPath any, delimiter any) Expr {
	return newBaseFunction("join", []Expr{asFieldExpr(exprOrFieldPath), asStringExpr(delimiter)})
}

// Substring creates an expression that returns a substring of a string.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a string.
// - index is the starting index of the substring.
// - offset is the length of the substring.
func Substring(exprOrFieldPath any, index any, offset any) Expr {
	return newBaseFunction("substring", []Expr{asFieldExpr(exprOrFieldPath), asInt64Expr(index), asInt64Expr(offset)})
}

// ToLower creates an expression that converts a string to lowercase.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a string.
func ToLower(exprOrFieldPath any) Expr {
	return newBaseFunction("to_lower", []Expr{asFieldExpr(exprOrFieldPath)})
}

// ToUpper creates an expression that converts a string to uppercase.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a string.
func ToUpper(exprOrFieldPath any) Expr {
	return newBaseFunction("to_upper", []Expr{asFieldExpr(exprOrFieldPath)})
}

// Trim creates an expression that removes leading and trailing whitespace from a string.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a string.
func Trim(exprOrFieldPath any) Expr {
	return newBaseFunction("trim", []Expr{asFieldExpr(exprOrFieldPath)})
}

// Split creates an expression that splits a string by a delimiter.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr] that evaluates to a string.
// - delimiter is the string to use to split by.
func Split(exprOrFieldPath any, delimiter any) Expr {
	return newBaseFunction("split", []Expr{asFieldExpr(exprOrFieldPath), asStringExpr(delimiter)})
}

// Type creates an expression that returns the type of the expression.
// - exprOrFieldPath can be a field path string, [FieldPath] or an [Expr].
func Type(exprOrFieldPath any) Expr {
	return newBaseFunction("type", []Expr{asFieldExpr(exprOrFieldPath)})
}

// CosineDistance creates an expression that calculates the cosine distance between two vectors.
//   - vector1 can be a field path string, [FieldPath] or [Expr].
//   - vector2 can be [Vector32], [Vector64], []float32, []float64 or [Expr].
func CosineDistance(vector1 any, vector2 any) Expr {
	return newBaseFunction("cosine_distance", []Expr{asFieldExpr(vector1), asVectorExpr(vector2)})
}

// DotProduct creates an expression that calculates the dot product of two vectors.
//   - vector1 can be a field path string, [FieldPath] or [Expr].
//   - vector2 can be [Vector32], [Vector64], []float32, []float64 or [Expr].
func DotProduct(vector1 any, vector2 any) Expr {
	return newBaseFunction("dot_product", []Expr{asFieldExpr(vector1), asVectorExpr(vector2)})
}

// EuclideanDistance creates an expression that calculates the euclidean distance between two vectors.
//   - vector1 can be a field path string, [FieldPath] or [Expr].
//   - vector2 can be [Vector32], [Vector64], []float32, []float64 or [Expr].
func EuclideanDistance(vector1 any, vector2 any) Expr {
	return newBaseFunction("euclidean_distance", []Expr{asFieldExpr(vector1), asVectorExpr(vector2)})
}

// VectorLength creates an expression that calculates the length of a vector.
//   - exprOrFieldPath can be a field path string, [FieldPath] or [Expr].
func VectorLength(exprOrFieldPath any) Expr {
	return newBaseFunction("vector_length", []Expr{asFieldExpr(exprOrFieldPath)})
}

// Length creates an expression that calculates the length of string, array, map or vector.
// - exprOrField can be a field path string, [FieldPath] or an [Expr] that returns a string, array, map or vector when evaluated.
//
// Example:
//
//	// Length of the 'name' field.
//	Length("name")
func Length(exprOrField any) Expr {
	return newBaseFunction("length", []Expr{asFieldExpr(exprOrField)})
}

// Reverse creates an expression that reverses a string, or array.
// - exprOrField can be a field path string, [FieldPath] or an [Expr] that returns a string, or array when evaluated.
//
// Example:
//
//	// Reverse the 'name' field.
//
// Reverse("name")
func Reverse(exprOrField any) Expr {
	return newBaseFunction("reverse", []Expr{asFieldExpr(exprOrField)})
}

// Concat creates an expression that concatenates expressions together.
// - exprOrField can be a field path string, [FieldPath] or an [Expr].
// - others can be a list of constants or [Expr].
//
// Example:
//
//	// Concat the 'name' field with a constant string.
//	Concat("name", "-suffix")
func Concat(exprOrField any, others ...any) Expr {
	return newBaseFunction("concat", append([]Expr{asFieldExpr(exprOrField)}, toArrayOfExprOrConstant(others)...))
}

// GetCollectionID creates an expression that returns the ID of the collection that contains the document.
// - exprOrField can be a field path string, [FieldPath] or an [Expr] that evaluates to a field path.
func GetCollectionID(exprOrField any) Expr {
	return newBaseFunction("collection_id", []Expr{asFieldExpr(exprOrField)})
}

// GetDocumentID creates an expression that returns the ID of the document.
// - exprStringOrDocRef can be a string, a [DocumentRef], or an [Expr] that evaluates to a document reference.
func GetDocumentID(exprStringOrDocRef any) Expr {
	var expr Expr
	switch v := exprStringOrDocRef.(type) {
	case string:
		expr = ConstantOf(v)
	case *DocumentRef:
		expr = ConstantOf(v)
	case Expr:
		expr = v
	default:
		return &baseFunction{baseExpr: &baseExpr{err: fmt.Errorf("firestore: value must be a string, DocumentRef, or Expr, but got %T", exprStringOrDocRef)}}
	}

	return newBaseFunction("document_id", []Expr{expr})
}

// Conditional creates an expression that evaluates a condition and returns one of two expressions.
// - condition is the boolean expression to evaluate.
// - thenVal is the expression to return if the condition is true.
// - elseVal is the expression to return if the condition is false.
func Conditional(condition BooleanExpr, thenVal, elseVal any) Expr {
	return newBaseFunction("conditional", []Expr{condition, toExprOrConstant(thenVal), toExprOrConstant(elseVal)})
}

// LogicalMaximum creates an expression that evaluates to the maximum value in a list of expressions.
// - exprOrField can be a field path string, [FieldPath] or an [Expr].
// - others can be a list of constants or [Expr].
func LogicalMaximum(exprOrField any, others ...any) Expr {
	return newBaseFunction("maximum", append([]Expr{asFieldExpr(exprOrField)}, toArrayOfExprOrConstant(others)...))
}

// LogicalMinimum creates an expression that evaluates to the minimum value in a list of expressions.
// - exprOrField can be a field path string, [FieldPath] or an [Expr].
// - others can be a list of constants or [Expr].
func LogicalMinimum(exprOrField any, others ...any) Expr {
	return newBaseFunction("minimum", append([]Expr{asFieldExpr(exprOrField)}, toArrayOfExprOrConstant(others)...))
}

// IfAbsent creates an expression that returns a default value if an expression evaluates to an absent value.
// - exprOrField can be a field path string, [FieldPath] or an [Expr].
// - elseValue is the value to return if the expression is absent.
func IfAbsent(exprOrField any, elseValue any) Expr {
	return newBaseFunction("if_absent", []Expr{asFieldExpr(exprOrField), toExprOrConstant(elseValue)})
}

// Map creates an expression that creates a Firestore map value from an input object.
// - elements: The input map to evaluate in the expression.
func Map(elements map[string]any) Expr {
	exprs := []Expr{}
	for k, v := range elements {
		exprs = append(exprs, ConstantOf(k), toExprOrConstant(v))
	}
	return newBaseFunction("map", exprs)
}

// MapGet creates an expression that accesses a value from a map (object) field using the provided key.
// - exprOrField: The expression representing the map.
// - strOrExprkey: The key to access in the map.
func MapGet(exprOrField any, strOrExprkey any) Expr {
	return newBaseFunction("map_get", []Expr{asFieldExpr(exprOrField), asStringExpr(strOrExprkey)})
}

// MapMerge creates an expression that merges multiple maps into a single map.
// If multiple maps have the same key, the later value is used.
// - exprOrField: First map expression that will be merged.
// - secondMap: Second map expression that will be merged.
// - otherMaps: Additional maps to merge.
func MapMerge(exprOrField any, secondMap Expr, otherMaps ...Expr) Expr {
	return newBaseFunction("map_merge", append([]Expr{asFieldExpr(exprOrField), secondMap}, otherMaps...))
}

// MapRemove creates an expression that removes a key from a map.
// - exprOrField: The expression representing the map.
// - strOrExprkey: The key to remove from the map.
func MapRemove(exprOrField any, strOrExprkey any) Expr {
	return newBaseFunction("map_remove", []Expr{asFieldExpr(exprOrField), asStringExpr(strOrExprkey)})
}
