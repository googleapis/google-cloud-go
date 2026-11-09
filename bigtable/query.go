// Copyright 2026 Google LLC
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

package bigtable

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"sync"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	metrics "cloud.google.com/go/bigtable/internal/metrics"
	"cloud.google.com/go/internal/trace"
	gax "github.com/googleapis/gax-go/v2"
	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	queryExpiredViolationType        = "PREPARED_QUERY_EXPIRED"
	preparedQueryExpireEarlyDuration = time.Second
	preparedQueryRefreshTimeout      = 30 * time.Second
	maxPlanRefreshRetries            = 3
)

// PreparedStatement stores the results of query preparation that can be used to
// create [BoundStatements]s to execute queries.
//
// Whenever possible this should be shared across different instances of the same query,
// in order to amortize query preparation costs.
type PreparedStatement struct {
	c          *Client
	query      string
	paramTypes map[string]SQLType
	opts       []PrepareOption

	refreshMutex sync.Mutex
	data         *preparedQueryData
	inflight     *inflightRefresh
}

type inflightRefresh struct {
	done chan struct{}
	data *preparedQueryData
	err  error
}

type preparedQueryData struct {
	// Structure of rows in the response stream of `ExecuteQueryResponse` for the
	// returned `prepared_query`.
	metadata *btpb.ResultSetMetadata
	// A serialized prepared query. It is an opaque
	// blob of bytes to send in `ExecuteQueryRequest`.
	preparedQuery []byte
	// The time at which the prepared query token becomes invalid.
	// A token may become invalid early due to changes in the data being read, but
	// it provides a guideline to refresh query plans asynchronously.
	validUntil *timestamppb.Timestamp
	// expired is set under PreparedStatement.refreshMutex when the server reports
	// PREPARED_QUERY_EXPIRED for this plan before validUntil has elapsed.
	expired bool

	Metadata *ResultRowMetadata
}

func (pqd *preparedQueryData) initializeMetadataAndMap() error {
	rrMetadata, err := newResultRowMetadata(pqd.metadata)
	if err != nil {
		return err
	}
	pqd.Metadata = rrMetadata
	return nil
}

// PrepareOption can be passed while preparing a query statement.
type PrepareOption interface{}

// PrepareStatement prepares a query for execution. If possible, this should be called once and
// reused across requests. This will amortize the cost of query preparation.
//
// The query string can be a parameterized query containing placeholders in the form of @ followed by the parameter name
// Parameter names may consist of any combination of letters, numbers, and underscores.
//
// Parameters can appear anywhere that a literal value is expected. The same parameter name can
// be used more than once, for example: WHERE cf["qualifier1"] = @value OR cf["qualifier2"] = @value
func (c *Client) PrepareStatement(ctx context.Context, query string, paramTypes map[string]SQLType, opts ...PrepareOption) (preparedStatement *PreparedStatement, err error) {
	md := metadata.Join(metadata.Pairs(
		resourcePrefixHeader, c.fullInstanceName(),
		requestParamsHeader, c.reqParamsHeaderValInstance(),
	), c.featureFlagsMD)

	ctx = mergeOutgoingMetadata(ctx, md)
	return c.prepareStatementWithMetadata(ctx, query, paramTypes, opts...)
}

// Called when context already has the required metadata
func (c *Client) prepareStatementWithMetadata(ctx context.Context, query string, paramTypes map[string]SQLType, opts ...PrepareOption) (preparedStatement *PreparedStatement, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/bigtable.PrepareQuery")
	defer func() { trace.EndSpan(ctx, err) }()

	mt := c.newBuiltinMetricsTracer(ctx, "", false)
	defer mt.RecordOperationCompletion()
	ctx = metrics.NewContext(ctx, mt)

	preparedStatement, err = c.prepareStatement(ctx, query, paramTypes, opts...)
	statusCode, statusErr := metrics.ConvertToGrpcStatusErr(err)
	mt.SetCurrOpStatus(statusCode)
	return preparedStatement, statusErr
}

