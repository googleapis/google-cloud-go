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
	"reflect"
)

// Constant represents a constant value that can be used in a Firestore pipeline expression.
type Constant struct {
	*baseExpr
}

func newConstant(val any) (*Constant, error) {
	pbVal, _, err := toProtoValue(reflect.ValueOf(val))
	if err != nil {
		return nil, err
	}

	return &Constant{baseExpr: &baseExpr{pbVal: pbVal}}, nil
}

// As assigns an alias to Constant.
// Aliases are useful for renaming fields in the output of a stage.
func (c *Constant) As(alias string) Selectable {
	return newExprWithAlias(c, alias)
}

// ConstantOf creates a new Constant expression from a Go value.
func ConstantOf(value any) *Constant {
	c, err := newConstant(value)
	if err != nil {
		return &Constant{baseExpr: &baseExpr{err: err}}
	}
	return c
}

// ConstantOfNull creates a new Constant expression representing a null value.
func ConstantOfNull() *Constant {
	return &Constant{baseExpr: &baseExpr{}}
}

// ConstantOfVector32 creates a new [Vector32] Constant expression from a slice of float32s.
func ConstantOfVector32(value []float32) *Constant {
	return ConstantOf(Vector32(value))
}

// ConstantOfVector64 creates a new [Vector64] Constant expression from a slice of flot64s.
func ConstantOfVector64(value []float64) *Constant {
	return ConstantOf(Vector64(value))
}
