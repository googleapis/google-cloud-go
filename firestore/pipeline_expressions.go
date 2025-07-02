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
	getSelectionDetails() (alias string, expr Expr, err error)
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

	// Aritmetic operations
	Add(other Expr) *AddFunc
	AddValue(other any) *AddFunc
	Subtract(other Expr) *SubtractFunc
	SubtractValue(other any) *SubtractFunc

	// Comparison operations
	Eq(other Expr) *EqCondition
	EqValue(other any) *EqCondition

	// Aggregators
	Sum() *SumAccumulator
}

// baseExpr provides common methods for all Expr implementations, allowing for method chaining.
type baseExpr struct {
	pbVal *pb.Value
	err   error
}

func (b *baseExpr) toProto() (*pb.Value, error) { return b.pbVal, nil }

// Aritmetic operations
func (b *baseExpr) Add(other Expr) *AddFunc               { return Add(b, other) }
func (b *baseExpr) AddValue(other any) *AddFunc           { return Add(b, ConstantOf(other)) }
func (b *baseExpr) Subtract(other Expr) *SubtractFunc     { return Subtract(b, other) }
func (b *baseExpr) SubtractValue(other any) *SubtractFunc { return Subtract(b, ConstantOf(other)) }

// Comparison operations
func (b *baseExpr) Eq(other Expr) *EqCondition     { return Eq(b, other) }
func (b *baseExpr) EqValue(other any) *EqCondition { return Eq(b, ConstantOf(other)) }

func (b *baseExpr) Sum() *SumAccumulator { return Sum(b) }

// Ensure that baseExpr implements the Expr interface.
var _ Expr = (*baseExpr)(nil)

// ExprWithAlias represents an expression with an alias.
type ExprWithAlias struct {
	*baseExpr
	alias string
}

func newExprWithAlias(expr Expr, alias string) *ExprWithAlias {
	return &ExprWithAlias{baseExpr: expr.(*baseExpr), alias: alias}
}

// As creates a new ExprWithAlias with the provided alias.
func (e *ExprWithAlias) As(alias string) Selectable {
	return newExprWithAlias(e.baseExpr, alias)
}

func (ewa *ExprWithAlias) getSelectionDetails() (string, Expr, error) {
	if ewa.err != nil {
		return "", nil, ewa.err
	}
	return ewa.alias, ewa.baseExpr, nil
}

func exprToProtoValue(expr Expr) (*pb.Value, error) {
	if expr == nil {
		return ConstantOfNull().pbVal, nil
	}
	return expr.toProto()
}
