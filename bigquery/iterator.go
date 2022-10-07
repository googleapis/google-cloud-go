// Copyright 2015 Google LLC
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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"

	bqStorage "cloud.google.com/go/bigquery/storage/apiv1"
	"github.com/apache/arrow/go/v10/arrow"
	"github.com/apache/arrow/go/v10/arrow/ipc"
	"github.com/apache/arrow/go/v10/arrow/memory"
	gax "github.com/googleapis/gax-go/v2"

	bq "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	bqStoragepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
	"google.golang.org/grpc"
)

// Construct a RowIterator.
func newRowIterator(ctx context.Context, src *rowSource, pf pageFetcher) *RowIterator {
	if src != nil && src.storage != nil {
		return newStorageAPIRowIterator(ctx, src)
	}
	it := &RowIterator{
		ctx: ctx,
		src: src,
		pf:  pf,
	}
	it.pageInfo, it.nextFunc = iterator.NewPageInfo(
		it.fetch,
		func() int { return len(it.rows) },
		func() interface{} { r := it.rows; it.rows = nil; return r })
	return it
}

// A RowIterator provides access to the result of a BigQuery lookup.
type RowIterator struct {
	ctx context.Context
	src *rowSource

	pageInfo *iterator.PageInfo
	nextFunc func() error
	pf       pageFetcher

	// StartIndex can be set before the first call to Next. If PageInfo().Token
	// is also set, StartIndex is ignored.
	StartIndex uint64

	// The schema of the table. Available after the first call to Next.
	Schema Schema

	// The total number of rows in the result. Available after the first call to Next.
	// May be zero just after rows were inserted.
	TotalRows uint64

	rows         [][]Value
	structLoader structLoader // used to populate a pointer to a struct
}

// SourceJob returns an instance of a Job if the RowIterator is backed by a query,
// or a nil.
func (ri *RowIterator) SourceJob() *Job {
	if ri.src == nil {
		return nil
	}
	if ri.src.j == nil {
		return nil
	}
	return &Job{
		c:         ri.src.j.c,
		projectID: ri.src.j.projectID,
		location:  ri.src.j.location,
		jobID:     ri.src.j.jobID,
	}
}

// We declare a function signature for fetching results.  The primary reason
// for this is to enable us to swap out the fetch function with alternate
// implementations (e.g. to enable testing).
type pageFetcher func(ctx context.Context, _ *rowSource, _ Schema, startIndex uint64, pageSize int64, pageToken string) (*fetchPageResult, error)

// Next loads the next row into dst. Its return value is iterator.Done if there
// are no more results. Once Next returns iterator.Done, all subsequent calls
// will return iterator.Done.
//
// dst may implement ValueLoader, or may be a *[]Value, *map[string]Value, or struct pointer.
//
// If dst is a *[]Value, it will be set to new []Value whose i'th element
// will be populated with the i'th column of the row.
//
// If dst is a *map[string]Value, a new map will be created if dst is nil. Then
// for each schema column name, the map key of that name will be set to the column's
// value. STRUCT types (RECORD types or nested schemas) become nested maps.
//
// If dst is pointer to a struct, each column in the schema will be matched
// with an exported field of the struct that has the same name, ignoring case.
// Unmatched schema columns and struct fields will be ignored.
//
// Each BigQuery column type corresponds to one or more Go types; a matching struct
// field must be of the correct type. The correspondences are:
//
//	STRING      string
//	BOOL        bool
//	INTEGER     int, int8, int16, int32, int64, uint8, uint16, uint32
//	FLOAT       float32, float64
//	BYTES       []byte
//	TIMESTAMP   time.Time
//	DATE        civil.Date
//	TIME        civil.Time
//	DATETIME    civil.DateTime
//	NUMERIC     *big.Rat
//	BIGNUMERIC  *big.Rat
//
// The big.Rat type supports numbers of arbitrary size and precision.
// See https://cloud.google.com/bigquery/docs/reference/standard-sql/data-types#numeric-type
// for more on NUMERIC.
//
// A repeated field corresponds to a slice or array of the element type. A STRUCT
// type (RECORD or nested schema) corresponds to a nested struct or struct pointer.
// All calls to Next on the same iterator must use the same struct type.
//
// It is an error to attempt to read a BigQuery NULL value into a struct field,
// unless the field is of type []byte or is one of the special Null types: NullInt64,
// NullFloat64, NullBool, NullString, NullTimestamp, NullDate, NullTime or
// NullDateTime. You can also use a *[]Value or *map[string]Value to read from a
// table with NULLs.
func (it *RowIterator) Next(dst interface{}) error {
	var vl ValueLoader
	switch dst := dst.(type) {
	case ValueLoader:
		vl = dst
	case *[]Value:
		vl = (*valueList)(dst)
	case *map[string]Value:
		vl = (*valueMap)(dst)
	default:
		if !isStructPtr(dst) {
			return fmt.Errorf("bigquery: cannot convert %T to ValueLoader (need pointer to []Value, map[string]Value, or struct)", dst)
		}
	}
	if err := it.nextFunc(); err != nil {
		return err
	}
	row := it.rows[0]
	it.rows = it.rows[1:]

	if vl == nil {
		// This can only happen if dst is a pointer to a struct. We couldn't
		// set vl above because we need the schema.
		if err := it.structLoader.set(dst, it.Schema); err != nil {
			return err
		}
		vl = &it.structLoader
	}
	return vl.Load(row, it.Schema)
}

