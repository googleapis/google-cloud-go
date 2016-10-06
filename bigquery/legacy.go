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

	"golang.org/x/net/context"

	bq "google.golang.org/api/bigquery/v2"
)

// OpenTable creates a handle to an existing BigQuery table. If the table does
// not already exist, subsequent uses of the *Table will fail.
//
// Deprecated: use Client.DatasetInProject.Table instead.
func (c *Client) OpenTable(projectID, datasetID, tableID string) *Table {
	return c.Table(projectID, datasetID, tableID)
}

// Table creates a handle to a BigQuery table.
//
// Use this method to reference a table in a project other than that of the
// Client.
//
// Deprecated: use Client.DatasetInProject.Table instead.
func (c *Client) Table(projectID, datasetID, tableID string) *Table {
	return &Table{ProjectID: projectID, DatasetID: datasetID, TableID: tableID, c: c}
}

// CreateTable creates a table in the BigQuery service and returns a handle to it.
//
// Deprecated: use Table.Create instead.
func (c *Client) CreateTable(ctx context.Context, projectID, datasetID, tableID string, options ...CreateTableOption) (*Table, error) {
	t := c.Table(projectID, datasetID, tableID)
	if err := t.Create(ctx, options...); err != nil {
		return nil, err
	}
	return t, nil
}

// Read fetches data from a ReadSource and returns the data via an Iterator.
//
// Deprecated: use Query.Read, Job.Read or Table.Read instead.
func (c *Client) Read(ctx context.Context, src ReadSource, options ...ReadOption) (*Iterator, error) {
	switch src := src.(type) {
	case *Job:
		return src.Read(ctx, options...)
	case *Query:
		// For compatibility, support Query values created by literal, rather
		// than Client.Query.
		if src.client == nil {
			src.client = c
		}
		return src.Read(ctx, options...)
	case *Table:
		return src.Read(ctx, options...)
	}
	return nil, fmt.Errorf("src (%T) does not support the Read operation", src)
}

// ListTables returns a list of all the tables contained in the Dataset.
//
// Deprecated: use Dataset.Tables instead.
func (d *Dataset) ListTables(ctx context.Context) ([]*Table, error) {
	var tables []*Table

	err := getPages("", func(pageToken string) (string, error) {
		ts, tok, err := d.c.service.listTables(ctx, d.projectID, d.id, -1, pageToken)
		if err == nil {
			for _, t := range ts {
				t.c = d.c
				tables = append(tables, t)
			}
		}
		return tok, err
	})

	if err != nil {
		return nil, err
	}
	return tables, nil
}

type loadOption interface {
	customizeLoad(conf *bq.JobConfigurationLoad)
}

func (c *Client) load(ctx context.Context, dst *Table, src *GCSReference, options []Option) (*Job, error) {
	job, options := initJobProto(c.projectID, options)
	payload := &bq.JobConfigurationLoad{}

	dst.customizeLoadDst(payload)
	src.customizeLoadSrc(payload)

	for _, opt := range options {
		o, ok := opt.(loadOption)
		if !ok {
			return nil, fmt.Errorf("option (%#v) not applicable to dst/src pair: dst: %T ; src: %T", opt, dst, src)
		}
		o.customizeLoad(payload)
	}

	job.Configuration = &bq.JobConfiguration{
		Load: payload,
	}
	return c.service.insertJob(ctx, job, c.projectID)
}

// DestinationSchema returns an Option that specifies the schema to use when loading data into a new table.
// A DestinationSchema Option must be supplied when loading data from Google Cloud Storage into a non-existent table.
// Caveat: DestinationSchema is not required if the data being loaded is a datastore backup.
// schema must not be nil.
//
// Deprecated: use GCSReference.Schema instead.
func DestinationSchema(schema Schema) Option { return destSchema{Schema: schema} }

type destSchema struct {
	Schema
}

func (opt destSchema) implementsOption() {}

func (opt destSchema) customizeLoad(conf *bq.JobConfigurationLoad) {
	conf.Schema = opt.asTableSchema()
}

// MaxBadRecords returns an Option that sets the maximum number of bad records that will be ignored.
// If this maximum is exceeded, the operation will be unsuccessful.
//
// Deprecated: use GCSReference.MaxBadRecords instead.
func MaxBadRecords(n int64) Option { return maxBadRecords(n) }

type maxBadRecords int64

func (opt maxBadRecords) implementsOption() {}

func (opt maxBadRecords) customizeLoad(conf *bq.JobConfigurationLoad) {
	conf.MaxBadRecords = int64(opt)
}

// AllowJaggedRows returns an Option that causes missing trailing optional columns to be tolerated in CSV data.  Missing values are treated as nulls.
//
// Deprecated: use GCSReference.AllowJaggedRows instead.
func AllowJaggedRows() Option { return allowJaggedRows{} }

type allowJaggedRows struct{}

func (opt allowJaggedRows) implementsOption() {}

func (opt allowJaggedRows) customizeLoad(conf *bq.JobConfigurationLoad) {
	conf.AllowJaggedRows = true
}

// AllowQuotedNewlines returns an Option that allows quoted data sections containing newlines in CSV data.
//
// Deprecated: use GCSReference.AllowQuotedNewlines instead.
func AllowQuotedNewlines() Option { return allowQuotedNewlines{} }

type allowQuotedNewlines struct{}

func (opt allowQuotedNewlines) implementsOption() {}

func (opt allowQuotedNewlines) customizeLoad(conf *bq.JobConfigurationLoad) {
	conf.AllowQuotedNewlines = true
}

// IgnoreUnknownValues returns an Option that causes values not matching the schema to be tolerated.
// Unknown values are ignored. For CSV this ignores extra values at the end of a line.
// For JSON this ignores named values that do not match any column name.
// If this Option is not used, records containing unknown values are treated as bad records.
// The MaxBadRecords Option can be used to customize how bad records are handled.
//
// Deprecated: use GCSReference.IgnoreUnknownValues instead.
func IgnoreUnknownValues() Option { return ignoreUnknownValues{} }

type ignoreUnknownValues struct{}

func (opt ignoreUnknownValues) implementsOption() {}

func (opt ignoreUnknownValues) customizeLoad(conf *bq.JobConfigurationLoad) {
	conf.IgnoreUnknownValues = true
}

func (opt TableCreateDisposition) customizeLoad(conf *bq.JobConfigurationLoad) {
	conf.CreateDisposition = string(opt)
}

func (opt TableWriteDisposition) customizeLoad(conf *bq.JobConfigurationLoad) {
	conf.WriteDisposition = string(opt)
}
