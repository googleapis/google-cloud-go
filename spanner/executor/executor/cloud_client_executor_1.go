package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"reflect"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	executorpb "cloud.google.com/go/spanner/executor/proto"
	executorservicepb "cloud.google.com/go/spanner/executor/proto"
	proto3 "github.com/golang/protobuf/ptypes/struct"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type currentActiveTransaction int

const (
	None currentActiveTransaction = iota
	Read
	ReadWrite
	Batch
)

// executionFlowContext represents a context in which SpannerActions are executed. Among other
// things, it includes currently active transactions and table metadata. There is exactly one
// instance of this per stubby call, created when the call is initialized and shared with all
// actionHandlers.
type executionFlowContext struct {
	mu                       sync.Mutex                             // protects all internal state
	database                 string                                 // current database path
	rwTxn                    *spanner.ReadWriteStmtBasedTransaction // Current read-write transaction
	roTxn                    *spanner.ReadOnlyTransaction           // Current read-only transaction
	batchTxn                 *spanner.BatchReadOnlyTransaction      // Current batch read-only transaction
	dbClient                 *spanner.Client                        // Current database client
	tableMetadata            *tableMetadataHelper                   // If in a txn (except batch), this has metadata info about table columns
	numPendingReads          int64                                  // Number of pending read/query actions.
	readAborted              bool                                   // Indicate whether there's a read/query action got aborted and the transaction need to be reset.
	transactionSeed          string                                 // Log the workid and op pair for tracing the thread.
	currentActiveTransaction currentActiveTransaction
	txnContext               context.Context
	// Contain the error string for buffered mutation if bad delete range error exists, this will be
	// used when commit reads only followed by bad delete range mutation.
	badDeleteRangeErr string
}

// Check if given mutation contains bad delete range.
func (c *executionFlowContext) checkBadDeleteRange(m *executorpb.MutationAction) {
	for _, mod := range m.Mod {
		if mod.GetDeleteKeys() != nil {
			for _, kr := range mod.GetDeleteKeys().GetRange() {
				start := kr.GetStart()
				limit := kr.GetLimit()
				for i, p := range start.GetValue() {
					if c.badDeleteRangeErr == "" && i < len(start.GetValue())-1 && p != limit.GetValue()[i] {
						//c.badDeleteRangeErr = fmt.Sprintf("For delete ranges, start and limit keys may only differ in the final key part: start=%s limit=%s", start.String(), limit.String())
						c.badDeleteRangeErr = ""
						return
					}
				}
			}
		}
	}
}

func (c *executionFlowContext) finishRead(code codes.Code) {
	if code == codes.Aborted {
		c.readAborted = true
	}
	c.numPendingReads--
	if c.readAborted && c.numPendingReads <= 0 {
		log.Println("Transaction reset due to read/query abort")
		c.readAborted = false
	}
}

// isTransactionActive returns true if any kind of transaction is currently active. Must hold c.mu
// when calling.
func (c *executionFlowContext) isTransactionActive() bool {
	return c.rwTxn != nil || c.roTxn != nil || c.batchTxn != nil
}

// Return current database. Must hold c.mu when calling.
func (c *executionFlowContext) getDatabase() (string, error) {
	if c.database == "" {
		return "", errors.New("database doesn't exist")
	}
	return c.database, nil
}

// Return current concurrency mode. Must hold c.mu when calling.
func (c *executionFlowContext) getTransactionForRead() (*spanner.ReadOnlyTransaction, error) {
	if c.roTxn != nil {
		return c.roTxn, nil
	}
	return nil, errors.New("no currently active transaction for read")
}

func (c *executionFlowContext) getTransactionForWrite() (*spanner.ReadWriteStmtBasedTransaction, error) {
	if c.rwTxn != nil {
		return c.rwTxn, nil
	}
	return nil, errors.New("no currently active transaction for read-write")
}

func (c *executionFlowContext) getBatchTransaction() (*spanner.BatchReadOnlyTransaction, error) {
	if c.batchTxn != nil {
		return c.batchTxn, nil
	}
	return nil, errors.New("no currently active batch transaction")
}

// finish attempts to finish the transaction by either committing it or exiting without committing.
// In order to follow the ExecuteActions protocol, we must distinguish Spanner-generated errors
// (e.g. RowNotFound) and internal errors (e.g. a precondition is not met). Errors returned from
// Spanner populate the status of SpannerActionOutcome. Internal errors, in contrast, break the
// stubby call. For this reason, finish() has two return values dedicated to errors. If any of
// these errors is not nil, other return values are undefined.
// Return values in order:
// 1. Whether or not the transaction is restarted. It will be true if commit has been attempted,
// but Spanner returned aborted and restarted instead. When that happens, all reads and writes
// should be replayed, followed by another commit attempt.
// 2. Commit timestamp. It's returned only if commit has succeeded.
// 3. Spanner error -- an error that Spanner client returned.
// 4. Internal error.
func (c *executionFlowContext) finish(txnFinishMode executorpb.FinishTransactionAction_Mode) (bool, *time.Time, error, error) {
	if txnFinishMode == executorpb.FinishTransactionAction_COMMIT {
		var err error
		ts, err := c.rwTxn.Commit(c.txnContext)
		if err != nil {
			log.Printf("transaction finished with error %v", err)
			if status.Code(err) == codes.Aborted {
				log.Println("transaction aborted")
				// TODO(harsha): transaction retry logic
				// c.rwTxn.ResetForRetry()
				return true, nil, nil, nil
			}
			return false, nil, err, nil
		}
		return false, &ts, nil, nil
	} else if txnFinishMode == executorpb.FinishTransactionAction_ABANDON {
		log.Printf("transaction abandoned")
		c.rwTxn.Rollback(c.txnContext)
		return false, nil, nil, nil
	} else {
		return false, nil, nil, fmt.Errorf("unrecognized finish mode %s", txnFinishMode.String())
	}
}

// actionHandler is an interface representing an entity responsible for executing a particular
// kind of SpannerActions.
type cloudActionHandler interface {
	executeAction(context.Context) error
}

// cloudStreamHandler handles a single streaming ExecuteActions request by performing incoming
// actions. It maintains a state associated with the request, such as current transaction.
//
// cloudStreamHandler uses contexts (context.Context) to coordinate execution of asynchronous
// actions. The Stubby stream's context becomes a parent for all individual actions' contexts. This
// is done so that we don't leak anything when the stream is closed.
//
// startTxnHandler is a bit different from other actions. Read-write transactions that it
// starts outlive the action itself, so the Stubby stream's context is used for transactions
// instead of the action's context.
//
// For more info about contexts in Go, read golang.org/pkg/context
type cloudStreamHandler struct {
	// members below should be set by the caller
	cloudProxyServer *CloudProxyServer
	stream           executorservicepb.SpannerExecutorProxy_ExecuteActionAsyncServer
	// members below represent internal state
	context *executionFlowContext
	mu      sync.Mutex // protects mutable internal state
}

// Update current database if necessary, h.mu hold by startHandlingRequest.
func (h *cloudStreamHandler) updateDatabase(dbPath string) error {
	if dbPath != "" {
		h.context.database = dbPath
	}
	return nil
}

// execute executes the given ExecuteActions request, blocking until it's done. It takes care of
// properly closing the request stream in the end.
func (h *cloudStreamHandler) execute() error {
	// When the stream is over, flush logs. This works around the problem that when systest
	// proxy exits abruptly, the most recent logs are missing.
	// defer google.Flush()

	log.Println("Start handling ExecuteActionAsync stream")

	// Init internal state
	var c *executionFlowContext
	func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		c = &executionFlowContext{}
		h.context = c
	}()

	// In case this function returns abruptly, or client misbehaves, make sure to dispose of
	// transactions.
	defer func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.roTxn != nil {
			log.Println("Closing a snapshot transaction that was open when execute() returned.")
			c.roTxn.Close()
		}
		if c.rwTxn != nil {
			log.Println("Abandon a read-write transaction that was open when execute() returned")
			_, _, _, err := c.finish(executorpb.FinishTransactionAction_ABANDON)
			if err != nil {
				log.Fatalf("Failed to abandon a read-write transaction: %v", err)
			}
		}
		if c.batchTxn != nil {
			log.Println("Closing a batch transaction that was open when execute() returned.")
			c.batchTxn.Close()
		}
	}()

	// Main loop that receives and executes actions.
	for {
		req, err := h.stream.Recv()
		if err == io.EOF {
			log.Println("Client called Done, half-closed the stream")
			break
		}
		if err != nil {
			log.Printf("Failed to receive request from client: %v", err)
			return err
		}
		if err = h.startHandlingRequest(h.stream.Context(), req); err != nil {
			log.Printf("Failed to handle request %v, half closed: %v", req, err)
			return err
		}
	}

	// If a transaction is still active when the stream is closed by the client, return an
	// error. The function deferred above will take care of closing the hanging transaction.
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.isTransactionActive() {
		log.Printf("Client closed the stream while a transaction was active")
		// TODO(Harsha): check why transaction not closed
		//return errors.New("a transaction remains active when the stream is done")
	}

	log.Println("Done executing actions")
	return nil
}

