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
	"strings"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
)

const (
	stageNameAddFields = "add_fields"
	stageNameSelect    = "select"
	stageNameWhere     = "where"
	stageNameAggregate = "aggregate"
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

type selectStage struct {
	stagePb *pb.Pipeline_Stage
}

func newSelectStage(fieldsOrSelectables ...any) (*selectStage, error) {
	selectables, err := fieldsOrSelectablesToSelectables(fieldsOrSelectables...)
	if err != nil {
		return nil, err
	}

	mapVal, err := projectionsToMapValue(selectables)
	if err != nil {
		return nil, err
	}

	return &selectStage{
		stagePb: &pb.Pipeline_Stage{
			Name: stageNameSelect,
			Args: []*pb.Value{mapVal},
		},
	}, nil
}
func (s *selectStage) name() string                         { return "select" }
func (s *selectStage) toProto() (*pb.Pipeline_Stage, error) { return s.stagePb, nil }

// addFieldsStage is the internal representation of a AddFields stage.
type addFieldsStage struct {
	stagePb *pb.Pipeline_Stage
}

func newAddFieldsStage(selectables ...Selectable) (*addFieldsStage, error) {
	mapVal, err := projectionsToMapValue(selectables)
	if err != nil {
		return nil, err
	}

	return &addFieldsStage{
		stagePb: &pb.Pipeline_Stage{
			Name: stageNameAddFields,
			Args: []*pb.Value{mapVal},
		},
	}, nil
}
func (s *addFieldsStage) name() string                         { return stageNameAddFields }
func (s *addFieldsStage) toProto() (*pb.Pipeline_Stage, error) { return s.stagePb, nil }

type whereStage struct {
	stagePb *pb.Pipeline_Stage
}

func newWhereStage(condition BooleanExpr) (*whereStage, error) {
	argsPb, err := condition.toProto()
	if err != nil {
		return nil, err
	}
	return &whereStage{
		stagePb: &pb.Pipeline_Stage{
			Name: stageNameWhere,
			Args: []*pb.Value{argsPb},
		},
	}, nil
}

func (s *whereStage) name() string                         { return stageNameWhere }
func (s *whereStage) toProto() (*pb.Pipeline_Stage, error) { return s.stagePb, nil }

type aggregateStage struct {
	stagePb *pb.Pipeline_Stage
}

func newAggregateStage(a *AggregateSpec) (*aggregateStage, error) {
	if a.err != nil {
		return nil, a.err
	}
	targetsPb, err := aliasedAggregatesToMapValue(a.accTargets)
	if err != nil {
		return nil, err
	}

	groupsPb, err := projectionsToMapValue(a.groups)
	if err != nil {
		return nil, err
	}

	return &aggregateStage{
		stagePb: &pb.Pipeline_Stage{
			Name: stageNameAggregate,
			Args: []*pb.Value{
				targetsPb,
				groupsPb,
			},
		},
	}, nil
}
func (s *aggregateStage) name() string                         { return stageNameAggregate }
func (s *aggregateStage) toProto() (*pb.Pipeline_Stage, error) { return s.stagePb, nil }
