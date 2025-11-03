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
	Add(other any) Expr
	Subtract(other any) Expr
	Multiply(other any) Expr
	Divide(other any) Expr
	Abs() Expr
	Floor() Expr
	Ceil() Expr
	Exp() Expr
	Log(other any) Expr
	Log10() Expr
	Ln() Expr
	Mod(other any) Expr
	Pow(other any) Expr
	Round() Expr
	Sqrt() Expr

	// Array operations
	ArrayContains(value any) BooleanExpr
	ArrayContainsAll(values any) BooleanExpr
	ArrayContainsAny(values any) BooleanExpr
	ArrayLength() Expr
	EqualAny(values any) BooleanExpr
	NotEqualAny(values any) BooleanExpr
	ArrayGet(offset any) Expr
	ArrayReverse() Expr
	ArrayConcat(otherArrays ...any) Expr
	ArraySum() Expr
	ArrayMaximum() Expr
	ArrayMinimum() Expr

	// Timestamp operations
	TimestampAdd(unit, amount any) Expr
	TimestampSubtract(unit, amount any) Expr
	TimestampToUnixMicros() Expr
	TimestampToUnixMillis() Expr
	TimestampToUnixSeconds() Expr
	UnixMicrosToTimestamp() Expr
	UnixMillisToTimestamp() Expr
	UnixSecondsToTimestamp() Expr

	// Comparison operations
	Equal(other any) BooleanExpr
	NotEqual(other any) BooleanExpr
	GreaterThan(other any) BooleanExpr
	GreaterThanOrEqual(other any) BooleanExpr
	LessThan(other any) BooleanExpr
	LessThanOrEqual(other any) BooleanExpr

	// General functions
	Length() Expr
	Reverse() Expr
	Concat(others ...any) Expr

	// Key functions
	CollectionId() Expr
	DocumentId() Expr

	// Logical functions
	IfError(catchExprOrValue any) Expr
	IfAbsent(catchExprOrValue any) Expr

	// Object functions
	MapGet(strOrExprkey any) Expr
	MapMerge(secondMap Expr, otherMaps ...Expr) Expr
	MapRemove(strOrExprkey any) Expr

	// Aggregators
	Sum() AggregateFunction
	Average() AggregateFunction
	Count() AggregateFunction

	// String functions
	ByteLength() Expr
	CharLength() Expr
	EndsWith(suffix any) BooleanExpr
	Like(suffix any) BooleanExpr
	RegexContains(pattern any) BooleanExpr
	RegexMatch(pattern any) BooleanExpr
	StartsWith(prefix any) BooleanExpr
	StringConcat(otherStrings ...any) Expr
	StringContains(substring any) BooleanExpr
	StringReverse() Expr
	Join(separator any) Expr
	Substring(index, offset any) Expr
	ToLower() Expr
	ToUpper() Expr
	Trim() Expr

	// Vector functions
	CosineDistance(other any) Expr
	DotProduct(other any) Expr
	EuclideanDistance(other any) Expr
	VectorLength() Expr

	// Ordering
	Ascending() Ordering
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
func (b *baseExpr) CollectionId() Expr { return CollectionID(b) }
func (b *baseExpr) DocumentId() Expr   { return DocumentIDFrom(b) }

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
func (b *baseExpr) Join(separator any) Expr                  { return Join(b, separator) }
func (b *baseExpr) Substring(index, offset any) Expr         { return Substring(b, index, offset) }
func (b *baseExpr) ToLower() Expr                            { return ToLower(b) }
func (b *baseExpr) ToUpper() Expr                            { return ToUpper(b) }
func (b *baseExpr) Trim() Expr                               { return Trim(b) }

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
