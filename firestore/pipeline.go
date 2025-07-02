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
	"errors"
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
		iter: newStreamPipelineResultIterator(ctx, p),
	}
}

func (p *Pipeline) toExecutePipelineRequest() (*pb.ExecutePipelineRequest, error) {
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
// The selected fields are defined using [Selectable] expressions, which can be:
//   - Field: References an existing field.
//   - Function: Represents the result of a function with an assigned alias name using [Function.As].
//
// If no selections are provided, the output of this stage is empty.
func (p *Pipeline) Select(selectables ...Selectable) *Pipeline {
	if p.err != nil {
		return p
	}
	stage, err := newSelectStage(selectables...)
	if err != nil {
		p.err = err
		return p
	}
	return p.append(stage)
}

// SelectPaths provides a convenient way to select a set of fields by their names.
// It is a shorthand for p.Select(FieldOf("field1"), FieldOf("field2"), FieldOf("field3.field4") ...).
// Each path argument can be a single field or a dot-separated sequence of
// fields which do not contain any of the runes "˜*/[]".
func (p *Pipeline) SelectPaths(paths ...string) *Pipeline {
	if p.err != nil {
		return p
	}
	selectables := make([]Selectable, len(paths))
	for i, name := range paths {
		if name == "" {
			p.err = errors.New("firestore: field name in SelectFields cannot be empty")
			return p
		}
		selectables[i] = FieldOf(name)
	}
	return p.Select(selectables...)
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