// startHandlingRequest takes care of the given request. It picks an actionHandler and starts
// a goroutine in which that action will be executed.
func (h *cloudStreamHandler) startHandlingRequest(ctx context.Context, req *executorservicepb.SpannerAsyncActionRequest) error {
	log.Printf("start handling request %v", req)
	h.mu.Lock()
	defer h.mu.Unlock()

	actionID := req.GetActionId()
	action := req.GetAction()
	outcomeSender := &outcomeSender{
		actionID:       actionID,
		stream:         h.stream,
		hasReadResult:  false,
		hasQueryResult: false,
	}

	if action == nil {
		log.Println("Invalid AsyncActionRequest")
		return outcomeSender.finishWithError(spanner.ToSpannerError(status.Error(codes.InvalidArgument, "invalid request")))
	}

	actionHandler, err := h.newActionHandler(ctx, action, outcomeSender)
	if err != nil {
		return err
	}

	// Create a channel to receive the error from the goroutine.
	errCh := make(chan error, 1)
	successCh := make(chan bool, 1)

	go func() {
		err := actionHandler.executeAction(ctx)
		if err != nil {
			log.Printf("Failed to execute action with error %v: %v", action, err)
			errCh <- err
		} else {
			successCh <- true
		}
	}()

	// Wait for the goroutine to finish or return an error if one occurs.
	select {
	case err := <-errCh:
		// An error occurred in the goroutine.
		log.Printf("Failed to execute action: %v", err)
		return err
	case <-successCh:
		// Success signal received.
		log.Println("Action executed successfully")
		return nil
		//case <-ctx.Done():
		// The context was canceled or timed out.
		//	return ctx.Err()
	}
}

