package executor

import (
	"errors"
	"fmt"
	"log"

	"cloud.google.com/go/spanner/apiv1/spannerpb"
	executorpb "cloud.google.com/go/spanner/executor/proto"
	status "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	gstatus "google.golang.org/grpc/status"
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
	t.initFromTableMetadatas(a.GetTable())
}

// initFromTableMetadatas extracts table metadata and make maps to store them.
func (t *tableMetadataHelper) initFromTableMetadatas(tables []*executorpb.TableMetadata) {
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

// getColumnType returns the column type of the given column at given table.
func (t *tableMetadataHelper) getColumnType(tableName, colName string) (*spannerpb.Type, error) {
	cols, ok := t.tableColumnsByName[tableName]
	if !ok {
		log.Printf("There is no metadata for table %s. Make sure that StartTransactionAction has TableMetadata correctly populated.", tableName)
		return nil, fmt.Errorf("there is no metadata for table %s", tableName)
	}
	colMetadata, ok := cols[colName]
	if !ok {
		log.Printf("Metadata for table %s contains no column named %s", tableName, colName)
		return nil, fmt.Errorf("no known column %s in table %s", colName, tableName)
	}
	return colMetadata.GetType(), nil
}

// getColumnTypes returns a list of column types of the given table.
func (t *tableMetadataHelper) getColumnTypes(tableName string) ([]*spannerpb.Type, error) {
	cols, ok := t.tableColumnsInOrder[tableName]
	if !ok {
		log.Printf("There is no metadata for table %s. Make sure that StartTransactionAction has TableMetadata correctly populated.", tableName)
		return nil, fmt.Errorf("no known table %s", tableName)
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

// outcomeSender is a tool that helps actionHandlers send outcomes of their actions. For reading
// actions, it buffers rows and sends partial read results every once in a while to prevent running
// out of memory.
type outcomeSender struct {
	actionID int32
	stream   executorpb.SpannerExecutorProxy_ExecuteActionAsyncServer
	// partialOutcome accumulates rows and other relevant information
	partialOutcome *executorpb.SpannerActionOutcome
	// All relevant values below should be set before first outcome is sent.
	timestamp              *timestamppb.Timestamp
	hasReadResult          bool
	hasQueryResult         bool
	hasChangeStreamRecords bool
	table                  string  // name of the table being read
	index                  *string // name of the secondary index used for read
	requestIndex           *int32  // request index (for multireads)
	rowType                *spannerpb.StructType
}

// createOutcomeIfNecessary will build the partialOutcome if it doesn't exist.
func (s *outcomeSender) createOutcomeIfNecessary() {
	if s.partialOutcome != nil {
		return
	}
	s.partialOutcome = &executorpb.SpannerActionOutcome{
		CommitTime: s.timestamp,
	}
	if s.hasReadResult {
		s.partialOutcome.ReadResult = &executorpb.ReadResult{
			Table:        s.table,
			Index:        s.index,
			RowType:      s.rowType,
			RequestIndex: s.requestIndex,
		}
	} else if s.hasQueryResult {
		s.partialOutcome.QueryResult = &executorpb.QueryResult{
			RowType: s.rowType,
		}
	}
}

const maxRowsPerBatch = 100

// appendRow adds another row to buffer. If buffer hits its size limit, the buffered rows are sent
// to the Stubby client.
func (s *outcomeSender) appendRow(row *executorpb.ValueList) error {
	if !s.hasReadResult && !s.hasQueryResult {
		return errors.New("either hasReadResult or hasQueryResult should be true")
	}
	if s.rowType == nil {
		return errors.New("set rowType first")
	}
	s.createOutcomeIfNecessary()
	var numRows int
	if s.hasReadResult {
		s.partialOutcome.ReadResult.Row = append(s.partialOutcome.ReadResult.Row, row)
		numRows = len(s.partialOutcome.ReadResult.Row)
		s.partialOutcome.ReadResult.RequestIndex = s.requestIndex
	} else if s.hasQueryResult {
		s.partialOutcome.QueryResult.Row = append(s.partialOutcome.QueryResult.Row, row)
		numRows = len(s.partialOutcome.QueryResult.Row)
	}
	if numRows >= maxRowsPerBatch {
		if err := s.flush(); err != nil {
			return err
		}
	}
	return nil
}

func (s *outcomeSender) appendDmlRowsModified(rowsModified int64) error {
	s.createOutcomeIfNecessary()
	s.partialOutcome.DmlRowsModified = append(s.partialOutcome.DmlRowsModified, rowsModified)
	return nil
}

// finishSuccessfully sends the last outcome with OK status.
func (s *outcomeSender) finishWithTransactionRestarted() error {
	s.createOutcomeIfNecessary()
	transactionRestarted := true
	s.partialOutcome.TransactionRestarted = &transactionRestarted
	s.partialOutcome.Status = &status.Status{Code: int32(codes.OK)}
	return s.flush()
}

// finishSuccessfully sends the last outcome with OK status.
func (s *outcomeSender) finishSuccessfully() error {
	s.createOutcomeIfNecessary()
	s.partialOutcome.Status = &status.Status{Code: int32(codes.OK)}
	log.Println("Printing row type during success")
	log.Println(s.rowType)
	return s.flush()
}

// finishWithError sends the last outcome with given error status.
func (s *outcomeSender) finishWithError(err error) error {
	s.createOutcomeIfNecessary()
	s.partialOutcome.Status = &status.Status{Code: int32(gstatus.Code(err)), Message: err.Error()}
	// errToStatus(err).ToProto(s.partialOutcome.Status)
	return s.flush()
}

// flush sends partialOutcome to the Stubby client. For internal use only.
func (s *outcomeSender) flush() error {
	if s == nil {
		log.Fatal("outcomeSender.flush() is called when there is no partial outcome to send. This is an internal error that should never happen")
		return errors.New("no partial outcome to send")
	}
	if err := s.sendOutcome(s.partialOutcome); err != nil {
		return err
	}
	s.partialOutcome = nil
	return nil
}

// sendOutcome sends the given SpannerActionOutcome.
func (s *outcomeSender) sendOutcome(outcome *executorpb.SpannerActionOutcome) error {
	log.Printf("sending result %v actionId %d", outcome, s.actionID)
	// google.Flush()
	resp := &executorpb.SpannerAsyncActionResponse{
		ActionId: s.actionID,
		Outcome:  outcome,
	}
	return s.stream.Send(resp)
}

// columnsOf converts columns to spanner ColumnList type.
/*func columnsOf(columns []string) *spanner.ColumnList {
	var v []any
	for _, c := range columns {
		v = append(v, c)
	}
	return spanner.Columns(v...)
}*/

/*
var protoTypes = map[string]protoreflect.MessageType{}
var enumTypes = map[string]protoreflect.Enum{}

// Init all the protos we will use in tests. Note that we can't dynamically
// register protos to Golang's central control, we need somehow hard-code a
// mapping between proto names and the proto descriptors for converting proto types.
func init() {
	protoTypes["spanner.TokenMap"] = new(token_mappb.TokenMap).ProtoReflect().Type()
	protoTypes["spanner.TokenMap.TokenAndPositions"] = new(token_mappb.TokenMap_TokenAndPositions).ProtoReflect().Type()
	protoTypes["spanner.test.LogProto"] = new(gopb.LogProto).ProtoReflect().Type()
	protoTypes["spanner.geometry.Geometry"] = new(geopb.Geometry).ProtoReflect().Type()
	protoTypes["spanner.geometry.Point"] = new(geopb.Point).ProtoReflect().Type()
	protoTypes["spanner.geometry.Line"] = new(geopb.Line).ProtoReflect().Type()
	protoTypes["spanner.geometry.Circle"] = new(geopb.Circle).ProtoReflect().Type()
	protoTypes["spanner.geometry.Rectangle"] = new(geopb.Rectangle).ProtoReflect().Type()
	protoTypes["spanner.geometry.Polygon"] = new(geopb.Polygon).ProtoReflect().Type()
	protoTypes["spanner.test.sample.Comment"] = new(samplepb.Comment).ProtoReflect().Type()
	protoTypes["spanner.systest.Comment"] = new(systestpb.Comment).ProtoReflect().Type()
	protoTypes["spanner.systest.TestProtoMessage"] = new(systestpb.TestProtoMessage).ProtoReflect().Type()
	protoTypes["spanner.systest.TestProtoMessage.InnerTestProtoMessage"] = new(systestpb.TestProtoMessage_InnerTestProtoMessage).ProtoReflect().Type()
	protoTypes["spanner.systest.AlteredMessage"] = new(systestpb.AlteredMessage).ProtoReflect().Type()
	protoTypes["spanner.systest.SimpleRecursiveMessage"] = new(systestpb.SimpleRecursiveMessage).ProtoReflect().Type()
	protoTypes["spanner.systest.RecursiveMessage"] = new(systestpb.RecursiveMessage).ProtoReflect().Type()
	protoTypes["spanner.systest.CircularRecursiveMessageA"] = new(systestpb.CircularRecursiveMessageA).ProtoReflect().Type()
	protoTypes["spanner.systest.CircularRecursiveMessageB"] = new(systestpb.CircularRecursiveMessageB).ProtoReflect().Type()
	protoTypes["spanner.systest.ContainsRecursiveMessage"] = new(systestpb.ContainsRecursiveMessage).ProtoReflect().Type()
	protoTypes["spanner.systest.MapProtoMessage"] = new(systestpb.MapProtoMessage).ProtoReflect().Type()
	protoTypes["spanner.systest.MapProtoMessage.NestedMapProtoMessage"] = new(systestpb.MapProtoMessage_NestedMapProtoMessage).ProtoReflect().Type()
	protoTypes["spanner.systest.SimpleOneOf"] = new(systestpb.SimpleOneOf).ProtoReflect().Type()
	protoTypes["spanner.systest.OneOfWrapper"] = new(systestpb.OneOfWrapper).ProtoReflect().Type()
	enumTypes["spanner.systest.OuterTestEnum"] = new(systestpb.OuterTestEnum).Enum()
	enumTypes["spanner.systest.TestProtoMessage.InnerTestEnum"] = new(systestpb.TestProtoMessage_InnerTestEnum).Enum()
	enumTypes["spanner.test.sample.TestEnum"] = new(samplepb.TestEnum).Enum()
}
*/
