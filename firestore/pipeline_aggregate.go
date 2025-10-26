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

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
)

// AggregateFunction represents an aggregation function in a pipeline.
type AggregateFunction interface {
	toProto() (*pb.Value, error)
	getBaseAggregateFunction() *baseAggregateFunction
	isAggregateFunction()
	As(alias string) *AliasedAggregate
}

// baseAggregateFunction provides common methods for all AggregateFunction implementations.
type baseAggregateFunction struct {
	pbVal *pb.Value
	err   error
}

func newBaseAggregateFunction(name string, fieldOrExpr any) *baseAggregateFunction {
	var argsPbVals []*pb.Value
	var err error

	if fieldOrExpr != nil {
		var valueExpr Expr
		switch value := fieldOrExpr.(type) {
		case string, FieldPath:
			valueExpr = FieldOf(value)
		case Expr:
			valueExpr = value
		default:
			err = fmt.Errorf("firestore: invalid type for parameter 'value' for %s: expected string, FieldPath, or Expr, but got %T", name, value)
		}

		if err == nil {
			var pbVal *pb.Value
			pbVal, err = valueExpr.toProto()
			if err == nil {
				argsPbVals = append(argsPbVals, pbVal)
			}
		}
	}

	if err != nil {
		return &baseAggregateFunction{err: err}
	}

	pbVal := &pb.Value{ValueType: &pb.Value_FunctionValue{
		FunctionValue: &pb.Function{
			Name: name,
			Args: argsPbVals,
		},
	}}
	return &baseAggregateFunction{pbVal: pbVal}
}

func (b *baseAggregateFunction) toProto() (*pb.Value, error) {
	if b.err != nil {
		return nil, b.err
	}
	return b.pbVal, nil
}

func (b *baseAggregateFunction) getBaseAggregateFunction() *baseAggregateFunction { return b }
func (b *baseAggregateFunction) isAggregateFunction()                             {}
func (b *baseAggregateFunction) As(alias string) *AliasedAggregate {
	return &AliasedAggregate{baseAggregateFunction: b, alias: alias}
}

// Ensure that baseAggregateFunction implements the AggregateFunction interface.
var _ AggregateFunction = (*baseAggregateFunction)(nil)

// AliasedAggregate is an aliased [AggregateFunction].
// It's used to give a name to the result of an aggregation.
type AliasedAggregate struct {
	*baseAggregateFunction
	alias string
}

// Sum creates an aggregation that calculates the sum of values from an expression or a field's values
// across multiple stage inputs.
//
// Example:
//
//		// Calculate the total revenue from a set of orders
//		Sum(FieldOf("orderAmount")).As("totalRevenue") // FieldOf returns Expr
//	 	Sum("orderAmount").As("totalRevenue")          // String implicitly becomes FieldOf(...).As(...)
func Sum(fieldOrExpr any) AggregateFunction {
	return newBaseAggregateFunction("sum", fieldOrExpr)
}

// Average creates an aggregation that calculates the average (mean) of values from an expression or a field's values
// across multiple stage inputs.
// fieldOrExpr can be a field path string, [FieldPath] or [Expr]
// Example:
//
//		// Calculate the average age of users
//		Average(FieldOf("info.age")).As("averageAge")       // FieldOf returns Expr
//		Average(FieldOfPath("info.age")).As("averageAge") // FieldOfPath returns Expr
//	    Average("info.age").As("averageAge")              // String implicitly becomes FieldOf(...).As(...)
//	    Average(FieldPath([]string{"info", "age"})).As("averageAge")
func Average(fieldOrExpr any) AggregateFunction {
	return newBaseAggregateFunction("average", fieldOrExpr)
}

// Count creates an aggregation that counts the number of stage inputs with valid evaluations of the
// provided field or expression.
// fieldOrExpr can be a field path string, [FieldPath] or [Expr]
// Example:
//
//		// Count the number of items where the price is greater than 10
//		Count(FieldOf("price").Gt(10)).As("expensiveItemCount") // FieldOf("price").Gt(10) is a BooleanExpr
//	    // Count the total number of products
//		Count("productId").As("totalProducts")                  // String implicitly becomes FieldOf(...).As(...)
func Count(fieldOrExpr any) AggregateFunction {
	return newBaseAggregateFunction("count", fieldOrExpr)
}

// CountAll creates an aggregation that counts the total number of stage inputs.
//
// Example:
//
//		// Count the total number of users
//	    CountAll().As("totalUsers")
func CountAll() AggregateFunction {
	return newBaseAggregateFunction("count", nil)
}
