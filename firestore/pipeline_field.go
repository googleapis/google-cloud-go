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
	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
)

// Field represents a reference to a field in a Firestore document, or outputs of a [Pipeline] stage.
//
// Field references are used to access document field values in expressions and to specify fields
// for sorting, filtering, and projecting data in Firestore pipelines.
type Field struct {
	*baseExpr
	fieldPath FieldPath
}

// FieldOf creates a new Field expression from a field path string.
func FieldOf(path string) *Field {
	fieldPath, err := parseDotSeparatedString(path)
	if err != nil {
		return &Field{baseExpr: &baseExpr{err: err}}
	}
	return FieldOfPath(fieldPath)
}

// FieldOfPath creates a new Field expression for the given [FieldPath].
func FieldOfPath(fieldPath FieldPath) *Field {
	if err := fieldPath.validate(); err != nil {
		return &Field{baseExpr: &baseExpr{err: err}}
	}

	pbVal := &pb.Value{
		ValueType: &pb.Value_FieldReferenceValue{
			FieldReferenceValue: fieldPath.toServiceFieldPath(),
		},
	}
	return &Field{fieldPath: fieldPath, baseExpr: &baseExpr{pbVal: pbVal}}
}

// As assigns an alias to Constant.
// Aliases are useful for renaming fields in the output of a stage.
func (f *Field) As(alias string) Selectable {
	return newExprWithAlias(f, alias)
}

func (f *Field) getSelectionDetails() (string, Expr) {
	return f.fieldPath.toServiceFieldPath(), f
}