// newActionHandler instantiates an actionHandler for executing the given action.
func (h *cloudStreamHandler) newActionHandler(ctx context.Context, action *executorpb.SpannerAction, outcomeSender *outcomeSender) (cloudActionHandler, error) {
	if action.DatabasePath != "" {
		h.context.database = action.DatabasePath
	}
	switch action.GetAction().(type) {
	case *executorpb.SpannerAction_Start:
		return &startTxnHandler{
			action:        action.GetStart(),
			txnContext:    h.stream.Context(),
			flowContext:   h.context,
			outcomeSender: outcomeSender,
			options:       h.cloudProxyServer.options,
		}, nil
	case *executorpb.SpannerAction_Finish:
		return &finishTxnHandler{
			action:        action.GetFinish(),
			flowContext:   h.context,
			outcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_Admin:
		adminAction := &adminActionHandler{
			action:        action.GetAdmin(),
			context:       h.stream.Context(),
			flowContext:   h.context,
			outcomeSender: outcomeSender,
			options:       h.cloudProxyServer.options,
		}
		adminAction.flowContext.txnContext = h.stream.Context()
		return adminAction, nil
	case *executorpb.SpannerAction_Read:
		return &readActionHandler{
			action:        action.GetRead(),
			flowContext:   h.context,
			outcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_Query:
		return &queryActionHandler{
			action:        action.GetQuery(),
			flowContext:   h.context,
			outcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_Mutation:
		return &mutationActionHandler{
			action:        action.GetMutation(),
			flowContext:   h.context,
			outcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_Write:
		return &writeActionHandler{
			action:        action.GetWrite().GetMutation(),
			flowContext:   h.context,
			outcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_Dml:
		return &dmlActionHandler{
			action:        action.GetDml(),
			flowContext:   h.context,
			outcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_StartBatchTxn:
		return &startBatchTxnHandler{
			action:        action.GetStartBatchTxn(),
			txnContext:    h.stream.Context(),
			flowContext:   h.context,
			outcomeSender: outcomeSender,
			options:       h.cloudProxyServer.options,
		}, nil
	case *executorpb.SpannerAction_GenerateDbPartitionsRead:
		return &partitionReadActionHandler{
			action:        action.GetGenerateDbPartitionsRead(),
			flowContext:   h.context,
			outcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_GenerateDbPartitionsQuery:
		return &partitionQueryActionHandler{
			action:        action.GetGenerateDbPartitionsQuery(),
			flowContext:   h.context,
			outcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_ExecutePartition:
		return &executePartition{
			action:        action.GetExecutePartition(),
			flowContext:   h.context,
			outcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_PartitionedUpdate:
		return &partitionedUpdate{
			action:        action.GetPartitionedUpdate(),
			flowContext:   h.context,
			outcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_CloseBatchTxn:
		return &closeBatchTxnHandler{
			action:        action.GetCloseBatchTxn(),
			flowContext:   h.context,
			outcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_BatchDml:
		return &batchDmlHandler{
			action:        action.GetBatchDml(),
			flowContext:   h.context,
			outcomeSender: outcomeSender,
		}, nil
	default:
		return nil, outcomeSender.finishWithError(status.Error(codes.Unimplemented, fmt.Sprintf("not implemented yet %T", action.GetAction())))
	}
}

type startTxnHandler struct {
	action *executorpb.StartTransactionAction
	// This action only starts a transaction but not finishes it, so the transaction outlives
	// the action. For this reason, the action's context can't be used to create
	// the transaction. Instead, this txnContext is used.
	txnContext    context.Context
	flowContext   *executionFlowContext
	outcomeSender *outcomeSender
	options       []option.ClientOption
}

func (h *startTxnHandler) executeAction(ctx context.Context) error {
	h.flowContext.mu.Lock()
	defer h.flowContext.mu.Unlock()
	if h.flowContext.database == "" {
		return h.outcomeSender.finishWithError(spanner.ToSpannerError(status.Error(codes.InvalidArgument, "database path must be set for this action")))
	}
	if h.action.GetTransactionSeed() != "" {
		h.flowContext.transactionSeed = h.action.GetTransactionSeed()
	}
	metadata := &tableMetadataHelper{}
	metadata.initFrom(h.action)
	h.flowContext.tableMetadata = metadata

	// TODO(harsha) where do I close the client? defer client.Close()
	client, err := spanner.NewClient(h.txnContext, h.flowContext.database, h.options...)
	if err != nil {
		return err
	}
	h.flowContext.dbClient = client
	if h.flowContext.isTransactionActive() {
		return h.outcomeSender.finishWithError(spanner.ToSpannerError(status.Error(codes.InvalidArgument, "already in a transaction")))
	}
	if h.action.Concurrency != nil {
		log.Printf("starting read-only transaction %s:\n %v", h.flowContext.transactionSeed, h.action)
		timestampBound, err := timestampBoundsFromConcurrency(h.action.GetConcurrency())
		if err != nil {
			return h.outcomeSender.finishWithError(err)
		}

		var txn *spanner.ReadOnlyTransaction
		singleUseReadOnlyTransactionNeeded := isSingleUseReadOnlyTransactionNeeded(h.action.GetConcurrency())
		if singleUseReadOnlyTransactionNeeded {
			txn = client.Single().WithTimestampBound(timestampBound)
		} else {
			txn = client.ReadOnlyTransaction().WithTimestampBound(timestampBound)
		}
		h.flowContext.roTxn = txn
		h.flowContext.currentActiveTransaction = Read
	} else {
		log.Printf("starting read-write transaction %s:\n %v", h.flowContext.transactionSeed, h.action)

		// Define the callable function to be executed within the transaction
		/*callable := func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
			// Perform transactional logic here
			_, err := txn.ReadRow(ctx, "YourTable", spanner.Key{"your-key"}, []string{"your-column"})
			if err != nil {
				return err
			}
			// ... additional transactional logic

			return nil
		}*/

		var txn *spanner.ReadWriteStmtBasedTransaction
		if h.action.GetExecutionOptions().GetOptimistic() {
			txn, err = spanner.NewReadWriteStmtBasedTransactionWithOptions(h.txnContext, client, spanner.TransactionOptions{ReadLockMode: sppb.TransactionOptions_ReadWrite_OPTIMISTIC})
		} else {
			txn, err = spanner.NewReadWriteStmtBasedTransaction(h.txnContext, client)
		}
		if err != nil {
			return err
		}
		h.flowContext.rwTxn = txn
		h.flowContext.currentActiveTransaction = ReadWrite
		h.flowContext.badDeleteRangeErr = ""
	}
	h.flowContext.txnContext = h.txnContext
	h.flowContext.readAborted = false
	h.flowContext.numPendingReads = 0
	return h.outcomeSender.finishSuccessfully()
}

type finishTxnHandler struct {
	action        *executorpb.FinishTransactionAction
	flowContext   *executionFlowContext
	outcomeSender *outcomeSender
}

func (h *finishTxnHandler) executeAction(ctx context.Context) error {
	log.Printf("finishing transaction %s\n %v", h.flowContext.transactionSeed, h.action)
	if h.flowContext.numPendingReads > 0 {
		return h.outcomeSender.finishWithError(spanner.ToSpannerError(status.Error(codes.FailedPrecondition, "Reads pending when trying to finish")))
	}
	o := &executorpb.SpannerActionOutcome{Status: &spb.Status{Code: int32(codes.OK)}}

	h.flowContext.mu.Lock()
	defer h.flowContext.mu.Unlock()

	if h.flowContext.roTxn != nil {
		// Finish a read-only transaction. Note that timestamp may not be available
		// if there were no reads or queries.
		ts, err := h.flowContext.roTxn.Timestamp()
		if err != nil {
			return h.outcomeSender.finishWithError(err)
		}

		o.CommitTime = timestamppb.New(ts)
		h.flowContext.roTxn = nil
		h.flowContext.rwTxn = nil
		h.flowContext.tableMetadata = nil
		return h.outcomeSender.sendOutcome(o)
	}

	if h.flowContext.rwTxn != nil {
		// Finish a read-write transaction.
		txnFinishMode := h.action.GetMode()
		restarted, ts, spanErr, internalErr := h.flowContext.finish(txnFinishMode)
		if internalErr != nil {
			return internalErr
		}
		if spanErr != nil {
			o.Status = errToStatus(spanErr)
			h.flowContext.rwTxn = nil
			h.flowContext.tableMetadata = nil
			h.flowContext.badDeleteRangeErr = ""
		} else if restarted {
			restart := true
			o.TransactionRestarted = &restart
			h.flowContext.badDeleteRangeErr = ""
		} else {
			if ts != nil {
				o.CommitTime = timestamppb.New(*ts)
			}
			h.flowContext.rwTxn = nil
			h.flowContext.tableMetadata = nil
			h.flowContext.badDeleteRangeErr = ""
		}
		return h.outcomeSender.sendOutcome(o)
	}

	if h.flowContext.batchTxn != nil {
		return h.outcomeSender.finishWithError(spanner.ToSpannerError(status.Error(codes.InvalidArgument, "Can't commit/abort a batch transaction")))
	}

	return h.outcomeSender.finishWithError(spanner.ToSpannerError(status.Error(codes.InvalidArgument, "no currently active transaction")))
}

type writeActionHandler struct {
	action        *executorpb.MutationAction
	flowContext   *executionFlowContext
	outcomeSender *outcomeSender
}

func (h *writeActionHandler) executeAction(ctx context.Context) error {
	log.Printf("writing mutation %v", h.action)
	h.flowContext.mu.Lock()
	defer h.flowContext.mu.Unlock()
	m, err := createMutation(h.action, h.flowContext.tableMetadata)
	if err != nil {
		return h.outcomeSender.finishWithError(err)
	}

	_, err = h.flowContext.dbClient.Apply(ctx, m)
	if err != nil {
		return h.outcomeSender.finishWithError(err)
	}
	return h.outcomeSender.finishSuccessfully()
}

type mutationActionHandler struct {
	action        *executorpb.MutationAction
	flowContext   *executionFlowContext
	outcomeSender *outcomeSender
}

func (h *mutationActionHandler) executeAction(ctx context.Context) error {
	log.Printf("buffering mutation %v", h.action)
	h.flowContext.mu.Lock()
	defer h.flowContext.mu.Unlock()
	txn, err := h.flowContext.getTransactionForWrite()
	if err != nil {
		return h.outcomeSender.finishWithError(err)
	}
	m, err := createMutation(h.action, h.flowContext.tableMetadata)
	if err != nil {
		return h.outcomeSender.finishWithError(err)
	}

	err = txn.BufferWrite(m)
	if err != nil {
		return h.outcomeSender.finishWithError(err)
	}
	// TODO(harsha): check if this checkBadDeleteRange is needed?
	// h.flowContext.checkBadDeleteRange(h.action)
	return h.outcomeSender.finishSuccessfully()
}

type readActionHandler struct {
	action        *executorpb.ReadAction
	flowContext   *executionFlowContext
	outcomeSender *outcomeSender
}

func (h *readActionHandler) executeAction(ctx context.Context) error {
	log.Printf("executing read %s:\n %v", h.flowContext.transactionSeed, h.action)
	h.flowContext.mu.Lock()
	defer h.flowContext.mu.Unlock()
	action := h.action
	_, err := h.flowContext.getDatabase()

	var typeList []*spannerpb.Type
	if action.Index != nil {
		typeList, err = extractTypes(action.GetTable(), action.GetColumn(), h.flowContext.tableMetadata)
		if err != nil {
			return h.outcomeSender.finishWithError(status.Error(codes.InvalidArgument, fmt.Sprintf("Can't extract types from metadata: %s", err)))
		}
	} else {
		typeList, err = h.flowContext.tableMetadata.getKeyColumnTypes(action.GetTable())
		if err != nil {
			return h.outcomeSender.finishWithError(status.Error(codes.InvalidArgument, fmt.Sprintf("Can't extract types from metadata: %s", err)))
		}
	}

	keySet, err := keySetProtoToCloudKeySet(action.GetKeys(), typeList)
	if err != nil {
		return h.outcomeSender.finishWithError(status.Error(codes.InvalidArgument, fmt.Sprintf("Can't convert rowSet: %s", err)))
	}

	var iter *spanner.RowIterator
	if h.flowContext.currentActiveTransaction == None {
		return h.outcomeSender.finishWithError(fmt.Errorf("no active transaction"))
	} else if h.flowContext.currentActiveTransaction == Batch {
		return h.outcomeSender.finishWithError(fmt.Errorf("can't execute regular read in a batch transaction"))
	} else if h.flowContext.currentActiveTransaction == Read {
		txn, err := h.flowContext.getTransactionForRead()
		if err != nil {
			return fmt.Errorf("can't get read transaction: %s", err)
		}
		h.flowContext.numPendingReads++
		if action.Index != nil {
			iter = txn.ReadUsingIndex(ctx, action.GetTable(), action.GetIndex(), keySet, action.GetColumn())
		} else {
			iter = txn.Read(ctx, action.GetTable(), keySet, action.GetColumn())
		}
	} else if h.flowContext.currentActiveTransaction == ReadWrite {
		txn, err := h.flowContext.getTransactionForWrite()
		if err != nil {
			return fmt.Errorf("can't get read-write transaction: %s", err)
		}
		h.flowContext.numPendingReads++
		if action.Index != nil {
			iter = txn.ReadUsingIndex(ctx, action.GetTable(), action.GetIndex(), keySet, action.GetColumn())
		} else {
			iter = txn.Read(ctx, action.GetTable(), keySet, action.GetColumn())
		}
	}
	defer iter.Stop()

	h.outcomeSender.hasReadResult = true
	h.outcomeSender.table = action.GetTable()
	if action.Index != nil {
		h.outcomeSender.index = action.Index
	}

	/*h.flowContext.numPendingReads++

	var iter *spanner.RowIterator
	if action.Index != nil {
		iter = txn.ReadUsingIndex(ctx, action.GetTable(), action.GetIndex(), keySet, action.GetColumn())
	} else {
		iter = txn.Read(ctx, action.GetTable(), keySet, action.GetColumn())
	}
	defer iter.Stop()*/
	log.Println("parsing read result")

	err = processResults(iter, int64(action.GetLimit()), h.outcomeSender, h.flowContext)
	if err != nil {
		h.flowContext.finishRead(status.Code(err))
		if status.Code(err) == codes.Aborted {
			return h.outcomeSender.finishWithTransactionRestarted()
		}
		return h.outcomeSender.finishWithError(err)
	}
	h.flowContext.finishRead(codes.OK)
	return h.outcomeSender.finishSuccessfully()
}

func processResults(iter *spanner.RowIterator, limit int64, outcomeSender *outcomeSender, flowContext *executionFlowContext) error {
	var rowCount int64 = 0
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			return nil
		}
		if err != nil {
			return err
		}
		spannerRow, rowType, err := convertSpannerRow(row)
		if err != nil {
			return err
		}
		outcomeSender.rowType = rowType
		err = outcomeSender.appendRow(spannerRow)
		if err != nil {
			return err
		}
		rowCount++
		if limit > 0 && rowCount >= limit {
			log.Printf("Stopping at row limit: %d", limit)
			break
		}
	}

	log.Printf("Successfully processed result: %s\n", flowContext.transactionSeed)
	return nil
}

type queryActionHandler struct {
	action        *executorpb.QueryAction
	flowContext   *executionFlowContext
	outcomeSender *outcomeSender
}

func (h *queryActionHandler) executeAction(ctx context.Context) error {
	log.Printf("executing query %v", h.action)
	stmt, err := buildQuery(h.action)
	if err != nil {
		return h.outcomeSender.finishWithError(err)
	}

	h.flowContext.mu.Lock()
	defer h.flowContext.mu.Unlock()

	var iter *spanner.RowIterator
	if h.flowContext.currentActiveTransaction == None {
		return h.outcomeSender.finishWithError(fmt.Errorf("no active transaction"))
	} else if h.flowContext.currentActiveTransaction == Batch {
		return h.outcomeSender.finishWithError(fmt.Errorf("can't execute regular read in a batch transaction"))
	} else if h.flowContext.currentActiveTransaction == Read {
		txn, err := h.flowContext.getTransactionForRead()
		if err != nil {
			return fmt.Errorf("can't get read transaction: %s", err)
		}
		h.outcomeSender.hasQueryResult = true
		h.flowContext.numPendingReads++
		iter = txn.Query(ctx, stmt)
	} else if h.flowContext.currentActiveTransaction == ReadWrite {
		txn, err := h.flowContext.getTransactionForWrite()
		if err != nil {
			return fmt.Errorf("can't get read-write transaction: %s", err)
		}
		h.outcomeSender.hasQueryResult = true
		h.flowContext.numPendingReads++
		iter = txn.Query(ctx, stmt)
	}
	defer iter.Stop()
	err = processResults(iter, 0, h.outcomeSender, h.flowContext)
	if err != nil {
		h.flowContext.finishRead(status.Code(err))
		if status.Code(err) == codes.Aborted {
			return h.outcomeSender.finishWithTransactionRestarted()
		}
		return h.outcomeSender.finishWithError(err)
	}
	h.flowContext.finishRead(codes.OK)
	return h.outcomeSender.finishSuccessfully()
}

type dmlActionHandler struct {
	action        *executorpb.DmlAction
	flowContext   *executionFlowContext
	outcomeSender *outcomeSender
}

func (h *dmlActionHandler) executeAction(ctx context.Context) error {
	log.Printf("executing dml update %v", h.action)
	log.Printf("start building query")
	stmt, err := buildQuery(h.action.GetUpdate())
	log.Printf("crossed building query")
	if err != nil {
		return err
	}

	h.flowContext.mu.Lock()
	defer h.flowContext.mu.Unlock()
	log.Printf("getting transaction")

	var iter *spanner.RowIterator
	if h.flowContext.currentActiveTransaction == None {
		return h.outcomeSender.finishWithError(fmt.Errorf("no active transaction"))
	} else if h.flowContext.currentActiveTransaction == Batch {
		return h.outcomeSender.finishWithError(fmt.Errorf("can't execute regular read in a batch transaction"))
	} else if h.flowContext.currentActiveTransaction == Read {
		log.Printf("inside read")
		txn, err := h.flowContext.getTransactionForRead()
		if err != nil {
			return fmt.Errorf("can't get read transaction: %s", err)
		}
		h.outcomeSender.hasQueryResult = true
		iter = txn.Query(ctx, stmt)
		//iter = txn.QueryWithStats(ctx, stmt)
	} else if h.flowContext.currentActiveTransaction == ReadWrite {
		log.Printf("inside readwrite")
		txn, err := h.flowContext.getTransactionForWrite()
		if err != nil {
			return fmt.Errorf("can't get read-write transaction: %s", err)
		}
		h.outcomeSender.hasQueryResult = true
		iter = txn.Query(ctx, stmt)
		//iter = txn.QueryWithStats(ctx, stmt)
	}
	log.Printf("processing results")
	err = processResults(iter, 0, h.outcomeSender, h.flowContext)
	log.Printf("crossing processing results")
	if err != nil {
		if status.Code(err) == codes.Aborted {
			return h.outcomeSender.finishWithTransactionRestarted()
		}
		return h.outcomeSender.finishWithError(err)
	}

	err = h.outcomeSender.appendDmlRowsModified(iter.RowCount)
	if err != nil {
		return h.outcomeSender.finishWithError(err)
	}
	defer iter.Stop()

	// rowCount, err := txn.Update(ctx, stmt)
	return h.outcomeSender.finishSuccessfully()
}

type partitionedUpdateActionHandler struct {
	action        *executorpb.PartitionedUpdateAction
	flowContext   *executionFlowContext
	outcomeSender *outcomeSender
}

// createMutation creates cloud spanner go mutation from given tech mutation.
func createMutation(action *executorpb.MutationAction, tableMetadata *tableMetadataHelper) ([]*spanner.Mutation, error) {
	prevTable := ""
	var m []*spanner.Mutation
	for _, mod := range action.Mod {
		table := mod.GetTable()
		if table == "" {
			table = prevTable
		}
		if table == "" {
			return nil, spanner.ToSpannerError(status.Error(codes.InvalidArgument, fmt.Sprintf("table name is missing from mod: action %s ", action.String())))
		}
		prevTable = table
		log.Printf("executing mutation mod: \n %s", mod.String())

		switch {
		case mod.Insert != nil:
			ia := mod.Insert
			cloudRows, err := cloudValuesFromExecutorValueLists(ia.GetValues(), ia.GetType())
			if err != nil {
				return nil, err
			}
			for _, cloudRow := range cloudRows {
				m = append(m, spanner.Insert(table, ia.GetColumn(), cloudRow))
			}
		case mod.Update != nil:
			ua := mod.Update
			cloudRows, err := cloudValuesFromExecutorValueLists(ua.GetValues(), ua.GetType())
			if err != nil {
				return nil, err
			}
			for _, cloudRow := range cloudRows {
				m = append(m, spanner.Update(table, ua.GetColumn(), cloudRow))
			}
		case mod.InsertOrUpdate != nil:
			ia := mod.InsertOrUpdate
			cloudRows, err := cloudValuesFromExecutorValueLists(ia.GetValues(), ia.GetType())
			if err != nil {
				return nil, err
			}
			for _, cloudRow := range cloudRows {
				m = append(m, spanner.InsertOrUpdate(table, ia.GetColumn(), cloudRow))
			}
		case mod.Replace != nil:
			ia := mod.Replace
			cloudRows, err := cloudValuesFromExecutorValueLists(ia.GetValues(), ia.GetType())
			if err != nil {
				return nil, err
			}
			for _, cloudRow := range cloudRows {
				m = append(m, spanner.Replace(table, ia.GetColumn(), cloudRow))
			}
		case mod.DeleteKeys != nil:
			keyColTypes, err := tableMetadata.getKeyColumnTypes(table)
			if err != nil {
				return nil, err
			}
			keySet, err := keySetProtoToCloudKeySet(mod.DeleteKeys, keyColTypes)
			m = append(m, spanner.Delete(table, keySet))
		default:
			return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "unsupported mod: %s", mod.String()))
		}
	}
	return m, nil
}

// processRow extracts results from spanner row and sends the response through outcomeSender.
func processRow(row *spanner.Row, outcomeSender *outcomeSender) error {
	v, t, err := convertSpannerRow(row)
	if err != nil {
		return err
	}
	if outcomeSender.rowType == nil {
		outcomeSender.rowType = t
	}
	err = outcomeSender.appendRow(v)
	if err != nil {
		return err
	}
	return nil
}

// extractTypes extracts types from given table and columns, while ignoring the child rows.
func extractTypes(table string, cols []string, metadata *tableMetadataHelper) ([]*spannerpb.Type, error) {
	var typeList []*spannerpb.Type
	for _, col := range cols {
		ctype, err := metadata.getColumnType(table, col)
		if err != nil {
			return nil, err
		}
		typeList = append(typeList, ctype)
	}
	return typeList, nil
}

// toInfraRowSet converts a tech API KeySet to an Infra Spanner RowSet instance. keyPartTypes are
// types of key columns, they are required to convert key values correctly.
func keySetProtoToCloudKeySet(keySetProto *executorpb.KeySet, typeList []*spannerpb.Type) (spanner.KeySet, error) {
	if keySetProto.GetAll() {
		return spanner.AllKeys(), nil
	}
	cloudKeySet := spanner.KeySets()
	for _, techKey := range keySetProto.GetPoint() {
		cloudKey, err := keyProtoToCloudKey(techKey, typeList)
		if err != nil {
			return nil, err
		}
		cloudKeySet = spanner.KeySets(cloudKey, cloudKeySet)
	}
	for _, techRange := range keySetProto.GetRange() {
		cloudRange, err := keyRangeProtoToCloudKeyRange(techRange, typeList)
		if err != nil {
			return nil, err
		}
		cloudKeySet = spanner.KeySets(cloudKeySet, cloudRange)
	}
	return cloudKeySet, nil
}

// techKeyToInfraKey converts given tech API key with type info to an infra spanner.Key.
func keyProtoToCloudKey(keyProto *executorpb.ValueList, typeList []*spannerpb.Type) (spanner.Key, error) {
	if len(typeList) < len(keyProto.GetValue()) {
		return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "there's more serviceKeyFile parts in %s than column types in %s", keyProto, typeList))
	}

	var cloudKey spanner.Key
	for i, part := range keyProto.GetValue() {
		type_ := typeList[i]
		key, err := techKeyPartToCloudKeyPart(part, type_)
		if err != nil {
			return nil, err
		}
		cloudKey = append(cloudKey, key)
	}
	return cloudKey, nil
}

// techKeyPartToInfraKeyPart converts a single Key.Part of the given type to a value suitable for
// Cloud Spanner API.
func techKeyPartToCloudKeyPart(part *executorpb.Value, type_ *spannerpb.Type) (any, error) {
	if part.GetIsNull() {
		return nil, nil
	}
	// Refer : inmem.go -> parseQueryParam(v *structpb.Value, typ *spannerpb.Type) for switch case
	switch v := part.ValueType.(type) {
	case *executorpb.Value_IsNull:
		switch type_.GetCode() {
		case sppb.TypeCode_BOOL:
		case sppb.TypeCode_INT64:
		case sppb.TypeCode_STRING:
		case sppb.TypeCode_BYTES:
		case sppb.TypeCode_FLOAT64:
		case sppb.TypeCode_DATE:
		case sppb.TypeCode_TIMESTAMP:
		case sppb.TypeCode_NUMERIC:
		case sppb.TypeCode_JSON:
			return nil, nil
		default:
			return nil, spanner.ToSpannerError(status.Error(codes.InvalidArgument, fmt.Sprintf("unsupported null serviceKeyFile part type: %s", type_.GetCode().String())))
		}
	case *executorpb.Value_IntValue:
		return v.IntValue, nil
	case *executorpb.Value_BoolValue:
		return v.BoolValue, nil
	case *executorpb.Value_DoubleValue:
		return v.DoubleValue, nil
	case *executorpb.Value_BytesValue:
		switch type_.GetCode() {
		case sppb.TypeCode_STRING:
			return string(v.BytesValue), nil
		case sppb.TypeCode_BYTES:
			return v.BytesValue, nil
		default:
			return nil, spanner.ToSpannerError(status.New(codes.InvalidArgument, fmt.Sprintf("unsupported serviceKeyFile part type: %s", type_.GetCode().String())).Err())
		}
	case *executorpb.Value_StringValue:
		switch type_.GetCode() {
		case sppb.TypeCode_NUMERIC:
			y, ok := (&big.Rat{}).SetString(v.StringValue)
			if !ok {
				return nil, spanner.ToSpannerError(status.New(codes.FailedPrecondition, fmt.Sprintf("unexpected string value %q for numeric number", v.StringValue)).Err())
			}
			return *y, nil
		default:
			return v.StringValue, nil
		}
	case *executorpb.Value_TimestampValue:
		y, err := time.Parse(time.RFC3339Nano, v.TimestampValue.String())
		if err != nil {
			return nil, err
		}
		return y, nil
	case *executorpb.Value_DateDaysValue:
		epoch1 := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
		epoch := civil.DateOf(epoch1)
		y := epoch.AddDays(int(v.DateDaysValue))
		return y, nil
	}
	return nil, spanner.ToSpannerError(status.Error(codes.InvalidArgument, fmt.Sprintf("unsupported serviceKeyFile part %s with type %s", part, type_)))
}

// techRangeToInfraRange converts a tech API KeyRange to an infra spanner.KeyRange. It uses the
// types information provided to correctly convert key part values.
func keyRangeProtoToCloudKeyRange(keyRangeProto *executorpb.KeyRange, typeList []*spannerpb.Type) (spanner.KeyRange, error) {
	start, err := keyProtoToCloudKey(keyRangeProto.GetStart(), typeList)
	if err != nil {
		return spanner.KeyRange{}, err
	}
	end, err := keyProtoToCloudKey(keyRangeProto.GetLimit(), typeList)
	if err != nil {
		return spanner.KeyRange{}, err
	}
	if keyRangeProto.Type == nil {
		return spanner.KeyRange{Start: start, End: end, Kind: spanner.ClosedOpen}, nil
	}
	// TODO(harsha): In java they have default of closedopen when keyRangeProto does not have a type
	switch keyRangeProto.GetType() {
	case executorpb.KeyRange_CLOSED_CLOSED:
		return spanner.KeyRange{Start: start, End: end, Kind: spanner.ClosedClosed}, nil
	case executorpb.KeyRange_CLOSED_OPEN:
		return spanner.KeyRange{Start: start, End: end, Kind: spanner.ClosedOpen}, nil
	case executorpb.KeyRange_OPEN_CLOSED:
		return spanner.KeyRange{Start: start, End: end, Kind: spanner.OpenClosed}, nil
	case executorpb.KeyRange_OPEN_OPEN:
		return spanner.KeyRange{Start: start, End: end, Kind: spanner.OpenOpen}, nil
	default:
		return spanner.KeyRange{}, spanner.ToSpannerError(status.Error(codes.InvalidArgument, fmt.Sprintf("unrecognized serviceKeyFile range type %s", keyRangeProto.GetType().String())))
	}
}

// buildQuery constructs a spanner query, bind the params from a tech query.
func buildQuery(queryAction *executorpb.QueryAction) (spanner.Statement, error) {
	stmt := spanner.Statement{SQL: queryAction.GetSql(), Params: make(map[string]interface{})}
	for _, param := range queryAction.GetParams() {
		/* TODO(harsha): Check if this condition is needed
		if param.GetValue().GetIsNull() {
			stmt.Params[param.GetName()] = nil
		}*/
		value, err := executorValueToSpannerValue(param.GetType(), param.GetValue(), param.GetValue().GetIsNull())
		if err != nil {
			return spanner.Statement{}, err
		}
		stmt.Params[param.GetName()] = value
	}
	log.Println(stmt)
	return stmt, nil
}

// convertSpannerRow takes an Cloud Spanner Row and translates it to tech API Value and Type. The result is
// always a struct, in which each value corresponds to a column of the Row.
func convertSpannerRow(row *spanner.Row) (*executorpb.ValueList, *sppb.StructType, error) {
	rowBuilder := &executorpb.ValueList{}
	rowTypeBuilder := &sppb.StructType{}
	for i := 0; i < row.Size(); i++ {
		rowTypeBuilderField := &sppb.StructType_Field{Name: row.ColumnName(i), Type: row.ColumnType(i)}
		rowTypeBuilder.Fields = append(rowTypeBuilder.Fields, rowTypeBuilderField)
		v, err := extractRowValue(row, i, row.ColumnType(i))
		if err != nil {
			return nil, nil, err
		}
		rowBuilder.Value = append(rowBuilder.Value, v)
	}
	return rowBuilder, rowTypeBuilder, nil
}

// extractRowValue extracts a single column's value at given index i from result row, it also handles nested row.
func extractRowValue(row *spanner.Row, i int, t *sppb.Type) (*executorpb.Value, error) {
	val := &executorpb.Value{}
	_, isNull := row.ColumnValue(i).Kind.(*proto3.Value_NullValue)
	if isNull {
		val.ValueType = &executorpb.Value_IsNull{IsNull: true}
		return val, nil
	}
	var err error
	// nested row
	if t.GetCode() == sppb.TypeCode_ARRAY && t.GetArrayElementType().GetCode() == sppb.TypeCode_STRUCT {
		/*log.Printf("inside extractRowValue where struct in array unimplemented")
		//var v []interface{}
		var v spanner.RowIterator
		err = row.Column(i, &v)
		if err != nil {
			fmt.Println("inside extractRowValue where struct in array unimplemented 1")
		}
		err := v.Do(func(r *spanner.Row) error {
			fmt.Println(r)
			return nil
		})
		fmt.Println(err)
		val.ValueType = &executorpb.Value_StructValue{StructValue: &executorpb.ValueList{
			Value: []*executorpb.Value{},
		}}
		return val, nil*/
		log.Println("with in array<struct> which is unimplemented")
	}
	switch t.GetCode() {
	case sppb.TypeCode_BOOL:
		var v bool
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		val.ValueType = &executorpb.Value_BoolValue{BoolValue: v}
	case sppb.TypeCode_INT64:
		var v int64
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		val.ValueType = &executorpb.Value_IntValue{IntValue: v}
	case sppb.TypeCode_FLOAT64:
		var v float64
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		val.ValueType = &executorpb.Value_DoubleValue{DoubleValue: v}
	case sppb.TypeCode_NUMERIC:
		var numeric big.Rat
		err = row.Column(i, &numeric)
		if err != nil {
			return nil, err
		}
		v := spanner.NumericString(&numeric)
		val.ValueType = &executorpb.Value_StringValue{StringValue: v}
	case sppb.TypeCode_STRING:
		var v string
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		val.ValueType = &executorpb.Value_StringValue{StringValue: v}
	case sppb.TypeCode_BYTES:
		var v []byte
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		//val.ValueType = &executorpb.Value_StringValue{StringValue: base64.StdEncoding.EncodeToString(v)}
		val.ValueType = &executorpb.Value_BytesValue{BytesValue: v}
	case sppb.TypeCode_DATE:
		var v civil.Date
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		epoch1 := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
		epoch := civil.DateOf(epoch1)
		val.ValueType = &executorpb.Value_DateDaysValue{DateDaysValue: int32(v.DaysSince(epoch))}
	case sppb.TypeCode_TIMESTAMP:
		var v time.Time
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		val.ValueType = &executorpb.Value_TimestampValue{TimestampValue: &timestamppb.Timestamp{Seconds: v.Unix(), Nanos: int32(v.Nanosecond())}}
	case sppb.TypeCode_JSON:
		var v spanner.NullJSON
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		val.ValueType = &executorpb.Value_StringValue{StringValue: v.String()}
	case sppb.TypeCode_ARRAY:
		val, err = extractRowArrayValue(row, i, t.GetArrayElementType())
		if err != nil {
			return nil, err
		}
	default:
		return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "unable to extract value: type %s not supported", t.GetCode()))
	}
	return val, nil
}

// extractRowArrayValue extracts a single column's array value at given index i from result row.
func extractRowArrayValue(row *spanner.Row, i int, t *sppb.Type) (*executorpb.Value, error) {
	val := &executorpb.Value{}
	var err error
	switch t.GetCode() {
	case sppb.TypeCode_BOOL:
		arrayValue := &executorpb.ValueList{}
		var v []*bool
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		for _, booleanValue := range v {
			if booleanValue == nil {
				value := &executorpb.Value{ValueType: &executorpb.Value_IsNull{IsNull: true}}
				arrayValue.Value = append(arrayValue.Value, value)
			} else {
				value := &executorpb.Value{ValueType: &executorpb.Value_BoolValue{BoolValue: *booleanValue}}
				arrayValue.Value = append(arrayValue.Value, value)
			}
		}
		val.ValueType = &executorpb.Value_ArrayValue{ArrayValue: arrayValue}
		val.ArrayType = &sppb.Type{Code: sppb.TypeCode_BOOL}
	case sppb.TypeCode_INT64:
		arrayValue := &executorpb.ValueList{}
		var v []*int64
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		for _, vv := range v {
			if vv == nil {
				value := &executorpb.Value{ValueType: &executorpb.Value_IsNull{IsNull: true}}
				arrayValue.Value = append(arrayValue.Value, value)
			} else {
				value := &executorpb.Value{ValueType: &executorpb.Value_IntValue{IntValue: *vv}}
				arrayValue.Value = append(arrayValue.Value, value)
			}
		}
		val.ValueType = &executorpb.Value_ArrayValue{ArrayValue: arrayValue}
		val.ArrayType = &sppb.Type{Code: sppb.TypeCode_INT64}
	case spannerpb.TypeCode_FLOAT64:
		arrayValue := &executorpb.ValueList{}
		var v []*float64
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		for _, vv := range v {
			if vv == nil {
				value := &executorpb.Value{ValueType: &executorpb.Value_IsNull{IsNull: true}}
				arrayValue.Value = append(arrayValue.Value, value)
			} else {
				value := &executorpb.Value{ValueType: &executorpb.Value_DoubleValue{DoubleValue: *vv}}
				arrayValue.Value = append(arrayValue.Value, value)
			}
		}
		val.ValueType = &executorpb.Value_ArrayValue{ArrayValue: arrayValue}
		val.ArrayType = &sppb.Type{Code: sppb.TypeCode_FLOAT64}
	case spannerpb.TypeCode_NUMERIC:
		arrayValue := &executorpb.ValueList{}
		var v []*big.Rat
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		for _, vv := range v {
			if vv == nil {
				value := &executorpb.Value{ValueType: &executorpb.Value_IsNull{IsNull: true}}
				arrayValue.Value = append(arrayValue.Value, value)
			} else {
				value := &executorpb.Value{ValueType: &executorpb.Value_StringValue{StringValue: spanner.NumericString(vv)}}
				arrayValue.Value = append(arrayValue.Value, value)
			}
		}
		val.ValueType = &executorpb.Value_ArrayValue{ArrayValue: arrayValue}
		val.ArrayType = &sppb.Type{Code: sppb.TypeCode_NUMERIC}
	case spannerpb.TypeCode_STRING:
		arrayValue := &executorpb.ValueList{}
		var v []*string
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		for _, vv := range v {
			if vv == nil {
				value := &executorpb.Value{ValueType: &executorpb.Value_IsNull{IsNull: true}}
				arrayValue.Value = append(arrayValue.Value, value)
			} else {
				value := &executorpb.Value{ValueType: &executorpb.Value_StringValue{StringValue: *vv}}
				arrayValue.Value = append(arrayValue.Value, value)
			}
		}
		val.ValueType = &executorpb.Value_ArrayValue{ArrayValue: arrayValue}
		val.ArrayType = &sppb.Type{Code: sppb.TypeCode_STRING}
	case spannerpb.TypeCode_BYTES:
		arrayValue := &executorpb.ValueList{}
		var v [][]byte
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		for _, vv := range v {
			if vv == nil {
				value := &executorpb.Value{ValueType: &executorpb.Value_IsNull{IsNull: true}}
				arrayValue.Value = append(arrayValue.Value, value)
			} else {
				value := &executorpb.Value{ValueType: &executorpb.Value_BytesValue{BytesValue: vv}}
				arrayValue.Value = append(arrayValue.Value, value)
			}
		}
		val.ValueType = &executorpb.Value_ArrayValue{ArrayValue: arrayValue}
		val.ArrayType = &sppb.Type{Code: sppb.TypeCode_BYTES}
	case spannerpb.TypeCode_DATE:
		arrayValue := &executorpb.ValueList{}
		var v []*civil.Date
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		epoch1 := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
		epoch := civil.DateOf(epoch1)
		for _, vv := range v {
			if vv == nil {
				value := &executorpb.Value{ValueType: &executorpb.Value_IsNull{IsNull: true}}
				arrayValue.Value = append(arrayValue.Value, value)
			} else {
				value := &executorpb.Value{ValueType: &executorpb.Value_DateDaysValue{DateDaysValue: int32(vv.DaysSince(epoch))}}
				arrayValue.Value = append(arrayValue.Value, value)
			}
		}
		val.ValueType = &executorpb.Value_ArrayValue{ArrayValue: arrayValue}
		val.ArrayType = &sppb.Type{Code: sppb.TypeCode_DATE}
	case spannerpb.TypeCode_TIMESTAMP:
		arrayValue := &executorpb.ValueList{}
		var v []*time.Time
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		for _, vv := range v {
			if vv == nil {
				value := &executorpb.Value{ValueType: &executorpb.Value_IsNull{IsNull: true}}
				arrayValue.Value = append(arrayValue.Value, value)
			} else {
				value := &executorpb.Value{ValueType: &executorpb.Value_TimestampValue{TimestampValue: &timestamppb.Timestamp{Seconds: vv.Unix(), Nanos: int32(vv.Nanosecond())}}}
				arrayValue.Value = append(arrayValue.Value, value)
			}
		}
		val.ValueType = &executorpb.Value_ArrayValue{ArrayValue: arrayValue}
		val.ArrayType = &sppb.Type{Code: sppb.TypeCode_TIMESTAMP}
	case spannerpb.TypeCode_JSON:
		arrayValue := &executorpb.ValueList{}
		var v []spanner.NullJSON
		err = row.Column(i, &v)
		if err != nil {
			return nil, err
		}
		for _, vv := range v {
			if !vv.Valid {
				value := &executorpb.Value{ValueType: &executorpb.Value_IsNull{IsNull: true}}
				arrayValue.Value = append(arrayValue.Value, value)
			} else {
				value := &executorpb.Value{ValueType: &executorpb.Value_StringValue{StringValue: vv.String()}}
				arrayValue.Value = append(arrayValue.Value, value)
			}
		}
		val.ValueType = &executorpb.Value_ArrayValue{ArrayValue: arrayValue}
		val.ArrayType = &sppb.Type{Code: sppb.TypeCode_JSON}
	default:
		return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "unable to extract value: type %s not supported", t.GetCode()))
	}
	return val, nil
}

// timestampFromMicros converts micros to time.Time
func timestampFromMicros(micros int64) time.Time {
	seconds := micros / 1000000
	nanos := (micros % 1000000) * 1000
	return time.Unix(seconds, nanos)
}

// timestampBoundsFromConcurrency converts a tech concurrency to spanner.TimestampBound.
func timestampBoundsFromConcurrency(c *executorpb.Concurrency) (spanner.TimestampBound, error) {
	switch c.GetConcurrencyMode().(type) {
	case *executorpb.Concurrency_StalenessSeconds:
		secs := c.GetStalenessSeconds()
		dur := time.Duration(secs) * time.Second
		return spanner.ExactStaleness(dur), nil
	case *executorpb.Concurrency_MinReadTimestampMicros:
		return spanner.MinReadTimestamp(timestampFromMicros(c.GetMinReadTimestampMicros())), nil
	case *executorpb.Concurrency_MaxStalenessSeconds:
		secs := c.GetMaxStalenessSeconds()
		dur := time.Duration(secs) * time.Second
		return spanner.MaxStaleness(dur), nil
	case *executorpb.Concurrency_ExactTimestampMicros:
		return spanner.ReadTimestamp(timestampFromMicros(c.GetExactTimestampMicros())), nil
	case *executorpb.Concurrency_Strong:
		return spanner.StrongRead(), nil
	case *executorpb.Concurrency_Batch:
		return spanner.TimestampBound{}, fmt.Errorf("batch mode should not be in snapshot transaction")
	default:
		return spanner.StrongRead(), fmt.Errorf("unsupported concurrency mode %s", c.String())
	}
}

// cloudValuesFromExecutorValueLists produces rows of Cloud Spanner values given Executor ValueLists and Types. Each
// ValueList results in a row, and all of them should have the same column types.
func cloudValuesFromExecutorValueLists(valueLists []*executorpb.ValueList, types []*spannerpb.Type) ([][]any, error) {
	var cloudRows [][]any
	for _, rowValues := range valueLists {
		if len(rowValues.GetValue()) != len(types) {
			return nil, spanner.ToSpannerError(status.Error(codes.InvalidArgument, "number of values doesn't equal to number of types"))
		}

		var cloudRow []any
		for i, v := range rowValues.GetValue() {
			isNull := false
			switch v.GetValueType().(type) {
			case *executorpb.Value_IsNull:
				isNull = true
			}
			val, err := executorValueToSpannerValue(types[i], v, isNull)
			if err != nil {
				return nil, err
			}
			cloudRow = append(cloudRow, val)
		}
		cloudRows = append(cloudRows, cloudRow)
	}
	return cloudRows, nil
}

// encodedJSON is a pre-encoded JSON value, so when marhsaled the underlying
// bytes are returned as-is.
type encodedJSON []byte

func (v encodedJSON) MarshalJSON() ([]byte, error) {
	return []byte(v), nil
}

// techValueToInfraValue converts a tech spanner Value with given type t into an infra Spanner's Value.
// Parameter null indicates whether this value is NULL.
func executorValueToSpannerValue(t *spannerpb.Type, v *executorpb.Value, null bool) (any, error) {
	if v.GetIsCommitTimestamp() {
		return spanner.NullTime{Time: spanner.CommitTimestamp, Valid: true}, nil
	}
	switch t.GetCode() {
	case spannerpb.TypeCode_INT64:
		return spanner.NullInt64{Int64: v.GetIntValue(), Valid: !null}, nil
	case spannerpb.TypeCode_FLOAT64:
		return spanner.NullFloat64{Float64: v.GetDoubleValue(), Valid: !null}, nil
	case spannerpb.TypeCode_STRING:
		return spanner.NullString{StringVal: v.GetStringValue(), Valid: !null}, nil
	case spannerpb.TypeCode_BYTES:
		if null {
			return []byte(nil), nil
		}
		out := v.GetBytesValue()
		if out == nil {
			out = make([]byte, 0)
		}
		return out, nil
	case spannerpb.TypeCode_BOOL:
		return spanner.NullBool{Bool: v.GetBoolValue(), Valid: !null}, nil
	case spannerpb.TypeCode_TIMESTAMP:
		if null {
			return spanner.NullTime{Time: time.Unix(0, 0), Valid: false}, nil
		}
		if v.GetIsCommitTimestamp() || v.GetBytesValue() != nil {
			return spanner.NullTime{Time: spanner.CommitTimestamp, Valid: true}, nil
		}
		return spanner.NullTime{Time: time.Unix(v.GetTimestampValue().Seconds, int64(v.GetTimestampValue().Nanos)), Valid: true}, nil
	case spannerpb.TypeCode_DATE:
		epoch1 := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
		epoch := civil.DateOf(epoch1)
		y := epoch.AddDays(int(v.GetDateDaysValue()))
		return spanner.NullDate{Date: y, Valid: !null}, nil
	case spannerpb.TypeCode_NUMERIC:
		if null {
			return spanner.NullNumeric{Numeric: big.Rat{}, Valid: false}, nil
		}
		x := v.GetStringValue()
		y, ok := (&big.Rat{}).SetString(x)
		if !ok {
			return nil, spanner.ToSpannerError(status.Error(codes.FailedPrecondition, fmt.Sprintf("unexpected string value %q for numeric number", x)))
		}
		return spanner.NullNumeric{Numeric: *y, Valid: true}, nil
	case spannerpb.TypeCode_JSON:
		if null {
			return spanner.NullJSON{}, nil
		}
		x := v.GetStringValue()
		return spanner.NullJSON{Value: encodedJSON(x), Valid: true}, nil
	case spannerpb.TypeCode_STRUCT:
		return executorStructValueToSpannerValue(t, v.GetStructValue(), null)
	case spannerpb.TypeCode_ARRAY:
		return executorArrayValueToSpannerValue(t, v, null)
	default:
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("executorValueToSpannerValue: type %s not supported", t.GetCode().String()))
	}
}

