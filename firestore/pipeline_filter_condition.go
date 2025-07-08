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

// FilterCondition is an interface that represents a filter condition in a pipeline expression.
type FilterCondition interface {
	Expr // Embed Expr interface
	isFilterCondition()
}

// baseFilterCondition provides common methods for all FilterCondition implementations.
type baseFilterCondition struct {
	*baseFunction // Embed Function to get Expr methods and toProto
}

func (b *baseFilterCondition) isFilterCondition() {}

// Ensure that baseFilterCondition implements the FilterCondition interface.
var _ FilterCondition = (*baseFilterCondition)(nil)

type ArrayContainsCondition struct{ *baseFilterCondition }

func ArrayContains(left, right any) *ArrayContainsCondition {
	return &ArrayContainsCondition{baseFilterCondition: &baseFilterCondition{baseFunction: leftRightToBaseFunction("array_contains", left, right)}}
}

type ArrayContainsAllCondition struct{ *baseFilterCondition }

func ArrayContainsAll(fieldOrExpr any, elements ...any) *ArrayContainsAllCondition {
	exprs, err := toExprList(fieldOrExpr, elements...)
	return &ArrayContainsAllCondition{baseFilterCondition: &baseFilterCondition{baseFunction: newBaseFunction("array_contains_all", exprs, err)}}
}

type ArrayContainsAnyCondition struct{ *baseFilterCondition }

func ArrayContainsAny(fieldOrExpr any, elements ...any) *ArrayContainsAnyCondition {
	exprs, err := toExprList(fieldOrExpr, elements...)
	return &ArrayContainsAnyCondition{baseFilterCondition: &baseFilterCondition{baseFunction: newBaseFunction("array_contains_any", exprs, err)}}
}

type EqCondition struct{ *baseFilterCondition }

func Eq(left, right any) *EqCondition {
	return &EqCondition{baseFilterCondition: &baseFilterCondition{baseFunction: leftRightToBaseFunction("eq", left, right)}}
}

type NeqCondition struct{ *baseFilterCondition }

func Neq(left, right any) *NeqCondition {
	return &NeqCondition{baseFilterCondition: &baseFilterCondition{baseFunction: leftRightToBaseFunction("neq", left, right)}}
}

type GtCondition struct{ *baseFilterCondition }

type LtCondition struct{ *baseFilterCondition }

func Lt(left, right any) *LtCondition {
	return &LtCondition{baseFilterCondition: &baseFilterCondition{baseFunction: leftRightToBaseFunction("lt", left, right)}}
}

type LteCondition struct{ *baseFilterCondition }

func Lte(left, right any) *LteCondition {
	return &LteCondition{baseFilterCondition: &baseFilterCondition{baseFunction: leftRightToBaseFunction("lte", left, right)}}
}

func Gt(left, right any) *GtCondition {
	return &GtCondition{baseFilterCondition: &baseFilterCondition{baseFunction: leftRightToBaseFunction("gt", left, right)}}
}

type GteCondition struct{ *baseFilterCondition }

func Gte(left, right any) *GteCondition {
	return &GteCondition{baseFilterCondition: &baseFilterCondition{baseFunction: leftRightToBaseFunction("gte", left, right)}}
}

func NewExists(expr Expr) *ExistsCondition {
	return &ExistsCondition{baseFilterCondition: &baseFilterCondition{baseFunction: newBaseFunction("exists", []Expr{expr}, nil)}}
}

type LogicalMinCondition struct{ *baseFilterCondition }

func LogicalMin(left, right any) *LogicalMinCondition {
	return &LogicalMinCondition{baseFilterCondition: &baseFilterCondition{baseFunction: leftRightToBaseFunction("logical_min", left, right)}}
}

type LogicalMaxCondition struct{ *baseFilterCondition }

func LogicalMax(left, right any) *LogicalMaxCondition {
	return &LogicalMaxCondition{baseFilterCondition: &baseFilterCondition{baseFunction: leftRightToBaseFunction("logical_max", left, right)}}
}

type ExistsCondition struct{ *baseFilterCondition }

type IsNaNCondition struct{ *baseFilterCondition }

func IsNaN(expr Expr) *IsNaNCondition {
	return &IsNaNCondition{baseFilterCondition: &baseFilterCondition{baseFunction: newBaseFunction("is_nan", []Expr{expr}, nil)}}
}
