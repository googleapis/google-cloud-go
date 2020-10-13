/*
Copyright 2019 Google LLC

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

/*
Package spannertest contains test helpers for working with Cloud Spanner.

This package is EXPERIMENTAL, and is lacking several features. See the README.md
file in this directory for more details.

In-memory fake

This package has an in-memory fake implementation of spanner. To use it,
create a Server, and then connect to it with no security:
	srv, err := spannertest.NewServer("localhost:0")
	...
	conn, err := grpc.DialContext(ctx, srv.Addr, grpc.WithInsecure())
	...
	client, err := spanner.NewClient(ctx, db, option.WithGRPCConn(conn))
	...

Alternatively, create a Server, then set the SPANNER_EMULATOR_HOST environment
variable and use the regular spanner.NewClient:
	srv, err := spannertest.NewServer("localhost:0")
	...
	os.Setenv("SPANNER_EMULATOR_HOST", srv.Addr)
	client, err := spanner.NewClient(ctx, db)
	...

The same server also supports database admin operations for use with
the cloud.google.com/go/spanner/admin/database/apiv1 package. This only
simulates the existence of a single database; its name is ignored.
*/
package spannertest

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	anypb "github.com/golang/protobuf/ptypes/any"
	emptypb "github.com/golang/protobuf/ptypes/empty"
	structpb "github.com/golang/protobuf/ptypes/struct"
	timestamppb "github.com/golang/protobuf/ptypes/timestamp"
	lropb "google.golang.org/genproto/googleapis/longrunning"
	adminpb "google.golang.org/genproto/googleapis/spanner/admin/database/v1"
	spannerpb "google.golang.org/genproto/googleapis/spanner/v1"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner/spansql"
)

// Server is an in-memory Cloud Spanner fake.
// It is unauthenticated, non-performant, and only a rough approximation.
type Server struct {
	Addr string

	l   net.Listener
	srv *grpc.Server
	s   *server
}

// server is the real implementation of the fake.
// It is a separate and unexported type so the API won't be cluttered with
// methods that are only relevant to the fake's implementation.
type server struct {
	logf Logger

	db    database
	start time.Time

	mu       sync.Mutex
	sessions map[string]*session
	lros     map[string]*lro

	// Any unimplemented methods will cause a panic.
	// TODO: Switch to Unimplemented at some point? spannerpb would need regenerating.
	adminpb.DatabaseAdminServer
	spannerpb.SpannerServer
	lropb.OperationsServer
}

type session struct {
	name     string
	creation time.Time

	// This context tracks the lifetime of this session.
	// It is canceled in DeleteSession.
	ctx    context.Context
	cancel func()

	mu           sync.Mutex
	lastUse      time.Time
	transactions map[string]*transaction
}

func (s *session) Proto() *spannerpb.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := &spannerpb.Session{
		Name:                   s.name,
		CreateTime:             timestampProto(s.creation),
		ApproximateLastUseTime: timestampProto(s.lastUse),
	}
	return m
}

// timestampProto returns a valid timestamp.Timestamp,
// or nil if the given time is zero or isn't representable.
func timestampProto(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	ts, err := ptypes.TimestampProto(t)
	if err != nil {
		return nil
	}
	return ts
}

// lro represents a Long-Running Operation, generally a schema change.
type lro struct {
	mu    sync.Mutex
	state *lropb.Operation

	// waitc is closed when anyone starts waiting on the LRO.
	// waitatom is CAS'd from 0 to 1 to make that closing safe.
	waitc    chan struct{}
	waitatom int32
}

func newLRO(initState *lropb.Operation) *lro {
	return &lro{
		state: initState,
		waitc: make(chan struct{}),
	}
}

func (l *lro) noWait() {
	if atomic.CompareAndSwapInt32(&l.waitatom, 0, 1) {
		close(l.waitc)
	}
}

func (l *lro) State() *lropb.Operation {
	l.mu.Lock()
	defer l.mu.Unlock()
	return proto.Clone(l.state).(*lropb.Operation)
}

// Logger is something that can be used for logging.
// It is matched by log.Printf and testing.T.Logf.
type Logger func(format string, args ...interface{})