// techStructValueToInfraValue converts a tech.spanner.proto.Struct with given type t to a dynamically
// created pointer to a Go struct value with a type derived from t. If null is set, returns a nil pointer
// of the Go struct's type for NULL struct values.
func executorStructValueToSpannerValue(t *spannerpb.Type, v *executorpb.ValueList, null bool) (any, error) {
	var fieldValues []*executorpb.Value
	fieldTypes := t.GetStructType().GetFields()
	if !null {
		fieldValues = v.GetValue()
		if len(fieldValues) != len(fieldTypes) {
			return nil, spanner.ToSpannerError(status.Error(codes.InvalidArgument, "Mismatch between number of expected fields and specified values for struct type"))
		}
	}

	cloudFields := make([]reflect.StructField, 0, len(fieldTypes))
	cloudFieldVals := make([]any, 0, len(fieldTypes))

	// Convert the fields to Go types and build the struct's dynamic type.
	for i := 0; i < len(fieldTypes); i++ {
		var techValue *executorpb.Value
		var isnull bool

		if null {
			isnull = true
			techValue = nil
		} else {
			isnull = isNullTechValue(fieldValues[i])
			techValue = fieldValues[i]
		}

		// Go structs do not allow empty and duplicate field names and lowercase field names
		// make the field unexported. We use struct tags for specifying field names.
		cloudFieldVal, err := executorValueToSpannerValue(fieldTypes[i].Type, techValue, isnull)
		if err != nil {
			return nil, err
		}
		if cloudFieldVal == nil {
			return nil, status.Error(codes.Internal,
				fmt.Sprintf("Was not able to calculate the type for %s", fieldTypes[i].Type))
		}

		cloudFields = append(cloudFields,
			reflect.StructField{
				Name: fmt.Sprintf("Field_%d", i),
				Type: reflect.TypeOf(cloudFieldVal),
				Tag:  reflect.StructTag(fmt.Sprintf(`spanner:"%s"`, fieldTypes[i].Name)),
			})
		cloudFieldVals = append(cloudFieldVals, cloudFieldVal)
	}

	cloudStructType := reflect.StructOf(cloudFields)
	if null {
		// Return a nil pointer to Go struct with the built struct type.
		return reflect.Zero(reflect.PtrTo(cloudStructType)).Interface(), nil
	}
	// For a non-null struct, set the field values.
	cloudStruct := reflect.New(cloudStructType)
	for i, fieldVal := range cloudFieldVals {
		cloudStruct.Elem().Field(i).Set(reflect.ValueOf(fieldVal))
	}
	// Returns a pointer to the Go struct.
	return cloudStruct.Interface(), nil
}

