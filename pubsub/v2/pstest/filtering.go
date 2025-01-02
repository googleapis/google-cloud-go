// Copyright 2024 Google LLC
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

package pstest

import (
	"go.einride.tech/aip/filtering"
	"go.einride.tech/aip/filtering/exprs"
	expr "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

const (
	attributesStr = "attributes"
	hasPrefixStr  = "hasPrefix"
)

// ValidateFilter validates if the filter string is parsable.
func ValidateFilter(filter string) error {
	_, err := parseFilter(filter)
	return err
}

// parseFilter validates a filter string and returns a Filter.
func parseFilter(filter string) (filtering.Filter, error) {
	request := request{filter}

	// Declare the functions and identifiers that are allowed in the filter.
	declarations, err := filtering.NewDeclarations(
		filtering.DeclareFunction(
			hasPrefixStr,
			filtering.NewFunctionOverload(hasPrefixStr,
				filtering.TypeBool,
				filtering.TypeString,
				filtering.TypeString,
			),
		),
		filtering.DeclareIdent(
			attributesStr,
			filtering.TypeMap(
				filtering.TypeString,
				filtering.TypeString,
			),
		),
		filtering.DeclareStandardFunctions(),
	)
	if err != nil {
		return filtering.Filter{}, err
	}
	return filtering.ParseFilter(request, declarations)
}

// request implements filtering.Request.
type request struct {
	filter string
}

func (r request) GetFilter() string {
	return r.filter
}

type messageAttrs map[string]string

// hasKey returns true if the attribute key exists.
func (a messageAttrs) hasKey(key string) bool {
	_, ok := a[key]
	return ok
}

// hasExpectedValue returns true if the attribute key exists and has the given value.
func (a messageAttrs) hasExpectedValue(key, value string) bool {
	v, ok := a[key]
	return ok && v == value
}

// hasPrefix returns true if the attribute key exists and has the given prefix.
func (a messageAttrs) hasPrefix(key, prefix string) bool {
	v, ok := a[key]
	if !ok {
		return false
	}
	return len(v) >= len(prefix) && v[:len(prefix)] == prefix
}

// atomicMatch matches expressions that are not dependent on child expressions.
func atomicMatch(attributes messageAttrs, currExpr *expr.Expr) bool {
	var key, value string
	// Match "=" function.
	// Example: `attributes.name = "com"`
	matcher := exprs.MatchFunction(
		filtering.FunctionEquals,
		exprs.MatchAnyMember(exprs.MatchText(attributesStr), &key),
		exprs.MatchAnyString(&value),
	)
	if matcher(currExpr) {
		return attributes.hasExpectedValue(key, value)
	}

	// Match "!=" function.
	// Example: `attributes.name != "com"`
	matcher = exprs.MatchFunction(
		filtering.FunctionNotEquals,
		exprs.MatchAnyMember(exprs.MatchText(attributesStr), &key),
		exprs.MatchAnyString(&value),
	)
	if matcher(currExpr) {
		return !attributes.hasExpectedValue(key, value)
	}

	// Match ":" function.
	// Example: `attributes:name`
	matcher = exprs.MatchFunction(
		filtering.FunctionHas,
		exprs.MatchText(attributesStr),
		exprs.MatchAnyString(&key),
	)
	if matcher(currExpr) {
		return attributes.hasKey(key)
	}

	// Match "hasPrefix" function.
	// Example: `hasPrefix(attributes.name, "co")`
	matcher = exprs.MatchFunction(
		"hasPrefix",
		exprs.MatchAnyMember(exprs.MatchText(attributesStr), &key),
		exprs.MatchAnyString(&value),
	)
	if matcher(currExpr) {
		return attributes.hasPrefix(key, value)
	}

	return true
}

// compositeMatch matches expressions that are dependent on child expressions.
func compositeMatch(attributes messageAttrs, currExpr *expr.Expr) bool {
	// Match "NOT" function.
	// Example: `NOT attributes:name`
	var e1, e2 *expr.Expr
	matcher := exprs.MatchFunction(
		filtering.FunctionNot,
		exprs.MatchAny(&e1),
	)
	if matcher(currExpr) {
		return !match(attributes, e1)
	}

	// Match "AND" function.
	// Example: `attributes:lang = "en" AND attributes:name`
	matcher = exprs.MatchFunction(
		filtering.FunctionAnd,
		exprs.MatchAny(&e1),
		exprs.MatchAny(&e2),
	)
	if matcher(currExpr) {
		return match(attributes, e1) && match(attributes, e2)
	}

	// Match "OR" function.
	// Example: `attributes:lang = "en" OR attributes:name`
	matcher = exprs.MatchFunction(
		filtering.FunctionOr,
		exprs.MatchAny(&e1),
		exprs.MatchAny(&e2),
	)
	if matcher(currExpr) {
		return match(attributes, e1) || match(attributes, e2)
	}

	return true
}

func match(attributes messageAttrs, currExpr *expr.Expr) bool {
	// atomicMatch first to avoid deep recursion.
	return atomicMatch(attributes, currExpr) && compositeMatch(attributes, currExpr)
}

// getAttrsFunc is a function that returns attributes from an item with any type.
type getAttrsFunc[T any] func(T) messageAttrs

// Make it generic so it's easy to be tested.
//
// Accept a map as input to efficiently delete unmatched items.
func filterByAttrs[T map[K]U, U any, K comparable](items T, filter *filtering.Filter, getAttrs getAttrsFunc[U]) {
	for key, item := range items {
		walkFn := func(currExpr, parentExpr *expr.Expr) bool {
			_, ok := currExpr.ExprKind.(*expr.Expr_CallExpr) // only match call expressions
			if !ok {
				return true
			}
			attrs := getAttrs(item)
			result := match(attrs, currExpr)
			if !result && parentExpr == nil {
				delete(items, key)
			}
			return result
		}
		filtering.Walk(walkFn, filter.CheckedExpr.Expr)
	}
}
