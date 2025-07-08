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

// Selectable is an interface for expressions that can be selected in a pipeline.
type Selectable interface {
	// getSelectionDetails returns the output alias and the underlying expression.
	getSelectionDetails() (alias string, expr Expr)
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
// - Aggregations: Calculate aggregate values (e.g., sum, average) over a set of documents.
//
// The [Expr] interface provides a fluent API for building expressions. You can chain together
// method calls to create complex expressions.
type Expr interface {
	toProto() (*pb.Value, error)
	getBaseExpr() *baseExpr

	// Aritmetic operations
	Add(other any) *AddFunc
	Subtract(other any) *SubtractFunc
	Divide(other any) *DivideFunc
	Multiply(other any) *MultiplyFunc
	Mod(other any) *ModFunc

	// Array operations
	ArrayConcat(elements ...any) *ArrayConcatFunc
	ArrayLength() *ArrayLengthFunc
	ArrayReverse() *ArrayReverseFunc

	// Array conditions
	ArrayContains(element any) *ArrayContainsCondition
	ArrayContainsAll(elements ...any) *ArrayContainsAllCondition
	ArrayContainsAny(elements ...any) *ArrayContainsAnyCondition

	// Comparison operations
	Eq(other any) *EqCondition
	Neq(other any) *NeqCondition
	Lt(other any) *LtCondition
	Lte(other any) *LteCondition
	Gt(other any) *GtCondition
	Gte(other any) *GteCondition

	// Ordering
	Ascending() *Ordering
	Descending() *Ordering

	// Aggregators
	Sum() *SumAccumulator
	Avg() *AvgAccumulator
	Count() *CountAccumulator

	// String
	ByteLength() *ByteLengthFunc
	CharLength() *CharLengthFunc

	// vector
	CosineDistance(other any) *CosineDistanceFunc
}

// baseExpr provides common methods for all Expr implementations, allowing for method chaining.
type baseExpr struct {
	pbVal *pb.Value
	err   error
}

func (b *baseExpr) toProto() (*pb.Value, error) { return b.pbVal, b.err }
func (b *baseExpr) getBaseExpr() *baseExpr      { return b }

// Aritmetic functions
func (b *baseExpr) Add(other any) *AddFunc           { return Add(b, other) }
func (b *baseExpr) Subtract(other any) *SubtractFunc { return Subtract(b, other) }
func (b *baseExpr) Divide(other any) *DivideFunc     { return Divide(b, other) }
func (b *baseExpr) Multiply(other any) *MultiplyFunc { return Multiply(b, other) }
func (b *baseExpr) Mod(other any) *ModFunc           { return Mod(b, other) }

// Array operations
func (b *baseExpr) ArrayConcat(others ...any) *ArrayConcatFunc {
	return ArrayConcat(b, others)
}
func (b *baseExpr) ArrayLength() *ArrayLengthFunc {
	return ArrayLength(b)
}
func (b *baseExpr) ArrayReverse() *ArrayReverseFunc {
	return ArrayReverse(b)
}

// Array functions
func (b *baseExpr) ArrayContains(other any) *ArrayContainsCondition { return ArrayContains(b, other) }
func (b *baseExpr) ArrayContainsAll(others ...any) *ArrayContainsAllCondition {
	return ArrayContainsAll(b, others)
}
func (b *baseExpr) ArrayContainsAny(others ...any) *ArrayContainsAnyCondition {
	return ArrayContainsAny(b, others)
}

// Comparison functions
func (b *baseExpr) Eq(other any) *EqCondition   { return Eq(b, other) }
func (b *baseExpr) Neq(other any) *NeqCondition { return Neq(b, other) }
func (b *baseExpr) Lt(other any) *LtCondition   { return Lt(b, other) }
func (b *baseExpr) Lte(other any) *LteCondition { return Lte(b, other) }
func (b *baseExpr) Gt(other any) *GtCondition   { return Gt(b, other) }
func (b *baseExpr) Gte(other any) *GteCondition { return Gte(b, other) }

// Logical
func (b *baseExpr) Exists() *ExistsCondition                  { return NewExists(b) }
func (b *baseExpr) LogicalMin(other any) *LogicalMinCondition { return LogicalMin(b, other) }
func (b *baseExpr) LogicalMax(other any) *LogicalMaxCondition { return LogicalMax(b, other) }

// type
func (b *baseExpr) IsNaN() *IsNaNCondition { return IsNaN(b) }

// ordering
func (b *baseExpr) Ascending() *Ordering  { return newOrdering(b, OrderingDirectionAscending) }
func (b *baseExpr) Descending() *Ordering { return newOrdering(b, OrderingDirectionDescending) }

// Aggregation operations
func (b *baseExpr) Sum() *SumAccumulator     { return Sum(b) }
func (b *baseExpr) Avg() *AvgAccumulator     { return Avg(b) }
func (b *baseExpr) Count() *CountAccumulator { return Count(b) }

// String
func (b *baseExpr) ByteLength() *ByteLengthFunc                  { return ByteLength(b) }
func (b *baseExpr) CharLength() *CharLengthFunc                  { return CharLength(b) }
func (b *baseExpr) CosineDistance(other any) *CosineDistanceFunc { return CosineDistance(b, other) }

// Ensure that baseExpr implements the Expr interface.
var _ Expr = (*baseExpr)(nil)

// ExprWithAlias represents an expression with an alias.
type ExprWithAlias struct {
	*baseExpr
	alias string
}

func newExprWithAlias(expr Expr, alias string) *ExprWithAlias {
	return &ExprWithAlias{baseExpr: expr.getBaseExpr(), alias: alias}
}

// As creates a new ExprWithAlias with the provided alias.
func (e *ExprWithAlias) As(alias string) Selectable {
	return newExprWithAlias(e.baseExpr, alias)
}

func (ewa *ExprWithAlias) getSelectionDetails() (string, Expr) {
	return ewa.alias, ewa.baseExpr
}

// Ordering represents a field and its direction for sorting.
type Ordering struct {
	expr Expr
	dir  OrderingDirection
}

// OrderingDirection represents the sort direction.
type OrderingDirection int

const (
	OrderingDirectionAscending OrderingDirection = iota
	OrderingDirectionDescending
)

func newOrdering(expr Expr, dir OrderingDirection) *Ordering {
	return &Ordering{expr: expr, dir: dir}
}

// Ascending creates an ascending order for the given expression.
func Ascending(expr Expr) *Ordering {
	return newOrdering(expr, OrderingDirectionAscending)
}

// Descending creates a descending order for the given expression.
func Descending(expr Expr) *Ordering {
	return newOrdering(expr, OrderingDirectionDescending)
}

func (o *Ordering) toProto() (*pb.Value, error) {
	exprProto, err := o.expr.toProto()
	if err != nil {
		return nil, err
	}

	dirStr := ""
	switch o.dir {
	case OrderingDirectionAscending:
		dirStr = "ascending"
	case OrderingDirectionDescending:
		dirStr = "descending"
	default:
		return nil, fmt.Errorf("firestore: unknown ordering direction %v", o.dir)
	}

	return &pb.Value{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: map[string]*pb.Value{
		"direction":  stringToProtoValue(dirStr),
		"expression": exprProto,
	}}}}, nil
}
