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

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/executor/apiv1/executorpb"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/outputstream"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/utility"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DmlActionHandler holds the necessary components required for DML action.
type DmlActionHandler struct {
	Action        *executorpb.DmlAction
	FlowContext   *ExecutionFlowContext
	OutcomeSender *outputstream.OutcomeSender
}

// ExecuteAction executes a DML update action request, store the results in the outputstream.OutcomeSender.
func (h *DmlActionHandler) ExecuteAction(ctx context.Context) error {
	h.FlowContext.mu.Lock()
	defer h.FlowContext.mu.Unlock()
	log.Printf("Executing dml update %s\n %v\n", h.FlowContext.transactionSeed, h.Action)
	stmt, err := utility.BuildQuery(h.Action.GetUpdate())
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}

	var iter *spanner.RowIterator
	txn, err := h.FlowContext.getTransactionForWrite()
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}
	h.OutcomeSender.InitForQuery()
	iter = txn.Query(ctx, stmt)
	defer iter.Stop()
	log.Printf("Parsing DML result %s\n", h.FlowContext.transactionSeed)
	err = processResults(iter, 0, h.OutcomeSender, h.FlowContext)
	if err != nil {
		if status.Code(err) == codes.Aborted {
			return h.OutcomeSender.FinishWithTransactionRestarted()
		}
		return h.OutcomeSender.FinishWithError(err)
	}
	h.OutcomeSender.AppendDmlRowsModified(iter.RowCount)
	return h.OutcomeSender.FinishSuccessfully()
}
