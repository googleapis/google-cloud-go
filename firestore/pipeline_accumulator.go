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

import "fmt"

// Accumulator is represents an aggregation function in a pipeline.
type Accumulator interface {
	Function
	isAccumulator()
	As(alias string) *AccumulatorTarget
}

// baseAccumulator provides common methods for all Accumulator implementations.
type baseAccumulator struct {
	*baseFunction
}

func newBaseAccumulator(name string, fieldOrExpr any) *baseAccumulator {
	if fieldOrExpr == nil {
		return &baseAccumulator{baseFunction: newBaseFunction(name, []Expr{}, nil)}
	}

	var valueExpr Expr
	var err error
	switch value := fieldOrExpr.(type) {
	case string:
		valueExpr = FieldOf(value)
	case FieldPath:
		valueExpr = FieldOfPath(value)
	case Expr:
		valueExpr = value
	default:
		err = fmt.Errorf("firestore: invalid type for parameter 'value': expected string, FieldPath, or Expr, but got %T", value)
	}

	return &baseAccumulator{baseFunction: newBaseFunction(name, []Expr{valueExpr}, err)}
}

func (b *baseAccumulator) isAccumulator() {}
func (b *baseAccumulator) As(alias string) *AccumulatorTarget {
	return &AccumulatorTarget{baseAccumulator: b, alias: alias}
}

// Ensure that baseAccumulator implements the Accumulator interface.
var _ Accumulator = (*baseAccumulator)(nil)

// AccumulatorTarget is an aliased [Accumulator].
type AccumulatorTarget struct {
	*baseAccumulator
	alias string
}

func (a *AccumulatorTarget) getSelectionDetails() (string, Expr) {
	return a.alias, a.baseAccumulator
}

// SumAccumulator is the result of a Sum aggregation.
type SumAccumulator struct {
	*baseAccumulator
}

// Sum creates an aggregation that calculates the sum of values from an expression or a field's values
// across multiple stage inputs.
//
// Example:
//
//		// Calculate the total revenue from a set of orders
//		Sum(FieldOf("orderAmount")).As("totalRevenue")
//	 	Sum("orderAmount").As("totalRevenue")
func Sum(fieldOrExpr any) *SumAccumulator {
	return &SumAccumulator{baseAccumulator: newBaseAccumulator("sum", fieldOrExpr)}
}

// AvgAccumulator is the result of an Avg aggregation.
type AvgAccumulator struct {
	*baseAccumulator
}

// Avg creates an aggregation that calculates the average (mean) of values from an expression or a field's values
// across multiple stage inputs.
// fieldOrExpr can be a field path string, [FieldPath] or [Expr]
// Example:
//
//		// Calculate the average age of users
//		Avg(FieldOf("info.age")).As("averageAge")
//		Avg(FieldOfPath("info.age")).As("averageAge")
//	    Avg("info.age").As("averageAge")
//	    Avg(FieldPath([]string{"info", "age"})).As("averageAge")
func Avg(fieldOrExpr any) *AvgAccumulator {
	return &AvgAccumulator{baseAccumulator: newBaseAccumulator("avg", fieldOrExpr)}
}

// CountAccumulator is the result of a Count aggregation.
type CountAccumulator struct {
	*baseAccumulator
}

// Count creates an aggregation that counts the number of stage inputs with valid evaluations of the
// provided field or expression.
// fieldOrExpr can be a field path string, [FieldPath] or [Expr]
// Example:
//
//		// Count the number of items where the price is greater than 10
//		Count(FieldOf("price").Gt(10)).As("expensiveItemCount")
//	    // Count the total number of products
//		Count("productId").As("totalProducts")
func Count(fieldOrExpr any) *CountAccumulator {
	return &CountAccumulator{baseAccumulator: newBaseAccumulator("count", fieldOrExpr)}
}

// CountAll creates an aggregation that counts the total number of stage inputs.
//
// Example:
//
//		// Count the total number of users
//	    CountAll().As("totalUsers")
func CountAll() *CountAccumulator {
	return &CountAccumulator{baseAccumulator: newBaseAccumulator("count", nil)}
}
