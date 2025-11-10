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
	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
)

// Selectable is an interface for expressions that can be selected in a pipeline.
type Selectable interface {
	// getSelectionDetails returns the output alias and the underlying expression.
	getSelectionDetails() (alias string, expr Expr)

	isSelectable()
}

// Expr represents an expression that can be evaluated to a value within the execution of a
// [Pipeline].
//
// Expressions are the building blocks for creating complex queries and transformations in
// Firestore pipelines. They can represent:
//
// - Field references: Access values from document fields.
// - Literals: Represent constant values (strings, numbers, booleans).
// - Function calls: Apply functions to one or more expressions.
// - Aggregations: Calculate aggregate values (e.g., sum, average) using [AggregateFunction] instances.
//
// The [Expr] interface provides a fluent API for building expressions. You can chain together
// method calls to create complex expressions.
type Expr interface {
	isExpr()
	toProto() (*pb.Value, error)
	getBaseExpr() *baseExpr

	// Aritmetic operations

	// Add creates an expression that adds two expressions together, returning it as an Expr.
	//
	// The parameter 'other' can be a numeric constant or a numeric [Expr].
	Add(other any) Expr
	// Subtract creates an expression that subtracts the right expression from the left expression, returning it as an Expr.
	//
	// The parameter 'other' can be a numeric constant or a numeric [Expr].
	Subtract(other any) Expr
	// Multiply creates an expression that multiplies the left and right expressions, returning it as an Expr.
	//
	// The parameter 'other' can be a numeric constant or a numeric [Expr].
	Multiply(other any) Expr
	// Divide creates an expression that divides the left expression by the right expression, returning it as an Expr.
	//
	// The parameter 'other' can be a numeric constant or a numeric [Expr].
	Divide(other any) Expr
	// Abs creates an expression that is the absolute value of the input field or expression.
	Abs() Expr
	// Floor creates an expression that is the largest integer that isn't less than the input field or expression.
	Floor() Expr
	// Ceil creates an expression that is the smallest integer that isn't less than the input field or expression.
	Ceil() Expr
	// Exp creates an expression that is the Euler's number e raised to the power of the input field or expression.
	Exp() Expr
	// Log creates an expression that is logarithm of the left expression to base as the right expression, returning it as an Expr.
	//
	// The parameter 'other' can be a numeric constant or a numeric [Expr].
	Log(other any) Expr
	// Log10 creates an expression that is the base 10 logarithm of the input field or expression.
	Log10() Expr
	// Ln creates an expression that is the natural logarithm (base e) of the input field or expression.
	Ln() Expr
	// Mod creates an expression that computes the modulo of the left expression by the right expression, returning it as an Expr.
	//
	// The parameter 'other' can be a numeric constant or a numeric [Expr].
	Mod(other any) Expr
	// Pow creates an expression that computes the left expression raised to the power of the right expression, returning it as an Expr.
	//
	// The parameter 'other' can be a numeric constant or a numeric [Expr].
	Pow(other any) Expr
	// Round creates an expression that rounds the input field or expression to nearest integer.
	Round() Expr
	// Sqrt creates an expression that is the square root of the input field or expression.
	Sqrt() Expr

	// Array operations
	// ArrayContains creates a boolean expression that checks if an array contains a specific value.
	//
	// The parameter 'value' can be a constant (e.g., string, int, bool) or an [Expr].
	ArrayContains(value any) BooleanExpr
	// ArrayContainsAll creates a boolean expression that checks if an array contains all the specified values.
	//
	// The parameter 'values' can be a slice of constants (e.g., []string, []int) or an [Expr] that evaluates to an array.
	ArrayContainsAll(values any) BooleanExpr
	// ArrayContainsAny creates a boolean expression that checks if an array contains any of the specified values.
	//
	// The parameter 'values' can be a slice of constants (e.g., []string, []int) or an [Expr] that evaluates to an array.
	ArrayContainsAny(values any) BooleanExpr
	// ArrayLength creates an expression that calculates the length of an array.
	ArrayLength() Expr
	// EqualAny creates a boolean expression that checks if the expression is equal to any of the specified values.
	//
	// The parameter 'values' can be a slice of constants (e.g., []string, []int) or an [Expr] that evaluates to an array.
	EqualAny(values any) BooleanExpr
	// NotEqualAny creates a boolean expression that checks if the expression is not equal to any of the specified values.
	//
	// The parameter 'values' can be a slice of constants (e.g., []string, []int) or an [Expr] that evaluates to an array.
	NotEqualAny(values any) BooleanExpr
	// ArrayGet creates an expression that retrieves an element from an array at a specified index.
	//
	// The parameter 'offset' is the 0-based index of the element to retrieve.
	// It can be an integer constant or an [Expr] that evaluates to an integer.
	ArrayGet(offset any) Expr
	// ArrayReverse creates an expression that reverses the order of elements in an array.
	ArrayReverse() Expr
	// ArrayConcat creates an expression that concatenates multiple arrays into a single array.
	//
	// The parameter 'otherArrays' can be a mix of array constants (e.g., []string, []int) or [Expr]s that evaluate to arrays.
	ArrayConcat(otherArrays ...any) Expr
	// ArraySum creates an expression that calculates the sum of all elements in a numeric array.
	ArraySum() Expr
	// ArrayMaximum creates an expression that finds the maximum element in a numeric array.
	ArrayMaximum() Expr
	// ArrayMinimum creates an expression that finds the minimum element in a numeric array.
	ArrayMinimum() Expr

	// Timestamp operations
	// TimestampAdd creates an expression that adds a specified amount of time to a timestamp.
	//
	// The parameter 'unit' can be a string constant (e.g.,  "day") or an [Expr] that evaluates to a valid unit string.
	// Valid units include "microsecond", "millisecond", "second", "minute", "hour" and "day".
	// The parameter 'amount' can be an integer constant or an [Expr] that evaluates to an integer.
	TimestampAdd(unit, amount any) Expr
	// TimestampSubtract creates an expression that subtracts a specified amount of time from a timestamp.
	//
	// The parameter 'unit' can be a string constant (e.g.,  "hour") or an [Expr] that evaluates to a valid unit string.
	// Valid units include "microsecond", "millisecond", "second", "minute", "hour" and "day".
	// The parameter 'amount' can be an integer constant or an [Expr] that evaluates to an integer.
	TimestampSubtract(unit, amount any) Expr
	// TimestampTruncate creates an expression that truncates a timestamp to a specified granularity.
	//
	// The parameter 'granularity' can be a string constant (e.g.,  "month") or an [Expr] that evaluates to a valid granularity string.
	// Valid values are "microsecond", "millisecond", "second", "minute", "hour", "day", "week", "week(monday)", "week(tuesday)",
	// "week(wednesday)", "week(thursday)", "week(friday)", "week(saturday)", "week(sunday)", "isoweek", "month", "quarter", "year", and "isoyear".
	TimestampTruncate(granularity any) Expr
	// TimestampTruncateWithTimezone creates an expression that truncates a timestamp to a specified granularity in a given timezone.
	//
	// The parameter 'granularity' can be a string constant (e.g.,  "week") or an [Expr] that evaluates to a valid granularity string.
	// Valid values are "microsecond", "millisecond", "second", "minute", "hour", "day", "week", "week(monday)", "week(tuesday)",
	// "week(wednesday)", "week(thursday)", "week(friday)", "week(saturday)", "week(sunday)", "isoweek", "month", "quarter", "year", and "isoyear".
	// The parameter 'timezone' can be a string constant (e.g., "America/Los_Angeles") or an [Expr] that evaluates to a valid timezone string.
	// Valid values are from the TZ database or in the format "Etc/GMT-1".
	TimestampTruncateWithTimezone(granularity any, timezone string) Expr
	// TimestampToUnixMicros creates an expression that converts a timestamp expression to the number of microseconds since
	// the Unix epoch (1970-01-01 00:00:00 UTC).
	TimestampToUnixMicros() Expr
	// TimestampToUnixMillis creates an expression that converts a timestamp expression to the number of milliseconds since
	// the Unix epoch (1970-01-01 00:00:00 UTC).
	TimestampToUnixMillis() Expr
	// TimestampToUnixSeconds creates an expression that converts a timestamp expression to the number of seconds since
	// the Unix epoch (1970-01-01 00:00:00 UTC).
	TimestampToUnixSeconds() Expr
	// UnixMicrosToTimestamp creates an expression that converts a Unix timestamp in microseconds to a Firestore timestamp.
	UnixMicrosToTimestamp() Expr
	// UnixMillisToTimestamp creates an expression that converts a Unix timestamp in milliseconds to a Firestore timestamp.
	UnixMillisToTimestamp() Expr
	// UnixSecondsToTimestamp creates an expression that converts a Unix timestamp in seconds to a Firestore timestamp.
	UnixSecondsToTimestamp() Expr

	// Comparison operations
	// Equal creates a boolean expression that checks if the expression is equal to the other value.
	//
	// The parameter 'other' can be a constant (e.g., string, int, bool) or an [Expr].
	Equal(other any) BooleanExpr
	// NotEqual creates a boolean expression that checks if the expression is not equal to the other value.
	//
	// The parameter 'other' can be a constant (e.g., string, int, bool) or an [Expr].
	NotEqual(other any) BooleanExpr
	// GreaterThan creates a boolean expression that checks if the expression is greater than the other value.
	//
	// The parameter 'other' can be a constant (e.g., string, int, bool) or an [Expr].
	GreaterThan(other any) BooleanExpr
	// GreaterThanOrEqual creates a boolean expression that checks if the expression is greater than or equal to the other value.
	//
	// The parameter 'other' can be a constant (e.g., string, int, bool) or an [Expr].
	GreaterThanOrEqual(other any) BooleanExpr
	// LessThan creates a boolean expression that checks if the expression is less than the other value.
	//
	// The parameter 'other' can be a constant (e.g., string, int, bool) or an [Expr].
	LessThan(other any) BooleanExpr
	// LessThanOrEqual creates a boolean expression that checks if the expression is less than or equal to the other value.
	//
	// The parameter 'other' can be a constant (e.g., string, int, bool) or an [Expr].
	LessThanOrEqual(other any) BooleanExpr

	// General functions
	// Length creates an expression that calculates the length of string, array, map or vector.
	Length() Expr
	// Reverse creates an expression that reverses a string, or array.
	Reverse() Expr
	// Concat creates an expression that concatenates expressions together.
	//
	// The parameter 'others' can be a list of constants (e.g., string, int) or [Expr].
	Concat(others ...any) Expr

	// Key functions
	// GetCollectionID creates an expression that returns the ID of the collection that contains the document.
	GetCollectionID() Expr
	// GetDocumentID creates an expression that returns the ID of the document.
	GetDocumentID() Expr

	// Logical functions
	// IfError creates an expression that evaluates and returns the receiver expression if it does not produce an error;
	// otherwise, it evaluates and returns `catchExprOrValue`.
	//
	// The parameter 'catchExprOrValue' is the expression or value to return if the receiver expression errors.
	IfError(catchExprOrValue any) Expr
	// IfAbsent creates an expression that returns a default value if an expression evaluates to an absent value.
	//
	// The parameter 'catchExprOrValue' is the value to return if the expression is absent.
	// It can be a constant or an [Expr].
	IfAbsent(catchExprOrValue any) Expr

	// Object functions
	// MapGet creates an expression that accesses a value from a map (object) field using the provided key.
	//
	// The parameter 'strOrExprkey' is the key to access in the map.
	// It can be a string constant or an [Expr] that evaluates to a string.
	MapGet(strOrExprkey any) Expr
	// MapMerge creates an expression that merges multiple maps into a single map.
	// If multiple maps have the same key, the later value is used.
	//
	// The parameter 'secondMap' is an [Expr] representing the second map.
	// The parameter 'otherMaps' is a list of additional [Expr]s representing maps to merge.
	MapMerge(secondMap Expr, otherMaps ...Expr) Expr
	// MapRemove creates an expression that removes a key from a map.
	//
	// The parameter 'strOrExprkey' is the key to remove from the map.
	// It can be a string constant or an [Expr] that evaluates to a string.
	MapRemove(strOrExprkey any) Expr

	// Aggregators
	// Sum creates an aggregate function that calculates the sum of the expression.
	Sum() AggregateFunction
	// Average creates an aggregate function that calculates the average of the expression.
	Average() AggregateFunction
	// Count creates an aggregate function that counts the number of documents.
	Count() AggregateFunction

	// String functions
	// ByteLength creates an expression that calculates the length of a string represented by a field or [Expr] in UTF-8
	// bytes.
	ByteLength() Expr
	// CharLength creates an expression that calculates the character length of a string field or expression in UTF8.
	CharLength() Expr
	// EndsWith creates a boolean expression that checks if the string expression ends with the specified suffix.
	//
	// The parameter 'suffix' can be a string constant or an [Expr] that evaluates to a string.
	EndsWith(suffix any) BooleanExpr
	// Like creates a boolean expression that checks if the string expression matches the specified pattern.
	//
	// The parameter 'suffix' can be a string constant or an [Expr] that evaluates to a string.
	Like(suffix any) BooleanExpr
	// RegexContains creates a boolean expression that checks if the string expression contains a match for the specified regex pattern.
	//
	// The parameter 'pattern' can be a string constant or an [Expr] that evaluates to a string.
	RegexContains(pattern any) BooleanExpr
	// RegexMatch creates a boolean expression that checks if the string expression matches the specified regex pattern.
	//
	// The parameter 'pattern' can be a string constant or an [Expr] that evaluates to a string.
	RegexMatch(pattern any) BooleanExpr
	// StartsWith creates a boolean expression that checks if the string expression starts with the specified prefix.
	//
	// The parameter 'prefix' can be a string constant or an [Expr] that evaluates to a string.
	StartsWith(prefix any) BooleanExpr
	// StringConcat creates an expression that concatenates multiple strings into a single string.
	//
	// The parameter 'otherStrings' can be a mix of string constants or [Expr]s that evaluate to strings.
	StringConcat(otherStrings ...any) Expr
	// StringContains creates a boolean expression that checks if the string expression contains the specified substring.
	//
	// The parameter 'substring' can be a string constant or an [Expr] that evaluates to a string.
	StringContains(substring any) BooleanExpr
	// StringReverse creates an expression that reverses a string.
	StringReverse() Expr
	// Join creates an expression that joins the elements of a string array into a single string.
	//
	// The parameter 'delimiter' can be a string constant or an [Expr] that evaluates to a string.
	Join(delimiter any) Expr
	// Substring creates an expression that returns a substring of a string.
	//
	// The parameter 'index' is the starting index of the substring.
	// It can be an integer constant or an [Expr] that evaluates to an integer.
	// The parameter 'offset' is the length of the substring.
	// It can be an integer constant or an [Expr] that evaluates to an integer.
	Substring(index, offset any) Expr
	// ToLower creates an expression that converts a string to lowercase.
	ToLower() Expr
	// ToUpper creates an expression that converts a string to uppercase.
	ToUpper() Expr
	// Trim creates an expression that removes leading and trailing whitespace from a string.
	Trim() Expr
	// Split creates an expression that splits a string by a delimiter.
	//
	// The parameter 'delimiter' can be a string constant or an [Expr] that evaluates to a string.
	Split(delimiter any) Expr

	// Type creates an expression that returns the type of the expression.
	Type() Expr

	// Vector functions
	// CosineDistance creates an expression that calculates the cosine distance between two vectors.
	//
	// The parameter 'other' can be [Vector32], [Vector64], []float32, []float64 or an [Expr] that evaluates to a vector.
	CosineDistance(other any) Expr
	// DotProduct creates an expression that calculates the dot product of two vectors.
	//
	// The parameter 'other' can be [Vector32], [Vector64], []float32, []float64 or an [Expr] that evaluates to a vector.
	DotProduct(other any) Expr
	// EuclideanDistance creates an expression that calculates the euclidean distance between two vectors.
	//
	// The parameter 'other' can be [Vector32], [Vector64], []float32, []float64 or an [Expr] that evaluates to a vector.
	EuclideanDistance(other any) Expr
	// VectorLength creates an expression that calculates the length of a vector.
	VectorLength() Expr

	// Ordering
	// Ascending creates an ordering expression for ascending order.
	Ascending() Ordering
	// Descending creates an ordering expression for descending order.
	Descending() Ordering

	// As assigns an alias to an expression.
	// Aliases are useful for renaming fields in the output of a stage.
	As(alias string) Selectable
}

