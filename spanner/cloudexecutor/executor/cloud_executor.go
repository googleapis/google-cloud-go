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

package executor

import (
	"fmt"
	"log"
	"strings"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	executorpb "cloud.google.com/go/spanner/cloudexecutor/proto"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	_ "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// tableMetadataHelper is used to hold and retrieve metadata of tables and columns involved
// in a transaction.
type tableMetadataHelper struct {
	tableColumnsInOrder    map[string][]*executorpb.ColumnMetadata
	tableColumnsByName     map[string]map[string]*executorpb.ColumnMetadata
	tableKeyColumnsInOrder map[string][]*executorpb.ColumnMetadata
}

// initFrom reads table metadata from the given StartTransactionAction.
func (t *tableMetadataHelper) initFrom(a *executorpb.StartTransactionAction) {
	t.initFromTableMetadata(a.GetTable())
}

// initFromTableMetadata extracts table metadata and make maps to store them.
func (t *tableMetadataHelper) initFromTableMetadata(tables []*executorpb.TableMetadata) {
	t.tableColumnsInOrder = make(map[string][]*executorpb.ColumnMetadata)
	t.tableColumnsByName = make(map[string]map[string]*executorpb.ColumnMetadata)
	t.tableKeyColumnsInOrder = make(map[string][]*executorpb.ColumnMetadata)
	for _, table := range tables {
		tableName := table.GetName()
		t.tableColumnsInOrder[tableName] = table.GetColumn()
		t.tableKeyColumnsInOrder[tableName] = table.GetKeyColumn()
		t.tableColumnsByName[tableName] = make(map[string]*executorpb.ColumnMetadata)
		for _, col := range table.GetColumn() {
			t.tableColumnsByName[tableName][col.GetName()] = col
		}
	}
}

// getColumnType returns the column type of the given table and column.
func (t *tableMetadataHelper) getColumnType(tableName string, colName string) (*spannerpb.Type, error) {
	cols, ok := t.tableColumnsByName[tableName]
	if !ok {
		log.Printf("There is no metadata for table %s. Make sure that StartTransactionAction has TableMetadata correctly populated.", tableName)
		return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "there is no metadata for table %s", tableName))
	}
	colMetadata, ok := cols[colName]
	if !ok {
		log.Printf("Metadata for table %s contains no column named %s", tableName, colName)
		return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "metadata for table %s contains no column named %s", tableName, colName))
	}
	return colMetadata.GetType(), nil
}

// getColumnTypes returns a list of column types of the given table.
func (t *tableMetadataHelper) getColumnTypes(tableName string) ([]*spannerpb.Type, error) {
	cols, ok := t.tableColumnsInOrder[tableName]
	if !ok {
		log.Printf("There is no metadata for table %s. Make sure that StartTransactionAction has TableMetadata correctly populated.", tableName)
		return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "there is no metadata for table %s", tableName))
	}
	var colTypes []*spannerpb.Type
	for _, col := range cols {
		colTypes = append(colTypes, col.GetType())
	}
	return colTypes, nil
}

// getKeyColumnTypes returns a list of key column types of the given table.
func (t *tableMetadataHelper) getKeyColumnTypes(tableName string) ([]*spannerpb.Type, error) {
	cols, ok := t.tableKeyColumnsInOrder[tableName]
	if !ok {
		log.Printf("There is no metadata for table %s. Make sure that StartTxnAction has TableMetadata correctly populated.", tableName)
		return nil, fmt.Errorf("there is no metadata for table %s", tableName)
	}
	var colTypes []*spannerpb.Type
	for _, col := range cols {
		colTypes = append(colTypes, col.GetType())
	}
	return colTypes, nil
}

