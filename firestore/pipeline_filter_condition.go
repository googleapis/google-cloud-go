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

type EqCondition struct {
	baseFilterCondition
}

func Eq(left, right Expr) *EqCondition {
	return &EqCondition{baseFilterCondition: baseFilterCondition{baseFunction: leftRightToBaseFunction("eq", left, right)}}
}
