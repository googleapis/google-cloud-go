package actions

import (
	"context"
	"log"
	"time"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/output_stream"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/utility"
	executorpb "cloud.google.com/go/spanner/test/cloudexecutor/proto"
	"google.golang.org/api/option"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type StartBatchTxnHandler struct {
	Action        *executorpb.StartBatchTransactionAction
	FlowContext   *ExecutionFlowContext
	OutcomeSender *output_stream.OutcomeSender
	Options       []option.ClientOption
}

// ExecuteAction that starts a batch transaction
func (h *StartBatchTxnHandler) ExecuteAction(ctx context.Context) error {
	log.Printf("starting batch transaction %v", h.Action)
	h.FlowContext.mu.Lock()
	defer h.FlowContext.mu.Unlock()
	if h.FlowContext.isTransactionActiveLocked() {
		return h.OutcomeSender.FinishWithError(spanner.ToSpannerError(status.Error(codes.InvalidArgument, "already in a transaction")))
	}

	if h.FlowContext.Database == "" {
		return h.OutcomeSender.FinishWithError(spanner.ToSpannerError(status.Error(codes.InvalidArgument, "database path must be set for this action")))
	}

	client, err := spanner.NewClient(ctx, h.FlowContext.Database, h.Options...)
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}
	var txn *spanner.BatchReadOnlyTransaction
	if h.Action.GetBatchTxnTime() != nil {
		timestamp := time.Unix(h.Action.GetBatchTxnTime().Seconds, int64(h.Action.GetBatchTxnTime().Nanos))
		txn, err = client.BatchReadOnlyTransaction(ctx, spanner.ReadTimestamp(timestamp))
		if err != nil {
			return h.OutcomeSender.FinishWithError(err)
		}
	} else if h.Action.GetTid() != nil {
		batchTransactionId := spanner.BatchReadOnlyTransactionID{}
		err = batchTransactionId.UnmarshalBinary(h.Action.GetTid())
		if err != nil {
			return h.OutcomeSender.FinishWithError(err)
		}
		txn = client.BatchReadOnlyTransactionFromID(batchTransactionId)
	} else {
		return h.OutcomeSender.FinishWithError(spanner.ToSpannerError(status.Error(codes.InvalidArgument, "Either timestamp or tid must be set")))
	}

	h.FlowContext.batchTxn = txn
	h.FlowContext.currentActiveTransaction = Batch
	h.FlowContext.initReadState()
	batchTxnIdMarshal, err := txn.ID.MarshalBinary()
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}
	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status:     &spb.Status{Code: int32(codes.OK)},
		BatchTxnId: batchTxnIdMarshal,
	}
	return h.OutcomeSender.SendOutcome(spannerActionOutcome)
}

type BatchDmlHandler struct {
	Action        *executorpb.BatchDmlAction
	FlowContext   *ExecutionFlowContext
	OutcomeSender *output_stream.OutcomeSender
}

// ExecuteAction that execute a BatchDml update action request, store the results in the OutcomeSender
func (h *BatchDmlHandler) ExecuteAction(ctx context.Context) error {
	log.Printf("executing BatchDml update %v", h.Action)
	h.FlowContext.mu.Lock()
	defer h.FlowContext.mu.Unlock()

	var queries []spanner.Statement
	for i, update := range h.Action.GetUpdates() {
		log.Printf("executing BatchDml update [%d] %s\n %s\n", i+1, h.FlowContext.transactionSeed, update)
		stmt, err := utility.BuildQuery(update)
		if err != nil {
			return h.OutcomeSender.FinishWithError(err)
		}
		queries = append(queries, stmt)
	}

	rowCounts, err := executeBatchDml(ctx, queries, h.FlowContext)
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}
	h.OutcomeSender.InitForQuery()
	for _, rowCount := range rowCounts {
		h.OutcomeSender.AppendDmlRowsModified(rowCount)
	}
	// The batchDml request failed. By design, `rowCounts` contains rows
	// modified for DML queries that succeeded only. Add 0 as the row count
	// for the last executed DML in the batch (that failed).
	if len(rowCounts) != len(queries) {
		h.OutcomeSender.AppendDmlRowsModified(0)
	}
	return h.OutcomeSender.FinishSuccessfully()
}

// Execute a batch of updates in a read-write transaction
func executeBatchDml(ctx context.Context, stmts []spanner.Statement, flowContext *ExecutionFlowContext) ([]int64, error) {
	for i, stmt := range stmts {
		log.Printf("executeBatchDml [%d]: %v", i+1, stmt)
	}
	txn, err := flowContext.getTransactionForWrite()
	if err != nil {
		return nil, err
	}

	return txn.BatchUpdate(ctx, stmts)
}

type CloseBatchTxnHandler struct {
	Action        *executorpb.CloseBatchTransactionAction
	FlowContext   *ExecutionFlowContext
	OutcomeSender *output_stream.OutcomeSender
}

// ExecuteAction that finishes a batch transaction
func (h *CloseBatchTxnHandler) ExecuteAction(ctx context.Context) error {
	log.Printf("closing batch transaction %v", h.Action)
	if h.Action.GetCleanup() {
		if h.FlowContext.batchTxn == nil {
			return h.OutcomeSender.FinishWithError(status.Error(codes.InvalidArgument, "not in a batch transaction"))
		}
		h.FlowContext.batchTxn.Close()
	}
	return h.OutcomeSender.FinishSuccessfully()
}
