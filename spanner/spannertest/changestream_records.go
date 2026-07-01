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

// changestream_records.go co-locates each change stream record type's Go struct,
// its toSlice() serialisation method, and the corresponding *spannerpb.Type
// proto descriptor. Keeping these adjacent means the field-order contract is
// visible at a glance: the fields in the proto type var and the fields returned
// by toSlice() must be in the same order, and both live in this file.

import (
	"time"

	"cloud.google.com/go/spanner/apiv1/spannerpb"
)

var (
	csStringType    = &spannerpb.Type{Code: spannerpb.TypeCode_STRING}
	csBoolType      = &spannerpb.Type{Code: spannerpb.TypeCode_BOOL}
	csInt64Type     = &spannerpb.Type{Code: spannerpb.TypeCode_INT64}
	csTimestampType = &spannerpb.Type{Code: spannerpb.TypeCode_TIMESTAMP}
	csJSONType      = &spannerpb.Type{Code: spannerpb.TypeCode_JSON}

	csStringArrayType = &spannerpb.Type{
		Code:             spannerpb.TypeCode_ARRAY,
		ArrayElementType: &spannerpb.Type{Code: spannerpb.TypeCode_STRING},
	}
)

// csColumnTypeElem is the proto type for a single column_types struct entry:
//
//	STRUCT<name STRING, type JSON, is_primary_key BOOL, ordinal_position INT64>
var (
	csColumnTypeElem = &spannerpb.Type{
		Code: spannerpb.TypeCode_STRUCT,
		StructType: &spannerpb.StructType{Fields: []*spannerpb.StructType_Field{
			{Name: "name", Type: csStringType},
			{Name: "type", Type: csJSONType},
			{Name: "is_primary_key", Type: csBoolType},
			{Name: "ordinal_position", Type: csInt64Type},
		}},
	}
	csColumnTypesType = &spannerpb.Type{Code: spannerpb.TypeCode_ARRAY, ArrayElementType: csColumnTypeElem}
)

// columnTypeRecord is the internal representation of one column_types entry.
// Field order in toSlice() must match csColumnTypeElem above.
type columnTypeRecord struct {
	Name            string
	Type            string // pre-serialised JSON, e.g. `{"code":"INT64"}`
	IsPrimaryKey    bool
	OrdinalPosition int64
}

func (c columnTypeRecord) toSlice() []interface{} {
	return []interface{}{c.Name, c.Type, c.IsPrimaryKey, c.OrdinalPosition}
}

// csModElem is the proto type for a single mods struct entry:
//
//	STRUCT<keys JSON, new_values JSON, old_values JSON>
var (
	csModElem = &spannerpb.Type{
		Code: spannerpb.TypeCode_STRUCT,
		StructType: &spannerpb.StructType{Fields: []*spannerpb.StructType_Field{
			{Name: "keys", Type: csJSONType},
			{Name: "new_values", Type: csJSONType},
			{Name: "old_values", Type: csJSONType},
		}},
	}
	csModsType = &spannerpb.Type{Code: spannerpb.TypeCode_ARRAY, ArrayElementType: csModElem}
)

// modRecord is the internal representation of one mods entry.
// Field order in toSlice() must match csModElem above.
type modRecord struct {
	Keys      string // pre-serialised JSON
	NewValues string // pre-serialised JSON
	OldValues string // pre-serialised JSON
}

func (m modRecord) toSlice() []interface{} {
	return []interface{}{m.Keys, m.NewValues, m.OldValues}
}

