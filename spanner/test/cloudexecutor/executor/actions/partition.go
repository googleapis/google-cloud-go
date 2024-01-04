// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package actions

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/executor/apiv1/executorpb"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/outputstream"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/utility"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PartitionReadActionHandler holds the necessary components required for performing partition read action.
type PartitionReadActionHandler struct {
	Action        *executorpb.GenerateDbPartitionsForReadAction
	FlowContext   *ExecutionFlowContext
	OutcomeSender *outputstream.OutcomeSender
}

// ExecuteAction executes action that generates database partitions for the given read.
func (h *PartitionReadActionHandler) ExecuteAction(ctx context.Context) error {
	metadata := &utility.TableMetadataHelper{}
	metadata.InitFromTableMetadata(h.Action.GetTable())

	h.FlowContext.mu.Lock()
	defer h.FlowContext.mu.Unlock()

	h.FlowContext.tableMetadata = metadata
	readAction := h.Action.GetRead()
	var err error

	var typeList []*spannerpb.Type
	if readAction.Index != nil {
		typeList, err = extractTypes(readAction.GetTable(), readAction.GetColumn(), h.FlowContext.tableMetadata)
	} else {
		typeList, err = h.FlowContext.tableMetadata.GetKeyColumnTypes(readAction.GetTable())
	}
	if err != nil {
		return h.OutcomeSender.FinishWithError(status.Error(codes.InvalidArgument, fmt.Sprintf("Can't extract types from metadata: %s", err)))
	}

	keySet, err := utility.KeySetProtoToCloudKeySet(readAction.GetKeys(), typeList)
	if err != nil {
		return h.OutcomeSender.FinishWithError(spanner.ToSpannerError(status.Error(codes.InvalidArgument, fmt.Sprintf("Can't convert rowSet: %s", err))))
	}

	batchTxn, err := h.FlowContext.getBatchTransaction()
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}

	partitionOptions := spanner.PartitionOptions{PartitionBytes: h.Action.GetDesiredBytesPerPartition(), MaxPartitions: h.Action.GetMaxPartitionCount()}
	var partitions []*spanner.Partition
	if readAction.Index != nil {
		partitions, err = batchTxn.PartitionReadUsingIndexWithOptions(ctx, readAction.GetTable(), readAction.GetIndex(), keySet, readAction.GetColumn(), partitionOptions, spanner.ReadOptions{})
	} else {
		partitions, err = batchTxn.PartitionReadWithOptions(ctx, readAction.GetTable(), keySet, readAction.GetColumn(), partitionOptions, spanner.ReadOptions{})
	}
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}
	var batchPartitions []*executorpb.BatchPartition
	for _, part := range partitions {
		partitionInstance, _ := part.MarshalBinary()
		batchPartition := &executorpb.BatchPartition{
			Partition:      partitionInstance,
			PartitionToken: part.GetPartitionToken(),
			Table:          &readAction.Table,
			Index:          readAction.Index,
		}
		batchPartitions = append(batchPartitions, batchPartition)
	}
	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status:      &spb.Status{Code: int32(codes.OK)},
		DbPartition: batchPartitions,
	}
	err = h.OutcomeSender.SendOutcome(spannerActionOutcome)
	if err != nil {
		log.Printf("GenerateDbPartitionsRead failed for %s", h.Action)
		return h.OutcomeSender.FinishWithError(err)
	}
	return err
}

// PartitionQueryActionHandler holds the necessary components required for performing partition query action.
type PartitionQueryActionHandler struct {
	Action        *executorpb.GenerateDbPartitionsForQueryAction
	FlowContext   *ExecutionFlowContext
	OutcomeSender *outputstream.OutcomeSender
}