func (c *Client) prepareStatement(ctx context.Context, query string, paramTypes map[string]SQLType, opts ...PrepareOption) (*PreparedStatement, error) {
	reqParamTypes := map[string]*btpb.Type{}
	for k, v := range paramTypes {
		if v == nil {
			return nil, errors.New("bigtable: invalid SQLType: nil")
		}
		if !v.isValidPrepareParamType() {
			return nil, fmt.Errorf("bigtable: %T cannot be used as parameter type", v)
		}
		tpb, err := v.typeProto()
		if err != nil {
			return nil, err
		}
		reqParamTypes[k] = tpb
	}
	req := &btpb.PrepareQueryRequest{
		InstanceName: c.fullInstanceName(),
		AppProfileId: c.appProfile,
		Query:        query,
		DataFormat: &btpb.PrepareQueryRequest_ProtoFormat{
			ProtoFormat: &btpb.ProtoFormat{},
		},
		ParamTypes: reqParamTypes,
	}
	var res *btpb.PrepareQueryResponse
	err := gaxInvokeWithRecorder(ctx, "PrepareQuery", func(ctx context.Context, headerMD, trailerMD *metadata.MD, _ gax.CallSettings) error {
		var err error
		res, err = c.client.PrepareQuery(ctx, req, grpc.Header(headerMD), grpc.Trailer(trailerMD))
		return err
	}, c.retryOption)
	if err != nil {
		return nil, err
	}

	pqd := &preparedQueryData{
		metadata:      res.Metadata,
		preparedQuery: res.PreparedQuery,
		validUntil:    res.ValidUntil,
	}
	if err := pqd.initializeMetadataAndMap(); err != nil {
		return nil, err
	}

	return &PreparedStatement{
		c:          c,
		data:       pqd,
		query:      query,
		paramTypes: paramTypes,
		opts:       opts,
	}, nil
}

// Bind binds a set of parameters to a prepared statement.
//
// Allowed parameter value types are []byte, string, int64, float32, float64, bool,
// time.Time, civil.Date, array, slice and nil
func (ps *PreparedStatement) Bind(values map[string]any) (*BoundStatement, error) {
	if ps == nil {
		return nil, errors.New("bigtable: nil prepared statement")
	}
	// check that every parameter is bound
	for paramName := range ps.paramTypes {
		_, found := values[paramName]
		if !found {
			return nil, fmt.Errorf("bigtable: parameter %q not bound in call to Bind", paramName)
		}
	}

	boundParams := map[string]*btpb.Value{}
	for paramName, paramVal := range values {
		// Validate that the parameter was specified during prepare
		psType, found := ps.paramTypes[paramName]
		if !found {
			return nil, errors.New("bigtable: no parameter with name " + paramName + " in prepared statement")
		}

		// Convert value specified by user to *btpb.Value
		pbVal, err := anySQLTypeToPbVal(paramVal, psType)
		if err != nil {
			return nil, err
		}
		boundParams[paramName] = pbVal
	}

	return &BoundStatement{
		ps:     ps,
		params: boundParams,
	}, nil
}

// Metadata returns the ResultRowMetadata describing the columns of the prepared query.
func (ps *PreparedStatement) Metadata() *ResultRowMetadata {
	ps.refreshMutex.Lock()
	defer ps.refreshMutex.Unlock()
	return ps.data.Metadata
}

// valid is true if the prepared query is valid at now, and validEarly is true
// if the prepared query is valid and has not reached the early expiration threshold.
func (pqd *preparedQueryData) valid(now time.Time) (valid bool, validEarly bool) {
	if pqd == nil || pqd.expired || pqd.validUntil == nil {
		return false, false
	}
	expireTime := pqd.validUntil.AsTime()
	return now.Before(expireTime), now.Add(preparedQueryExpireEarlyDuration).Before(expireTime)
}

func (ps *PreparedStatement) planForAttempt(
	baseCtx, attemptCtx context.Context,
	usedPlan *preparedQueryData,
	forceRefresh bool,
) (*preparedQueryData, error) {
	/*
	   | valid | validEarly | behaviour            |
	   |-------|------------|----------------------|
	   | true  |   true     | nil                  |
	   | false |   true     | impossible condition |
	   | true  |   false    | async refresh token  |
	   | false |   false    | sync refresh token   |
	*/
	now := time.Now().UTC()

	ps.refreshMutex.Lock()
	cur := ps.data
	if forceRefresh && (usedPlan == nil || cur == usedPlan) && cur != nil {
		cur.expired = true
	}
	valid, validEarly := cur.valid(now)

	if !forceRefresh {
		if validEarly {
			// Token valid
			ps.refreshMutex.Unlock()
			return cur, nil
		}
		if valid {
			// Token about to expire: trigger background refresh if not already running,
			// and return cur immediately without blocking the caller.
			ps.startRefreshLocked(baseCtx)
			ps.refreshMutex.Unlock()
			return cur, nil
		}
	} else if usedPlan != nil && cur != usedPlan && valid {
		// Another goroutine already refreshed the expired plan usedPlan.
		if !validEarly {
			ps.startRefreshLocked(baseCtx)
		}
		ps.refreshMutex.Unlock()
		return cur, nil
	}

	// Synchronous refresh required (either !valid or forceRefresh on cur == usedPlan).
	if err := attemptCtx.Err(); err != nil {
		ps.refreshMutex.Unlock()
		return nil, err
	}
	call := ps.startRefreshLocked(baseCtx)
	ps.refreshMutex.Unlock()

	select {
	case <-call.done:
		return call.data, call.err
	case <-attemptCtx.Done():
		return nil, attemptCtx.Err()
	}
}

