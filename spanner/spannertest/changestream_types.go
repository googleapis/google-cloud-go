/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spannertest

// changestream_types.go defines exported Go struct types for decoding change
// stream TVF results using the Spanner Go client. Consumers can use these
// directly instead of redeclaring them:
//
//	var crs []*spannertest.ChangeRecord
//	if err := row.Column(0, &crs); err != nil { ... }

import (
	"time"

	"cloud.google.com/go/spanner"
)

// ColumnType describes one column in a change stream data_change_record.
type ColumnType struct {
	Name            string           `spanner:"name"`
	Type            spanner.NullJSON `spanner:"type"`
	IsPrimaryKey    bool             `spanner:"is_primary_key"`
	OrdinalPosition int64            `spanner:"ordinal_position"`
}

// Mod holds the key and value snapshots for one row mutation in a
// data_change_record.
type Mod struct {
	Keys      spanner.NullJSON `spanner:"keys"`
	NewValues spanner.NullJSON `spanner:"new_values"`
	OldValues spanner.NullJSON `spanner:"old_values"`
}

// DataChangeRecord describes mutations to a single table within a transaction.
type DataChangeRecord struct {
	CommitTimestamp                      time.Time     `spanner:"commit_timestamp"`
	RecordSequence                       string        `spanner:"record_sequence"`
	ServerTransactionID                  string        `spanner:"server_transaction_id"`
	IsLastRecordInTransactionInPartition bool          `spanner:"is_last_record_in_transaction_in_partition"`
	TableName                            string        `spanner:"table_name"`
	ColumnTypes                          []*ColumnType `spanner:"column_types"`
	Mods                                 []*Mod        `spanner:"mods"`
	ModType                              string        `spanner:"mod_type"`
	ValueCaptureType                     string        `spanner:"value_capture_type"`
	NumberOfRecordsInTransaction         int64         `spanner:"number_of_records_in_transaction"`
	NumberOfPartitionsInTransaction      int64         `spanner:"number_of_partitions_in_transaction"`
	TransactionTag                       string        `spanner:"transaction_tag"`
	IsSystemTransaction                  bool          `spanner:"is_system_transaction"`
}

// HeartbeatRecord is emitted periodically when no data changes occur within
// the heartbeat interval.
type HeartbeatRecord struct {
	Timestamp time.Time `spanner:"timestamp"`
}

// ChildPartition identifies a partition token that a consumer should query
// after receiving a ChildPartitionsRecord.
type ChildPartition struct {
	Token                 string   `spanner:"token"`
	ParentPartitionTokens []string `spanner:"parent_partition_tokens"`
}

// ChildPartitionsRecord is returned when querying a change stream with
// partition_token => NULL, identifying the initial set of partitions.
type ChildPartitionsRecord struct {
	StartTimestamp  time.Time         `spanner:"start_timestamp"`
	RecordSequence  string            `spanner:"record_sequence"`
	ChildPartitions []*ChildPartition `spanner:"child_partitions"`
}

// ChangeRecord is one row returned by a change stream TVF. Exactly one of its
// fields will be non-empty for any given row.
type ChangeRecord struct {
	DataChangeRecords      []*DataChangeRecord      `spanner:"data_change_record"`
	HeartbeatRecords       []*HeartbeatRecord       `spanner:"heartbeat_record"`
	ChildPartitionsRecords []*ChildPartitionsRecord `spanner:"child_partitions_record"`
}
