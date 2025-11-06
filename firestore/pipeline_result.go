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
	"io"
	"time"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"cloud.google.com/go/internal/trace"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// PipelineResult is a result returned from executing a pipeline.
type PipelineResult struct {
	// Ref is the DocumentRef for this result. It may be nil if the result
	// does not correspond to a specific Firestore document (e.g., an aggregation result
	// without grouping, or a synthetic document from a stage).
	Ref *DocumentRef

	// CreateTime is the time at which the document was created.
	// It may be nil if the result does not correspond to a specific Firestore document
	CreateTime *time.Time

	// UpdateTime is the time at which the document was last changed.
	// It may be nil if the result does not correspond to a specific Firestore document
	UpdateTime *time.Time

	// ExecutionTime is the time at which the document(s) were read.
	ExecutionTime *time.Time

	c     *Client
	proto *pb.Document
}

func newPipelineResult(ref *DocumentRef, proto *pb.Document, c *Client, executionTime *timestamppb.Timestamp) (*PipelineResult, error) {
	pr := &PipelineResult{
		Ref:   ref,
		c:     c,
		proto: proto,
	}
	if proto != nil {
		if proto.GetCreateTime() != nil {
			if err := proto.GetCreateTime().CheckValid(); err != nil {
				return nil, err
			}
			createTime := proto.GetCreateTime().AsTime()
			pr.CreateTime = &createTime
		}
		if proto.GetUpdateTime() != nil {
			if err := proto.GetUpdateTime().CheckValid(); err != nil {
				return nil, err
			}
			updateTime := proto.GetUpdateTime().AsTime()
			pr.UpdateTime = &updateTime
		}
	}
	if executionTime != nil {
		if err := executionTime.CheckValid(); err != nil {
			return nil, err
		}
		execTime := executionTime.AsTime()
		pr.ExecutionTime = &execTime
	}
	return pr, nil
}

// Exists reports whether the PipelineResult represents an  document.
// Even if Exists returns false, the rest of the fields are valid.
func (p *PipelineResult) Exists() bool {
	return p.proto != nil
}

// Data returns the PipelineResult's fields as a map.
// It is equivalent to
//
//	var m map[string]any
//	p.DataTo(&m)
func (p *PipelineResult) Data() map[string]any {
	if p == nil || !p.Exists() {
		return nil
	}
	m, err := createMapFromValueMap(p.proto.Fields, p.c)

	// Any error here is a bug in the client.
	if err != nil {
		panic(fmt.Sprintf("firestore: %v", err))
	}
	return m
}

// DataTo uses the PipelineResult's fields to populate v, which can be a pointer to a
// map[string]any or a pointer to a struct.
// This is similar to [DocumentSnapshot.DataTo]
func (p *PipelineResult) DataTo(v any) error {
	if p == nil || !p.Exists() {
		return status.Errorf(codes.NotFound, "document does not exist")
	}
	return setFromProtoValue(v, &pb.Value{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: p.proto.Fields}}}, p.c)
}

// PipelineResultIterator is an iterator over PipelineResults from a pipeline execution.
type PipelineResultIterator struct {
	iter pipelineResultIteratorInternal
	err  error // Stores sticky error from Next() or construction
}

// Next returns the next result. Its second return value is iterator.Done if there
// are no more results. Once Next returns Done, all subsequent calls will return
// Done.
func (it *PipelineResultIterator) Next() (*PipelineResult, error) {
	if it.err != nil {
		return nil, it.err
	}
	if it.iter == nil { // Iterator was stopped or not initialized
		return nil, iterator.Done
	}

	pr, err := it.iter.next()
	if err != nil {
		it.err = err // Store sticky error
	}
	return pr, err
}

// Stop stops the iterator, freeing its resources.
// Always call Stop when you are done with a DocumentIterator.
// It is not safe to call Stop concurrently with Next.
func (it *PipelineResultIterator) Stop() {
	if it.iter != nil {
		it.iter.stop()
	}
	// Set a sticky error indicating the iterator is now done if not already errored.
	if it.err == nil {
		it.err = iterator.Done
	}
}

// GetAll returns all the documents remaining from the iterator.
// It is not necessary to call Stop on the iterator after calling GetAll.
func (it *PipelineResultIterator) GetAll() ([]*PipelineResult, error) {
	if it.err != nil {
		return nil, it.err
	}
	defer it.Stop()

	var results []*PipelineResult
	for {
		pr, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return results, err
		}
		results = append(results, pr)
	}
	return results, nil
}

// ExplainStats returns stats from query explain.
// If [ExecuteExplainOptions.ExecutionMode] was set to [ExecutionExplainOptionsModeExplain] or left unset, then this returns nil
func (it *PipelineResultIterator) ExplainStats() *ExplainStats {
	if it == nil {
		return &ExplainStats{err: errors.New("firestore: iterator is nil")}
	}
	if it.err == nil || it.err != iterator.Done {
		return &ExplainStats{err: errStatsBeforeEnd}
	}
	statsPb, statsErr := it.iter.getExplainStats()
	return &ExplainStats{statsPb: statsPb, err: statsErr}
}