func isStructPtr(x interface{}) bool {
	t := reflect.TypeOf(x)
	return t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Struct
}

// PageInfo supports pagination. See the google.golang.org/api/iterator package for details.
func (it *RowIterator) PageInfo() *iterator.PageInfo { return it.pageInfo }

func (it *RowIterator) fetch(pageSize int, pageToken string) (string, error) {
	res, err := it.pf(it.ctx, it.src, it.Schema, it.StartIndex, int64(pageSize), pageToken)
	if err != nil {
		return "", err
	}
	it.rows = append(it.rows, res.rows...)
	if it.Schema == nil {
		it.Schema = res.schema
	}
	it.TotalRows = res.totalRows
	return res.pageToken, nil
}

type streamIterator struct {
	done bool
	more chan struct{}
	errs chan error
	wg   sync.WaitGroup

	ctx context.Context
	src *rowSource

	table   *Table
	tableID string

	parser *arrowParser

	rowsLock sync.Mutex
	rowIt    *RowIterator
}

func newStorageAPIRowIterator(ctx context.Context, src *rowSource) *RowIterator {
	it := &RowIterator{
		ctx: ctx,
		src: src,
	}

	streamIt := &streamIterator{
		ctx:   ctx,
		src:   src,
		rowIt: it,
		more:  make(chan struct{}, 0),
		errs:  make(chan error, 0),
	}
	it.nextFunc = func() error {
		if err := streamIt.next(); err != nil {
			return err
		}
		return nil
	}
	return it
}

type arrowParser struct {
	mem            *memory.GoAllocator
	tableSchema    Schema
	rawArrowSchema []byte
	arrowSchema    *arrow.Schema
}

func newArrowParserFromSession(session *bqStoragepb.ReadSession, schema Schema) (*arrowParser, error) {
	arrowSerializedSchema := session.GetArrowSchema().GetSerializedSchema()
	mem := memory.NewGoAllocator()
	buf := bytes.NewBuffer(arrowSerializedSchema)
	r, err := ipc.NewReader(buf, ipc.WithAllocator(mem))
	if err != nil {
		return nil, err
	}

	p := &arrowParser{
		mem:            mem,
		tableSchema:    schema,
		rawArrowSchema: arrowSerializedSchema,
		arrowSchema:    r.Schema(),
	}
	return p, nil
}

func (it *streamIterator) init() error {
	if it.parser == nil { // Not initialized
		err := it.setup()
		if err != nil {
			return err
		}
		err = it.start()
		if err != nil {
			return err
		}
	}
	return nil
}

func (it *streamIterator) setup() error {
	table := it.src.t
	if it.src.j != nil {
		cfg, err := it.src.j.Config()
		if err != nil {
			return err
		}
		qcfg := cfg.(*QueryConfig)
		if qcfg.Dst == nil {
			// TODO: script job ?
			return fmt.Errorf("nil job destination table")
		}
		table = qcfg.Dst
	}
	tableID, err := table.Identifier(StorageAPIResourceID)
	if err != nil {
		return err
	}
	it.table = table
	it.tableID = tableID
	return nil
}

