package actions

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/outputstream"
	executorpb "cloud.google.com/go/spanner/test/cloudexecutor/proto"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ReadResult is the result of the read change records from the partition.
type ReadResult struct {
	PartitionToken string          `json:"partition_token"`
	ChangeRecords  []*ChangeRecord `spanner:"ChangeRecord" json:"change_record"`
}

// ChangeRecord is the single unit of the records from the change stream.
type ChangeRecord struct {
	DataChangeRecords      []*DataChangeRecord      `spanner:"data_change_record" json:"data_change_record"`
	HeartbeatRecords       []*HeartbeatRecord       `spanner:"heartbeat_record" json:"heartbeat_record"`
	ChildPartitionsRecords []*ChildPartitionsRecord `spanner:"child_partitions_record" json:"child_partitions_record"`
}

// DataChangeRecord contains a set of changes to the table.
type DataChangeRecord struct {
	CommitTimestamp                      time.Time     `spanner:"commit_timestamp" json:"commit_timestamp"`
	RecordSequence                       string        `spanner:"record_sequence" json:"record_sequence"`
	ServerTransactionID                  string        `spanner:"server_transaction_id" json:"server_transaction_id"`
	IsLastRecordInTransactionInPartition bool          `spanner:"is_last_record_in_transaction_in_partition" json:"is_last_record_in_transaction_in_partition"`
	TableName                            string        `spanner:"table_name" json:"table_name"`
	ColumnTypes                          []*ColumnType `spanner:"column_types" json:"column_types"`
	Mods                                 []*Mod        `spanner:"mods" json:"mods"`
	ModType                              string        `spanner:"mod_type" json:"mod_type"`
	ValueCaptureType                     string        `spanner:"value_capture_type" json:"value_capture_type"`
	NumberOfRecordsInTransaction         int64         `spanner:"number_of_records_in_transaction" json:"number_of_records_in_transaction"`
	NumberOfPartitionsInTransaction      int64         `spanner:"number_of_partitions_in_transaction" json:"number_of_partitions_in_transaction"`
	TransactionTag                       string        `spanner:"transaction_tag" json:"transaction_tag"`
	IsSystemTransaction                  bool          `spanner:"is_system_transaction" json:"is_system_transaction"`
}

// HeartbeatRecord is the heartbeat record returned from Cloud Spanner.
type HeartbeatRecord struct {
	Timestamp time.Time `spanner:"timestamp" json:"timestamp"`
}

// ChildPartitionsRecord contains the child partitions of the stream.
type ChildPartitionsRecord struct {
	StartTimestamp  time.Time         `spanner:"start_timestamp" json:"start_timestamp"`
	RecordSequence  string            `spanner:"record_sequence" json:"record_sequence"`
	ChildPartitions []*ChildPartition `spanner:"child_partitions" json:"child_partitions"`
}

// ChildPartition contains the child partition token.
type ChildPartition struct {
	Token                 string   `spanner:"token" json:"token"`
	ParentPartitionTokens []string `spanner:"parent_partition_tokens" json:"parent_partition_tokens"`
}

// ColumnType is the metadata of the column.
type ColumnType struct {
	Name            string           `spanner:"name" json:"name"`
	Type            spanner.NullJSON `spanner:"type" json:"type"`
	IsPrimaryKey    bool             `spanner:"is_primary_key" json:"is_primary_key"`
	OrdinalPosition int64            `spanner:"ordinal_position" json:"ordinal_position"`
}

// Mod is the changes that were made on the table.
type Mod struct {
	Keys      spanner.NullJSON `spanner:"keys" json:"keys"`
	NewValues spanner.NullJSON `spanner:"new_values" json:"new_values"`
	OldValues spanner.NullJSON `spanner:"old_values" json:"old_values"`
}

// ChangeStreamActionHandler holds the necessary components and options required for performing admin tasks.
type ChangeStreamActionHandler struct {
	Action        *executorpb.ExecuteChangeStreamQuery
	FlowContext   *ExecutionFlowContext
	OutcomeSender *outputstream.OutcomeSender
	Options       []option.ClientOption
}

type changeStreamRecords struct {
	dataChangeRecord      []*executorpb.DataChangeRecord
	heartbeatRecord       []*executorpb.HeartbeatRecord
	childPartitionsRecord []*executorpb.ChildPartitionsRecord
}

