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
	Equivalent(other any) BooleanExpr

	// Aggregators
	Sum() AggregateFunction
	Average() AggregateFunction
	Count() AggregateFunction

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
func (b *baseExpr) Equivalent(other any) BooleanExpr         { return Equivalent(b, other) }

// Aggregation operations
func (b *baseExpr) Sum() AggregateFunction           { return Sum(b) }
func (b *baseExpr) Average() AggregateFunction       { return Average(b) }
func (b *baseExpr) Count() AggregateFunction         { return Count(b) }
func (b *baseExpr) CountDistinct() AggregateFunction { return CountDistinct(b) }
func (b *baseExpr) CountIf() AggregateFunction       { return CountIf(b) }
func (b *baseExpr) Maximum() AggregateFunction       { return Maximum(b) }
func (b *baseExpr) Minimum() AggregateFunction       { return Minimum(b) }

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
