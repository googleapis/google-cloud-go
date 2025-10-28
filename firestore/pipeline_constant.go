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
	"fmt"
	"reflect"
	"time"

	"google.golang.org/genproto/googleapis/type/latlng"
	ts "google.golang.org/protobuf/types/known/timestamppb"
)

// constant represents a constant value that can be used in a Firestore pipeline expression.
// It implements the [Expr] interface.
type constant struct {
	*baseExpr
}

// ConstantOf creates a new constant [Expr] from a Go value.
func ConstantOf(value any) Expr {
	if value == nil {
		return ConstantOfNull()
	}

	switch value := value.(type) {
	case *constant: // If it's already our private constant type
		return value
	case Expr:
		// If it's already an Expr that isn't *constant, we create a new constant from it if possible.
		// This path is primarily for if a user passes, e.g., a function result to ConstantOf.
		// if it's not *constant, we fall through to scalar type checking.
		break
	}

	// Handle known scalar types
	switch value.(type) {
	case bool, string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, float32, float64, time.Time, *ts.Timestamp, []byte, Vector32, Vector64, *latlng.LatLng:
		pbVal, _, err := toProtoValue(reflect.ValueOf(value))
		if err != nil {
			return &constant{baseExpr: &baseExpr{err: err}}
		}
		return &constant{baseExpr: &baseExpr{pbVal: pbVal}}
	default:
		return &constant{baseExpr: &baseExpr{err: fmt.Errorf("firestore: unknown constant type: %T", value)}}
	}
}

// ConstantOfNull creates a new constant [Expr] representing a null value.
func ConstantOfNull() Expr {
	pbVal, _, err := toProtoValue(reflect.ValueOf(nil))
	return &constant{baseExpr: &baseExpr{pbVal: pbVal, err: err}}
}

// ConstantOfVector32 creates a new [Vector32] constant [Expr] from a slice of float32s.
func ConstantOfVector32(value []float32) Expr {
	return ConstantOf(Vector32(value))
}

// ConstantOfVector64 creates a new [Vector64] constant [Expr] from a slice of float64s.
func ConstantOfVector64(value []float64) Expr {
	return ConstantOf(Vector64(value))
}
