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
	"context"
	"fmt"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
)

// Pipeline class provides a flexible and expressive framework for building complex data
// transformation and query pipelines for Firestore.
//
// A pipeline takes data sources, such as Firestore collections or collection groups, and applies
// a series of stages that are chained together. Each stage takes the output from the previous stage
// (or the data source) and produces an output for the next stage (or as the final output of the
// pipeline).
//
// Expressions can be used within
// each stages to filter and transform data through the stage.
//
// NOTE: The chained stages do not prescribe exactly how Firestore will execute the pipeline.
// Instead, Firestore only guarantees that the result is the same as if the chained stages were
// executed in order.
type Pipeline struct {
	c      *Client
	stages []pipelineStage
	err    error
}

func newPipeline(client *Client, initialStage pipelineStage) *Pipeline {
	return &Pipeline{
		c:      client,
		stages: []pipelineStage{initialStage},
	}
}

// Execute executes the pipeline and returns an iterator for streaming the results.
// TODO: Accept PipelineOptions
func (p *Pipeline) Execute(ctx context.Context) *PipelineResultIterator {
	return &PipelineResultIterator{
		iter: newStreamPipelineResultIterator(withResourceHeader(ctx, p.c.path()), p),
	}
}

func (p *Pipeline) toExecutePipelineRequest() (*pb.ExecutePipelineRequest, error) {
	if p.err != nil {
		return nil, p.err
	}
	protoStages := make([]*pb.Pipeline_Stage, len(p.stages))
	for i, s := range p.stages {
		ps, err := s.toProto()
		if err != nil {
			return nil, fmt.Errorf("firestore: error converting stage %q to proto: %w", s.name(), err)
		}
		protoStages[i] = ps
	}

	req := &pb.ExecutePipelineRequest{
		Database: p.c.path(),
		PipelineType: &pb.ExecutePipelineRequest_StructuredPipeline{
			StructuredPipeline: &pb.StructuredPipeline{
				Pipeline: &pb.Pipeline{
					Stages: protoStages,
				},
			},
		},
		// TODO: Add consistencyselector
	}

	return req, nil
}

// append creates a new Pipeline by adding a stage to the current one.
func (p *Pipeline) append(s pipelineStage) *Pipeline {
	if p.err != nil {
		return p
	}
	newP := &Pipeline{
		c:      p.c,
		stages: make([]pipelineStage, len(p.stages)+1),
	}
	copy(newP.stages, p.stages)
	newP.stages[len(p.stages)] = s
	return newP
}

// Limit limits the maximum number of documents returned by previous stages.
func (p *Pipeline) Limit(limit int) *Pipeline {
	return p.append(newLimitStage(limit))
}

// Select selects or creates a set of fields from the outputs of previous stages.
// The selected fields are defined using field path string, [FieldPath] or [Selectable] expressions.
// [Selectable] expressions can be:
//   - Field: References an existing field.
//   - Function: Represents the result of a function with an assigned alias name using [Function.As].
//
// Example:
//
//		client.Pipeline().Collection("users").Select("info.email")
//		client.Pipeline().Collection("users").Select(FieldOf("info.email"))
//		client.Pipeline().Collection("users").Select(FieldOfPath([]string{"info", "email"}))
//		client.Pipeline().Collection("users").Select(FieldOfPath([]string{"info", "email"}))
//	 	client.Pipeline().Collection("users").Select(Add("age", 5).As("agePlus5"))
func (p *Pipeline) Select(fieldsOrSelectables ...any) *Pipeline {
	if p.err != nil {
		return p
	}
	stage, err := newSelectStage(fieldsOrSelectables...)
	if err != nil {
		p.err = err
		return p
	}
	return p.append(stage)
}

// AddFields adds new fields to outputs from previous stages.
//
// This stage allows you to compute values on-the-fly based on existing data from previous
// stages or constants. You can use this to create new fields or overwrite existing ones (if there
// is name overlaps).
//
// The added fields are defined using [Selectable]s
func (p *Pipeline) AddFields(selectables ...Selectable) *Pipeline {
	if p.err != nil {
		return p
	}
	stage, err := newAddFieldsStage(selectables...)
	if err != nil {
		p.err = err
		return p
	}
	return p.append(stage)
}

// Where filters the documents from previous stages to only include those matching the specified [FilterCondition].
//
// This stage allows you to apply conditions to the data, similar to a "WHERE" clause in SQL.
func (p *Pipeline) Where(condition FilterCondition) *Pipeline {
	if p.err != nil {
		return p
	}
	stage, err := newWhereStage(condition)
	if err != nil {
		p.err = err
		return p
	}
	return p.append(stage)
}

// AggregateSpec is used to perform aggregation operations.
type AggregateSpec struct {
	groups     []Selectable
	accTargets []*AccumulatorTarget
	err        error
}

// NewAggregateSpec creates a new AggregateSpec with the given accumulator targets.
func NewAggregateSpec(accumulators ...*AccumulatorTarget) *AggregateSpec {
	return &AggregateSpec{accTargets: accumulators}
}

// WithGroups sets the grouping keys for the aggregation.
func (a *AggregateSpec) WithGroups(fieldsOrSelectables ...any) *AggregateSpec {
	a.groups, a.err = fieldsOrSelectablesToSelectables(fieldsOrSelectables...)
	return a
}

// Aggregate performs aggregation operations on the documents from previous stages.
// This stage allows you to calculate aggregate values over a set of documents. You define the
// aggregations to perform using [AccumulatorTarget] expressions which are typically results of
// calling [AccumulatorTarget.As] on [AccumulatorTarget] instances.
// Example:
//
//	client.Pipeline().Collection("users").
//		Aggregate(Sum("age").As("age_sum"))
func (p *Pipeline) Aggregate(accumulators ...*AccumulatorTarget) *Pipeline {
	a := NewAggregateSpec(accumulators...)
	aggStage, err := newAggregateStage(a)
	if err != nil {
		p.err = err
		return p
	}
	return p.append(aggStage)
}

// AggregateWithSpec performs optionally grouped aggregation operations on the documents from previous stages.
// This stage allows you to calculate aggregate values over a set of documents, optionally
// grouped by one or more fields or functions. You can specify:
//   - Grouping Fields or Functions: One or more fields or functions to group the documents
//     by. For each distinct combination of values in these fields, a separate group is created.
//     If no grouping fields are provided, a single group containing all documents is used. Not
//     specifying groups is the same as putting the entire inputs into one group.
//   - Accumulator targets: One or more accumulation operations to perform within each group. These
//     are defined using [AccumulatorTarget] expressions which are typically results of calling
//     [AccumulatorTarget.As] on [AccumulatorTarget] instances. Each aggregation
//     calculates a value (e.g., sum, average, count) based on the documents within its group.
//
// Example:
//
//		// Calculate the average rating for each genre.
//		client.Pipeline().Collection("books").
//	        AggregateWithSpec(NewAggregateSpec(Avg("rating").As("avg_rating")).WithGroups("genre"))
func (p *Pipeline) AggregateWithSpec(spec *AggregateSpec) *Pipeline {
	aggStage, err := newAggregateStage(spec)
	if err != nil {
		p.err = err
		return p
	}
	return p.append(aggStage)
}