// outcomeSender is a utility class used for sending action outcomes back to the client. For read
// actions, it buffers rows and sends partial read results in batches.
type outcomeSender struct {
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

// if rowCount exceed this value, we should send rows back in batch.
const maxRowsPerBatch = 100

// setTimestamp sets the timestamp for commit.
func (s *outcomeSender) setTimestamp(timestamp *timestamppb.Timestamp) {
	s.timestamp = timestamp
}

// setRowType sets the rowType for appending row.
func (s *outcomeSender) setRowType(rowType *spannerpb.StructType) {
	s.rowType = rowType
}

// initForRead init the sender for read action, then set the table and index if there exists.
func (s *outcomeSender) initForRead(table string, index *string) {
	s.hasReadResult = true
	s.table = table
	if index != nil {
		s.index = index
	}
}

// initForQuery init the sender for query action
func (s *outcomeSender) initForQuery() {
	s.hasQueryResult = true
}

// initForBatchRead init the sender for batch read action, then set the table and index if there exists.
func (s *outcomeSender) initForBatchRead(table string, index *string) {
	s.initForRead(table, index)
	// Cloud API supports only simple batch reads (not multi reads), so request index is always 0.
	requestIndex := int32(0)
	s.requestIndex = &requestIndex
}

// Add rows modified in dml to result
func (s *outcomeSender) appendDmlRowsModified(rowsModified int64) {
	// s.buildOutcome()
	s.rowsModified = append(s.rowsModified, rowsModified)
}

// finishSuccessfully sends the last outcome with OK status.
func (s *outcomeSender) finishSuccessfully() error {
	s.buildOutcome()
	s.partialOutcome.Status = &spb.Status{Code: int32(codes.OK)}
	return s.flush()
}

// finishWithTransactionRestarted sends the last outcome with aborted error,
// this will set the TransactionRestarted to true
func (s *outcomeSender) finishWithTransactionRestarted() error {
	s.buildOutcome()
	transactionRestarted := true
	s.partialOutcome.TransactionRestarted = &transactionRestarted
	s.partialOutcome.Status = &spb.Status{Code: int32(codes.OK)}
	return s.flush()
}

// finishWithError sends the last outcome with given error status.
func (s *outcomeSender) finishWithError(err error) error {
	s.buildOutcome()
	//s.partialOutcome.Status = &status.Status{Code: int32(gstatus.Code(err)), Message: err.Error()}
	s.partialOutcome.Status = errToStatus(err)
	return s.flush()
}

// appendRow adds another row to buffer. If buffer hits its size limit, the buffered rows will be sent back.
func (s *outcomeSender) appendRow(row *executorpb.ValueList) error {
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
func (s *outcomeSender) buildOutcome() {
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
func (s *outcomeSender) flush() error {
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
	err := s.sendOutcome(s.partialOutcome)
	s.partialOutcome = nil
	s.readResult = nil
	s.queryResult = nil
	s.rowCount = 0
	s.rowsModified = []int64{}
	return err
}

// sendOutcome sends the given SpannerActionOutcome.
func (s *outcomeSender) sendOutcome(outcome *executorpb.SpannerActionOutcome) error {
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

// errToStatus maps cloud error to Status
func errToStatus(e error) *spb.Status {
	log.Print(e.Error())
	if strings.Contains(e.Error(), "Transaction outcome unknown") {
		return &spb.Status{Code: int32(codes.DeadlineExceeded), Message: e.Error()}
	}
	if status.Code(e) == codes.InvalidArgument {
		return &spb.Status{Code: int32(codes.InvalidArgument), Message: e.Error()}
	}
	if status.Code(e) == codes.PermissionDenied {
		return &spb.Status{Code: int32(codes.PermissionDenied), Message: e.Error()}
	}
	if status.Code(e) == codes.Aborted {
		return &spb.Status{Code: int32(codes.Aborted), Message: e.Error()}
	}
	if status.Code(e) == codes.AlreadyExists {
		return &spb.Status{Code: int32(codes.AlreadyExists), Message: e.Error()}
	}
	if status.Code(e) == codes.Canceled {
		return &spb.Status{Code: int32(codes.Canceled), Message: e.Error()}
	}
	if status.Code(e) == codes.Internal {
		return &spb.Status{Code: int32(codes.Internal), Message: e.Error()}
	}
	if status.Code(e) == codes.FailedPrecondition {
		return &spb.Status{Code: int32(codes.FailedPrecondition), Message: e.Error()}
	}
	if status.Code(e) == codes.NotFound {
		return &spb.Status{Code: int32(codes.NotFound), Message: e.Error()}
	}
	if status.Code(e) == codes.DeadlineExceeded {
		return &spb.Status{Code: int32(codes.DeadlineExceeded), Message: e.Error()}
	}
	if status.Code(e) == codes.ResourceExhausted {
		return &spb.Status{Code: int32(codes.ResourceExhausted), Message: e.Error()}
	}
	if status.Code(e) == codes.OutOfRange {
		return &spb.Status{Code: int32(codes.OutOfRange), Message: e.Error()}
	}
	if status.Code(e) == codes.Unauthenticated {
		return &spb.Status{Code: int32(codes.Unauthenticated), Message: e.Error()}
	}
	if status.Code(e) == codes.Unimplemented {
		return &spb.Status{Code: int32(codes.Unimplemented), Message: e.Error()}
	}
	if status.Code(e) == codes.Unavailable {
		return &spb.Status{Code: int32(codes.Unavailable), Message: e.Error()}
	}
	if status.Code(e) == codes.Unknown {
		return &spb.Status{Code: int32(codes.Unknown), Message: e.Error()}
	}
	return &spb.Status{Code: int32(codes.Unknown), Message: fmt.Sprintf("Error: %v, Unsupported Spanner error code: %v", e.Error(), status.Code(e))}
}