// csDataChangeRecordElem is the proto type for a single data_change_record entry.
var (
	csDataChangeRecordElem = &spannerpb.Type{
		Code: spannerpb.TypeCode_STRUCT,
		StructType: &spannerpb.StructType{Fields: []*spannerpb.StructType_Field{
			{Name: "commit_timestamp", Type: csTimestampType},
			{Name: "record_sequence", Type: csStringType},
			{Name: "server_transaction_id", Type: csStringType},
			{Name: "is_last_record_in_transaction_in_partition", Type: csBoolType},
			{Name: "table_name", Type: csStringType},
			{Name: "column_types", Type: csColumnTypesType},
			{Name: "mods", Type: csModsType},
			{Name: "mod_type", Type: csStringType},
			{Name: "value_capture_type", Type: csStringType},
			{Name: "number_of_records_in_transaction", Type: csInt64Type},
			{Name: "number_of_partitions_in_transaction", Type: csInt64Type},
			{Name: "transaction_tag", Type: csStringType},
			{Name: "is_system_transaction", Type: csBoolType},
		}},
	}
	csDataChangeRecordType = &spannerpb.Type{Code: spannerpb.TypeCode_ARRAY, ArrayElementType: csDataChangeRecordElem}
)

// dataChangeRecord is the internal representation of one data_change_record entry.
// Field order in toSlice() must match csDataChangeRecordElem above.
type dataChangeRecord struct {
	CommitTimestamp                      time.Time
	RecordSequence                       string
	ServerTransactionID                  string
	IsLastRecordInTransactionInPartition bool
	TableName                            string
	ColumnTypes                          []columnTypeRecord
	Mods                                 []modRecord
	ModType                              string
	ValueCaptureType                     string
	NumberOfRecordsInTransaction         int64
	NumberOfPartitionsInTransaction      int64
	TransactionTag                       string
	IsSystemTransaction                  bool
}

func (d dataChangeRecord) toSlice() []interface{} {
	colTypes := make([]interface{}, len(d.ColumnTypes))
	for i, ct := range d.ColumnTypes {
		colTypes[i] = ct.toSlice()
	}
	mods := make([]interface{}, len(d.Mods))
	for i, m := range d.Mods {
		mods[i] = m.toSlice()
	}
	return []interface{}{
		d.CommitTimestamp,
		d.RecordSequence,
		d.ServerTransactionID,
		d.IsLastRecordInTransactionInPartition,
		d.TableName,
		colTypes,
		mods,
		d.ModType,
		d.ValueCaptureType,
		d.NumberOfRecordsInTransaction,
		d.NumberOfPartitionsInTransaction,
		d.TransactionTag,
		d.IsSystemTransaction,
	}
}

// csHeartbeatRecordElem is the proto type for a single heartbeat_record entry:
//
//	STRUCT<timestamp TIMESTAMP>
var (
	csHeartbeatRecordElem = &spannerpb.Type{
		Code: spannerpb.TypeCode_STRUCT,
		StructType: &spannerpb.StructType{Fields: []*spannerpb.StructType_Field{
			{Name: "timestamp", Type: csTimestampType},
		}},
	}
	csHeartbeatRecordType = &spannerpb.Type{Code: spannerpb.TypeCode_ARRAY, ArrayElementType: csHeartbeatRecordElem}
)

// heartbeatRecord is the internal representation of one heartbeat_record entry.
// Field order in toSlice() must match csHeartbeatRecordElem above.
type heartbeatRecord struct {
	Timestamp time.Time
}

func (h heartbeatRecord) toSlice() []interface{} {
	return []interface{}{h.Timestamp}
}

// csChildPartitionElem is the proto type for one element of child_partitions:
//
//	STRUCT<token STRING, parent_partition_tokens ARRAY<STRING>>
var csChildPartitionElem = &spannerpb.Type{
	Code: spannerpb.TypeCode_STRUCT,
	StructType: &spannerpb.StructType{Fields: []*spannerpb.StructType_Field{
		{Name: "token", Type: csStringType},
		{Name: "parent_partition_tokens", Type: csStringArrayType},
	}},
}

// childPartition is the internal representation of one child_partitions entry.
// Field order in toSlice() must match csChildPartitionElem above.
type childPartition struct {
	Token                 string
	ParentPartitionTokens []string
}