// baseExpr provides common methods for all Expr implementations, allowing for method chaining.
type baseExpr struct {
	pbVal *pb.Value
	err   error
}

func (b *baseExpr) isExpr()                     {}
func (b *baseExpr) toProto() (*pb.Value, error) { return b.pbVal, b.err }
func (b *baseExpr) getBaseExpr() *baseExpr      { return b }

// Aritmetic functions
func (b *baseExpr) Add(other any) Expr      { return Add(b, other) }
func (b *baseExpr) Subtract(other any) Expr { return Subtract(b, other) }
func (b *baseExpr) Multiply(other any) Expr { return Multiply(b, other) }
func (b *baseExpr) Divide(other any) Expr   { return Divide(b, other) }
func (b *baseExpr) Abs() Expr               { return Abs(b) }
func (b *baseExpr) Floor() Expr             { return Floor(b) }
func (b *baseExpr) Ceil() Expr              { return Ceil(b) }
func (b *baseExpr) Exp() Expr               { return Exp(b) }
func (b *baseExpr) Log(other any) Expr      { return Log(b, other) }
func (b *baseExpr) Log10() Expr             { return Log10(b) }
func (b *baseExpr) Ln() Expr                { return Ln(b) }
func (b *baseExpr) Mod(other any) Expr      { return Mod(b, other) }
func (b *baseExpr) Pow(other any) Expr      { return Pow(b, other) }
func (b *baseExpr) Round() Expr             { return Round(b) }
func (b *baseExpr) Sqrt() Expr              { return Sqrt(b) }