// NewServer creates a new Server.
// The Server will be listening for gRPC connections, without TLS, on the provided TCP address.
// The resolved address is available in the Addr field.
func NewServer(laddr string) (*Server, error) {
	l, err := net.Listen("tcp", laddr)
	if err != nil {
		return nil, err
	}

	s := &Server{
		Addr: l.Addr().String(),
		l:    l,
		srv:  grpc.NewServer(),
		s: &server{
			logf: func(format string, args ...interface{}) {
				log.Printf("spannertest.inmem: "+format, args...)
			},
			start:    time.Now(),
			sessions: make(map[string]*session),
			lros:     make(map[string]*lro),
		},
	}
	adminpb.RegisterDatabaseAdminServer(s.srv, s.s)
	spannerpb.RegisterSpannerServer(s.srv, s.s)
	lropb.RegisterOperationsServer(s.srv, s.s)

	go s.srv.Serve(s.l)

	return s, nil
}

// SetLogger sets a logger for the server.
// You can use a *testing.T as this argument to collate extra information
// from the execution of the server.
func (s *Server) SetLogger(l Logger) { s.s.logf = l }

// Close shuts down the server.
func (s *Server) Close() {
	s.srv.Stop()
	s.l.Close()
}

func genRandomSession() string {
	var b [4]byte
	rand.Read(b[:])
	return fmt.Sprintf("%x", b)
}

func genRandomTransaction() string {
	var b [6]byte
	rand.Read(b[:])
	return fmt.Sprintf("tx-%x", b)
}

func genRandomOperation() string {
	var b [3]byte
	rand.Read(b[:])
	return fmt.Sprintf("op-%x", b)
}

func (s *server) GetOperation(ctx context.Context, req *lropb.GetOperationRequest) (*lropb.Operation, error) {
	s.mu.Lock()
	lro, ok := s.lros[req.Name]
	s.mu.Unlock()
	if !ok {
		return nil, status.Errorf(codes.NotFound, "unknown LRO %q", req.Name)
	}

	// Someone is waiting on this LRO. Disable sleeping in its Run method.
	lro.noWait()

	return lro.State(), nil
}

func (s *server) GetDatabase(ctx context.Context, req *adminpb.GetDatabaseRequest) (*adminpb.Database, error) {
	s.logf("GetDatabase(%q)", req.Name)

	return &adminpb.Database{
		Name:       req.Name,
		State:      adminpb.Database_READY,
		CreateTime: timestampProto(s.start),
	}, nil
}

// UpdateDDL applies the given DDL to the server.
//
// This is a convenience method for tests that may assume an existing schema.
// The more general approach is to dial this server using an admin client, and
// use the UpdateDatabaseDdl RPC method.
func (s *Server) UpdateDDL(ddl *spansql.DDL) error {
	ctx := context.Background()
	for _, stmt := range ddl.List {
		if st := s.s.runOneDDL(ctx, stmt); st.Code() != codes.OK {
			return st.Err()
		}
	}
	return nil
}

func (s *server) UpdateDatabaseDdl(ctx context.Context, req *adminpb.UpdateDatabaseDdlRequest) (*lropb.Operation, error) {
	// Parse all the DDL statements first.
	var stmts []spansql.DDLStmt
	for _, s := range req.Statements {
		stmt, err := spansql.ParseDDLStmt(s)
		if err != nil {
			// TODO: check what code the real Spanner returns here.
			return nil, status.Errorf(codes.InvalidArgument, "bad DDL statement %q: %v", s, err)
		}
		stmts = append(stmts, stmt)
	}

	// Nothing should be depending on the exact structure of this,
	// but it is specified in google/spanner/admin/database/v1/spanner_database_admin.proto.
	id := "projects/fake-proj/instances/fake-instance/databases/fake-db/operations/" + genRandomOperation()
	lro := newLRO(&lropb.Operation{Name: id})
	s.mu.Lock()
	s.lros[id] = lro
	s.mu.Unlock()

	go lro.Run(s, stmts)
	return lro.State(), nil
}

