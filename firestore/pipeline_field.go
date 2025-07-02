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

// Field represents a reference to a field in a document.
type Field struct {
	baseExpr
	path FieldPath
	err  error
}

// FieldOf creates a new Field expression
func FieldOf(path string) *Field {
	fp, err := parseDotSeparatedString(path)
	if err != nil {
		return &Field{err: err}
	}
	return &Field{path: fp}
}

// FieldOfPath creates a new Field expression for the given field path
func FieldOfPath(fieldPath FieldPath) *Field {
	if err := fieldPath.validate(); err != nil {
		return &Field{err: err}
	}
	return &Field{path: fieldPath}
}

func (f *Field) toArgumentProto() (*pb.Value, error) {
	return &pb.Value{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: f.path.toServiceFieldPath()}}, nil
}

func (f *Field) getSelectionDetails() (string, Expr, error) {
	return f.path.toServiceFieldPath(), f, nil
}

// As creates an aliased expression, turning this Field into a Selectable.
func (f *Field) As(alias string) Selectable { return newExprWithAlias(f, alias) }
