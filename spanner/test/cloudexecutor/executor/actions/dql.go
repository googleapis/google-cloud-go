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
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ReadActionHandler holds the necessary components required for executorpb.ReadAction.
type ReadActionHandler struct {
	Action        *executorpb.ReadAction
	FlowContext   *ExecutionFlowContext
	OutcomeSender *outputstream.OutcomeSender
}

// ExecuteAction executes a read action request, store the results in the OutcomeSender.
func (h *ReadActionHandler) ExecuteAction(ctx context.Context) error {
	h.FlowContext.mu.Lock()
	defer h.FlowContext.mu.Unlock()
	log.Printf("Executing read %s:\n %v", h.FlowContext.transactionSeed, h.Action)
	action := h.Action
	var err error

	var typeList []*spannerpb.Type
	if action.Index != nil {
		typeList, err = extractTypes(action.GetTable(), action.GetColumn(), h.FlowContext.tableMetadata)
		if err != nil {
			return h.OutcomeSender.FinishWithError(status.Error(codes.InvalidArgument, fmt.Sprintf("Can't extract types from metadata: %s", err)))
		}
	} else {
		typeList, err = h.FlowContext.tableMetadata.GetKeyColumnTypes(action.GetTable())
		if err != nil {
			return h.OutcomeSender.FinishWithError(status.Error(codes.InvalidArgument, fmt.Sprintf("Can't extract types from metadata: %s", err)))
		}
	}

	keySet, err := utility.KeySetProtoToCloudKeySet(action.GetKeys(), typeList)
	if err != nil {
		return h.OutcomeSender.FinishWithError(status.Error(codes.InvalidArgument, fmt.Sprintf("Can't convert rowSet: %s", err)))
	}

	var iter *spanner.RowIterator
	if h.FlowContext.currentActiveTransaction == None {
		return h.OutcomeSender.FinishWithError(status.Error(codes.InvalidArgument, "no active transaction"))
	} else if h.FlowContext.currentActiveTransaction == Batch {
		return h.OutcomeSender.FinishWithError(status.Error(codes.InvalidArgument, "can't execute regular read in a batch transaction"))
	} else if h.FlowContext.currentActiveTransaction == Read {
		txn, err := h.FlowContext.getTransactionForRead()
		if err != nil {
			return h.OutcomeSender.FinishWithError(err)
		}
		h.OutcomeSender.InitForRead(action.GetTable(), action.Index)
		h.FlowContext.numPendingReads++
		if action.Index != nil {
			iter = txn.ReadUsingIndex(ctx, action.GetTable(), action.GetIndex(), keySet, action.GetColumn())
		} else {
			iter = txn.Read(ctx, action.GetTable(), keySet, action.GetColumn())
		}
	} else if h.FlowContext.currentActiveTransaction == ReadWrite {
		txn, err := h.FlowContext.getTransactionForWrite()
		if err != nil {
			return h.OutcomeSender.FinishWithError(err)
		}
		h.OutcomeSender.InitForRead(action.GetTable(), action.Index)
		h.FlowContext.numPendingReads++
		if action.Index != nil {
			iter = txn.ReadUsingIndex(ctx, action.GetTable(), action.GetIndex(), keySet, action.GetColumn())
		} else {
			iter = txn.Read(ctx, action.GetTable(), keySet, action.GetColumn())
		}
	}
	defer iter.Stop()
	log.Printf("Parsing read result %s\n", h.FlowContext.transactionSeed)
	err = processResults(iter, int64(action.GetLimit()), h.OutcomeSender, h.FlowContext)
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

// QueryActionHandler holds the necessary components required for executorpb.QueryAction.
type QueryActionHandler struct {
	Action        *executorpb.QueryAction
	FlowContext   *ExecutionFlowContext
	OutcomeSender *outputstream.OutcomeSender
}

// ExecuteAction executes a query action request, store the results in the OutcomeSender.
func (h *QueryActionHandler) ExecuteAction(ctx context.Context) error {
	h.FlowContext.mu.Lock()
	defer h.FlowContext.mu.Unlock()
	log.Printf("Executing query %s\n %v\n", h.FlowContext.transactionSeed, h.Action)
	stmt, err := utility.BuildQuery(h.Action)
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}

	var iter *spanner.RowIterator
	if h.FlowContext.currentActiveTransaction == None {
		return h.OutcomeSender.FinishWithError(status.Error(codes.InvalidArgument, "no active transaction"))
	} else if h.FlowContext.currentActiveTransaction == Batch {
		return h.OutcomeSender.FinishWithError(status.Error(codes.InvalidArgument, "can't execute regular read in a batch transaction"))
	} else if h.FlowContext.currentActiveTransaction == Read {
		txn, err := h.FlowContext.getTransactionForRead()
		if err != nil {
			return h.OutcomeSender.FinishWithError(err)
		}
		h.OutcomeSender.InitForQuery()
		h.FlowContext.numPendingReads++
		iter = txn.Query(ctx, stmt)
	} else if h.FlowContext.currentActiveTransaction == ReadWrite {
		txn, err := h.FlowContext.getTransactionForWrite()
		if err != nil {
			return h.OutcomeSender.FinishWithError(err)
		}
		h.OutcomeSender.InitForQuery()
		h.FlowContext.numPendingReads++
		iter = txn.Query(ctx, stmt)
	}
	defer iter.Stop()
	log.Printf("Parsing query result %s\n", h.FlowContext.transactionSeed)
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

// processResults processes a ResultSet from a read/query/dml and store the results in the OutcomeSender.
func processResults(iter *spanner.RowIterator, limit int64, outcomeSender *outputstream.OutcomeSender, flowContext *ExecutionFlowContext) error {
	var rowCount int64 = 0
	log.Printf("Iterating result set: %s\n", flowContext.transactionSeed)
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			return nil
		}
		if err != nil {
			return err
		}
		spannerRow, rowType, err := utility.ConvertSpannerRow(row)
		if err != nil {
			return err
		}
		outcomeSender.SetRowType(rowType)
		// outcomeSender.rowType = rowType
		err = outcomeSender.AppendRow(spannerRow)
		if err != nil {
			return err
		}
		rowCount++
		if limit > 0 && rowCount >= limit {
			log.Printf("Stopping at row limit: %d", limit)
			break
		}
	}
	log.Printf("Successfully processed result: %s\n", flowContext.transactionSeed)
	return nil
}

// extractTypes extracts types from given table and columns, while ignoring the child rows.
func extractTypes(table string, cols []string, metadata *utility.TableMetadataHelper) ([]*spannerpb.Type, error) {
	var typeList []*spannerpb.Type
	for _, col := range cols {
		ctype, err := metadata.GetColumnType(table, col)
		if err != nil {
			return nil, err
		}
		typeList = append(typeList, ctype)
	}
	return typeList, nil
}
