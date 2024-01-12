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

package utility

import (
	"fmt"
	"log"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/executor/apiv1/executorpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TableMetadataHelper is used to hold and retrieve metadata of tables and columns involved
// in a transaction.
type TableMetadataHelper struct {
	tableColumnsInOrder    map[string][]*executorpb.ColumnMetadata
	tableColumnsByName     map[string]map[string]*executorpb.ColumnMetadata
	tableKeyColumnsInOrder map[string][]*executorpb.ColumnMetadata
}

// InitFrom reads table metadata from the given StartTransactionAction.
func (t *TableMetadataHelper) InitFrom(a *executorpb.StartTransactionAction) {
	t.InitFromTableMetadata(a.GetTable())
}

// InitFromTableMetadata extracts table metadata and make maps to store them.
func (t *TableMetadataHelper) InitFromTableMetadata(tables []*executorpb.TableMetadata) {
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

// GetColumnType returns the column type of the given table and column.
func (t *TableMetadataHelper) GetColumnType(tableName string, colName string) (*spannerpb.Type, error) {
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
func (t *TableMetadataHelper) getColumnTypes(tableName string) ([]*spannerpb.Type, error) {
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

// GetKeyColumnTypes returns a list of key column types of the given table.
func (t *TableMetadataHelper) GetKeyColumnTypes(tableName string) ([]*spannerpb.Type, error) {
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