// Array functions
func (b *baseExpr) ArrayContains(value any) BooleanExpr     { return ArrayContains(b, value) }
func (b *baseExpr) ArrayContainsAll(values any) BooleanExpr { return ArrayContainsAll(b, values) }
func (b *baseExpr) ArrayContainsAny(values any) BooleanExpr { return ArrayContainsAny(b, values) }
func (b *baseExpr) ArrayLength() Expr                       { return ArrayLength(b) }
func (b *baseExpr) EqualAny(values any) BooleanExpr         { return EqualAny(b, values) }
func (b *baseExpr) NotEqualAny(values any) BooleanExpr      { return NotEqualAny(b, values) }
func (b *baseExpr) ArrayGet(offset any) Expr                { return ArrayGet(b, offset) }
func (b *baseExpr) ArrayReverse() Expr                      { return ArrayReverse(b) }
func (b *baseExpr) ArrayConcat(otherArrays ...any) Expr     { return ArrayConcat(b, otherArrays...) }
func (b *baseExpr) ArraySum() Expr                          { return ArraySum(b) }
func (b *baseExpr) ArrayMaximum() Expr                      { return ArrayMaximum(b) }
func (b *baseExpr) ArrayMinimum() Expr                      { return ArrayMinimum(b) }

// Timestamp functions
func (b *baseExpr) TimestampAdd(unit, amount any) Expr { return TimestampAdd(b, unit, amount) }
func (b *baseExpr) TimestampSubtract(unit, amount any) Expr {
	return TimestampSubtract(b, unit, amount)
}
func (b *baseExpr) TimestampTruncate(granularity any) Expr {
	return TimestampTruncate(b, granularity)
}
func (b *baseExpr) TimestampTruncateWithTimezone(granularity any, timezone string) Expr {
	return TimestampTruncateWithTimezone(b, granularity, timezone)
}
func (b *baseExpr) TimestampToUnixMicros() Expr  { return TimestampToUnixMicros(b) }
func (b *baseExpr) TimestampToUnixMillis() Expr  { return TimestampToUnixMillis(b) }
func (b *baseExpr) TimestampToUnixSeconds() Expr { return TimestampToUnixSeconds(b) }
func (b *baseExpr) UnixMicrosToTimestamp() Expr  { return UnixMicrosToTimestamp(b) }
func (b *baseExpr) UnixMillisToTimestamp() Expr  { return UnixMillisToTimestamp(b) }
func (b *baseExpr) UnixSecondsToTimestamp() Expr { return UnixSecondsToTimestamp(b) }