func (ps *PreparedStatement) startRefreshLocked(baseCtx context.Context) *inflightRefresh {
	if ps.inflight != nil {
		return ps.inflight
	}
	call := &inflightRefresh{done: make(chan struct{})}
	ps.inflight = call

	detachedCtx := context.WithoutCancel(baseCtx)
	go func() {
		refreshCtx, cancel := context.WithTimeout(detachedCtx, preparedQueryRefreshTimeout)
		defer cancel()

		newPs, err := ps.c.prepareStatementWithMetadata(refreshCtx, ps.query, ps.paramTypes, ps.opts...)

		ps.refreshMutex.Lock()
		if err == nil {
			ps.data = newPs.data
			call.data = newPs.data
		} else {
			call.err = err
		}
		ps.inflight = nil
		ps.refreshMutex.Unlock()
		close(call.done)
	}()
	return call
}

type planRefreshError struct {
	err       error
	retryable bool
}

func (e *planRefreshError) Error() string { return e.err.Error() }

// BoundStatement is a statement that has been bound to a set of parameters.
// It is created by calling [PreparedStatement.Bind].
type BoundStatement struct {
	ps     *PreparedStatement
	params map[string]*btpb.Value
}

// ExecuteOption is an optional argument to Execute.
type ExecuteOption interface{}

// Execute executes a previously prepared query. f is called for each row in result set.
// If f returns false, the stream is shut down and Execute returns.
// f owns its argument, and f is called serially in order of results returned.
// f will be executed in the same Go routine as the caller.
func (bs *BoundStatement) Execute(ctx context.Context, f func(ResultRow) bool, opts ...ExecuteOption) (err error) {
	md := metadata.Join(metadata.Pairs(
		resourcePrefixHeader, bs.ps.c.fullInstanceName(),
		requestParamsHeader, bs.ps.c.reqParamsHeaderValInstance(),
	), bs.ps.c.featureFlagsMD)
	ctx = mergeOutgoingMetadata(ctx, md)

	ctx = trace.StartSpan(ctx, "cloud.google.com/go/bigtable.ExecuteQuery")
	defer func() { trace.EndSpan(ctx, err) }()

	mt := bs.ps.c.newBuiltinMetricsTracer(ctx, "", true)
	defer mt.RecordOperationCompletion()
	ctx = metrics.NewContext(ctx, mt)

	err = bs.execute(ctx, f)
	statusCode, statusErr := metrics.ConvertToGrpcStatusErr(err)
	mt.SetCurrOpStatus(statusCode)
	return statusErr
}

