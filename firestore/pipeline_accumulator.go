// Copyright 2022 Google LLC
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

type SumAccumulator struct {
	*baseAccumulator
}

// Sum creates an aggregation that calculates the sum of values from an expression across multiple
// stage inputs.
func Sum(fieldOrExpr any) *SumAccumulator {
	return &SumAccumulator{baseAccumulator: newBaseAccumulator("sum", fieldOrExpr)}
}

type AvgAccumulator struct {
	*baseAccumulator
}

// Avg creates an aggregation that calculates the sum of values from an expression across multiple
// stage inputs.
func Avg(fieldOrExpr any) *AvgAccumulator {
	return &AvgAccumulator{baseAccumulator: newBaseAccumulator("avg", fieldOrExpr)}
}

type CountAccumulator struct {
	*baseAccumulator
}

// Avg creates an aggregation that calculates the sum of values from an expression across multiple
// stage inputs.
func Count(fieldOrExpr any) *CountAccumulator {
	return &CountAccumulator{baseAccumulator: newBaseAccumulator("count", fieldOrExpr)}
}

// Avg creates an aggregation that calculates the sum of values from an expression across multiple
// stage inputs.
func CountAll() *CountAccumulator {
	return &CountAccumulator{baseAccumulator: newBaseAccumulator("count", nil)}
}
