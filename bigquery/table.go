// Copyright 2015 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bigquery

import (
	"fmt"

	bq "google.golang.org/api/bigquery/v2"
)

// A Table is a reference to a BigQuery table.
type Table struct {
	// ProjectID, DatasetID and TableID may be omitted if the Table is the destination for a query.
	// In this case the result will be stored in an ephemeral table.
	ProjectID string
	DatasetID string
	// TableID must contain only letters (a-z, A-Z), numbers (0-9), or underscores (_).
	// The maximum length is 1,024 characters.
	TableID string

	// All following fields are optional.
	CreateDisposition CreateDisposition // default is CreateIfNeeded.
	WriteDisposition  WriteDisposition  // default is WriteAppend.

	// Name is the user-friendly name for this table.
	Name string
	Type TableType
}

// Tables is a group of tables. The tables may belong to differing projects or datasets.
type Tables []*Table

// CreateDisposition specifies the circumstances under which destination table will be created.
type CreateDisposition string

const (
	// The table will be created if it does not already exist.  Tables are created atomically on successful completion of a job.
	CreateIfNeeded CreateDisposition = "CREATE_IF_NEEDED"

	// The table must already exist and will not be automatically created.
	CreateNever CreateDisposition = "CREATE_NEVER"
)

// WriteDisposition specifies how existing data in a destination table is treated.
type WriteDisposition string

const (
	// Data will be appended to any existing data in the destination table.
	// Data is appended atomically on successful completion of a job.
	WriteAppend WriteDisposition = "WRITE_APPEND"

	// Existing data in the destination table will be overwritten.
	// Data is overwritten atomically on successful completion of a job.
	WriteTruncate WriteDisposition = "WRITE_TRUNCATE"

	// Writes will fail if the destination table already contains data.
	WriteEmpty WriteDisposition = "WRITE_EMPTY"
)

// TableType is the type of table.
type TableType string

const (
	RegularTable TableType = "TABLE"
	ViewTable    TableType = "VIEW"
)

func (t *Table) implementsSource()      {}
func (t *Table) implementsReadSource()  {}
func (t *Table) implementsDestination() {}
func (ts Tables) implementsSource()     {}

func (t *Table) tableRefProto() *bq.TableReference {
	return &bq.TableReference{
		ProjectId: t.ProjectID,
		DatasetId: t.DatasetID,
		TableId:   t.TableID,
	}
}

// FullyQualifiedName returns the ID of the table in projectID:datasetID.tableID format.
func (t *Table) FullyQualifiedName() string {
	return fmt.Sprintf("%s:%s.%s", t.ProjectID, t.DatasetID, t.TableID)
}

// implicitTable reports whether Table is an empty placeholder, which signifies that a new table should be created with an auto-generated Table ID.
func (t *Table) implicitTable() bool {
	return t.ProjectID == "" && t.DatasetID == "" && t.TableID == ""
}

func (t *Table) customizeLoadDst(conf *bq.JobConfigurationLoad, projectID string) {
	conf.DestinationTable = t.tableRefProto()
	conf.CreateDisposition = string(t.CreateDisposition)
	conf.WriteDisposition = string(t.WriteDisposition)
}

func (t *Table) customizeExtractSrc(conf *bq.JobConfigurationExtract, projectID string) {
	conf.SourceTable = t.tableRefProto()
}

func (t *Table) customizeCopyDst(conf *bq.JobConfigurationTableCopy, projectID string) {
	conf.DestinationTable = t.tableRefProto()
	conf.CreateDisposition = string(t.CreateDisposition)
	conf.WriteDisposition = string(t.WriteDisposition)
}

func (ts Tables) customizeCopySrc(conf *bq.JobConfigurationTableCopy, projectID string) {
	for _, t := range ts {
		conf.SourceTables = append(conf.SourceTables, t.tableRefProto())
	}
}

func (t *Table) customizeQueryDst(conf *bq.JobConfigurationQuery, projectID string) {
	if !t.implicitTable() {
		conf.DestinationTable = t.tableRefProto()
	}
	conf.CreateDisposition = string(t.CreateDisposition)
	conf.WriteDisposition = string(t.WriteDisposition)
}

func (t *Table) customizeReadSrc(conf *readTabledataConf) {
	conf.projectID = t.ProjectID
	conf.datasetID = t.DatasetID
	conf.tableID = t.TableID
}