func (bs *BoundStatement) execute(execCtx context.Context, f func(ResultRow) bool) error {
	// buffer data constructed from the fields in PartialRows`
	var ongoingResultBatch bytes.Buffer

	// data buffered since the last non-empty `ResumeToken`
	valuesBuffer := []*btpb.Value{}

	var resumeToken []byte

	receivedResumeToken := false
	needsPlanRefresh := false
	planRefreshRetries := 0
	var prevError error

	mt := metrics.FromContext(execCtx)
	firstResponseRecorded := false

	// Metadata could change on planned query refresh.
	// E.g.
	// 1. 'SELECT *' request with ps started at t1
	// 2. A column family is added to the table
	// 3. Some other request triggers refresh of ps at t2
	// 4. If the metadata from the refreshed ps at t2 is used, metadata contains the new column family,
	//    the responses do not (because the request used the plan from t1)`
	//
	// So, do not use latest metadata from `bs.ps`
	var finalizedStmt *preparedQueryData
	var candFinalizedStmt *preparedQueryData

	err := gaxInvokeWithRecorder(execCtx, "ExecuteQuery", func(attemptCtx context.Context, headerMD, trailerMD *metadata.MD, _ gax.CallSettings) error {
		attemptCtx, cancel := context.WithCancel(attemptCtx) // for aborting the stream
		defer cancel()

		ongoingResultBatch.Reset()
		valuesBuffer = nil

		if !receivedResumeToken {
			// Once we have a resume token we need the prepared query to never change.
			// The Bigtable service will only send the query expired error for requests without a token
			// (before sending any responses).
			// We don't want the plan to change on a transient error once we've already received a token.
			if isQueryExpiredViolation(prevError) {
				needsPlanRefresh = true
			}
			nextStmt, err := bs.ps.planForAttempt(execCtx, attemptCtx, candFinalizedStmt, needsPlanRefresh)
			if err != nil {
				if attemptCtx.Err() == nil {
					retryable := planRefreshRetries < maxPlanRefreshRetries
					planRefreshRetries++
					err = &planRefreshError{err: err, retryable: retryable}
				}
				prevError = err
				return err
			}
			candFinalizedStmt = nextStmt
			needsPlanRefresh = false
			planRefreshRetries = 0
		} else {
			candFinalizedStmt = finalizedStmt
		}

		req := &btpb.ExecuteQueryRequest{
			InstanceName:  bs.ps.c.fullInstanceName(),
			AppProfileId:  bs.ps.c.appProfile,
			PreparedQuery: candFinalizedStmt.preparedQuery,
			Params:        bs.params,
			ResumeToken:   resumeToken,
		}
		stream, err := bs.ps.c.client.ExecuteQuery(attemptCtx, req)
		if err != nil {
			if receivedResumeToken && isQueryExpiredViolation(err) {
				err = status.Error(codes.Internal, "bigtable: Query plan expired during a retry attempt")
			}
			prevError = err
			return err
		}

		// Ignore error since header is only being used to record builtin metrics
		// Failure to record metrics should not fail the operation
		*headerMD, _ = stream.Header()
		eqResp := new(btpb.ExecuteQueryResponse)
		for {
			proto.Reset(eqResp)
			err := stream.RecvMsg(eqResp)
			if !firstResponseRecorded && (err == nil || err == io.EOF) {
				firstResponseRecorded = true
				mt.SetFirstRespTime(time.Now())
			}
			if err != nil {
				if receivedResumeToken && isQueryExpiredViolation(err) {
					err = status.Error(codes.Internal, "bigtable: Query plan expired during a retry attempt")
				}
				return handleExecuteStreamEnd(stream, trailerMD, &ongoingResultBatch, valuesBuffer, err, &prevError)
			}

			resp := eqResp.GetResponse()
			results, ok := resp.(*btpb.ExecuteQueryResponse_Results)
			if !ok || results == nil || results.Results == nil {
				prevError = status.Error(codes.Internal, "bigtable: unexpected response type")
				return prevError
			}

			partialResultSet := results.Results
			if partialResultSet.GetReset_() {
				valuesBuffer = []*btpb.Value{}
				ongoingResultBatch.Reset()
			}

			var batchData []byte
			if partialResultSet.GetProtoRowsBatch() != nil {
				batchData = partialResultSet.GetProtoRowsBatch().GetBatchData()
				ongoingResultBatch.Write(batchData)
			}

			// Validate checksum if exists
			var protoRows *btpb.ProtoRows
			if partialResultSet.BatchChecksum != nil {
				// Current batch is now complete

				// Validate checksum
				currBatchChecksum := crc32.Checksum(ongoingResultBatch.Bytes(), crc32cTable)
				if *partialResultSet.BatchChecksum != currBatchChecksum {
					prevError = status.Error(codes.Internal, "bigtable: batch_checksum mismatch")
					return prevError
				}

				// Parse the batch
				protoRows = new(btpb.ProtoRows)
				if err := proto.Unmarshal(ongoingResultBatch.Bytes(), protoRows); err != nil {
					prevError = status.Errorf(codes.Internal, "bigtable: %v", err)
					return prevError
				}
				valuesBuffer = append(valuesBuffer, protoRows.GetValues()...)

				// Prepare to receive next batch of results
				ongoingResultBatch.Reset()
			}
			if len(partialResultSet.GetResumeToken()) > 0 {
				// Values can be yielded to the caller

				// If `resume_token` is non-empty and any data has been received since the
				// last one, BatchChecksum is guaranteed to be non-empty. In other words, a batch will
				// never cross a `resume_token` boundary. It is an error otherwise
				if ongoingResultBatch.Len() != 0 &&
					partialResultSet.BatchChecksum == nil {
					prevError = status.Error(codes.Internal, "bigtable: received resume_token with buffered data and no batch_checksum")
					return prevError
				}

				if !receivedResumeToken {
					// first ResumeToken received
					finalizedStmt = candFinalizedStmt
					receivedResumeToken = true
				}

				// Save ResumeToken for subsequent requests
				resumeToken = partialResultSet.GetResumeToken()

				if finalizedStmt.metadata == nil || finalizedStmt.metadata.GetProtoSchema() == nil || finalizedStmt.Metadata == nil {
					prevError = status.Error(codes.Internal, "bigtable: metadata missing")
					return prevError
				}
				cols := finalizedStmt.metadata.GetProtoSchema().GetColumns()
				numCols := len(cols)
				if numCols == 0 {
					prevError = status.Error(codes.Internal, "bigtable: columns cannot be empty")
					return prevError
				}

				// Parse rows
				for len(valuesBuffer) != 0 {
					var completeRowValues []*btpb.Value

					// Pop first 'numCols' values to create a row
					if len(valuesBuffer) < numCols {
						prevError = status.Errorf(codes.Internal, "bigtable: metadata and data mismatch: %d columns in metadata but received %d values", numCols, len(valuesBuffer))
						return prevError
					}

					completeRowValues, valuesBuffer = valuesBuffer[0:numCols], valuesBuffer[numCols:]

					// Construct ResultRow
					rr, err := newResultRow(completeRowValues, finalizedStmt.metadata, finalizedStmt.Metadata)
					if err != nil {
						prevError = err
						return err
					}
					appBlockingLatencyStart := time.Now()
					continueReading := f(*rr)
					mt.IncrementAppBlockingLatency(metrics.ConvertToMs(time.Since(appBlockingLatencyStart)))
					if !continueReading {
						// Cancel and drain stream.
						cancel()
						for {
							proto.Reset(eqResp)
							if err := stream.RecvMsg(eqResp); err != nil {
								handleExecuteStreamEnd(stream, trailerMD, &ongoingResultBatch, valuesBuffer, err, &prevError)
								// The stream has ended. We don't return an error
								// because the caller has intentionally interrupted the scan.
								return nil
							}
						}
					}
				}
			}
		}
	}, bs.ps.c.executeQueryRetryOption)
	if err != nil {
		var refreshErr *planRefreshError
		if errors.As(err, &refreshErr) {
			return refreshErr.err
		}
		return err
	}
	return nil
}