func (l *lro) Run(s *server, stmts []spansql.DDLStmt) {
	ctx := context.Background()

	for _, stmt := range stmts {
		// Simulate delayed DDL application, but only if nobody is waiting.
		select {
		case <-time.After(100 * time.Millisecond):
		case <-l.waitc:
		}

		if st := s.runOneDDL(ctx, stmt); st.Code() != codes.OK {
			l.mu.Lock()
			l.state.Done = true
			l.state.Result = &lropb.Operation_Error{st.Proto()}
			l.mu.Unlock()
			return
		}
	}

	l.mu.Lock()
	l.state.Done = true
	l.state.Result = &lropb.Operation_Response{&anypb.Any{}}
	l.mu.Unlock()
}

func (s *server) runOneDDL(ctx context.Context, stmt spansql.DDLStmt) *status.Status {
	return s.db.ApplyDDL(stmt)
}

func (s *server) GetDatabaseDdl(ctx context.Context, req *adminpb.GetDatabaseDdlRequest) (*adminpb.GetDatabaseDdlResponse, error) {
	s.logf("GetDatabaseDdl(%q)", req.Database)

	var resp adminpb.GetDatabaseDdlResponse
	for _, stmt := range s.db.GetDDL() {
		resp.Statements = append(resp.Statements, stmt.SQL())
	}
	return &resp, nil
}

func (s *server) CreateSession(ctx context.Context, req *spannerpb.CreateSessionRequest) (*spannerpb.Session, error) {
	//s.logf("CreateSession(%q)", req.Database)
	return s.newSession(), nil
}

func (s *server) newSession() *spannerpb.Session {
	id := genRandomSession()
	now := time.Now()
	sess := &session{
		name:         id,
		creation:     now,
		lastUse:      now,
		transactions: make(map[string]*transaction),
	}
	sess.ctx, sess.cancel = context.WithCancel(context.Background())

	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()

	return sess.Proto()
}

func (s *server) BatchCreateSessions(ctx context.Context, req *spannerpb.BatchCreateSessionsRequest) (*spannerpb.BatchCreateSessionsResponse, error) {
	//s.logf("BatchCreateSessions(%q)", req.Database)

	var sessions []*spannerpb.Session
	for i := int32(0); i < req.GetSessionCount(); i++ {
		sessions = append(sessions, s.newSession())
	}

	return &spannerpb.BatchCreateSessionsResponse{Session: sessions}, nil
}

func (s *server) GetSession(ctx context.Context, req *spannerpb.GetSessionRequest) (*spannerpb.Session, error) {
	s.mu.Lock()
	sess, ok := s.sessions[req.Name]
	s.mu.Unlock()

	if !ok {
		// TODO: what error does the real Spanner return?
		return nil, status.Errorf(codes.NotFound, "unknown session %q", req.Name)
	}

	return sess.Proto(), nil
}

// TODO: ListSessions

func (s *server) DeleteSession(ctx context.Context, req *spannerpb.DeleteSessionRequest) (*emptypb.Empty, error) {
	//s.logf("DeleteSession(%q)", req.Name)

	s.mu.Lock()
	sess, ok := s.sessions[req.Name]
	delete(s.sessions, req.Name)
	s.mu.Unlock()

	if !ok {
		// TODO: what error does the real Spanner return?
		return nil, status.Errorf(codes.NotFound, "unknown session %q", req.Name)
	}

	// Terminate any operations in this session.
	sess.cancel()

	return &emptypb.Empty{}, nil
}

// popTx returns an existing transaction, removing it from the session.
// This is called when a transaction is finishing (Commit, Rollback).
func (s *server) popTx(sessionID, tid string) (tx *transaction, err error) {
	s.mu.Lock()
	sess, ok := s.sessions[sessionID]
	s.mu.Unlock()
	if !ok {
		// TODO: what error does the real Spanner return?
		return nil, status.Errorf(codes.NotFound, "unknown session %q", sessionID)
	}

	sess.mu.Lock()
	sess.lastUse = time.Now()
	tx, ok = sess.transactions[tid]
	if ok {
		delete(sess.transactions, tid)
	}
	sess.mu.Unlock()
	if !ok {
		// TODO: what error does the real Spanner return?
		return nil, status.Errorf(codes.NotFound, "unknown transaction ID %q", tid)
	}
	return tx, nil
}

