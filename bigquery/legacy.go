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
		// Query used not to contain a QueryConfig. By moving its
		// top-level fields down into a QueryConfig field, we break
		// code that uses a Query literal.  If users make the minimal
		// change to fix this (e.g. moving the "Q" field into a nested
		// QueryConfig within the Query), they will end up with a Query
		// that has no Client.  It's preferable to make Read continue
		// to work in this case too, at least until we delete Read
		// completely. So we copy QueryConfig into a Query with an
		// actual client.
		if src.client == nil {
			src = &Query{
				client:           c,
				QueryConfig:      src.QueryConfig,
				Q:                src.Q,
				DefaultProjectID: src.DefaultProjectID,
				DefaultDatasetID: src.DefaultDatasetID,
				TableDefinitions: src.TableDefinitions,
			}
		}
		return src.Read(ctx, options...)
	case *QueryConfig:
		// For compatibility, support QueryConfig values created by literal, rather
		// than Client.Query.
		q := &Query{client: c, QueryConfig: *src}
		return q.Read(ctx, options...)
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

// CreateDisposition returns an Option that specifies the TableCreateDisposition to use.
// Deprecated: use the CreateDisposition field in Query, CopyConfig or LoadConfig instead.
func CreateDisposition(disp TableCreateDisposition) Option { return disp }

func (opt TableCreateDisposition) implementsOption() {}

func (opt TableCreateDisposition) customizeCopy(conf *bq.JobConfigurationTableCopy) {
	conf.CreateDisposition = string(opt)
}

func (opt TableCreateDisposition) customizeLoad(conf *bq.JobConfigurationLoad) {
	conf.CreateDisposition = string(opt)
}

func (opt TableCreateDisposition) customizeQuery(conf *bq.JobConfigurationQuery) {
	conf.CreateDisposition = string(opt)
}

// WriteDisposition returns an Option that specifies the TableWriteDisposition to use.
// Deprecated: use the WriteDisposition field in Query, CopyConfig or LoadConfig instead.
func WriteDisposition(disp TableWriteDisposition) Option { return disp }

func (opt TableWriteDisposition) implementsOption() {}

func (opt TableWriteDisposition) customizeCopy(conf *bq.JobConfigurationTableCopy) {
	conf.WriteDisposition = string(opt)
}

func (opt TableWriteDisposition) customizeLoad(conf *bq.JobConfigurationLoad) {
	conf.WriteDisposition = string(opt)
}

func (opt TableWriteDisposition) customizeQuery(conf *bq.JobConfigurationQuery) {
	conf.WriteDisposition = string(opt)
}

type extractOption interface {
	customizeExtract(conf *bq.JobConfigurationExtract)
}

// DisableHeader returns an Option that disables the printing of a header row in exported data.
//
// Deprecated: use Extractor.DisableHeader instead.
func DisableHeader() Option { return disableHeader{} }

type disableHeader struct{}

func (opt disableHeader) implementsOption() {}

func (opt disableHeader) customizeExtract(conf *bq.JobConfigurationExtract) {
	f := false
	conf.PrintHeader = &f
}

func (c *Client) extract(ctx context.Context, dst *GCSReference, src *Table, options []Option) (*Job, error) {
	job, options := initJobProto(c.projectID, options)
	payload := &bq.JobConfigurationExtract{}

	dst.customizeExtractDst(payload)
	src.customizeExtractSrc(payload)

	for _, opt := range options {
		o, ok := opt.(extractOption)
		if !ok {
			return nil, fmt.Errorf("option (%#v) not applicable to dst/src pair: dst: %T ; src: %T", opt, dst, src)
		}
		o.customizeExtract(payload)
	}

	job.Configuration = &bq.JobConfiguration{
		Extract: payload,
	}
	return c.service.insertJob(ctx, job, c.projectID)
}

type copyOption interface {
	customizeCopy(conf *bq.JobConfigurationTableCopy)
}

func (c *Client) cp(ctx context.Context, dst *Table, src Tables, options []Option) (*Job, error) {
	job, options := initJobProto(c.projectID, options)
	payload := &bq.JobConfigurationTableCopy{}

	dst.customizeCopyDst(payload)
	src.customizeCopySrc(payload)

	for _, opt := range options {
		o, ok := opt.(copyOption)
		if !ok {
			return nil, fmt.Errorf("option (%#v) not applicable to dst/src pair: dst: %T ; src: %T", opt, dst, src)
		}
		o.customizeCopy(payload)
	}

	job.Configuration = &bq.JobConfiguration{
		Copy: payload,
	}
	return c.service.insertJob(ctx, job, c.projectID)
}