func handleExecuteStreamEnd(
	stream btpb.Bigtable_ExecuteQueryClient,
	trailerMD *metadata.MD,
	ongoingResultBatch *bytes.Buffer,
	valuesBuffer []*btpb.Value,
	err error,
	prevError *error,
) error {
	if stream != nil {
		*trailerMD = stream.Trailer()
	}
	if err != nil && err != io.EOF {
		*prevError = err
		return err
	}
	if len(valuesBuffer) != 0 || ongoingResultBatch.Len() != 0 {
		err = status.Error(codes.Internal, "bigtable: server stream ended without sending a resume token")
		*prevError = err
		return err
	}
	*prevError = nil
	return nil
}

func clientOnlyExecuteQueryRetry(backoff *gax.Backoff, err error) (time.Duration, bool) {
	var refreshErr *planRefreshError
	if errors.As(err, &refreshErr) {
		if !refreshErr.retryable {
			return 0, false
		}
		return backoff.Pause(), true
	}
	if isQueryExpiredViolation(err) {
		return backoff.Pause(), true
	}
	return clientOnlyRetry(backoff, err)
}

func isQueryExpiredViolation(err error) bool {
	apiErr, ok := apierror.FromError(err)
	if ok && apiErr != nil && apiErr.Details().PreconditionFailure != nil && status.Code(err) == codes.FailedPrecondition {
		for _, violation := range apiErr.Details().PreconditionFailure.GetViolations() {
			if violation != nil && violation.GetType() == queryExpiredViolationType {
				return true
			}
		}
	}
	return false
}
