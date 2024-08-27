/*
Copyright 2024 Google LLC

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

package spanner

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"

	vkit "cloud.google.com/go/spanner/apiv1"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal"
)

const (
	directPathIPV6Prefix = "[2001:4860:8040"
	directPathIPV4Prefix = "34.126"
)

// spannerClient is an interface that defines the methods available from Cloud Spanner API.
type spannerClient interface {
	CallOptions() *vkit.CallOptions
	Close() error
	setGoogleClientInfo(...string)
	Connection() *grpc.ClientConn
	CreateSession(context.Context, *spannerpb.CreateSessionRequest, ...gax.CallOption) (*spannerpb.Session, error)
	BatchCreateSessions(context.Context, *spannerpb.BatchCreateSessionsRequest, ...gax.CallOption) (*spannerpb.BatchCreateSessionsResponse, error)
	GetSession(context.Context, *spannerpb.GetSessionRequest, ...gax.CallOption) (*spannerpb.Session, error)
	ListSessions(context.Context, *spannerpb.ListSessionsRequest, ...gax.CallOption) *vkit.SessionIterator
	DeleteSession(context.Context, *spannerpb.DeleteSessionRequest, ...gax.CallOption) error
	ExecuteSql(context.Context, *spannerpb.ExecuteSqlRequest, ...gax.CallOption) (*spannerpb.ResultSet, error)
	ExecuteStreamingSql(context.Context, *spannerpb.ExecuteSqlRequest, ...gax.CallOption) (spannerpb.Spanner_ExecuteStreamingSqlClient, error)
	ExecuteBatchDml(context.Context, *spannerpb.ExecuteBatchDmlRequest, ...gax.CallOption) (*spannerpb.ExecuteBatchDmlResponse, error)
	Read(context.Context, *spannerpb.ReadRequest, ...gax.CallOption) (*spannerpb.ResultSet, error)
	StreamingRead(context.Context, *spannerpb.ReadRequest, ...gax.CallOption) (spannerpb.Spanner_StreamingReadClient, error)
	BeginTransaction(context.Context, *spannerpb.BeginTransactionRequest, ...gax.CallOption) (*spannerpb.Transaction, error)
	Commit(context.Context, *spannerpb.CommitRequest, ...gax.CallOption) (*spannerpb.CommitResponse, error)
	Rollback(context.Context, *spannerpb.RollbackRequest, ...gax.CallOption) error
	PartitionQuery(context.Context, *spannerpb.PartitionQueryRequest, ...gax.CallOption) (*spannerpb.PartitionResponse, error)
	PartitionRead(context.Context, *spannerpb.PartitionReadRequest, ...gax.CallOption) (*spannerpb.PartitionResponse, error)
	BatchWrite(context.Context, *spannerpb.BatchWriteRequest, ...gax.CallOption) (spannerpb.Spanner_BatchWriteClient, error)
}

// grpcSpannerClient is the gRPC API implementation of the transport-agnostic
// spannerClient interface.
type grpcSpannerClient struct {
	raw                  *vkit.Client
	connPool             gtransport.ConnPool
	client               spannerpb.SpannerClient
	xGoogHeaders         []string
	metricsTracerFactory *builtinMetricsTracerFactory
}

var (
	// Ensure that grpcSpannerClient implements spannerClient.
	_ spannerClient = (*grpcSpannerClient)(nil)
)

// newGRPCSpannerClient initializes a new spannerClient that uses the gRPC
// Spanner API.
func newGRPCSpannerClient(ctx context.Context, sc *sessionClient, opts ...option.ClientOption) (spannerClient, error) {
	raw, err := vkit.NewClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	clientOpts := vkit.DefaultClientOptions()
	connPool, err := gtransport.DialPool(ctx, append(clientOpts, opts...)...)
	if err != nil {
		return nil, err
	}
	clientInfo := []string{"gccl", internal.Version}
	if sc.userAgent != "" {
		agentWithVersion := strings.SplitN(sc.userAgent, "/", 2)
		if len(agentWithVersion) == 2 {
			clientInfo = append(clientInfo, agentWithVersion[0], agentWithVersion[1])
		}
	}
	if sc.callOptions != nil {
		raw.CallOptions = mergeCallOptions(raw.CallOptions, sc.callOptions)
	}
	if strings.HasPrefix(connPool.Conn().Target(), "google-c2p") {
		sc.metricsTracerFactory.isDirectPathEnabled = true
	}
	g := &grpcSpannerClient{raw: raw, metricsTracerFactory: sc.metricsTracerFactory, connPool: connPool, client: spannerpb.NewSpannerClient(connPool)}
	g.setGoogleClientInfo(clientInfo...)
	return g, nil
}

func (g *grpcSpannerClient) newBuiltinMetricsTracer(ctx context.Context, isStreaming bool) *builtinMetricsTracer {
	mt := g.metricsTracerFactory.createBuiltinMetricsTracer(ctx, isStreaming)
	return &mt
}

func (g *grpcSpannerClient) CallOptions() *vkit.CallOptions {
	return g.raw.CallOptions
}

func (g *grpcSpannerClient) setGoogleClientInfo(keyval ...string) {
	g.raw.SetGoogleClientInfo(keyval...)
	kv := append([]string{"gl-go", gax.GoVersion}, keyval...)
	kv = append(kv, "gapic", internal.Version, "gax", gax.Version, "grpc", grpc.Version)
	g.xGoogHeaders = []string{
		"x-goog-api-client", gax.XGoogHeader(kv...),
	}
}

func (g *grpcSpannerClient) Close() error {
	return g.raw.Close()
}

func (g *grpcSpannerClient) Connection() *grpc.ClientConn {
	return g.raw.Connection()
}

func (g *grpcSpannerClient) CreateSession(ctx context.Context, req *spannerpb.CreateSessionRequest, opts ...gax.CallOption) (*spannerpb.Session, error) {
	mt := g.newBuiltinMetricsTracer(ctx, false)
	defer recordOperationCompletion(mt)
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "database", url.QueryEscape(req.GetDatabase()))}

	hds = append(g.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*g.raw.CallOptions).CreateSession[0:len((*g.raw.CallOptions).CreateSession):len((*g.raw.CallOptions).CreateSession)], opts...)
	var resp *spannerpb.Session
	err := gaxInvokeWithRecorder(ctx, mt, "Spanner.CreateSession", func(ctx context.Context, settings gax.CallSettings) (context.Context, error) {
		var err error
		resp, err = g.client.CreateSession(ctx, req, settings.GRPC...)
		return ctx, err
	}, opts...)
	statusCode, _ := convertToGrpcStatusErr(err)
	mt.currOp.setStatus(statusCode.String())
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *grpcSpannerClient) BatchCreateSessions(ctx context.Context, req *spannerpb.BatchCreateSessionsRequest, opts ...gax.CallOption) (*spannerpb.BatchCreateSessionsResponse, error) {
	mt := g.newBuiltinMetricsTracer(ctx, false)
	defer recordOperationCompletion(mt)
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "database", url.QueryEscape(req.GetDatabase()))}

	hds = append(g.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*g.raw.CallOptions).BatchCreateSessions[0:len((*g.raw.CallOptions).BatchCreateSessions):len((*g.raw.CallOptions).BatchCreateSessions)], opts...)
	var resp *spannerpb.BatchCreateSessionsResponse
	err := gaxInvokeWithRecorder(ctx, mt, "Spanner.BatchCreateSessions", func(ctx context.Context, settings gax.CallSettings) (context.Context, error) {
		var err error
		resp, err = g.client.BatchCreateSessions(ctx, req, settings.GRPC...)
		return ctx, err
	}, opts...)
	statusCode, _ := convertToGrpcStatusErr(err)
	mt.currOp.setStatus(statusCode.String())
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *grpcSpannerClient) GetSession(ctx context.Context, req *spannerpb.GetSessionRequest, opts ...gax.CallOption) (*spannerpb.Session, error) {
	mt := g.newBuiltinMetricsTracer(ctx, false)
	defer recordOperationCompletion(mt)
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "name", url.QueryEscape(req.GetName()))}

	hds = append(g.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*g.raw.CallOptions).GetSession[0:len((*g.raw.CallOptions).GetSession):len((*g.raw.CallOptions).GetSession)], opts...)
	var resp *spannerpb.Session
	err := gaxInvokeWithRecorder(ctx, mt, "Spanner.GetSession", func(ctx context.Context, settings gax.CallSettings) (context.Context, error) {
		var err error
		resp, err = g.client.GetSession(ctx, req, settings.GRPC...)
		return ctx, err
	}, opts...)
	statusCode, _ := convertToGrpcStatusErr(err)
	mt.currOp.setStatus(statusCode.String())
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *grpcSpannerClient) ListSessions(ctx context.Context, req *spannerpb.ListSessionsRequest, opts ...gax.CallOption) *vkit.SessionIterator {
	return g.raw.ListSessions(ctx, req, opts...)
}

func (g *grpcSpannerClient) DeleteSession(ctx context.Context, req *spannerpb.DeleteSessionRequest, opts ...gax.CallOption) error {
	mt := g.newBuiltinMetricsTracer(ctx, false)
	defer recordOperationCompletion(mt)
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "name", url.QueryEscape(req.GetName()))}

	hds = append(g.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*g.raw.CallOptions).DeleteSession[0:len((*g.raw.CallOptions).DeleteSession):len((*g.raw.CallOptions).DeleteSession)], opts...)
	err := gaxInvokeWithRecorder(ctx, mt, "Spanner.DeleteSession", func(ctx context.Context, settings gax.CallSettings) (context.Context, error) {
		var err error
		_, err = g.client.DeleteSession(ctx, req, settings.GRPC...)
		return ctx, err
	}, opts...)
	statusCode, _ := convertToGrpcStatusErr(err)
	mt.currOp.setStatus(statusCode.String())
	return err
}

func (g *grpcSpannerClient) ExecuteSql(ctx context.Context, req *spannerpb.ExecuteSqlRequest, opts ...gax.CallOption) (*spannerpb.ResultSet, error) {
	mt := g.newBuiltinMetricsTracer(ctx, false)
	defer recordOperationCompletion(mt)
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "session", url.QueryEscape(req.GetSession()))}

	hds = append(g.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*g.raw.CallOptions).ExecuteSql[0:len((*g.raw.CallOptions).ExecuteSql):len((*g.raw.CallOptions).ExecuteSql)], opts...)
	var resp *spannerpb.ResultSet
	err := gaxInvokeWithRecorder(ctx, mt, "Spanner.ExecuteSql", func(ctx context.Context, settings gax.CallSettings) (context.Context, error) {
		var err error
		resp, err = g.client.ExecuteSql(ctx, req, settings.GRPC...)
		return ctx, err
	}, opts...)
	statusCode, _ := convertToGrpcStatusErr(err)
	mt.currOp.setStatus(statusCode.String())
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *grpcSpannerClient) ExecuteStreamingSql(ctx context.Context, req *spannerpb.ExecuteSqlRequest, opts ...gax.CallOption) (spannerpb.Spanner_ExecuteStreamingSqlClient, error) {
	return g.raw.ExecuteStreamingSql(ctx, req, opts...)
}

func (g *grpcSpannerClient) ExecuteBatchDml(ctx context.Context, req *spannerpb.ExecuteBatchDmlRequest, opts ...gax.CallOption) (*spannerpb.ExecuteBatchDmlResponse, error) {
	mt := g.newBuiltinMetricsTracer(ctx, false)
	defer recordOperationCompletion(mt)
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "session", url.QueryEscape(req.GetSession()))}

	hds = append(g.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*g.raw.CallOptions).ExecuteBatchDml[0:len((*g.raw.CallOptions).ExecuteBatchDml):len((*g.raw.CallOptions).ExecuteBatchDml)], opts...)
	var resp *spannerpb.ExecuteBatchDmlResponse
	err := gaxInvokeWithRecorder(ctx, mt, "Spanner.ExecuteBatchDml", func(ctx context.Context, settings gax.CallSettings) (context.Context, error) {
		var err error
		resp, err = g.client.ExecuteBatchDml(ctx, req, settings.GRPC...)
		return ctx, err
	}, opts...)
	statusCode, _ := convertToGrpcStatusErr(err)
	mt.currOp.setStatus(statusCode.String())
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *grpcSpannerClient) Read(ctx context.Context, req *spannerpb.ReadRequest, opts ...gax.CallOption) (*spannerpb.ResultSet, error) {
	mt := g.newBuiltinMetricsTracer(ctx, false)
	defer recordOperationCompletion(mt)
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "session", url.QueryEscape(req.GetSession()))}

	hds = append(g.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*g.raw.CallOptions).Read[0:len((*g.raw.CallOptions).Read):len((*g.raw.CallOptions).Read)], opts...)
	var resp *spannerpb.ResultSet
	err := gaxInvokeWithRecorder(ctx, mt, "Spanner.Read", func(ctx context.Context, settings gax.CallSettings) (context.Context, error) {
		var err error
		resp, err = g.client.Read(ctx, req, settings.GRPC...)
		return ctx, err
	}, opts...)
	statusCode, _ := convertToGrpcStatusErr(err)
	mt.currOp.setStatus(statusCode.String())
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *grpcSpannerClient) StreamingRead(ctx context.Context, req *spannerpb.ReadRequest, opts ...gax.CallOption) (spannerpb.Spanner_StreamingReadClient, error) {
	return g.raw.StreamingRead(ctx, req, opts...)
}

func (g *grpcSpannerClient) BeginTransaction(ctx context.Context, req *spannerpb.BeginTransactionRequest, opts ...gax.CallOption) (*spannerpb.Transaction, error) {
	mt := g.newBuiltinMetricsTracer(ctx, true)
	defer recordOperationCompletion(mt)
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "session", url.QueryEscape(req.GetSession()))}

	hds = append(g.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*g.raw.CallOptions).BeginTransaction[0:len((*g.raw.CallOptions).BeginTransaction):len((*g.raw.CallOptions).BeginTransaction)], opts...)
	var resp *spannerpb.Transaction
	err := gaxInvokeWithRecorder(ctx, mt, "Spanner.BeginTransaction", func(ctx context.Context, settings gax.CallSettings) (context.Context, error) {
		var err error
		resp, err = g.client.BeginTransaction(ctx, req, settings.GRPC...)
		return ctx, err
	}, opts...)
	statusCode, _ := convertToGrpcStatusErr(err)
	mt.currOp.setStatus(statusCode.String())
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *grpcSpannerClient) Commit(ctx context.Context, req *spannerpb.CommitRequest, opts ...gax.CallOption) (*spannerpb.CommitResponse, error) {
	mt := g.newBuiltinMetricsTracer(ctx, false)
	defer recordOperationCompletion(mt)
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "session", url.QueryEscape(req.GetSession()))}

	hds = append(g.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*g.raw.CallOptions).Commit[0:len((*g.raw.CallOptions).Commit):len((*g.raw.CallOptions).Commit)], opts...)
	var resp *spannerpb.CommitResponse
	err := gaxInvokeWithRecorder(ctx, mt, "Spanner.Commit", func(ctx context.Context, settings gax.CallSettings) (context.Context, error) {
		var err error
		resp, err = g.client.Commit(ctx, req, settings.GRPC...)
		return ctx, err
	}, opts...)
	statusCode, _ := convertToGrpcStatusErr(err)
	mt.currOp.setStatus(statusCode.String())
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *grpcSpannerClient) Rollback(ctx context.Context, req *spannerpb.RollbackRequest, opts ...gax.CallOption) error {
	mt := g.newBuiltinMetricsTracer(ctx, false)
	defer recordOperationCompletion(mt)
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "session", url.QueryEscape(req.GetSession()))}

	hds = append(g.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*g.raw.CallOptions).Rollback[0:len((*g.raw.CallOptions).Rollback):len((*g.raw.CallOptions).Rollback)], opts...)
	err := gaxInvokeWithRecorder(ctx, mt, "Spanner.Rollback", func(ctx context.Context, settings gax.CallSettings) (context.Context, error) {
		var err error
		_, err = g.client.Rollback(ctx, req, settings.GRPC...)
		return ctx, err
	}, opts...)
	statusCode, _ := convertToGrpcStatusErr(err)
	mt.currOp.setStatus(statusCode.String())
	return err
}

func (g *grpcSpannerClient) PartitionQuery(ctx context.Context, req *spannerpb.PartitionQueryRequest, opts ...gax.CallOption) (*spannerpb.PartitionResponse, error) {
	mt := g.newBuiltinMetricsTracer(ctx, false)
	defer recordOperationCompletion(mt)
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "session", url.QueryEscape(req.GetSession()))}

	hds = append(g.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*g.raw.CallOptions).PartitionQuery[0:len((*g.raw.CallOptions).PartitionQuery):len((*g.raw.CallOptions).PartitionQuery)], opts...)
	var resp *spannerpb.PartitionResponse
	err := gaxInvokeWithRecorder(ctx, mt, "Spanner.PartitionQuery", func(ctx context.Context, settings gax.CallSettings) (context.Context, error) {
		var err error
		resp, err = g.client.PartitionQuery(ctx, req, settings.GRPC...)
		return ctx, err
	}, opts...)
	statusCode, _ := convertToGrpcStatusErr(err)
	mt.currOp.setStatus(statusCode.String())
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *grpcSpannerClient) PartitionRead(ctx context.Context, req *spannerpb.PartitionReadRequest, opts ...gax.CallOption) (*spannerpb.PartitionResponse, error) {
	mt := g.newBuiltinMetricsTracer(ctx, false)
	defer recordOperationCompletion(mt)
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "session", url.QueryEscape(req.GetSession()))}

	hds = append(g.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*g.raw.CallOptions).PartitionRead[0:len((*g.raw.CallOptions).PartitionRead):len((*g.raw.CallOptions).PartitionRead)], opts...)
	var resp *spannerpb.PartitionResponse
	err := gaxInvokeWithRecorder(ctx, mt, "Spanner.PartitionRead", func(ctx context.Context, settings gax.CallSettings) (context.Context, error) {
		var err error
		resp, err = g.client.PartitionRead(ctx, req, settings.GRPC...)
		return ctx, err
	}, opts...)
	statusCode, _ := convertToGrpcStatusErr(err)
	mt.currOp.setStatus(statusCode.String())
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *grpcSpannerClient) BatchWrite(ctx context.Context, req *spannerpb.BatchWriteRequest, opts ...gax.CallOption) (spannerpb.Spanner_BatchWriteClient, error) {
	return g.raw.BatchWrite(ctx, req, opts...)
}

// gaxInvokeWithRecorder:
// - wraps 'f' in a new function 'callWrapper' that:
//   - updates tracer state and records built in attempt specific metrics
//   - does not return errors seen while recording the metrics
//
// - then, calls gax.Invoke with 'callWrapper' as an argument
func gaxInvokeWithRecorder(ctx context.Context, mt *builtinMetricsTracer, method string,
	f func(ctx context.Context, _ gax.CallSettings) (context.Context, error), opts ...gax.CallOption) error {

	mt.method = method
	callWrapper := func(ctx context.Context, callSettings gax.CallSettings) error {
		peerInfo := &peer.Peer{}
		ctx = peer.NewContext(ctx, peerInfo)
		// Increment number of attempts
		mt.currOp.incrementAttemptCount()

		mt.currOp.currAttempt = attemptTracer{}

		// record start time
		mt.currOp.currAttempt.setStartTime(time.Now())

		// f makes calls to Spanner service
		rpcCtx, err := f(ctx, callSettings)
		// Set attempt status
		statusCode, _ := convertToGrpcStatusErr(err)
		mt.currOp.currAttempt.setStatus(statusCode.String())
		var ok bool
		isDirectPathUsed := false
		peerInfo, ok = peer.FromContext(rpcCtx)
		if ok {
			if peerInfo.Addr != nil {
				remoteIP := peerInfo.Addr.String()
				if strings.HasPrefix(remoteIP, directPathIPV4Prefix) || strings.HasPrefix(remoteIP, directPathIPV6Prefix) {
					isDirectPathUsed = true
				}
			}
		}
		mt.currOp.currAttempt.setDirectPathUsed(isDirectPathUsed)

		// in case of streaming calls skip recording attempt when io.EOF error is returned
		if err == io.EOF {
			return nil
		}
		// Record attempt specific metrics
		recordAttemptCompletion(mt)
		return err
	}
	return gax.Invoke(ctx, callWrapper, opts...)
}