// Comparison functions
func (b *baseExpr) Equal(other any) BooleanExpr              { return Equal(b, other) }
func (b *baseExpr) NotEqual(other any) BooleanExpr           { return NotEqual(b, other) }
func (b *baseExpr) GreaterThan(other any) BooleanExpr        { return GreaterThan(b, other) }
func (b *baseExpr) GreaterThanOrEqual(other any) BooleanExpr { return GreaterThanOrEqual(b, other) }
func (b *baseExpr) LessThan(other any) BooleanExpr           { return LessThan(b, other) }
func (b *baseExpr) LessThanOrEqual(other any) BooleanExpr    { return LessThanOrEqual(b, other) }

// General functions
func (b *baseExpr) Length() Expr              { return Length(b) }
func (b *baseExpr) Reverse() Expr             { return Reverse(b) }
func (b *baseExpr) Concat(others ...any) Expr { return Concat(b, others...) }

// Key functions
func (b *baseExpr) GetCollectionID() Expr { return GetCollectionID(b) }
func (b *baseExpr) GetDocumentID() Expr   { return GetDocumentID(b) }

// Logical functions
func (b *baseExpr) IfError(catchExprOrValue any) Expr  { return IfError(b, catchExprOrValue) }
func (b *baseExpr) IfAbsent(catchExprOrValue any) Expr { return IfAbsent(b, catchExprOrValue) }