// executorArrayValueToSpannerValue converts a tech spanner array Value with given type t into an cloud Spanner's Value.
func executorArrayValueToSpannerValue(t *spannerpb.Type, v *executorpb.Value, null bool) (any, error) {
	if null {
		// For null array type, simply return untyped nil
		// Returning untyped nil fails for some testcases
		// return nil, nil
	}
	switch t.GetArrayElementType().GetCode() {
	case spannerpb.TypeCode_INT64:
		if null {
			return ([]spanner.NullInt64)(nil), nil
		}
		out := make([]spanner.NullInt64, 0)
		for _, value := range v.GetArrayValue().GetValue() {
			out = append(out, spanner.NullInt64{value.GetIntValue(), !value.GetIsNull()})
		}
		return out, nil
	case spannerpb.TypeCode_STRING:
		if null {
			return ([]spanner.NullString)(nil), nil
		}
		out := make([]spanner.NullString, 0)
		for _, value := range v.GetArrayValue().GetValue() {
			out = append(out, spanner.NullString{value.GetStringValue(), !value.GetIsNull()})
		}
		return out, nil
	case spannerpb.TypeCode_BOOL:
		if null {
			return ([]spanner.NullBool)(nil), nil
		}
		out := make([]spanner.NullBool, 0)
		for _, value := range v.GetArrayValue().GetValue() {
			out = append(out, spanner.NullBool{Bool: value.GetBoolValue(), Valid: !value.GetIsNull()})
		}
		return out, nil
	case spannerpb.TypeCode_BYTES:
		if null {
			return ([][]byte)(nil), nil
		}
		out := make([][]byte, 0)
		for _, value := range v.GetArrayValue().GetValue() {
			if !value.GetIsNull() {
				out = append(out, value.GetBytesValue())
			}
		}
		return out, nil
	case spannerpb.TypeCode_FLOAT64:
		if null {
			return ([]spanner.NullFloat64)(nil), nil
		}
		out := make([]spanner.NullFloat64, 0)
		for _, value := range v.GetArrayValue().GetValue() {
			out = append(out, spanner.NullFloat64{value.GetDoubleValue(), !value.GetIsNull()})
		}
		return out, nil
	case spannerpb.TypeCode_NUMERIC:
		if null {
			return ([]spanner.NullNumeric)(nil), nil
		}
		out := make([]spanner.NullNumeric, 0)
		for _, value := range v.GetArrayValue().GetValue() {
			if value.GetIsNull() {
				out = append(out, spanner.NullNumeric{Numeric: big.Rat{}, Valid: false})
			} else {
				y, ok := (&big.Rat{}).SetString(value.GetStringValue())
				if !ok {
					return nil, spanner.ToSpannerError(status.Error(codes.InvalidArgument, fmt.Sprintf("unexpected string value %q for numeric number", value.GetStringValue())))
				}
				out = append(out, spanner.NullNumeric{*y, true})
			}
		}
		return out, nil
	case spannerpb.TypeCode_TIMESTAMP:
		if null {
			return ([]spanner.NullTime)(nil), nil
		}
		out := make([]spanner.NullTime, 0)
		for _, value := range v.GetArrayValue().GetValue() {
			spannerValue, err := executorValueToSpannerValue(t.GetArrayElementType(), value, value.GetIsNull())
			if err != nil {
				return nil, err
			}
			if v, ok := spannerValue.(spanner.NullTime); ok {
				out = append(out, v)
			}
		}
		return out, nil
	case spannerpb.TypeCode_DATE:
		if null {
			return ([]spanner.NullDate)(nil), nil
		}
		out := make([]spanner.NullDate, 0)
		for _, value := range v.GetArrayValue().GetValue() {
			spannerValue, err := executorValueToSpannerValue(t.GetArrayElementType(), value, value.GetIsNull())
			if err != nil {
				return nil, err
			}
			if v, ok := spannerValue.(spanner.NullDate); ok {
				out = append(out, v)
			}
		}
		return out, nil
	case spannerpb.TypeCode_JSON:
		if null {
			return ([]spanner.NullJSON)(nil), nil
		}
		out := make([]spanner.NullJSON, 0)
		for _, value := range v.GetArrayValue().GetValue() {
			spannerValue, err := executorValueToSpannerValue(t.GetArrayElementType(), value, value.GetIsNull())
			if err != nil {
				return nil, err
			}
			if v, ok := spannerValue.(spanner.NullJSON); ok {
				out = append(out, v)
			}
		}
		return out, nil
	case spannerpb.TypeCode_STRUCT:
		if null {
			log.Println("Failing again due to passing untyped nil value for array of structs. Might need to change to typed nil similar to other trpes")
			return nil, nil
		}
		// Non-NULL array of structs
		structElemType := t.GetArrayElementType()
		in := v.GetArrayValue()

		// Create a dummy struct value to get the element type.
		dummyStructPtr, err := executorStructValueToSpannerValue(structElemType, nil, true)
		if err != nil {
			return nil, err
		}
		goStructType := reflect.TypeOf(dummyStructPtr)

		out := reflect.MakeSlice(reflect.SliceOf(goStructType), 0, len(in.GetValue()))
		for _, value := range in.GetValue() {
			cv, err := executorStructValueToSpannerValue(structElemType, value.GetStructValue(), false)
			if err != nil {
				return nil, err
			}
			et := reflect.TypeOf(cv)
			if !reflect.DeepEqual(et, goStructType) {
				return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "Mismatch between computed struct array element type %v and received element type %v", goStructType, et))
			}
			out = reflect.Append(out, reflect.ValueOf(cv))
		}
		return out.Interface(), nil
	default:
		return nil, spanner.ToSpannerError(status.Error(codes.InvalidArgument, fmt.Sprintf("executorArrayValueToSpannerValue: unsupported array element type while converting from executor proto of type: %s", t.GetArrayElementType().GetCode().String())))
	}
}

