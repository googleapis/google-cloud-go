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

import (
	"fmt"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
)

// Function represents Firestore [Pipeline] functions, which can be evaluated within pipeline
// execution.
type Function interface {
	Expr
}

type baseFunction struct {
	*baseExpr
}

// Ensure that baseFunction implements the Function interface.
var _ Function = (*baseFunction)(nil)

func newBaseFunction(name string, params ...Expr) *baseFunction {
	argsPbVals := make([]*pb.Value, len(params))
	for i, param := range params {
		var err error
		argsPbVals[i], err = param.toProto()
		if err != nil {
			return &baseFunction{baseExpr: &baseExpr{err: fmt.Errorf("error converting arg %d for function %q: %w", i, name, err)}}
		}
	}
	pbVal := &pb.Value{ValueType: &pb.Value_FunctionValue{
		FunctionValue: &pb.Function{
			Name: name,
			Args: argsPbVals,
		},
	}}

	return &baseFunction{baseExpr: &baseExpr{pbVal: pbVal}}
}

// As assigns an alias to Function.
// Aliases are useful for renaming fields in the output of a stage or for giving meaningful
// names to calculated values.
func (f *baseFunction) As(alias string) Selectable {
	return newExprWithAlias(f, alias)
}

type AddFunc struct {
	*baseFunction
}

func Add(left, right Expr) *AddFunc {
	return &AddFunc{baseFunction: leftRightToBaseFunction("add", left, right)}
}

type SubtractFunc struct {
	*baseFunction
}

func Subtract(left, right Expr) *SubtractFunc {
	return &SubtractFunc{baseFunction: leftRightToBaseFunction("subtract", left, right)}
}