// Object functions
func (b *baseExpr) MapGet(strOrExprkey any) Expr { return MapGet(b, strOrExprkey) }
func (b *baseExpr) MapMerge(secondMap Expr, otherMaps ...Expr) Expr {
	return MapMerge(b, secondMap, otherMaps...)
}
func (b *baseExpr) MapRemove(strOrExprkey any) Expr { return MapRemove(b, strOrExprkey) }

// Aggregation operations
func (b *baseExpr) Sum() AggregateFunction           { return Sum(b) }
func (b *baseExpr) Average() AggregateFunction       { return Average(b) }
func (b *baseExpr) Count() AggregateFunction         { return Count(b) }
func (b *baseExpr) CountDistinct() AggregateFunction { return CountDistinct(b) }
func (b *baseExpr) CountIf() AggregateFunction       { return CountIf(b) }
func (b *baseExpr) Maximum() AggregateFunction       { return Maximum(b) }
func (b *baseExpr) Minimum() AggregateFunction       { return Minimum(b) }

// String functions
func (b *baseExpr) ByteLength() Expr                         { return ByteLength(b) }
func (b *baseExpr) CharLength() Expr                         { return CharLength(b) }
func (b *baseExpr) EndsWith(suffix any) BooleanExpr          { return EndsWith(b, suffix) }
func (b *baseExpr) Like(suffix any) BooleanExpr              { return Like(b, suffix) }
func (b *baseExpr) RegexContains(pattern any) BooleanExpr    { return RegexContains(b, pattern) }
func (b *baseExpr) RegexMatch(pattern any) BooleanExpr       { return RegexMatch(b, pattern) }
func (b *baseExpr) StartsWith(prefix any) BooleanExpr        { return StartsWith(b, prefix) }
func (b *baseExpr) StringConcat(otherStrings ...any) Expr    { return StringConcat(b, otherStrings...) }
func (b *baseExpr) StringContains(substring any) BooleanExpr { return StringContains(b, substring) }
func (b *baseExpr) StringReverse() Expr                      { return StringReverse(b) }
func (b *baseExpr) Join(delimiter any) Expr                  { return Join(b, delimiter) }
func (b *baseExpr) Substring(index, offset any) Expr         { return Substring(b, index, offset) }
func (b *baseExpr) ToLower() Expr                            { return ToLower(b) }
func (b *baseExpr) ToUpper() Expr                            { return ToUpper(b) }
func (b *baseExpr) Trim() Expr                               { return Trim(b) }
func (b *baseExpr) Split(delimiter any) Expr                 { return Split(b, delimiter) }