// isNullTechValue returns whether a tech value is Value_Null or not.
func isNullTechValue(tv *executorpb.Value) bool {
	switch tv.GetValueType().(type) {
	case *executorpb.Value_IsNull:
		return true
	default:
		return false
	}
}

// isSingleUseReadOnlyTransactionNeeded decides type of read-only transaction based on concurrency.
func isSingleUseReadOnlyTransactionNeeded(c *executorpb.Concurrency) bool {
	switch c.GetConcurrencyMode().(type) {
	case *executorpb.Concurrency_MinReadTimestampMicros:
		return true
	case *executorpb.Concurrency_MaxStalenessSeconds:
		return true
	default:
		return false
	}
}

func errToStatus(e error) *spb.Status {
	log.Print(e.Error())
	if strings.Contains(e.Error(), "Transaction outcome unknown") {
		return &spb.Status{Code: int32(codes.DeadlineExceeded), Message: e.Error()}
	}
	if status.Code(e) == codes.InvalidArgument {
		return &spb.Status{Code: int32(codes.InvalidArgument), Message: e.Error()}
	}
	if status.Code(e) == codes.PermissionDenied {
		return &spb.Status{Code: int32(codes.PermissionDenied), Message: e.Error()}
	}
	if status.Code(e) == codes.Aborted {
		return &spb.Status{Code: int32(codes.Aborted), Message: e.Error()}
	}
	if status.Code(e) == codes.AlreadyExists {
		return &spb.Status{Code: int32(codes.AlreadyExists), Message: e.Error()}
	}
	if status.Code(e) == codes.Canceled {
		return &spb.Status{Code: int32(codes.Canceled), Message: e.Error()}
	}
	if status.Code(e) == codes.Internal {
		return &spb.Status{Code: int32(codes.Internal), Message: e.Error()}
	}
	if status.Code(e) == codes.FailedPrecondition {
		return &spb.Status{Code: int32(codes.FailedPrecondition), Message: e.Error()}
	}
	if status.Code(e) == codes.NotFound {
		return &spb.Status{Code: int32(codes.NotFound), Message: e.Error()}
	}
	if status.Code(e) == codes.DeadlineExceeded {
		return &spb.Status{Code: int32(codes.DeadlineExceeded), Message: e.Error()}
	}
	if status.Code(e) == codes.ResourceExhausted {
		return &spb.Status{Code: int32(codes.ResourceExhausted), Message: e.Error()}
	}
	if status.Code(e) == codes.OutOfRange {
		return &spb.Status{Code: int32(codes.OutOfRange), Message: e.Error()}
	}
	if status.Code(e) == codes.Unauthenticated {
		return &spb.Status{Code: int32(codes.Unauthenticated), Message: e.Error()}
	}
	if status.Code(e) == codes.Unimplemented {
		return &spb.Status{Code: int32(codes.Unimplemented), Message: e.Error()}
	}
	if status.Code(e) == codes.Unavailable {
		return &spb.Status{Code: int32(codes.Unavailable), Message: e.Error()}
	}
	if status.Code(e) == codes.Unknown {
		return &spb.Status{Code: int32(codes.Unknown), Message: e.Error()}
	}
	return &spb.Status{Code: int32(codes.Unknown), Message: fmt.Sprintf("Error: %v, Unsupported Spanner error code: %v", e.Error(), status.Code(e))}
}
