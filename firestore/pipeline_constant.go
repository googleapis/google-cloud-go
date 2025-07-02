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

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
)

// Constant represents a literal value in an expression.
type Constant struct {
	baseExpr
	val *pb.Value
	err error
}

// ConstantOf creates a Constant from a literal Go value.
// val can be string, int
// TODO(bhshkh): Ensure this handles null values. Check whether structs should be allowed
func ConstantOf(val any) *Constant {
	c, err := constantOf(val)
	if err != nil {
		return &Constant{err: err}
	}
	return c
}

func constantOf(val any) (*Constant, error) {
	protoVal, _, err := toProtoValue(reflect.ValueOf(val))
	if err != nil {
		return nil, err
	}
	return (&Constant{val: protoVal}), nil
}
func (c *Constant) toArgumentProto() (*pb.Value, error) {
	return c.val, c.err
}

func (c *Constant) As(alias string) Selectable { return newExprWithAlias(c, alias) }
