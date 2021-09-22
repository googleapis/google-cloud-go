/*
Copyright 2017 Google LLC

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
	"log"
	"os"
	"regexp"
	"time"

	"cloud.google.com/go/internal/trace"
	vkit "cloud.google.com/go/spanner/apiv1"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	gtransport "google.golang.org/api/transport/grpc"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

const (
	// resourcePrefixHeader is the name of the metadata header used to indicate
	// the resource being operated on.
	resourcePrefixHeader = "google-cloud-resource-prefix"

	// numChannels is the default value for NumChannels of client.
	numChannels = 4
)

const (
	// Scope is the scope for Cloud Spanner Data API.
	Scope = "https://www.googleapis.com/auth/spanner.data"

	// AdminScope is the scope for Cloud Spanner Admin APIs.
	AdminScope = "https://www.googleapis.com/auth/spanner.admin"
)

var (
	validDBPattern = regexp.MustCompile("^projects/(?P<project>[^/]+)/instances/(?P<instance>[^/]+)/databases/(?P<database>[^/]+)$")
)

func validDatabaseName(db string) error {
	if matched := validDBPattern.MatchString(db); !matched {
		return fmt.Errorf("database name %q should conform to pattern %q",
			db, validDBPattern.String())
	}
	return nil
}

func parseDatabaseName(db string) (project, instance, database string, err error) {
	matches := validDBPattern.FindStringSubmatch(db)
	if len(matches) == 0 {
		return "", "", "", fmt.Errorf("Failed to parse database name from %q according to pattern %q",
			db, validDBPattern.String())
	}
	return matches[1], matches[2], matches[3], nil
}

// Client is a client for reading and writing data to a Cloud Spanner database.
// A client is safe to use concurrently, except for its Close method.
type Client struct {
	sc           *sessionClient
	idleSessions *sessionPool
	logger       *log.Logger
	qo           QueryOptions
}

// DatabaseName returns the full name of a database, e.g.,
// "projects/spanner-cloud-test/instances/foo/databases/foodb".
func (c *Client) DatabaseName() string {
	return c.sc.database
}

// ClientConfig has configurations for the client.
type ClientConfig struct {
	// NumChannels is the number of gRPC channels.
	// If zero, a reasonable default is used based on the execution environment.
	//
	// Deprecated: The Spanner client now uses a pool of gRPC connections. Use
	// option.WithGRPCConnectionPool(numConns) instead to specify the number of
	// connections the client should use. The client will default to a
	// reasonable default if this option is not specified.
	NumChannels int

	// SessionPoolConfig is the configuration for session pool.
	SessionPoolConfig

	// SessionLabels for the sessions created by this client.
	// See https://cloud.google.com/spanner/docs/reference/rpc/google.spanner.v1#session
	// for more info.
	SessionLabels map[string]string

	// QueryOptions is the configuration for executing a sql query.
	QueryOptions QueryOptions

	// CallOptions is the configuration for providing custom retry settings that
	// override the default values.
	CallOptions *vkit.CallOptions

	// logger is the logger to use for this client. If it is nil, all logging
	// will be directed to the standard logger.
	logger *log.Logger
}

func contextWithOutgoingMetadata(ctx context.Context, md metadata.MD) context.Context {
	existing, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		md = metadata.Join(existing, md)
	}
	return metadata.NewOutgoingContext(ctx, md)
}

// NewClient creates a client to a database. A valid database name has the
// form projects/PROJECT_ID/instances/INSTANCE_ID/databases/DATABASE_ID. It uses
// a default configuration.
func NewClient(ctx context.Context, database string, opts ...option.ClientOption) (*Client, error) {
	return NewClientWithConfig(ctx, database, ClientConfig{SessionPoolConfig: DefaultSessionPoolConfig}, opts...)
}

// NewClientWithConfig creates a client to a database. A valid database name has
// the form projects/PROJECT_ID/instances/INSTANCE_ID/databases/DATABASE_ID.
func NewClientWithConfig(ctx context.Context, database string, config ClientConfig, opts ...option.ClientOption) (c *Client, err error) {
	// Validate database path.
	if err := validDatabaseName(database); err != nil {
		return nil, err
	}

	ctx = trace.StartSpan(ctx, "cloud.google.com/go/spanner.NewClient")
	defer func() { trace.EndSpan(ctx, err) }()

	// Append emulator options if SPANNER_EMULATOR_HOST has been set.
	if emulatorAddr := os.Getenv("SPANNER_EMULATOR_HOST"); emulatorAddr != "" {
		emulatorOpts := []option.ClientOption{
			option.WithEndpoint(emulatorAddr),
			option.WithGRPCDialOption(grpc.WithInsecure()),
			option.WithoutAuthentication(),
			internaloption.SkipDialSettingsValidation(),
		}
		opts = append(emulatorOpts, opts...)
	}

	// Prepare gRPC channels.
	hasNumChannelsConfig := config.NumChannels > 0
	if config.NumChannels == 0 {
		config.NumChannels = numChannels
	}
	// gRPC options.
	allOpts := allClientOpts(config.NumChannels, opts...)
	pool, err := gtransport.DialPool(ctx, allOpts...)
	if err != nil {
		return nil, err
	}
	if hasNumChannelsConfig && pool.Num() != config.NumChannels {
		pool.Close()
		return nil, spannerErrorf(codes.InvalidArgument, "Connection pool mismatch: NumChannels=%v, WithGRPCConnectionPool=%v. Only set one of these options, or set both to the same value.", config.NumChannels, pool.Num())
	}

	// TODO(loite): Remove as the original map cannot be changed by the user
	// anyways, and the client library is also not changing it.
	// Make a copy of labels.
	sessionLabels := make(map[string]string)
	for k, v := range config.SessionLabels {
		sessionLabels[k] = v
	}

	// Default configs for session pool.
	if config.MaxOpened == 0 {
		config.MaxOpened = uint64(pool.Num() * 100)
	}
	if config.MaxBurst == 0 {
		config.MaxBurst = DefaultSessionPoolConfig.MaxBurst
	}
	if config.incStep == 0 {
		config.incStep = DefaultSessionPoolConfig.incStep
	}
	// Create a session client.
	sc := newSessionClient(pool, database, sessionLabels, metadata.Pairs(resourcePrefixHeader, database), config.logger, config.CallOptions)
	// Create a session pool.
	config.SessionPoolConfig.sessionLabels = sessionLabels
	sp, err := newSessionPool(sc, config.SessionPoolConfig)
	if err != nil {
		sc.close()
		return nil, err
	}
	c = &Client{
		sc:           sc,
		idleSessions: sp,
		logger:       config.logger,
		qo:           getQueryOptions(config.QueryOptions),
	}
	return c, nil
}

// Combines the default options from the generated client, the default options
// of the hand-written client and the user options to one list of options.
// Precedence: userOpts > clientDefaultOpts > generatedDefaultOpts
func allClientOpts(numChannels int, userOpts ...option.ClientOption) []option.ClientOption {
	generatedDefaultOpts := vkit.DefaultClientOptions()
	clientDefaultOpts := []option.ClientOption{
		option.WithGRPCConnectionPool(numChannels),
		option.WithUserAgent(clientUserAgent),
		internaloption.EnableDirectPath(true),
	}
	allDefaultOpts := append(generatedDefaultOpts, clientDefaultOpts...)
	return append(allDefaultOpts, userOpts...)
}

// getQueryOptions returns the query options overwritten by the environment
// variables if exist. The input parameter is the query options set by users
// via application-level configuration. If the environment variables are set,
// this will return the overwritten query options.
func getQueryOptions(opts QueryOptions) QueryOptions {
	if opts.Options == nil {
		opts.Options = &sppb.ExecuteSqlRequest_QueryOptions{}
	}
	opv := os.Getenv("SPANNER_OPTIMIZER_VERSION")
	if opv != "" {
		opts.Options.OptimizerVersion = opv
	}
	opsp := os.Getenv("SPANNER_OPTIMIZER_STATISTICS_PACKAGE")
	if opsp != "" {
		opts.Options.OptimizerStatisticsPackage = opsp
	}
	return opts
}

// Close closes the client.
func (c *Client) Close() {
	if c.idleSessions != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c.idleSessions.close(ctx)
	}
	c.sc.close()
}

// Single provides a read-only snapshot transaction optimized for the case
// where only a single read or query is needed.  This is more efficient than
// using ReadOnlyTransaction() for a single read or query.
//
// Single will use a strong TimestampBound by default. Use
// ReadOnlyTransaction.WithTimestampBound to specify a different
// TimestampBound. A non-strong bound can be used to reduce latency, or
// "time-travel" to prior versions of the database, see the documentation of
// TimestampBound for details.
func (c *Client) Single() *ReadOnlyTransaction {
	t := &ReadOnlyTransaction{singleUse: true}
	t.txReadOnly.sp = c.idleSessions
	t.txReadOnly.txReadEnv = t
	t.txReadOnly.qo = c.qo
	t.txReadOnly.replaceSessionFunc = func(ctx context.Context) error {
		if t.sh == nil {
			return spannerErrorf(codes.InvalidArgument, "missing session handle on transaction")
		}
		// Remove the session that returned 'Session not found' from the pool.
		t.sh.destroy()
		// Reset the transaction, acquire a new session and retry.
		t.state = txNew
		sh, _, err := t.acquire(ctx)
		if err != nil {
			return err
		}
		t.sh = sh
		return nil
	}
	return t
}

// ReadOnlyTransaction returns a ReadOnlyTransaction that can be used for
// multiple reads from the database.  You must call Close() when the
// ReadOnlyTransaction is no longer needed to release resources on the server.
//
// ReadOnlyTransaction will use a strong TimestampBound by default.  Use
// ReadOnlyTransaction.WithTimestampBound to specify a different
// TimestampBound.  A non-strong bound can be used to reduce latency, or
// "time-travel" to prior versions of the database, see the documentation of
// TimestampBound for details.
func (c *Client) ReadOnlyTransaction() *ReadOnlyTransaction {
	t := &ReadOnlyTransaction{
		singleUse:       false,
		txReadyOrClosed: make(chan struct{}),
	}
	t.txReadOnly.sp = c.idleSessions
	t.txReadOnly.txReadEnv = t
	t.txReadOnly.qo = c.qo
	return t
}

// BatchReadOnlyTransaction returns a BatchReadOnlyTransaction that can be used
// for partitioned reads or queries from a snapshot of the database. This is
// useful in batch processing pipelines where one wants to divide the work of
// reading from the database across multiple machines.
//
// Note: This transaction does not use the underlying session pool but creates a
// new session each time, and the session is reused across clients.
//
// You should call Close() after the txn is no longer needed on local
// client, and call Cleanup() when the txn is finished for all clients, to free
// the session.
func (c *Client) BatchReadOnlyTransaction(ctx context.Context, tb TimestampBound) (*BatchReadOnlyTransaction, error) {
	var (
		tx  transactionID
		rts time.Time
		s   *session
		sh  *sessionHandle
		err error
	)
	defer func() {
		if err != nil && sh != nil {
			s.delete(ctx)
		}
	}()

	// Create session.
	s, err = c.sc.createSession(ctx)
	if err != nil {
		return nil, err
	}
	sh = &sessionHandle{session: s}

	// Begin transaction.
	res, err := sh.getClient().BeginTransaction(contextWithOutgoingMetadata(ctx, sh.getMetadata()), &sppb.BeginTransactionRequest{
		Session: sh.getID(),
		Options: &sppb.TransactionOptions{
			Mode: &sppb.TransactionOptions_ReadOnly_{
				ReadOnly: buildTransactionOptionsReadOnly(tb, true),
			},
		},
	})
	if err != nil {
		return nil, ToSpannerError(err)
	}
	tx = res.Id
	if res.ReadTimestamp != nil {
		rts = time.Unix(res.ReadTimestamp.Seconds, int64(res.ReadTimestamp.Nanos))
	}

	t := &BatchReadOnlyTransaction{
		ReadOnlyTransaction: ReadOnlyTransaction{
			tx:              tx,
			txReadyOrClosed: make(chan struct{}),
			state:           txActive,
			rts:             rts,
		},
		ID: BatchReadOnlyTransactionID{
			tid: tx,
			sid: sh.getID(),
			rts: rts,
		},
	}
	t.txReadOnly.sh = sh
	t.txReadOnly.txReadEnv = t
	t.txReadOnly.qo = c.qo
	return t, nil
}

// BatchReadOnlyTransactionFromID reconstruct a BatchReadOnlyTransaction from
// BatchReadOnlyTransactionID
func (c *Client) BatchReadOnlyTransactionFromID(tid BatchReadOnlyTransactionID) *BatchReadOnlyTransaction {
	s, err := c.sc.sessionWithID(tid.sid)
	if err != nil {
		logf(c.logger, "unexpected error: %v\nThis is an indication of an internal error in the Spanner client library.", err)
		// Use an invalid session. Preferably, this method should just return
		// the error instead of this, but that would mean an API change.
		s = &session{}
	}
	sh := &sessionHandle{session: s}

	t := &BatchReadOnlyTransaction{
		ReadOnlyTransaction: ReadOnlyTransaction{
			tx:              tid.tid,
			txReadyOrClosed: make(chan struct{}),
			state:           txActive,
			rts:             tid.rts,
		},
		ID: tid,
	}
	t.txReadOnly.sh = sh
	t.txReadOnly.txReadEnv = t
	t.txReadOnly.qo = c.qo
	return t
}

type transactionInProgressKey struct{}

func checkNestedTxn(ctx context.Context) error {
	if ctx.Value(transactionInProgressKey{}) != nil {
		return spannerErrorf(codes.FailedPrecondition, "Cloud Spanner does not support nested transactions")
	}
	return nil
}

// ReadWriteTransaction executes a read-write transaction, with retries as
// necessary.
//
// The function f will be called one or more times. It must not maintain
// any state between calls.
//
// If the transaction cannot be committed or if f returns an ABORTED error,
// ReadWriteTransaction will call f again. It will continue to call f until the
// transaction can be committed or the Context times out or is cancelled.  If f
// returns an error other than ABORTED, ReadWriteTransaction will abort the
// transaction and return the error.
//
// To limit the number of retries, set a deadline on the Context rather than
// using a fixed limit on the number of attempts. ReadWriteTransaction will
// retry as needed until that deadline is met.
//
// See https://godoc.org/cloud.google.com/go/spanner#ReadWriteTransaction for
// more details.
func (c *Client) ReadWriteTransaction(ctx context.Context, f func(context.Context, *ReadWriteTransaction) error) (commitTimestamp time.Time, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/spanner.ReadWriteTransaction")
	defer func() { trace.EndSpan(ctx, err) }()
	resp, err := c.rwTransaction(ctx, f, TransactionOptions{})
	return resp.CommitTs, err
}

// ReadWriteTransactionWithOptions executes a read-write transaction with
// configurable options, with retries as necessary.
//
// ReadWriteTransactionWithOptions is a configurable ReadWriteTransaction.
//
// See https://godoc.org/cloud.google.com/go/spanner#ReadWriteTransaction for
// more details.
func (c *Client) ReadWriteTransactionWithOptions(ctx context.Context, f func(context.Context, *ReadWriteTransaction) error, options TransactionOptions) (resp CommitResponse, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/spanner.ReadWriteTransactionWithOptions")
	defer func() { trace.EndSpan(ctx, err) }()
	resp, err = c.rwTransaction(ctx, f, options)
	return resp, err
}

func (c *Client) rwTransaction(ctx context.Context, f func(context.Context, *ReadWriteTransaction) error, options TransactionOptions) (resp CommitResponse, err error) {
	if err := checkNestedTxn(ctx); err != nil {
		return resp, err
	}
	var (
		sh *sessionHandle
	)
	defer func() {
		if sh != nil {
			sh.recycle()
		}
	}()
	err = runWithRetryOnAbortedOrSessionNotFound(ctx, func(ctx context.Context) error {
		var (
			err error
			t   *ReadWriteTransaction
		)
		if sh == nil || sh.getID() == "" || sh.getClient() == nil {
			// Session handle hasn't been allocated or has been destroyed.
			sh, err = c.idleSessions.takeWriteSession(ctx)
			if err != nil {
				// If session retrieval fails, just fail the transaction.
				return err
			}
			t = &ReadWriteTransaction{
				tx: sh.getTransactionID(),
			}
		} else {
			t = &ReadWriteTransaction{}
		}
		t.txReadOnly.sh = sh
		t.txReadOnly.txReadEnv = t
		t.txReadOnly.qo = c.qo
		t.txOpts = options

		trace.TracePrintf(ctx, map[string]interface{}{"transactionID": string(sh.getTransactionID())},
			"Starting transaction attempt")
		if err = t.begin(ctx); err != nil {
			return err
		}
		resp, err = t.runInTransaction(ctx, f)
		return err
	})
	return resp, err
}

// applyOption controls the behavior of Client.Apply.
type applyOption struct {
	// If atLeastOnce == true, Client.Apply will execute the mutations on Cloud
	// Spanner at least once.
	atLeastOnce bool
	// transactionTag will be included with the CommitRequest.
	transactionTag string
	// priority is the RPC priority that is used for the commit operation.
	priority sppb.RequestOptions_Priority
}

// An ApplyOption is an optional argument to Apply.
type ApplyOption func(*applyOption)

// ApplyAtLeastOnce returns an ApplyOption that removes replay protection.
//
// With this option, Apply may attempt to apply mutations more than once; if
// the mutations are not idempotent, this may lead to a failure being reported
// when the mutation was applied more than once. For example, an insert may
// fail with ALREADY_EXISTS even though the row did not exist before Apply was
// called. For this reason, most users of the library will prefer not to use
// this option.  However, ApplyAtLeastOnce requires only a single RPC, whereas
// Apply's default replay protection may require an additional RPC.  So this
// option may be appropriate for latency sensitive and/or high throughput blind
// writing.
func ApplyAtLeastOnce() ApplyOption {
	return func(ao *applyOption) {
		ao.atLeastOnce = true
	}
}

// TransactionTag returns an ApplyOption that will include the given tag as a
// transaction tag for a write-only transaction.
func TransactionTag(tag string) ApplyOption {
	return func(ao *applyOption) {
		ao.transactionTag = tag
	}
}

// Priority returns an ApplyOptions that sets the RPC priority to use for the
// commit operation.
func Priority(priority sppb.RequestOptions_Priority) ApplyOption {
	return func(ao *applyOption) {
		ao.priority = priority
	}
}

// Apply applies a list of mutations atomically to the database.
func (c *Client) Apply(ctx context.Context, ms []*Mutation, opts ...ApplyOption) (commitTimestamp time.Time, err error) {
	ao := &applyOption{}
	for _, opt := range opts {
		opt(ao)
	}

	ctx = trace.StartSpan(ctx, "cloud.google.com/go/spanner.Apply")
	defer func() { trace.EndSpan(ctx, err) }()

	if !ao.atLeastOnce {
		resp, err := c.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, t *ReadWriteTransaction) error {
			return t.BufferWrite(ms)
		}, TransactionOptions{CommitPriority: ao.priority, TransactionTag: ao.transactionTag})
		return resp.CommitTs, err
	}
	t := &writeOnlyTransaction{sp: c.idleSessions, commitPriority: ao.priority, transactionTag: ao.transactionTag}
	return t.applyAtLeastOnce(ctx, ms...)
}

// logf logs the given message to the given logger, or the standard logger if
// the given logger is nil.
func logf(logger *log.Logger, format string, v ...interface{}) {
	if logger == nil {
		log.Printf(format, v...)
	} else {
		logger.Printf(format, v...)
	}
}