func (it *streamIterator) start() error {
	tableReadOptions := &bqStoragepb.ReadSession_TableReadOptions{
		SelectedFields: []string{},
	}
	createReadSessionRequest := &bqStoragepb.CreateReadSessionRequest{
		Parent: fmt.Sprintf("projects/%s", it.table.ProjectID),
		ReadSession: &bqStoragepb.ReadSession{
			Table:       it.tableID,
			DataFormat:  bqStoragepb.DataFormat_ARROW,
			ReadOptions: tableReadOptions,
		},
		MaxStreamCount: 4, // TODO: control when to open multiple streams
	}
	rpcOpts := gax.WithGRPCOptions(
		grpc.MaxCallRecvMsgSize(1024 * 1024 * 129), // TODO: why needs to be of this size
	)
	session, err := it.src.storage.CreateReadSession(it.ctx, createReadSessionRequest, rpcOpts)
	if err != nil {
		return err
	}

	if len(session.GetStreams()) == 0 {
		it.errs <- iterator.Done
		return nil
	}

	meta, err := it.table.Metadata(it.ctx)
	if err != nil {
		return err
	}
	schema := meta.Schema

	parser, err := newArrowParserFromSession(session, schema)
	if err != nil {
		return err
	}
	it.parser = parser

	go func() {
		it.wg.Wait()
		close(it.more)
		it.done = true
	}()

	for _, readStream := range session.GetStreams() {
		it.wg.Add(1)
		go it.processStream(readStream)
	}
	return nil
}

func (it *streamIterator) processStream(stream *bqStoragepb.ReadStream) {
	var offset int64
	for {
		rowStream, err := it.src.storage.ReadRows(it.ctx, &bqStoragepb.ReadRowsRequest{
			ReadStream: stream.Name,
			Offset:     offset,
		})
		if err != nil {
			it.errs <- err
		}
		for {
			r, err := rowStream.Recv()
			if err == io.EOF {
				it.wg.Done()
				return
			}
			if err != nil {
				it.errs <- err
			}
			rc := r.GetRowCount()
			if rc > 0 {
				offset += rc
				arrowRecords := r.GetArrowRecordBatch()
				rows, err := it.parser.convertArrowRows(arrowRecords)
				if err != nil {
					it.errs <- err
				}
				it.rowsLock.Lock()
				it.rowIt.rows = append(it.rowIt.rows, rows...)
				it.rowsLock.Unlock()
				it.more <- struct{}{}
			}
		}
	}
}

func (it *streamIterator) next() error {
	if err := it.init(); err != nil {
		return err
	}
	if len(it.rowIt.rows) > 0 {
		return nil
	}
	if it.done {
		return iterator.Done
	}
	select {
	case <-it.more:
		for range it.more {
		}
		return nil
	case err := <-it.errs:
		return err
	case <-it.ctx.Done():
		return it.ctx.Err()
	}
}

// rowSource represents one of the multiple sources of data for a row iterator.
// Rows can be read directly from a BigQuery table or from a job reference.
// If a job is present, that's treated as the authoritative source.
//
// rowSource can also cache results for special situations, primarily for the
// fast execution query path which can return status, rows, and schema all at
// once.  Our cache data expectations are as follows:
//
//   - We can only cache data from the start of a source.
//   - We need to cache schema, rows, and next page token to effective service
//     a request from cache.
//   - cache references are destroyed as soon as they're interrogated.  We don't
//     want to retain the data unnecessarily, and we expect that the backend
//     can always provide them if needed.
type rowSource struct {
	j       *Job
	t       *Table
	storage *bqStorage.BigQueryReadClient

	cachedRows      []*bq.TableRow
	cachedSchema    *bq.TableSchema
	cachedNextToken string
}

// fetchPageResult represents a page of rows returned from the backend.
type fetchPageResult struct {
	pageToken string
	rows      [][]Value
	totalRows uint64
	schema    Schema
}

// fetchPage is our generalized fetch mechanism.  It interrogates from cache, and
// then dispatches to either the appropriate job or table-based backend mechanism
// as needed.
func fetchPage(ctx context.Context, src *rowSource, schema Schema, startIndex uint64, pageSize int64, pageToken string) (*fetchPageResult, error) {
	result, err := fetchCachedPage(ctx, src, schema, startIndex, pageSize, pageToken)
	if err != nil {
		if err != errNoCacheData {
			// This likely means something more severe, like a problem with schema.
			return nil, err
		}
		// If we failed to fetch data from cache, invoke the appropriate service method.
		if src.j != nil {
			return fetchJobResultPage(ctx, src, schema, startIndex, pageSize, pageToken)
		}
		return fetchTableResultPage(ctx, src, schema, startIndex, pageSize, pageToken)
	}
	return result, nil
}

