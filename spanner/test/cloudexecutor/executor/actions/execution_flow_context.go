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
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/executor/apiv1/executorpb"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/utility"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type currentActiveTransaction int

// Current transaction status
const (
	None currentActiveTransaction = iota
	Read
	ReadWrite
	Batch
)

// ExecutionFlowContext represents a context in which SpannerActions are executed. Among other
// things, it includes currently active transactions and table metadata. There is exactly one
// instance of this per stubby call, created when the call is initialized and shared with all
// actionHandlers.
type ExecutionFlowContext struct {
	mu                       sync.Mutex                             // protects all internal state
	Database                 string                                 // current database path
	rwTxn                    *spanner.ReadWriteStmtBasedTransaction // Current read-write transaction
	roTxn                    *spanner.ReadOnlyTransaction           // Current read-only transaction
	batchTxn                 *spanner.BatchReadOnlyTransaction      // Current batch read-only transaction
	DbClient                 *spanner.Client                        // Current database client
	tableMetadata            *utility.TableMetadataHelper           // If in a txn (except batch), this has metadata info about table columns
	numPendingReads          int64                                  // Number of pending read/query actions.
	readAborted              bool                                   // Indicate whether there's a read/query action got aborted and the transaction need to be reset.
	transactionSeed          string                                 // Log the workid and op pair for tracing the thread.
	currentActiveTransaction currentActiveTransaction
	TxnContext               context.Context
}

// isTransactionActiveLocked returns true if any kind of transaction is currently active. Must hold c.mu
// when calling.
func (c *ExecutionFlowContext) isTransactionActiveLocked() bool {
	return c.rwTxn != nil || c.roTxn != nil || c.batchTxn != nil
}

// IsTransactionActive returns true if any kind of transaction is currently active.
func (c *ExecutionFlowContext) IsTransactionActive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.isTransactionActiveLocked()
}

// Return current database. Must hold c.mu when calling.
func (c *ExecutionFlowContext) getDatabase() (string, error) {
	if c.Database == "" {
		return "", status.Error(codes.InvalidArgument, "database doesn't exist")
	}
	return c.Database, nil
}

// Return current transaction that can be used for performing read/query actions. Must hold c.mu when calling.
func (c *ExecutionFlowContext) getTransactionForRead() (*spanner.ReadOnlyTransaction, error) {
	if c.roTxn != nil {
		return c.roTxn, nil
	}
	return nil, status.Error(codes.InvalidArgument, "no currently active transaction for read")
}

// Return current transaction that can be used for performing mutation/update actions. Must hold c.mu when calling.
func (c *ExecutionFlowContext) getTransactionForWrite() (*spanner.ReadWriteStmtBasedTransaction, error) {
	if c.rwTxn != nil {
		return c.rwTxn, nil
	}
	return nil, status.Error(codes.InvalidArgument, "no currently active transaction for read-write")
}

// Return current transaction that can be used for performing batch actions. Must hold c.mu when calling.
func (c *ExecutionFlowContext) getBatchTransaction() (*spanner.BatchReadOnlyTransaction, error) {
	if c.batchTxn != nil {
		return c.batchTxn, nil
	}
	return nil, status.Error(codes.InvalidArgument, "no currently active batch transaction")
}

// Increase the read count when a read/query is issued.
func (c *ExecutionFlowContext) startRead() {
	c.numPendingReads++
}

// Decrease the read count when a read/query is finished, if status is aborted and there's no
// pending read/query, reset the transaction for retry.
func (c *ExecutionFlowContext) finishRead(code codes.Code) {
	if code == codes.Aborted {
		c.readAborted = true
	}
	c.numPendingReads--
	if c.readAborted && c.numPendingReads <= 0 {
		log.Println("Transaction reset due to read/query abort")
		c.readAborted = false
	}
}

// Initialize the read count and aborted status when transaction started.
func (c *ExecutionFlowContext) initReadState() {
	c.readAborted = false
	c.numPendingReads = 0
}

// clear clears the transaction related variables.
func (c *ExecutionFlowContext) clear() {
	c.roTxn = nil
	c.rwTxn = nil
	c.tableMetadata = nil
}

// finish attempts to finish the transaction by either committing it or exiting without committing.
// In order to follow the ExecuteActions protocol, we must distinguish Spanner-generated errors
// (e.g. RowNotFound) and internal errors (e.g. a precondition is not met). Errors returned from
// Spanner populate the status of SpannerActionOutcome. Internal errors, in contrast, break the
// stubby call. For this reason, finish() has two return values dedicated to errors. If any of
// these errors is not nil, other return values are undefined.
// Return values in order:
// 1. Whether or not the transaction is restarted. It will be true if commit has been attempted,
// but Spanner returned aborted and restarted instead. When that happens, all reads and writes
// should be replayed, followed by another commit attempt.
// 2. Commit timestamp. It's returned only if commit has succeeded.
// 3. Spanner error -- an error that Spanner client returned.
// 4. Internal error.
func (c *ExecutionFlowContext) finish(ctx context.Context, txnFinishMode executorpb.FinishTransactionAction_Mode) (bool, *time.Time, error, error) {
	if txnFinishMode == executorpb.FinishTransactionAction_COMMIT {
		var err error
		ts, err := c.rwTxn.Commit(ctx)
		if err != nil {
			log.Printf("Transaction finished with error %v", err)
			if status.Code(err) == codes.Aborted {
				log.Println("Transaction Aborted. Sending status to outcome sender to restart the transaction.")
				return true, nil, nil, nil
			}
			// Filter expected errors
			if status.Code(err) == codes.Unknown && strings.Contains(err.Error(), "Transaction outcome unknown") {
				return false, nil, spanner.ToSpannerError(status.Error(codes.DeadlineExceeded, "Transaction outcome unknown")), nil
			}
			// TODO(harsha): check if this is an internal or spanner error
			return false, nil, err, nil
		}
		return false, &ts, nil, nil
	} else if txnFinishMode == executorpb.FinishTransactionAction_ABANDON {
		log.Printf("Transaction Abandoned")
		c.rwTxn.Rollback(ctx)
		return false, nil, nil, nil
	} else {
		return false, nil, nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "Unsupported finish mode %s", txnFinishMode.String()))
	}
}

// CloseOpenTransactions cleans up all the active transactions if the stubby call is closing.
func (c *ExecutionFlowContext) CloseOpenTransactions() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.roTxn != nil {
		log.Println("A read only transaction was active when stubby call closed.")
		c.roTxn.Close()
	}
	if c.rwTxn != nil {
		log.Println("Abandon a read-write transaction that was active when stubby call closed.")
		_, _, _, err := c.finish(context.Background(), executorpb.FinishTransactionAction_ABANDON)
		if err != nil {
			log.Printf("Failed to abandon a read-write transaction: %v\n", err)
		}
	}
	if c.batchTxn != nil {
		log.Println("A batch transaction was active when stubby call closed.")
		c.batchTxn.Close()
	}
}