// readTx returns a transaction for the given session and transaction selector.
// It is used by read/query operations (ExecuteStreamingSql, StreamingRead).
func (s *server) readTx(ctx context.Context, session string, tsel *spannerpb.TransactionSelector) (tx *transaction, cleanup func(), err error) {
	s.mu.Lock()
	sess, ok := s.sessions[session]
	s.mu.Unlock()
	if !ok {
		// TODO: what error does the real Spanner return?
		return nil, nil, status.Errorf(codes.NotFound, "unknown session %q", session)
	}

	sess.mu.Lock()
	sess.lastUse = time.Now()
	sess.mu.Unlock()

	// Only give a read-only transaction regardless of whether the selector
	// is requesting a read-write or read-only one, since this is in readTx
	// and so shouldn't be mutating anyway.
	singleUse := func() (*transaction, func(), error) {
		tx := s.db.NewReadOnlyTransaction()
		return tx, tx.Rollback, nil
	}

	if tsel.GetSelector() == nil {
		return singleUse()
	}

	switch sel := tsel.Selector.(type) {
	default:
		return nil, nil, fmt.Errorf("TransactionSelector type %T not supported", sel)
	case *spannerpb.TransactionSelector_SingleUse:
		// Ignore options (e.g. timestamps).
		switch mode := sel.SingleUse.Mode.(type) {
		case *spannerpb.TransactionOptions_ReadOnly_:
			return singleUse()
		case *spannerpb.TransactionOptions_ReadWrite_:
			return singleUse()
		default:
			return nil, nil, fmt.Errorf("single use transaction in mode %T not supported", mode)
		}
	case *spannerpb.TransactionSelector_Id:
		sess.mu.Lock()
		tx, ok := sess.transactions[string(sel.Id)]
		sess.mu.Unlock()
		if !ok {
			return nil, nil, fmt.Errorf("no transaction with id %q", sel.Id)
		}
		return tx, func() {}, nil
	}
}

func (s *server) ExecuteSql(ctx context.Context, req *spannerpb.ExecuteSqlRequest) (*spannerpb.ResultSet, error) {
	// Assume this is probably a DML statement. Queries tend to use ExecuteStreamingSql.
	// TODO: Expand this to support more things.

	obj, ok := req.Transaction.Selector.(*spannerpb.TransactionSelector_Id)
	if !ok {
		return nil, fmt.Errorf("unsupported transaction type %T", req.Transaction.Selector)
	}
	tid := string(obj.Id)
	_ = tid // TODO: lookup an existing transaction by ID.

	stmt, err := spansql.ParseDMLStmt(req.Sql)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad DML: %v", err)
	}
	params, err := parseQueryParams(req.GetParams(), req.ParamTypes)
	if err != nil {
		return nil, err
	}

	s.logf("Executing: %s", stmt.SQL())
	if len(params) > 0 {
		s.logf("        ▹ %v", params)
	}

	n, err := s.db.Execute(stmt, params)
	if err != nil {
		return nil, err
	}
	return &spannerpb.ResultSet{
		Stats: &spannerpb.ResultSetStats{
			RowCount: &spannerpb.ResultSetStats_RowCountExact{int64(n)},
		},
	}, nil
}

func (s *server) ExecuteStreamingSql(req *spannerpb.ExecuteSqlRequest, stream spannerpb.Spanner_ExecuteStreamingSqlServer) error {
	tx, cleanup, err := s.readTx(stream.Context(), req.Session, req.Transaction)
	if err != nil {
		return err
	}
	defer cleanup()

	q, err := spansql.ParseQuery(req.Sql)
	if err != nil {
		// TODO: check what code the real Spanner returns here.
		return status.Errorf(codes.InvalidArgument, "bad query: %v", err)
	}

	params, err := parseQueryParams(req.GetParams(), req.ParamTypes)
	if err != nil {
		return err
	}

	s.logf("Querying: %s", q.SQL())
	if len(params) > 0 {
		s.logf("        ▹ %v", params)
	}

	ri, err := s.db.Query(q, params)
	if err != nil {
		return err
	}
	return s.readStream(stream.Context(), tx, stream.Send, ri)
}

// TODO: Read