// initJobProto creates and returns a bigquery Job proto.
// The proto is customized using any jobOptions in options.
// The list of Options is returned with the jobOptions removed.
func initJobProto(projectID string, options []Option) (*bq.Job, []Option) {
	job := &bq.Job{}

	var other []Option
	for _, opt := range options {
		if o, ok := opt.(jobOption); ok {
			o.customizeJob(job, projectID)
		} else {
			other = append(other, opt)
		}
	}
	return job, other
}

// Copy starts a BigQuery operation to copy data from a Source to a Destination.
//
// Deprecated: use one of Table.LoaderFrom, Table.CopierFrom, Table.ExtractorTo, or
// Client.Query.
func (c *Client) Copy(ctx context.Context, dst Destination, src Source, options ...Option) (*Job, error) {
	switch dst := dst.(type) {
	case *Table:
		switch src := src.(type) {
		case *GCSReference:
			return c.load(ctx, dst, src, options)
		case *Table:
			return c.cp(ctx, dst, Tables{src}, options)
		case Tables:
			return c.cp(ctx, dst, src, options)
		case *Query:
			return c.query(ctx, dst, src, options)
		case *QueryConfig:
			q := &Query{QueryConfig: *src}
			return c.query(ctx, dst, q, options)
		}
	case *GCSReference:
		if src, ok := src.(*Table); ok {
			return c.extract(ctx, dst, src, options)
		}
	}
	return nil, fmt.Errorf("no Copy operation matches dst/src pair: dst: %T ; src: %T", dst, src)
}

// A Source is a source of data for the Copy function.
type Source interface {
	implementsSource()
}

// A Destination is a destination of data for the Copy function.
type Destination interface {
	implementsDestination()
}

// A ReadSource is a source of data for the Read function.
type ReadSource interface {
	implementsReadSource()
}

type queryOption interface {
	customizeQuery(conf *bq.JobConfigurationQuery)
}

// DisableQueryCache returns an Option that prevents results being fetched from the query cache.
// If this Option is not used, results are fetched from the cache if they are available.
// The query cache is a best-effort cache that is flushed whenever tables in the query are modified.
// Cached results are only available when TableID is unspecified in the query's destination Table.
// For more information, see https://cloud.google.com/bigquery/querying-data#querycaching
//
// Deprecated: use Query.DisableQueryCache instead.
func DisableQueryCache() Option { return disableQueryCache{} }

type disableQueryCache struct{}

func (opt disableQueryCache) implementsOption() {}

func (opt disableQueryCache) customizeQuery(conf *bq.JobConfigurationQuery) {
	f := false
	conf.UseQueryCache = &f
}

// DisableFlattenedResults returns an Option that prevents results being flattened.
// If this Option is not used, results from nested and repeated fields are flattened.
// DisableFlattenedResults implies AllowLargeResults
// For more information, see https://cloud.google.com/bigquery/docs/data#nested
// Deprecated: use Query.DisableFlattenedResults instead.
func DisableFlattenedResults() Option { return disableFlattenedResults{} }

type disableFlattenedResults struct{}

func (opt disableFlattenedResults) implementsOption() {}

func (opt disableFlattenedResults) customizeQuery(conf *bq.JobConfigurationQuery) {
	f := false
	conf.FlattenResults = &f
	// DisableFlattenedResults implies AllowLargeResults
	allowLargeResults{}.customizeQuery(conf)
}

// AllowLargeResults returns an Option that allows the query to produce arbitrarily large result tables.
// The destination must be a table.
// When using this option, queries will take longer to execute, even if the result set is small.
// For additional limitations, see https://cloud.google.com/bigquery/querying-data#largequeryresults
// Deprecated: use Query.AllowLargeResults instead.
func AllowLargeResults() Option { return allowLargeResults{} }

type allowLargeResults struct{}

func (opt allowLargeResults) implementsOption() {}

func (opt allowLargeResults) customizeQuery(conf *bq.JobConfigurationQuery) {
	conf.AllowLargeResults = true
}

