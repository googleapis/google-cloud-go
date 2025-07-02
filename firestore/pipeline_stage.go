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
	"strings"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
)

// internal interface for pipeline stages.
type pipelineStage interface {
	toProto() (*pb.Pipeline_Stage, error)
	name() string // For identification, logging, and potential validation
}

// inputStageCollection returns all documents from the entire collection.
type inputStageCollection struct {
	path string
}

func newInputStageCollection(path string) *inputStageCollection {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return &inputStageCollection{path: path}
}
func (s *inputStageCollection) name() string { return "collection" }
func (s *inputStageCollection) toProto() (*pb.Pipeline_Stage, error) {
	arg := &pb.Value{ValueType: &pb.Value_ReferenceValue{ReferenceValue: s.path}}
	return &pb.Pipeline_Stage{
		Name: s.name(),
		Args: []*pb.Value{arg},
	}, nil
}

// inputStageCollection returns all documents from the entire collection.
type inputStageCollectionGroup struct {
	collectionID string
	ancestor     string
}

func newInputStageCollectionGroup(ancestor, collectionID string) *inputStageCollectionGroup {
	return &inputStageCollectionGroup{ancestor: ancestor, collectionID: collectionID}
}
func (s *inputStageCollectionGroup) name() string { return "collection_group" }
func (s *inputStageCollectionGroup) toProto() (*pb.Pipeline_Stage, error) {
	ancestor := &pb.Value{ValueType: &pb.Value_ReferenceValue{ReferenceValue: s.ancestor}}
	collectionID := &pb.Value{ValueType: &pb.Value_StringValue{StringValue: s.collectionID}}
	return &pb.Pipeline_Stage{
		Name: s.name(),
		Args: []*pb.Value{ancestor, collectionID},
	}, nil
}

// inputStageDatabase returns all documents from the entire database.
type inputStageDatabase struct{}

func newInputStageDatabase() *inputStageDatabase {
	return &inputStageDatabase{}
}
func (s *inputStageDatabase) name() string { return "database" }
func (s *inputStageDatabase) toProto() (*pb.Pipeline_Stage, error) {
	return &pb.Pipeline_Stage{
		Name: s.name(),
	}, nil
}

type limitStage struct {
	limit int
}

func newLimitStage(limit int) *limitStage {
	return &limitStage{limit: limit}
}
func (s *limitStage) name() string { return "limit" }
func (s *limitStage) toProto() (*pb.Pipeline_Stage, error) {
	arg := &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: int64(s.limit)}}
	return &pb.Pipeline_Stage{
		Name: s.name(),
		Args: []*pb.Value{arg},
	}, nil
}

// processSelectablesToExprMap is a shared helper function that converts a slice of
// Selectable items into a map of alias->expression.
func processSelectablesToExprMap(selectables ...Selectable) (map[string]Expr, error) {
	if len(selectables) == 0 {
		return map[string]Expr{}, nil // An empty slice is valid, results in an empty map.
	}

	fields := make(map[string]Expr, len(selectables))
	for _, s := range selectables {
		alias, expr, err := s.getSelectionDetails()
		if err != nil {
			return nil, err
		}
		if _, exists := fields[alias]; exists {
			return nil, fmt.Errorf("firestore: duplicate alias or field name %q in selectables", alias)
		}
		fields[alias] = expr
	}
	return fields, nil
}

// selectStage is the internal representation of a Select stage.
type selectStage struct {
	projections map[string]Expr
}

// newSelectStage is the unexported constructor for a selectStage.
func newSelectStage(selectables ...Selectable) (*selectStage, error) {
	projections, err := processSelectablesToExprMap(selectables...)
	if err != nil {
		return nil, err
	}
	return &selectStage{projections: projections}, nil
}

func (s *selectStage) name() string { return "select" }

func (s *selectStage) toProto() (*pb.Pipeline_Stage, error) {
	fieldsProto := make(map[string]*pb.Value, len(s.projections))
	for alias, expr := range s.projections {
		protoVal, err := expr.toArgumentProto()
		if err != nil {
			return nil, fmt.Errorf("error processing expression for alias %q in Select stage: %w", alias, err)
		}
		fieldsProto[alias] = protoVal
	}

	arg := &pb.Value{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: fieldsProto}}}
	return &pb.Pipeline_Stage{
		Name: s.name(),
		Args: []*pb.Value{arg},
	}, nil
}

// addFieldsStage is the internal representation of a AddFields stage.
type addFieldsStage struct {
	fields map[string]Expr
}

// newAddFieldsStage is the unexported constructor for a addFieldsStage.
func newAddFieldsStage(selectables ...Selectable) (*addFieldsStage, error) {
	fields, err := processSelectablesToExprMap(selectables...)
	if err != nil {
		return nil, err
	}
	return &addFieldsStage{fields: fields}, nil
}

func (s *addFieldsStage) name() string { return "add_fields" }

func (s *addFieldsStage) toProto() (*pb.Pipeline_Stage, error) {
	fieldsProto := make(map[string]*pb.Value, len(s.fields))
	for alias, expr := range s.fields {
		protoVal, err := expr.toArgumentProto()
		if err != nil {
			return nil, fmt.Errorf("error processing expression for alias %q in AddFields stage: %w", alias, err)
		}
		fieldsProto[alias] = protoVal
	}

	arg := &pb.Value{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: fieldsProto}}}
	return &pb.Pipeline_Stage{
		Name: s.name(),
		Args: []*pb.Value{arg},
	}, nil
}
