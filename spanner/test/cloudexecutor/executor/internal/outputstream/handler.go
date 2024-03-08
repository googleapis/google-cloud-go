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

package outputstream

import (
	"log"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/executor/apiv1/executorpb"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/utility"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// if OutcomeSender.rowCount exceed maxRowsPerBatch value, we should send rows back to the client in batch.
const maxRowsPerBatch = 100

// OutcomeSender is a utility class used for sending action outcomes back to the client. For read
// actions, it buffers rows and sends partial read results in batches.
type OutcomeSender struct {
	actionID int32
	stream   executorpb.SpannerExecutorProxy_ExecuteActionAsyncServer

	// partialOutcome accumulates rows and other relevant information
	partialOutcome *executorpb.SpannerActionOutcome
	readResult     *executorpb.ReadResult
	queryResult    *executorpb.QueryResult

	// All the relevant variables below should be set before first outcome is sent back,
	// and unused variables should leave null.
	timestamp              *timestamppb.Timestamp
	hasReadResult          bool
	hasQueryResult         bool
	hasChangeStreamRecords bool
	table                  string  // name of the table being read
	index                  *string // name of the secondary index used for read
	requestIndex           *int32  // request index (for multireads)
	rowType                *spannerpb.StructType

	// Current row count in read/query result
	rowCount int64
	// modified row count in dml result
	rowsModified []int64
}

// NewOutcomeSender returns an OutcomeSender with default fields set.
func NewOutcomeSender(actionID int32, stream executorpb.SpannerExecutorProxy_ExecuteActionAsyncServer) *OutcomeSender {
	return &OutcomeSender{
		actionID:       actionID,
		stream:         stream,
		hasReadResult:  false,
		hasQueryResult: false,
	}
}

// SetTimestamp sets the timestamp for commit.
func (s *OutcomeSender) SetTimestamp(timestamp *timestamppb.Timestamp) {
	s.timestamp = timestamp
}

// SetRowType sets the rowType for appending row.
func (s *OutcomeSender) SetRowType(rowType *spannerpb.StructType) {
	s.rowType = rowType
}

// InitForRead init the sender for read action, then set the table and index if there exists.
func (s *OutcomeSender) InitForRead(table string, index *string) {
	s.hasReadResult = true
	s.table = table
	if index != nil {
		s.index = index
	}
}

// InitForQuery init the sender for query action
func (s *OutcomeSender) InitForQuery() {
	s.hasQueryResult = true
}

// InitForBatchRead init the sender for batch read action, then set the table and index if there exists.
func (s *OutcomeSender) InitForBatchRead(table string, index *string) {
	s.InitForRead(table, index)
	// Cloud API supports only simple batch reads (not multi reads), so request index is always 0.
	requestIndex := int32(0)
	s.requestIndex = &requestIndex
}

// AppendDmlRowsModified add rows modified in dml to result
func (s *OutcomeSender) AppendDmlRowsModified(rowsModified int64) {
	s.rowsModified = append(s.rowsModified, rowsModified)
}

// FinishSuccessfully sends the last outcome with OK status.
func (s *OutcomeSender) FinishSuccessfully() error {
	s.buildOutcome()
	s.partialOutcome.Status = &spb.Status{Code: int32(codes.OK)}
	return s.flush()
}

// FinishWithTransactionRestarted sends the last outcome with aborted error,
// this will set the TransactionRestarted to true
func (s *OutcomeSender) FinishWithTransactionRestarted() error {
	s.buildOutcome()
	transactionRestarted := true
	s.partialOutcome.TransactionRestarted = &transactionRestarted
	s.partialOutcome.Status = &spb.Status{Code: int32(codes.OK)}
	return s.flush()
}

// FinishWithError sends the last outcome with given error status.
func (s *OutcomeSender) FinishWithError(err error) error {
	s.buildOutcome()
	s.partialOutcome.Status = utility.ErrToStatus(err)
	return s.flush()
}

// AppendRow adds another row to buffer. If buffer hits its size limit, the buffered rows will be sent back.
func (s *OutcomeSender) AppendRow(row *executorpb.ValueList) error {
	if !s.hasReadResult && !s.hasQueryResult {
		return spanner.ToSpannerError(status.Error(codes.InvalidArgument, "either hasReadResult or hasQueryResult should be true"))
	}
	if s.rowType == nil {
		return spanner.ToSpannerError(status.Error(codes.InvalidArgument, "rowType should be set first"))
	}
	s.buildOutcome()
	if s.hasReadResult {
		s.readResult.Row = append(s.readResult.Row, row)
		s.rowCount++
	} else if s.hasQueryResult {
		s.queryResult.Row = append(s.queryResult.Row, row)
		s.rowCount++
	}
	if s.rowCount >= maxRowsPerBatch {
		return s.flush()
	}
	return nil
}

// buildOutcome will build the partialOutcome if not exists using relevant variables.
func (s *OutcomeSender) buildOutcome() {
	if s.partialOutcome != nil {
		return
	}
	s.partialOutcome = &executorpb.SpannerActionOutcome{
		CommitTime: s.timestamp,
	}
	if s.hasReadResult {
		s.readResult = &executorpb.ReadResult{
			Table:        s.table,
			Index:        s.index,
			RowType:      s.rowType,
			RequestIndex: s.requestIndex,
		}
	} else if s.hasQueryResult {
		s.queryResult = &executorpb.QueryResult{
			RowType: s.rowType,
		}
	}
}

// flush sends partialOutcome to stream and clear the internal state
func (s *OutcomeSender) flush() error {
	if s == nil || s.partialOutcome == nil {
		log.Println("outcomeSender.flush() is called when there is no partial outcome to send. This is an internal error that should never happen")
		return spanner.ToSpannerError(status.Error(codes.InvalidArgument, "either outcome sender or partial outcome is nil"))
	}
	s.partialOutcome.DmlRowsModified = s.rowsModified
	if s.hasReadResult {
		s.partialOutcome.ReadResult = s.readResult
	} else if s.hasQueryResult {
		s.partialOutcome.QueryResult = s.queryResult
	}
	err := s.SendOutcome(s.partialOutcome)
	s.partialOutcome = nil
	s.readResult = nil
	s.queryResult = nil
	s.rowCount = 0
	s.rowsModified = []int64{}
	return err
}

// SendOutcome sends the given SpannerActionOutcome.
func (s *OutcomeSender) SendOutcome(outcome *executorpb.SpannerActionOutcome) error {
	log.Printf("sending result %v actionId %d", outcome, s.actionID)
	resp := &executorpb.SpannerAsyncActionResponse{
		ActionId: s.actionID,
		Outcome:  outcome,
	}
	err := s.stream.Send(resp)
	if err != nil {
		log.Printf("Failed to send outcome with error: %s", err.Error())
	} else {
		log.Printf("Sent result %v actionId %d", outcome, s.actionID)
	}
	return err
}