// ExplainStats is query explain stats.
//
// Contains all metadata related to pipeline planning and execution, specific
// contents depend on the supplied pipeline options.
type ExplainStats struct {
	statsPb *pb.ExplainStats
	err     error
}

// GetRawData returns the explain stats in an encoded proto format, as returned from the Firestore backend.
// The caller is responsible for unpacking this proto message.
func (es *ExplainStats) GetRawData() (*anypb.Any, error) {
	if es.err != nil {
		return nil, es.err
	}
	if es.statsPb == nil {
		return nil, nil
	}

	return es.statsPb.GetData(), nil
}

// GetText returns the explain stats string verbatim as returned from the Firestore backend
// when explain stats were requested with `outputFormat = 'text'`, this
// If explain stats were requested with `outputFormat = 'json'`, this returns the explain stats
// as stringified JSON, which was returned from the Firestore backend.
func (es *ExplainStats) GetText() (string, error) {
	if es.err != nil {
		return "", es.err
	}
	if es.statsPb == nil || es.statsPb.GetData() == nil {
		return "", nil
	}

	var data wrapperspb.StringValue
	if err := es.statsPb.GetData().UnmarshalTo(&data); err != nil {
		return "", fmt.Errorf("firestore: failed to unmarshal Any to wrapperspb.StringValue: %w", err)
	}

	return data.GetValue(), nil
}

// pipelineResultIteratorInternal is an unexported interface defining the core iteration logic.
type pipelineResultIteratorInternal interface {
	next() (*PipelineResult, error)
	stop()
	getExplainStats() (*pb.ExplainStats, error)
}

// streamPipelineResultIterator is the concrete implementation for gRPC streaming of pipeline results.
type streamPipelineResultIterator struct {
	ctx                context.Context
	cancel             func()
	p                  *Pipeline
	streamClient       pb.Firestore_ExecutePipelineClient
	currResp           *pb.ExecutePipelineResponse
	currRespResultsIdx int
	statsPb            *pb.ExplainStats
}

// Ensure that streamPipelineResultIterator implements the pipelineResultIteratorInternal interface.
var _ pipelineResultIteratorInternal = (*streamPipelineResultIterator)(nil)

func newStreamPipelineResultIterator(ctx context.Context, p *Pipeline) *streamPipelineResultIterator {
	ctx, cancel := context.WithCancel(ctx)
	return &streamPipelineResultIterator{
		ctx:    ctx,
		cancel: cancel,
		p:      p,
	}
}

// Each ExecutePipelineResponse received from Firestore service contains a list of Documents
// On each next() call, return a single document.
func (it *streamPipelineResultIterator) next() (_ *PipelineResult, err error) {
	client := it.p.c

	// streamClient is initialized on first next call
	if it.streamClient == nil {
		it.ctx = trace.StartSpan(it.ctx, "cloud.google.com/go/firestore.ExecutePipeline")
		defer func() {
			if errors.Is(err, iterator.Done) {
				trace.EndSpan(it.ctx, nil)
			} else {
				trace.EndSpan(it.ctx, err)
			}
		}()
		req, err := it.p.toExecutePipelineRequest()
		if err != nil {
			return nil, err
		}

		ctx := withRequestParamsHeader(it.ctx, reqParamsHeaderVal(client.path()))
		ctx = withResourceHeader(ctx, client.path())

		it.streamClient, err = client.c.ExecutePipeline(ctx, req)
		if err != nil {
			return nil, err
		}
	}

	// If the current response is nil or all its results have been processed,
	// receive the next response from the stream.
	if it.currResp == nil || it.currRespResultsIdx >= len(it.currResp.GetResults()) {
		var res *pb.ExecutePipelineResponse
		for {
			res, err = it.streamClient.Recv()
			if err == io.EOF {
				return nil, iterator.Done
			}
			if err != nil {
				return nil, err
			}
			if res.GetResults() != nil {
				it.currResp = res
				it.currRespResultsIdx = 0
				it.statsPb = res.GetExplainStats()
				break
			}
			// No results => partial progress; keep receiving
		}
	}

	// Get the next document proto from the current response.
	docProto := it.currResp.GetResults()[it.currRespResultsIdx]
	it.currRespResultsIdx++

	var docRef *DocumentRef
	if len(docProto.GetName()) != 0 {
		var pathErr error
		docRef, pathErr = pathToDoc(docProto.GetName(), client)
		if pathErr != nil {
			return nil, pathErr
		}
	}

	pr, err := newPipelineResult(docRef, docProto, client, it.currResp.GetExecutionTime())
	if err != nil {
		return nil, err
	}
	return pr, nil
}

func (it *streamPipelineResultIterator) stop() {
	it.cancel()
}

func (it *streamPipelineResultIterator) getExplainStats() (*pb.ExplainStats, error) {
	if it == nil {
		return nil, fmt.Errorf("firestore: iterator is nil")
	}
	return it.statsPb, nil
}