func (s *server) StreamingRead(req *spannerpb.ReadRequest, stream spannerpb.Spanner_StreamingReadServer) error {
	tx, cleanup, err := s.readTx(stream.Context(), req.Session, req.Transaction)
	if err != nil {
		return err
	}
	defer cleanup()

	// Bail out if various advanced features are being used.
	if req.Index != "" {
		// This is okay; we can still return results.
		s.logf("Warning: index reads (%q) not supported", req.Index)
	}
	if len(req.ResumeToken) > 0 {
		// This should only happen if we send resume_token ourselves.
		return fmt.Errorf("read resumption not supported")
	}
	if len(req.PartitionToken) > 0 {
		return fmt.Errorf("partition restrictions not supported")
	}

	var ri rowIter
	if req.KeySet.All {
		s.logf("Reading all from %s (cols: %v)", req.Table, req.Columns)
		ri, err = s.db.ReadAll(spansql.ID(req.Table), idList(req.Columns), req.Limit)
	} else {
		s.logf("Reading rows from %d keys and %d ranges from %s (cols: %v)", len(req.KeySet.Keys), len(req.KeySet.Ranges), req.Table, req.Columns)
		ri, err = s.db.Read(spansql.ID(req.Table), idList(req.Columns), req.KeySet.Keys, makeKeyRangeList(req.KeySet.Ranges), req.Limit)
	}
	if err != nil {
		return err
	}

	// TODO: Figure out the right contexts to use here. There's the session one (sess.ctx),
	// but also this specific RPC one (stream.Context()). Which takes precedence?
	// They appear to be independent.

	return s.readStream(stream.Context(), tx, stream.Send, ri)
}

func (s *server) readStream(ctx context.Context, tx *transaction, send func(*spannerpb.PartialResultSet) error, ri rowIter) error {
	// Build the result set metadata.
	rsm := &spannerpb.ResultSetMetadata{
		RowType: &spannerpb.StructType{},
		// TODO: transaction info?
	}
	for _, ci := range ri.Cols() {
		st, err := spannerTypeFromType(ci.Type)
		if err != nil {
			return err
		}
		rsm.RowType.Fields = append(rsm.RowType.Fields, &spannerpb.StructType_Field{
			Name: string(ci.Name),
			Type: st,
		})
	}

	for {
		row, err := ri.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		values := make([]*structpb.Value, len(row))
		for i, x := range row {
			v, err := spannerValueFromValue(x)
			if err != nil {
				return err
			}
			values[i] = v
		}

		prs := &spannerpb.PartialResultSet{
			Metadata: rsm,
			Values:   values,
		}
		if err := send(prs); err != nil {
			return err
		}

		// ResultSetMetadata is only set for the first PartialResultSet.
		rsm = nil
	}

	return nil
}

func (s *server) BeginTransaction(ctx context.Context, req *spannerpb.BeginTransactionRequest) (*spannerpb.Transaction, error) {
	//s.logf("BeginTransaction(%v)", req)

	s.mu.Lock()
	sess, ok := s.sessions[req.Session]
	s.mu.Unlock()
	if !ok {
		// TODO: what error does the real Spanner return?
		return nil, status.Errorf(codes.NotFound, "unknown session %q", req.Session)
	}

	id := genRandomTransaction()
	tx := s.db.NewTransaction()

	sess.mu.Lock()
	sess.lastUse = time.Now()
	sess.transactions[id] = tx
	sess.mu.Unlock()

	tr := &spannerpb.Transaction{Id: []byte(id)}

	if req.GetOptions().GetReadOnly().GetReturnReadTimestamp() {
		// Return the last commit timestamp.
		// This isn't wholly accurate, but may be good enough for simple use cases.
		tr.ReadTimestamp = timestampProto(s.db.LastCommitTimestamp())
	}

	return tr, nil
}

