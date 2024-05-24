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
	"log"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/executor/apiv1/executorpb"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/outputstream"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/utility"
	"google.golang.org/api/option"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// StartTxnHandler holds the necessary components and options required for start transaction action.
type StartTxnHandler struct {
	Action        *executorpb.StartTransactionAction
	FlowContext   *ExecutionFlowContext
	OutcomeSender *outputstream.OutcomeSender
	Options       []option.ClientOption
}

// ExecuteAction that starts a read-write or read-only transaction.
func (h *StartTxnHandler) ExecuteAction(ctx context.Context) error {
	h.FlowContext.mu.Lock()
	defer h.FlowContext.mu.Unlock()
	if h.FlowContext.Database == "" {
		return h.OutcomeSender.FinishWithError(spanner.ToSpannerError(status.Error(codes.InvalidArgument, "database path must be set for this action")))
	}
	if h.Action.GetTransactionSeed() != "" {
		h.FlowContext.transactionSeed = h.Action.GetTransactionSeed()
	}
	metadata := &utility.TableMetadataHelper{}
	metadata.InitFrom(h.Action)
	h.FlowContext.tableMetadata = metadata

	// TODO(harsha) where do I close the client? defer client.Close()
	client, err := spanner.NewClient(ctx, h.FlowContext.Database, h.Options...)
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}
	h.FlowContext.DbClient = client
	if h.FlowContext.isTransactionActiveLocked() {
		return h.OutcomeSender.FinishWithError(spanner.ToSpannerError(status.Error(codes.InvalidArgument, "already in a transaction")))
	}
	if h.Action.Concurrency != nil {
		log.Printf("starting read-only transaction %s:\n %v", h.FlowContext.transactionSeed, h.Action)
		timestampBound, err := timestampBoundsFromConcurrency(h.Action.GetConcurrency())
		if err != nil {
			return h.OutcomeSender.FinishWithError(err)
		}
		// Start a read-only transaction.
		var txn *spanner.ReadOnlyTransaction
		singleUseReadOnlyTransactionNeeded := isSingleUseReadOnlyTransactionNeeded(h.Action.GetConcurrency())
		if singleUseReadOnlyTransactionNeeded {
			txn = client.Single().WithTimestampBound(timestampBound)
		} else {
			txn = client.ReadOnlyTransaction().WithTimestampBound(timestampBound)
		}
		h.FlowContext.roTxn = txn
		h.FlowContext.currentActiveTransaction = Read
	} else {
		log.Printf("starting read-write transaction %s:\n %v", h.FlowContext.transactionSeed, h.Action)
		// Start a read-write transaction.
		var txn *spanner.ReadWriteStmtBasedTransaction
		if h.Action.GetExecutionOptions().GetOptimistic() {
			txn, err = spanner.NewReadWriteStmtBasedTransactionWithOptions(ctx, client, spanner.TransactionOptions{ReadLockMode: sppb.TransactionOptions_ReadWrite_OPTIMISTIC})
		} else {
			txn, err = spanner.NewReadWriteStmtBasedTransaction(ctx, client)
		}
		if err != nil {
			return h.OutcomeSender.FinishWithError(err)
		}
		h.FlowContext.rwTxn = txn
		h.FlowContext.currentActiveTransaction = ReadWrite
	}
	h.FlowContext.initReadState()
	return h.OutcomeSender.FinishSuccessfully()
}

// FinishTxnHandler holds the necessary components and options required for finish transaction action.
type FinishTxnHandler struct {
	Action        *executorpb.FinishTransactionAction
	FlowContext   *ExecutionFlowContext
	OutcomeSender *outputstream.OutcomeSender
}

