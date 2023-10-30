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
	"sync"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/utility"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type currentActiveTransaction int

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
	dbClient                 *spanner.Client                        // Current database client
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