func (s *server) Commit(ctx context.Context, req *spannerpb.CommitRequest) (resp *spannerpb.CommitResponse, err error) {
	//s.logf("Commit(%q, %q)", req.Session, req.Transaction)

	obj, ok := req.Transaction.(*spannerpb.CommitRequest_TransactionId)
	if !ok {
		return nil, fmt.Errorf("unsupported transaction type %T", req.Transaction)
	}
	tid := string(obj.TransactionId)

	tx, err := s.popTx(req.Session, tid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	tx.Start()

	for _, m := range req.Mutations {
		switch op := m.Operation.(type) {
		default:
			return nil, fmt.Errorf("unsupported mutation operation type %T", op)
		case *spannerpb.Mutation_Insert:
			ins := op.Insert
			err := s.db.Insert(tx, spansql.ID(ins.Table), idList(ins.Columns), ins.Values)
			if err != nil {
				return nil, err
			}
		case *spannerpb.Mutation_Update:
			up := op.Update
			err := s.db.Update(tx, spansql.ID(up.Table), idList(up.Columns), up.Values)
			if err != nil {
				return nil, err
			}
		case *spannerpb.Mutation_InsertOrUpdate:
			iou := op.InsertOrUpdate
			err := s.db.InsertOrUpdate(tx, spansql.ID(iou.Table), idList(iou.Columns), iou.Values)
			if err != nil {
				return nil, err
			}
		case *spannerpb.Mutation_Delete_:
			del := op.Delete
			ks := del.KeySet

			err := s.db.Delete(tx, spansql.ID(del.Table), ks.Keys, makeKeyRangeList(ks.Ranges), ks.All)
			if err != nil {
				return nil, err
			}
		}

	}

	ts, err := tx.Commit()
	if err != nil {
		return nil, err
	}

	return &spannerpb.CommitResponse{
		CommitTimestamp: timestampProto(ts),
	}, nil
}

func (s *server) Rollback(ctx context.Context, req *spannerpb.RollbackRequest) (*emptypb.Empty, error) {
	s.logf("Rollback(%v)", req)

	tx, err := s.popTx(req.Session, string(req.TransactionId))
	if err != nil {
		return nil, err
	}

	tx.Rollback()

	return &emptypb.Empty{}, nil
}

// TODO: PartitionQuery, PartitionRead

func parseQueryParams(p *structpb.Struct, types map[string]*spannerpb.Type) (queryParams, error) {
	params := make(queryParams)
	for k, v := range p.GetFields() {
		p, err := parseQueryParam(v, types[k])
		if err != nil {
			return nil, err
		}
		params[k] = p
	}
	return params, nil
}

func parseQueryParam(v *structpb.Value, typ *spannerpb.Type) (queryParam, error) {
	// TODO: Use valForType and typeFromSpannerType more comprehensively here?
	// They are only used for StringValue vs, since that's what mostly needs parsing.

	rawv := v
	switch v := v.Kind.(type) {
	default:
		return queryParam{}, fmt.Errorf("unsupported well-known type value kind %T", v)
	case *structpb.Value_NullValue:
		return queryParam{Value: nil}, nil // TODO: set a type?
	case *structpb.Value_BoolValue:
		return queryParam{Value: v.BoolValue, Type: boolType}, nil
	case *structpb.Value_NumberValue:
		return queryParam{Value: v.NumberValue, Type: float64Type}, nil
	case *structpb.Value_StringValue:
		t, err := typeFromSpannerType(typ)
		if err != nil {
			return queryParam{}, err
		}
		val, err := valForType(rawv, t)
		if err != nil {
			return queryParam{}, err
		}
		return queryParam{Value: val, Type: t}, nil
	case *structpb.Value_ListValue:
		var list []interface{}
		for _, elem := range v.ListValue.Values {
			// TODO: Change the type parameter passed through? We only look at the code.
			p, err := parseQueryParam(elem, typ)
			if err != nil {
				return queryParam{}, err
			}
			list = append(list, p.Value)
		}
		t, err := typeFromSpannerType(typ)
		if err != nil {
			return queryParam{}, err
		}
		return queryParam{Value: list, Type: t}, nil
	}
}

func typeFromSpannerType(st *spannerpb.Type) (spansql.Type, error) {
	switch st.Code {
	default:
		return spansql.Type{}, fmt.Errorf("unhandled spanner type code %v", st.Code)
	case spannerpb.TypeCode_BOOL:
		return spansql.Type{Base: spansql.Bool}, nil
	case spannerpb.TypeCode_INT64:
		return spansql.Type{Base: spansql.Int64}, nil
	case spannerpb.TypeCode_FLOAT64:
		return spansql.Type{Base: spansql.Float64}, nil
	case spannerpb.TypeCode_TIMESTAMP:
		return spansql.Type{Base: spansql.Timestamp}, nil
	case spannerpb.TypeCode_DATE:
		return spansql.Type{Base: spansql.Date}, nil
	case spannerpb.TypeCode_STRING:
		return spansql.Type{Base: spansql.String}, nil // no len
	case spannerpb.TypeCode_BYTES:
		return spansql.Type{Base: spansql.Bytes}, nil // no len
	case spannerpb.TypeCode_ARRAY:
		typ, err := typeFromSpannerType(st.ArrayElementType)
		if err != nil {
			return spansql.Type{}, err
		}
		typ.Array = true
		return typ, nil
	}
}

func spannerTypeFromType(typ spansql.Type) (*spannerpb.Type, error) {
	var code spannerpb.TypeCode
	switch typ.Base {
	default:
		return nil, fmt.Errorf("unhandled base type %d", typ.Base)
	case spansql.Bool:
		code = spannerpb.TypeCode_BOOL
	case spansql.Int64:
		code = spannerpb.TypeCode_INT64
	case spansql.Float64:
		code = spannerpb.TypeCode_FLOAT64
	case spansql.String:
		code = spannerpb.TypeCode_STRING
	case spansql.Bytes:
		code = spannerpb.TypeCode_BYTES
	case spansql.Date:
		code = spannerpb.TypeCode_DATE
	case spansql.Timestamp:
		code = spannerpb.TypeCode_TIMESTAMP
	}
	st := &spannerpb.Type{Code: code}
	if typ.Array {
		st = &spannerpb.Type{
			Code:             spannerpb.TypeCode_ARRAY,
			ArrayElementType: st,
		}
	}
	return st, nil
}

func spannerValueFromValue(x interface{}) (*structpb.Value, error) {
	switch x := x.(type) {
	default:
		return nil, fmt.Errorf("unhandled database value type %T", x)
	case bool:
		return &structpb.Value{Kind: &structpb.Value_BoolValue{x}}, nil
	case int64:
		// The Spanner int64 is actually a decimal string.
		s := strconv.FormatInt(x, 10)
		return &structpb.Value{Kind: &structpb.Value_StringValue{s}}, nil
	case float64:
		return &structpb.Value{Kind: &structpb.Value_NumberValue{x}}, nil
	case string:
		return &structpb.Value{Kind: &structpb.Value_StringValue{x}}, nil
	case []byte:
		return &structpb.Value{Kind: &structpb.Value_StringValue{base64.StdEncoding.EncodeToString(x)}}, nil
	case civil.Date:
		// RFC 3339 date format.
		return &structpb.Value{Kind: &structpb.Value_StringValue{x.String()}}, nil
	case time.Time:
		// RFC 3339 timestamp format with zone Z.
		s := x.Format("2006-01-02T15:04:05.999999999Z")
		return &structpb.Value{Kind: &structpb.Value_StringValue{s}}, nil
	case nil:
		return &structpb.Value{Kind: &structpb.Value_NullValue{}}, nil
	case []interface{}:
		var vs []*structpb.Value
		for _, elem := range x {
			v, err := spannerValueFromValue(elem)
			if err != nil {
				return nil, err
			}
			vs = append(vs, v)
		}
		return &structpb.Value{Kind: &structpb.Value_ListValue{
			&structpb.ListValue{Values: vs},
		}}, nil
	}
}

func makeKeyRangeList(ranges []*spannerpb.KeyRange) keyRangeList {
	var krl keyRangeList
	for _, r := range ranges {
		krl = append(krl, makeKeyRange(r))
	}
	return krl
}

func makeKeyRange(r *spannerpb.KeyRange) *keyRange {
	var kr keyRange
	switch s := r.StartKeyType.(type) {
	case *spannerpb.KeyRange_StartClosed:
		kr.start = s.StartClosed
		kr.startClosed = true
	case *spannerpb.KeyRange_StartOpen:
		kr.start = s.StartOpen
	}
	switch e := r.EndKeyType.(type) {
	case *spannerpb.KeyRange_EndClosed:
		kr.end = e.EndClosed
		kr.endClosed = true
	case *spannerpb.KeyRange_EndOpen:
		kr.end = e.EndOpen
	}
	return &kr
}

func idList(ss []string) (ids []spansql.ID) {
	for _, s := range ss {
		ids = append(ids, spansql.ID(s))
	}
	return
}