// ExecuteAction executes action that generates database partitions for the given query.
func (h *PartitionQueryActionHandler) ExecuteAction(ctx context.Context) error {
	h.FlowContext.mu.Lock()
	defer h.FlowContext.mu.Unlock()

	batchTxn, err := h.FlowContext.getBatchTransaction()
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}
	stmt, err := utility.BuildQuery(h.Action.GetQuery())
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}
	partitionOptions := spanner.PartitionOptions{PartitionBytes: h.Action.GetDesiredBytesPerPartition()}
	partitions, err := batchTxn.PartitionQuery(ctx, stmt, partitionOptions)
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}
	var batchPartitions []*executorpb.BatchPartition
	for _, partition := range partitions {
		partitionInstance, err := partition.MarshalBinary()
		if err != nil {
			return h.OutcomeSender.FinishWithError(err)
		}
		batchPartition := &executorpb.BatchPartition{
			Partition:      partitionInstance,
			PartitionToken: partition.GetPartitionToken(),
		}
		batchPartitions = append(batchPartitions, batchPartition)
	}

	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status:      &spb.Status{Code: int32(codes.OK)},
		DbPartition: batchPartitions,
	}
	err = h.OutcomeSender.SendOutcome(spannerActionOutcome)
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}
	return err
}

// ExecutePartition holds the necessary components required for executing partition.
type ExecutePartition struct {
	Action        *executorpb.ExecutePartitionAction
	FlowContext   *ExecutionFlowContext
	OutcomeSender *outputstream.OutcomeSender
}

// ExecuteAction executes a read or query for the given partitions.
func (h *ExecutePartition) ExecuteAction(ctx context.Context) error {
	h.FlowContext.mu.Lock()
	defer h.FlowContext.mu.Unlock()

	batchTxn, err := h.FlowContext.getBatchTransaction()
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}

	partitionBinary := h.Action.GetPartition().GetPartition()
	if partitionBinary == nil || len(partitionBinary) == 0 {
		return h.OutcomeSender.FinishWithError(spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "Invalid batchPartition %s", h.Action)))
	}
	if h.Action.GetPartition().Table != nil {
		h.OutcomeSender.InitForBatchRead(h.Action.GetPartition().GetTable(), h.Action.GetPartition().Index)
	} else {
		h.OutcomeSender.InitForQuery()
	}
	partition := &spanner.Partition{}
	if err = partition.UnmarshalBinary(partitionBinary); err != nil {
		return h.OutcomeSender.FinishWithError(spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "ExecutePartitionAction: deserializing Partition failed %v", err)))
	}
	h.FlowContext.startRead()
	iter := batchTxn.Execute(ctx, partition)
	defer iter.Stop()
	err = processResults(iter, 0, h.OutcomeSender, h.FlowContext)
	if err != nil {
		h.FlowContext.finishRead(status.Code(err))
		if status.Code(err) == codes.Aborted {
			return h.OutcomeSender.FinishWithTransactionRestarted()
		}
		return h.OutcomeSender.FinishWithError(err)
	}
	h.FlowContext.finishRead(codes.OK)
	return h.OutcomeSender.FinishSuccessfully()
}

// PartitionedUpdate holds the necessary components required for performing partitioned update.
type PartitionedUpdate struct {
	Action        *executorpb.PartitionedUpdateAction
	FlowContext   *ExecutionFlowContext
	OutcomeSender *outputstream.OutcomeSender
}

// ExecuteAction executes a partitioned update which runs different partitions in parallel.
func (h *PartitionedUpdate) ExecuteAction(ctx context.Context) error {
	h.FlowContext.mu.Lock()
	defer h.FlowContext.mu.Unlock()

	opts := h.Action.GetOptions()
	stmt := spanner.Statement{SQL: h.Action.GetUpdate().GetSql()}
	count, err := h.FlowContext.DbClient.PartitionedUpdateWithOptions(ctx, stmt, spanner.QueryOptions{
		Priority:   opts.GetRpcPriority(),
		RequestTag: opts.GetTag(),
	})
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}
	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status:          &spb.Status{Code: int32(codes.OK)},
		DmlRowsModified: []int64{count},
	}
	err = h.OutcomeSender.SendOutcome(spannerActionOutcome)
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}
	return err
}