// JobPriority returns an Option that causes a query to be scheduled with the specified priority.
// The default priority is InteractivePriority.
// For more information, see https://cloud.google.com/bigquery/querying-data#batchqueries
// Deprecated: use Query.Priority instead.
func JobPriority(priority string) Option { return jobPriority(priority) }

type jobPriority string

func (opt jobPriority) implementsOption() {}

func (opt jobPriority) customizeQuery(conf *bq.JobConfigurationQuery) {
	conf.Priority = string(opt)
}

// MaxBillingTier returns an Option that sets the maximum billing tier for a Query.
// Queries that have resource usage beyond this tier will fail (without
// incurring a charge). If this Option is not used, the project default will be used.
// Deprecated: use Query.MaxBillingTier instead.
func MaxBillingTier(tier int) Option { return maxBillingTier(tier) }

type maxBillingTier int

func (opt maxBillingTier) implementsOption() {}

func (opt maxBillingTier) customizeQuery(conf *bq.JobConfigurationQuery) {
	tier := int64(opt)
	conf.MaximumBillingTier = &tier
}

// MaxBytesBilled returns an Option that limits the number of bytes billed for
// this job.  Queries that would exceed this limit will fail (without incurring
// a charge).
// If this Option is not used, or bytes is < 1, the project default will be
// used.
// Deprecated: use Query.MaxBytesBilled instead.
func MaxBytesBilled(bytes int64) Option { return maxBytesBilled(bytes) }

type maxBytesBilled int64

func (opt maxBytesBilled) implementsOption() {}

func (opt maxBytesBilled) customizeQuery(conf *bq.JobConfigurationQuery) {
	if opt >= 1 {
		conf.MaximumBytesBilled = int64(opt)
	}
}

// QueryUseStandardSQL returns an Option that set the query to use standard SQL.
// The default setting is false (using legacy SQL).
// Deprecated: use Query.UseStandardSQL instead.
func QueryUseStandardSQL() Option { return queryUseStandardSQL{} }

type queryUseStandardSQL struct{}

func (opt queryUseStandardSQL) implementsOption() {}

func (opt queryUseStandardSQL) customizeQuery(conf *bq.JobConfigurationQuery) {
	conf.UseLegacySql = false
	conf.ForceSendFields = append(conf.ForceSendFields, "UseLegacySql")
}

func (c *Client) query(ctx context.Context, dst *Table, src *Query, options []Option) (*Job, error) {
	job, options := initJobProto(c.projectID, options)
	payload := &bq.JobConfigurationQuery{}

	dst.customizeQueryDst(payload)

	// QueryConfig now contains a Dst field.  If it is set, it will override dst.
	// This should not affect existing client code which does not set QueryConfig.Dst.
	src.QueryConfig.customizeQuerySrc(payload)

	// For compatability, allow some legacy fields to be set directly on the query.
	// TODO(jba): delete this code when deleting Client.Copy.
	if src.Q != "" {
		payload.Query = src.Q
	}
	if src.DefaultProjectID != "" || src.DefaultDatasetID != "" {
		payload.DefaultDataset = &bq.DatasetReference{
			DatasetId: src.DefaultDatasetID,
			ProjectId: src.DefaultProjectID,
		}
	}

	if len(src.TableDefinitions) > 0 {
		payload.TableDefinitions = make(map[string]bq.ExternalDataConfiguration)
	}
	for name, data := range src.TableDefinitions {
		payload.TableDefinitions[name] = data.externalDataConfig()
	}
	// end of compatability code.

	for _, opt := range options {
		o, ok := opt.(queryOption)
		if !ok {
			return nil, fmt.Errorf("option (%#v) not applicable to dst/src pair: dst: %T ; src: %T", opt, dst, src)
		}
		o.customizeQuery(payload)
	}

	job.Configuration = &bq.JobConfiguration{
		Query: payload,
	}
	j, err := c.service.insertJob(ctx, job, c.projectID)
	if err != nil {
		return nil, err
	}
	j.isQuery = true
	return j, nil
}

// Read submits a query for execution and returns the results via an Iterator.
// Deprecated: Call Read on the Job returned by Query.Run instead.
func (q *Query) Read(ctx context.Context, options ...ReadOption) (*Iterator, error) {
	job, err := q.Run(ctx)
	if err != nil {
		return nil, err
	}
	return job.Read(ctx, options...)
}