// Type functions
func (b *baseExpr) Type() Expr { return Type(b) }

// Vector functions
func (b *baseExpr) CosineDistance(other any) Expr    { return CosineDistance(b, other) }
func (b *baseExpr) DotProduct(other any) Expr        { return DotProduct(b, other) }
func (b *baseExpr) EuclideanDistance(other any) Expr { return EuclideanDistance(b, other) }
func (b *baseExpr) VectorLength() Expr               { return VectorLength(b) }

// Ordering
func (b *baseExpr) Ascending() Ordering  { return Ascending(b) }
func (b *baseExpr) Descending() Ordering { return Descending(b) }

func (b *baseExpr) As(alias string) Selectable {
	return newAliasedExpr(b, alias)
}

// Ensure that baseExpr implements the Expr interface.
var _ Expr = (*baseExpr)(nil)

// AliasedExpr represents an expression with an alias.
// It implements the [Selectable] interface, allowing it to be used in projection stages like `Select` and `AddFields`.
type AliasedExpr struct {
	*baseExpr
	alias string
}

func newAliasedExpr(expr Expr, alias string) *AliasedExpr {
	return &AliasedExpr{baseExpr: expr.getBaseExpr(), alias: alias}
}

// getSelectionDetails returns the alias and the underlying expression for this AliasedExpr.
// This method allows AliasedExpr to satisfy the Selectable interface.
func (e *AliasedExpr) getSelectionDetails() (string, Expr) {
	return e.alias, e.baseExpr
}

func (e *AliasedExpr) isSelectable() {}

// Ensure that AliasedExpr implements the Selectable interface.
var _ Selectable = (*AliasedExpr)(nil)