func (h *ChangeStreamActionHandler) ExecuteAction(ctx context.Context) error {
	log.Printf("Start executing change change stream query: \n %v", h.Action)
	h.FlowContext.mu.Lock()
	defer h.FlowContext.mu.Unlock()
	if h.FlowContext.Database == "" {
		return h.OutcomeSender.FinishWithError(spanner.ToSpannerError(status.Error(codes.InvalidArgument, "database path must be set for this action")))
	}

	// Retrieve TVF parameters from the action.
	action := h.Action
	changeStreamName := action.GetName()
	// For initial partition query (no partition token) we simulate precision of the timestamp
	// in nanoseconds as that's closer inlined with the production client code.
	startTime := action.GetStartTime().AsTime().Format(time.RFC3339Nano)
	startTimeStr := fmt.Sprintf("\"%s\"", startTime)
	endTimeStr := "null"
	if action.GetEndTime() != nil {
		endTime := action.GetEndTime().AsTime().Format(time.RFC3339Nano)
		endTimeStr = fmt.Sprintf("\"%s\"", endTime)
	}
	heartBeat := "null"
	if action.HeartbeatMilliseconds != nil {
		heartBeat = strconv.FormatInt(int64(*action.HeartbeatMilliseconds), 10)
	}
	partitionToken := "null"
	if action.PartitionToken != nil {
		partitionToken = fmt.Sprintf("\"%s\"", action.GetPartitionToken())
	}

	tvfQuery := fmt.Sprintf("SELECT * FROM READ_%s(%s,%s,%s,%s);", changeStreamName, startTimeStr, endTimeStr, partitionToken, heartBeat)
	log.Printf("Start executing change stream TVF: \n %s", tvfQuery)

	h.OutcomeSender.InitForChangeStreamQuery(int(h.Action.GetHeartbeatMilliseconds()), h.Action.GetName(), h.Action.PartitionToken)
	client, err := spanner.NewClient(ctx, h.FlowContext.Database, h.Options...)
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}
	stmt := spanner.Statement{
		SQL: tvfQuery,
	}
	iter := client.Single().Query(ctx, stmt)
	for {
		var changeStreamRecords []*executorpb.ChangeStreamRecord
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return h.OutcomeSender.FinishWithError(err)
		}
		var x ReadResult
		if err := row.ToStructLenient(&x); err != nil {
			log.Printf("Error in change stream iteration %v ", err)
		}
		changeRecord := x.ChangeRecords[0]
		log.Printf("Change Record in row : \n%v", changeRecord)
		for _, dataChangeRecord := range changeRecord.DataChangeRecords {
			if dataChangeRecord != nil {
				executorDataChangeRecord := buildDataChangeRecord(dataChangeRecord)
				csRecord := &executorpb.ChangeStreamRecord{
					Record: &executorpb.ChangeStreamRecord_DataChange{
						DataChange: executorDataChangeRecord,
					},
				}
				changeStreamRecords = append(changeStreamRecords, csRecord)
			}
		}
		for _, heartBeatRecord := range changeRecord.HeartbeatRecords {
			if heartBeatRecord != nil {
				executorHeartBeatRecord := buildHeartBeatRecord(heartBeatRecord)
				csRecord := &executorpb.ChangeStreamRecord{
					Record: &executorpb.ChangeStreamRecord_Heartbeat{
						Heartbeat: executorHeartBeatRecord,
					},
				}
				changeStreamRecords = append(changeStreamRecords, csRecord)
			}
		}
		for _, childPartitionRecord := range changeRecord.ChildPartitionsRecords {
			if childPartitionRecord != nil {
				executorChildPartitionRecord := buildChildPartitionRecord(childPartitionRecord)
				csRecord := &executorpb.ChangeStreamRecord{
					Record: &executorpb.ChangeStreamRecord_ChildPartition{
						ChildPartition: executorChildPartitionRecord,
					},
				}
				changeStreamRecords = append(changeStreamRecords, csRecord)
			}
		}

		if h.OutcomeSender.GetIsPartitionedChangeStreamQuery() {
			lastReceivedTimestamp := h.OutcomeSender.GetChangeStreamRecordReceivedTimestamp()
			// Get the Unix timestamp in milliseconds
			currentChangeRecordReceivedTimestamp := int(time.Now().UnixNano() / int64(time.Millisecond))
			discrepancyMillis := currentChangeRecordReceivedTimestamp - lastReceivedTimestamp
			if lastReceivedTimestamp > 0 && discrepancyMillis > h.OutcomeSender.GetChangeStreamHeartbeatMilliSeconds()*10 && h.OutcomeSender.GetChangeStreamHeartbeatMilliSeconds() > 5000 {
				log.Printf("Does not pass the heartbeat interval check. The last record was received seconds %d ago, which is more than ten times the heartbeat interval, which is %d seconds.", discrepancyMillis/1000, h.OutcomeSender.GetChangeStreamHeartbeatMilliSeconds()/1000)
			}
			h.OutcomeSender.UpdateChangeStreamRecordReceivedTimestamp(currentChangeRecordReceivedTimestamp)
		}
		err = h.OutcomeSender.AppendChangeStreamRecord(changeStreamRecords)
		if err != nil {
			return h.OutcomeSender.FinishWithError(err)
		}
	}
	return h.OutcomeSender.FinishSuccessfully()
}