// ExecuteAction that finish a transaction. For read-write transaction, either commit or abandon the transaction is allowed.
// Batch transaction is not supported here.
func (h *FinishTxnHandler) ExecuteAction(ctx context.Context) error {
	h.FlowContext.mu.Lock()
	defer h.FlowContext.mu.Unlock()
	log.Printf("Finishing transaction %s\n %v", h.FlowContext.transactionSeed, h.Action)

	if h.FlowContext.numPendingReads > 0 {
		return h.OutcomeSender.FinishWithError(spanner.ToSpannerError(status.Error(codes.FailedPrecondition, "Reads pending when trying to finish")))
	}
	outcome := &executorpb.SpannerActionOutcome{Status: &spb.Status{Code: int32(codes.OK)}}

	// Finish active transaction in given finishMode, then send outcome back to client.
	if h.FlowContext.roTxn != nil {
		// Finish a read-only transaction. Note that timestamp may not be available
		// if there were no reads or queries.
		ts, err := h.FlowContext.roTxn.Timestamp()
		if err != nil {
			return h.OutcomeSender.FinishWithError(err)
		}
		outcome.CommitTime = timestamppb.New(ts)

		h.FlowContext.roTxn.Close()
		h.FlowContext.clear()
		return h.OutcomeSender.SendOutcome(outcome)
	}

	if h.FlowContext.rwTxn != nil {
		// Finish a read-write transaction.
		txnFinishMode := h.Action.GetMode()
		restarted, ts, spanErr, internalErr := h.FlowContext.finish(ctx, txnFinishMode)
		if internalErr != nil {
			log.Printf("Unexpected error when finishing read-write transaction: %s", internalErr.Error())
			return h.OutcomeSender.FinishWithError(internalErr)
		} else if spanErr != nil {
			outcome.Status = utility.ErrToStatus(spanErr)
			h.FlowContext.clear()
		} else if restarted {
			log.Println("Transaction restarted")
			transactionRestarted := true
			outcome.TransactionRestarted = &transactionRestarted
		} else {
			if ts != nil {
				outcome.CommitTime = timestamppb.New(*ts)
			}
			h.FlowContext.clear()
		}
		return h.OutcomeSender.SendOutcome(outcome)
	}

	if h.FlowContext.batchTxn != nil {
		return h.OutcomeSender.FinishWithError(spanner.ToSpannerError(status.Error(codes.InvalidArgument, "Can't commit/abort a batch transaction")))
	}
	return h.OutcomeSender.FinishWithError(spanner.ToSpannerError(status.Error(codes.InvalidArgument, "no currently active transaction")))
}

// isSingleUseReadOnlyTransactionNeeded decides type of read-only transaction based on concurrency.
func isSingleUseReadOnlyTransactionNeeded(c *executorpb.Concurrency) bool {
	switch c.GetConcurrencyMode().(type) {
	case *executorpb.Concurrency_MinReadTimestampMicros:
		return true
	case *executorpb.Concurrency_MaxStalenessSeconds:
		return true
	default:
		return false
	}
}

// timestampFromMicros converts micros to time.Time
func timestampFromMicros(micros int64) time.Time {
	seconds := micros / 1000000
	nanos := (micros % 1000000) * 1000
	return time.Unix(seconds, nanos)
}

// timestampBoundsFromConcurrency converts executorpb.Concurrency to spanner.TimestampBound.
func timestampBoundsFromConcurrency(c *executorpb.Concurrency) (spanner.TimestampBound, error) {
	switch c.GetConcurrencyMode().(type) {
	case *executorpb.Concurrency_StalenessSeconds:
		secs := c.GetStalenessSeconds()
		dur := time.Duration(secs) * time.Second
		return spanner.ExactStaleness(dur), nil
	case *executorpb.Concurrency_MinReadTimestampMicros:
		return spanner.MinReadTimestamp(timestampFromMicros(c.GetMinReadTimestampMicros())), nil
	case *executorpb.Concurrency_MaxStalenessSeconds:
		secs := c.GetMaxStalenessSeconds()
		dur := time.Duration(secs) * time.Second
		return spanner.MaxStaleness(dur), nil
	case *executorpb.Concurrency_ExactTimestampMicros:
		return spanner.ReadTimestamp(timestampFromMicros(c.GetExactTimestampMicros())), nil
	case *executorpb.Concurrency_Strong:
		return spanner.StrongRead(), nil
	case *executorpb.Concurrency_Batch:
		return spanner.TimestampBound{}, spanner.ToSpannerError(status.Error(codes.InvalidArgument, "batch mode should not be in snapshot transaction"))
	default:
		return spanner.StrongRead(), spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "unsupported concurrency mode %s", c.String()))
	}
}
