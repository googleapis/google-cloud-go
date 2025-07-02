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
	"errors"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
)

// Selectable represents an item that can be used in projection stages like
// `Select` or `AddFields`, or as a group key in `Aggregate`.
type Selectable interface {
	// getSelectionDetails returns the output alias and the underlying expression.
	getSelectionDetails() (alias string, expr Expr, err error)
}

// Expr represents an expression that can be evaluated to a value.
type Expr interface {
	isExpr()
	toArgumentProto() (*pb.Value, error)
	As(alias string) Selectable

	// Arithmetic
	Add(other interface{}) *AddExpr
	Subtract(other interface{}) *SubtractExpr
	Multiply(other interface{}) *MultiplyExpr
	Divide(other interface{}) *DivideExpr
}

// baseExpr is an unexported struct embedded in all expression types to provide
// the fluent chaining methods and the base isExpr() marker method.
type baseExpr struct{ self Expr }

func (b *baseExpr) isExpr()                          {}
func (b *baseExpr) Add(other any) *AddExpr           { return Add(b.self, other) }
func (b *baseExpr) Subtract(other any) *SubtractExpr { return Subtract(b.self, other) }
func (b *baseExpr) Multiply(other any) *MultiplyExpr { return Multiply(b.self, other) }
func (b *baseExpr) Divide(other any) *DivideExpr     { return Divide(b.self, other) }

// ExprWithAlias wraps an expression with an alias. It implements Selectable.
type ExprWithAlias[T Expr] struct {
	expr  T
	alias string
	err   error
}

func newExprWithAlias[T Expr](expr T, alias string) *ExprWithAlias[T] {
	if alias == "" {
		return &ExprWithAlias[T]{expr: expr, err: errors.New("firestore: alias cannot be empty")}
	}
	return &ExprWithAlias[T]{expr: expr, alias: alias}
}

func (ewa *ExprWithAlias[T]) getSelectionDetails() (string, Expr, error) {
	if ewa.err != nil {
		return "", nil, ewa.err
	}
	return ewa.alias, ewa.expr, nil
}