func buildHeartBeatRecord(record *HeartbeatRecord) *executorpb.HeartbeatRecord {
	heartBeatRecord := &executorpb.HeartbeatRecord{}
	heartBeatRecord.HeartbeatTime = timestamppb.New(record.Timestamp)
	return heartBeatRecord
}

func buildDataChangeRecord(record *DataChangeRecord) *executorpb.DataChangeRecord {
	dataChangeRecord := &executorpb.DataChangeRecord{}
	dataChangeRecord.CommitTime = timestamppb.New(record.CommitTimestamp)
	dataChangeRecord.RecordSequence = record.RecordSequence
	dataChangeRecord.TransactionId = record.ServerTransactionID
	dataChangeRecord.IsLastRecord = record.IsLastRecordInTransactionInPartition
	dataChangeRecord.Table = record.TableName
	for _, column := range record.ColumnTypes {
		dataChangeRecordColumnType := &executorpb.DataChangeRecord_ColumnType{}
		dataChangeRecordColumnType.Name = column.Name
		dataChangeRecordColumnType.Type = column.Type.String()
		dataChangeRecordColumnType.IsPrimaryKey = column.IsPrimaryKey
		dataChangeRecordColumnType.OrdinalPosition = column.OrdinalPosition
		dataChangeRecord.ColumnTypes = append(dataChangeRecord.ColumnTypes, dataChangeRecordColumnType)
	}
	for _, mod := range record.Mods {
		dataChangeRecordMod := &executorpb.DataChangeRecord_Mod{}
		dataChangeRecordMod.Keys = mod.Keys.String()
		dataChangeRecordMod.NewValues = mod.NewValues.String()
		dataChangeRecordMod.OldValues = mod.OldValues.String()
		dataChangeRecord.Mods = append(dataChangeRecord.Mods, dataChangeRecordMod)
	}
	dataChangeRecord.ModType = record.ModType
	dataChangeRecord.ValueCaptureType = record.ValueCaptureType
	dataChangeRecord.TransactionTag = record.TransactionTag
	dataChangeRecord.IsSystemTransaction = record.IsSystemTransaction
	return dataChangeRecord
}

func buildChildPartitionRecord(record *ChildPartitionsRecord) *executorpb.ChildPartitionsRecord {
	childPartitionRecord := &executorpb.ChildPartitionsRecord{}
	childPartitionRecord.StartTime = timestamppb.New(record.StartTimestamp)
	childPartitionRecord.RecordSequence = record.RecordSequence
	for _, childPartitions := range record.ChildPartitions {
		childPartitionRecordChildPartition := &executorpb.ChildPartitionsRecord_ChildPartition{}
		childPartitionRecordChildPartition.Token = childPartitions.Token
		childPartitionRecordChildPartition.ParentPartitionTokens = childPartitions.ParentPartitionTokens
		childPartitionRecord.ChildPartitions = append(childPartitionRecord.ChildPartitions, childPartitionRecordChildPartition)
	}
	return childPartitionRecord
}