func (cp childPartition) toSlice() []interface{} {
	parents := make([]interface{}, len(cp.ParentPartitionTokens))
	for i, p := range cp.ParentPartitionTokens {
		parents[i] = p
	}
	return []interface{}{cp.Token, parents}
}

// csChildPartitionsRecordElem is the proto type for a single child_partitions_record entry:
//
//	STRUCT<start_timestamp TIMESTAMP, record_sequence STRING,
//	       child_partitions ARRAY<STRUCT<token STRING, parent_partition_tokens ARRAY<STRING>>>>
var (
	csChildPartitionsRecordElem = &spannerpb.Type{
		Code: spannerpb.TypeCode_STRUCT,
		StructType: &spannerpb.StructType{Fields: []*spannerpb.StructType_Field{
			{Name: "start_timestamp", Type: csTimestampType},
			{Name: "record_sequence", Type: csStringType},
			{Name: "child_partitions", Type: &spannerpb.Type{
				Code:             spannerpb.TypeCode_ARRAY,
				ArrayElementType: csChildPartitionElem,
			}},
		}},
	}
	csChildPartitionsRecordType = &spannerpb.Type{Code: spannerpb.TypeCode_ARRAY, ArrayElementType: csChildPartitionsRecordElem}
)

// childPartitionsRecord is the internal representation of one child_partitions_record entry.
// Field order in toSlice() must match csChildPartitionsRecordElem above.
type childPartitionsRecord struct {
	StartTimestamp  time.Time
	RecordSequence  string
	ChildPartitions []childPartition
}

func (cpr childPartitionsRecord) toSlice() []interface{} {
	cps := make([]interface{}, len(cpr.ChildPartitions))
	for i, cp := range cpr.ChildPartitions {
		cps[i] = cp.toSlice()
	}
	return []interface{}{cpr.StartTimestamp, cpr.RecordSequence, cps}
}

// csChangeRecordElem is the proto type for one ChangeRecord struct:
//
//	STRUCT<data_change_record ARRAY<...>, heartbeat_record ARRAY<...>, child_partitions_record ARRAY<...>>
//
// csChangeRecordColType is the proto type for the ChangeRecord TVF result column:
//
//	ARRAY<STRUCT<data_change_record ARRAY<...>, ...>>
var (
	csChangeRecordElem = &spannerpb.Type{
		Code: spannerpb.TypeCode_STRUCT,
		StructType: &spannerpb.StructType{Fields: []*spannerpb.StructType_Field{
			{Name: "data_change_record", Type: csDataChangeRecordType},
			{Name: "heartbeat_record", Type: csHeartbeatRecordType},
			{Name: "child_partitions_record", Type: csChildPartitionsRecordType},
		}},
	}
	csChangeRecordColType = &spannerpb.Type{Code: spannerpb.TypeCode_ARRAY, ArrayElementType: csChangeRecordElem}
)

// changeRecord is the internal representation of one ChangeRecord struct.
// Field order in toSlice() must match csChangeRecordElem above.
type changeRecord struct {
	DataChangeRecords      []dataChangeRecord
	HeartbeatRecords       []heartbeatRecord
	ChildPartitionsRecords []childPartitionsRecord
}

func (r changeRecord) toSlice() []interface{} {
	dcrs := make([]interface{}, len(r.DataChangeRecords))
	for i, d := range r.DataChangeRecords {
		dcrs[i] = d.toSlice()
	}
	hbs := make([]interface{}, len(r.HeartbeatRecords))
	for i, h := range r.HeartbeatRecords {
		hbs[i] = h.toSlice()
	}
	cprs := make([]interface{}, len(r.ChildPartitionsRecords))
	for i, c := range r.ChildPartitionsRecords {
		cprs[i] = c.toSlice()
	}
	return []interface{}{dcrs, hbs, cprs}
}
