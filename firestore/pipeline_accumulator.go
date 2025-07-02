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

// Accumulator is an interface that represents an aggregation function in a pipeline.
type Accumulator interface {
	Function
	isAccumulator()
	As(alias string) *ExprWithAlias
}

// baseAccumulator provides common methods for all Accumulator implementations.
type baseAccumulator struct {
	*baseFunction
}

func (b *baseAccumulator) isAccumulator() {}
func (b *baseAccumulator) As(alias string) *ExprWithAlias {
	return newExprWithAlias(b, alias)
}

// Ensure that baseAccumulator implements the Accumulator interface.
var _ Accumulator = (*baseAccumulator)(nil)

type SumAccumulator struct {
	*baseAccumulator
}

// Creates an aggregation that calculates the sum of values from an expression across multiple
// stage inputs.
func Sum(value Expr) *SumAccumulator {
	return &SumAccumulator{baseAccumulator: &baseAccumulator{baseFunction: newBaseFunction("sum", value)}}
}