func fetchTableResultPage(ctx context.Context, src *rowSource, schema Schema, startIndex uint64, pageSize int64, pageToken string) (*fetchPageResult, error) {
	// Fetch the table schema in the background, if necessary.
	errc := make(chan error, 1)
	if schema != nil {
		errc <- nil
	} else {
		go func() {
			var bqt *bq.Table
			err := runWithRetry(ctx, func() (err error) {
				bqt, err = src.t.c.bqs.Tables.Get(src.t.ProjectID, src.t.DatasetID, src.t.TableID).
					Fields("schema").
					Context(ctx).
					Do()
				return err
			})
			if err == nil && bqt.Schema != nil {
				schema = bqToSchema(bqt.Schema)
			}
			errc <- err
		}()
	}
	call := src.t.c.bqs.Tabledata.List(src.t.ProjectID, src.t.DatasetID, src.t.TableID)
	setClientHeader(call.Header())
	if pageToken != "" {
		call.PageToken(pageToken)
	} else {
		call.StartIndex(startIndex)
	}
	if pageSize > 0 {
		call.MaxResults(pageSize)
	}
	var res *bq.TableDataList
	err := runWithRetry(ctx, func() (err error) {
		res, err = call.Context(ctx).Do()
		return err
	})
	if err != nil {
		return nil, err
	}
	err = <-errc
	if err != nil {
		return nil, err
	}
	rows, err := convertRows(res.Rows, schema)
	if err != nil {
		return nil, err
	}
	return &fetchPageResult{
		pageToken: res.PageToken,
		rows:      rows,
		totalRows: uint64(res.TotalRows),
		schema:    schema,
	}, nil
}

func fetchJobResultPage(ctx context.Context, src *rowSource, schema Schema, startIndex uint64, pageSize int64, pageToken string) (*fetchPageResult, error) {
	// reduce data transfered by leveraging api projections
	projectedFields := []googleapi.Field{"rows", "pageToken", "totalRows"}
	call := src.j.c.bqs.Jobs.GetQueryResults(src.j.projectID, src.j.jobID).Location(src.j.location)
	if schema == nil {
		// only project schema if we weren't supplied one.
		projectedFields = append(projectedFields, "schema")
	}
	call = call.Fields(projectedFields...)
	setClientHeader(call.Header())
	if pageToken != "" {
		call.PageToken(pageToken)
	} else {
		call.StartIndex(startIndex)
	}
	if pageSize > 0 {
		call.MaxResults(pageSize)
	}
	var res *bq.GetQueryResultsResponse
	err := runWithRetry(ctx, func() (err error) {
		res, err = call.Context(ctx).Do()
		return err
	})
	if err != nil {
		return nil, err
	}
	// Populate schema in the rowsource if it's missing
	if schema == nil {
		schema = bqToSchema(res.Schema)
	}
	rows, err := convertRows(res.Rows, schema)
	if err != nil {
		return nil, err
	}
	return &fetchPageResult{
		pageToken: res.PageToken,
		rows:      rows,
		totalRows: uint64(res.TotalRows),
		schema:    schema,
	}, nil
}

var errNoCacheData = errors.New("no rows in rowSource cache")

// fetchCachedPage attempts to service the first page of results.  For the jobs path specifically, we have an
// opportunity to fetch rows before the iterator is constructed, and thus serve that data as the first request
// without an unnecessary network round trip.
func fetchCachedPage(ctx context.Context, src *rowSource, schema Schema, startIndex uint64, pageSize int64, pageToken string) (*fetchPageResult, error) {
	// we have no cached data
	if src.cachedRows == nil {
		return nil, errNoCacheData
	}
	// we have no schema for decoding.  convert from the cached representation if available.
	if schema == nil {
		if src.cachedSchema == nil {
			// We can't progress with no schema, destroy references and return a miss.
			src.cachedRows = nil
			src.cachedNextToken = ""
			return nil, errNoCacheData
		}
		schema = bqToSchema(src.cachedSchema)
	}
	// Only serve from cache where we're confident we know someone's asking for the first page
	// without having to align data.
	//
	// Future consideration: we could service pagesizes smaller than the cache if we're willing to handle generation
	// of pageTokens for the cache.
	if pageToken == "" &&
		startIndex == 0 &&
		(pageSize == 0 || pageSize == int64(len(src.cachedRows))) {
		converted, err := convertRows(src.cachedRows, schema)
		if err != nil {
			// destroy cache references and return error
			src.cachedRows = nil
			src.cachedSchema = nil
			src.cachedNextToken = ""
			return nil, err
		}
		result := &fetchPageResult{
			pageToken: src.cachedNextToken,
			rows:      converted,
			schema:    schema,
			totalRows: uint64(len(converted)),
		}
		// clear cache references and return response.
		src.cachedRows = nil
		src.cachedSchema = nil
		src.cachedNextToken = ""
		return result, nil
	}
	// All other cases are invalid.  Destroy any cache references on the way out the door.
	src.cachedRows = nil
	src.cachedSchema = nil
	src.cachedNextToken = ""
	return nil, errNoCacheData
}
