// Copyright 2014 Google LLC
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

package datastore

import (
	"context"
	"errors"
	"sync"
	"time"

	pb "cloud.google.com/go/datastore/apiv1/datastorepb"
	"cloud.google.com/go/internal/trace"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const maxIndividualReqTxnRetry = 5

// ErrConcurrentTransaction is returned when a transaction is rolled back due
// to a conflict with a concurrent transaction.
var ErrConcurrentTransaction = errors.New("datastore: concurrent transaction")

var (
	errExpiredTransaction             = errors.New("datastore: transaction expired")
	errEventualConsistencyTransaction = errors.New("datastore: cannot use EventualConsistency query in a transaction")

	txnBackoff = gax.Backoff{
		Initial:    20 * time.Millisecond,
		Max:        32 * time.Second,
		Multiplier: 1.3,
	}

	// Do not include codes.Unavailable here since client already retries for Unavailable error
	beginTxnRetryCodes = []codes.Code{codes.DeadlineExceeded, codes.Internal}
	rollbackRetryCodes = []codes.Code{codes.DeadlineExceeded, codes.Internal}
	txnRetryCodes      = []codes.Code{codes.Aborted, codes.Canceled, codes.Unknown, codes.DeadlineExceeded,
		codes.Internal, codes.Unauthenticated}

	gaxSleep = gax.Sleep
)

type transactionSettings struct {
	attempts int
	readOnly bool
	prevID   []byte // ID of the transaction to retry
	readTime *timestamppb.Timestamp

	// When set, skips the initial BeginTransaction RPC call to obtain txn id and
	// uses the piggybacked txn id from first read rpc call.
	// If there are no read operations on transaction, BeginTransaction RPC call is made
	// before rollback or commit
	beginLater bool
}

// newTransactionSettings creates a transactionSettings with a given TransactionOption slice.
// Unconfigured options will be set to default values.
func newTransactionSettings(opts []TransactionOption) *transactionSettings {
	s := &transactionSettings{attempts: 3}
	for _, o := range opts {
		if o == nil {
			panic("nil TransactionOption")
		}
		o.apply(s)
	}
	return s
}

// TransactionOption configures the way a transaction is executed.
type TransactionOption interface {
	apply(*transactionSettings)
}

// MaxAttempts returns a TransactionOption that overrides the default 3 attempt times.
func MaxAttempts(attempts int) TransactionOption {
	return maxAttempts(attempts)
}

type maxAttempts int

func (w maxAttempts) apply(s *transactionSettings) {
	if w > 0 {
		s.attempts = int(w)
	}
}

// WithReadTime returns a TransactionOption that specifies a snapshot of the
// database to view.
func WithReadTime(t time.Time) TransactionOption {
	return readTime{t}
}

type readTime struct {
	time.Time
}

func (rt readTime) apply(s *transactionSettings) {
	if !rt.Time.IsZero() {
		s.readTime = timestamppb.New(rt.Time)
	}
}

// ReadOnly is a TransactionOption that marks the transaction as read-only.
var ReadOnly TransactionOption

func init() {
	ReadOnly = readOnly{}
	BeginLater = beginLater{}
}

type readOnly struct{}

func (readOnly) apply(s *transactionSettings) {
	s.readOnly = true
}

// BeginLater is a TransactionOption that can be used to improve transaction performance
var BeginLater TransactionOption

type beginLater struct{}

func (beginLater) apply(s *transactionSettings) {
	s.beginLater = true
}

type transactionState int

const (
	transactionStateNotStarted transactionState = iota
	transactionStateInProgress
	transactionStateExpired
)

// Transaction represents a set of datastore operations to be committed atomically.
//
// Operations are enqueued by calling the Put and Delete methods on Transaction
// (or their Multi-equivalents).  These operations are only committed when the
// Commit method is invoked. To ensure consistency, reads must be performed by
// using Transaction's Get method or by using the Transaction method when
// building a query.
//
// A Transaction must be committed or rolled back exactly once.
type Transaction struct {
	id        []byte
	client    *Client
	ctx       context.Context
	mutations []*pb.Mutation      // The mutations to apply.
	pending   map[int]*PendingKey // Map from mutation index to incomplete keys pending transaction completion.
	settings  *transactionSettings
	state     transactionState
	stateLock sync.Mutex
}

// NewTransaction starts a new transaction.
func (c *Client) NewTransaction(ctx context.Context, opts ...TransactionOption) (t *Transaction, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/datastore.NewTransaction")
	defer func() { trace.EndSpan(ctx, err) }()

	for _, o := range opts {
		if _, ok := o.(maxAttempts); ok {
			return nil, errors.New("datastore: NewTransaction does not accept MaxAttempts option")
		}
	}
	return c.newTransaction(ctx, newTransactionSettings(opts))
}

func (t *Transaction) parseTransactionOptions() (*pb.TransactionOptions, string) {
	if t.settings == nil {
		return nil, ""
	}

	if t.settings.readOnly {
		ro := &pb.TransactionOptions_ReadOnly{}
		if !t.settings.readTime.AsTime().IsZero() {
			ro.ReadTime = t.settings.readTime
		}

		return &pb.TransactionOptions{
			Mode: &pb.TransactionOptions_ReadOnly_{ReadOnly: ro},
		}, "cloud.google.com/go/datastore.Transaction.ReadOnlyTransaction"
	}

	if t.settings.prevID != nil {
		return &pb.TransactionOptions{
			Mode: &pb.TransactionOptions_ReadWrite_{ReadWrite: &pb.TransactionOptions_ReadWrite{
				PreviousTransaction: t.settings.prevID,
			}},
		}, "cloud.google.com/go/datastore.Transaction.ReadWriteTransaction"
	}
	return nil, ""
}

// beginTransaction makes BeginTransaction rpc
func (t *Transaction) beginTransaction() (txnID []byte, err error) {

	req := &pb.BeginTransactionRequest{
		ProjectId:  t.client.dataset,
		DatabaseId: t.client.databaseID,
	}

	txOptionsPb, spanName := t.parseTransactionOptions()
	if txOptionsPb != nil {
		t.ctx = trace.StartSpan(t.ctx, spanName)
		defer func() { trace.EndSpan(t.ctx, err) }()
		req.TransactionOptions = txOptionsPb
	}

	resp, err := t.client.client.BeginTransaction(t.ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Transaction, nil
}

// beginLaterTransaction makes BeginTransaction rpc if transaction has not yet started
func (t *Transaction) beginLaterTransaction() (err error) {
	if t.state != transactionStateNotStarted {
		return nil
	}

	// Obtain state lock since the state needs to be updated
	// after transaction has started
	t.stateLock.Lock()
	defer t.stateLock.Unlock()
	if t.state != transactionStateNotStarted {
		return nil
	}

	txnID, err := t.beginTransaction()
	if err != nil {
		return err
	}

	t.setToInProgress(txnID)
	return nil
}

// Acquires state lock if transaction has not started. No-op otherwise
// The returned function unlocks the state if it was locked.
//
// Usage:
//
//	func (t *Transaction) someFunction() {
//		...
//		if t != nil {
//			defer t.stateLockDeferUnlock()()
//		}
//		....
//	}
//
// This ensures that state is locked before any of the following lines are exexcuted
// The lock will be released after 'someFunction' ends
func (t *Transaction) stateLockDeferUnlock() func() {
	if t.state == transactionStateNotStarted {
		t.stateLock.Lock()
		// Check whether state changed while waiting to acquire lock
		if t.state == transactionStateNotStarted {
			return func() { t.stateLock.Unlock() }
		}
		t.stateLock.Unlock()
	}
	return func() {}
}

func (t *Transaction) setToInProgress(id []byte) {
	t.id = id
	t.state = transactionStateInProgress
}

// backoffBeforeRetry returns:
// - original error if error shouldn't be retried
// - sleep error if sleep fails
// - nil if successfully backed off
func backoffBeforeRetry(ctx context.Context, retryer gax.Retryer, err error) error {
	delay, shouldRetry := retryer.Retry(err)
	if !shouldRetry {
		return err
	}
	if sleepErr := gaxSleep(ctx, delay); sleepErr != nil {
		return sleepErr
	}
	return nil
}

// Retries BeginTransaction 5 times. returns last seen error if not successful
func (c *Client) newTransactionWithRetry(ctx context.Context, s *transactionSettings) (*Transaction, error) {
	var t *Transaction
	var newTxnErr error
	retryer := gax.OnCodes(beginTxnRetryCodes, txnBackoff)
	for attempt := 0; attempt < maxIndividualReqTxnRetry; attempt++ {
		t, newTxnErr = c.newTransaction(ctx, s)
		if newTxnErr == nil {
			return t, newTxnErr
		}
		// Check if BeginTransaction should be retried
		if backoffErr := backoffBeforeRetry(ctx, retryer, newTxnErr); backoffErr != nil {
			return nil, backoffErr
		}
	}
	return t, newTxnErr
}

// returns any error seen on BeginTransaction RPC call
func (c *Client) newTransaction(ctx context.Context, s *transactionSettings) (_ *Transaction, err error) {
	t := &Transaction{
		id:        nil,
		ctx:       ctx,
		client:    c,
		mutations: nil,
		pending:   make(map[int]*PendingKey),
		settings:  s,
	}

	t.state = transactionStateNotStarted
	if !s.beginLater {
		txnID, err := t.beginTransaction()
		if err != nil {
			return nil, err
		}
		t.setToInProgress(txnID)
	}

	return t, nil
}

// RunInTransaction runs f in a transaction. f is invoked with a Transaction
// that f should use for all the transaction's datastore operations.
//
// f must not call Commit or Rollback on the provided Transaction.
//
// If f returns nil, RunInTransaction commits the transaction,
// returning the Commit and a nil error if it succeeds. If the commit fails,
// transaction is rolled back. If commit error is retryable, RunInTransaction
// retries f with a new Transaction. It gives up and returns last seen commit
// error or f error after three failed attempts (or as configured with MaxAttempts).
//
// If f returns non-nil, then the transaction will be rolled back. If f error
// is retryable, RunInTransaction retries f with a new Transaction. It gives up
// and returns last seen commit or f error after three failed attempts (or as
// configured with MaxAttempts).
//
// Note that when f returns, the transaction is not committed. Calling code
// must not assume that any of f's changes have been committed until
// RunInTransaction returns nil.
//
// Since f may be called multiple times, f should usually be idempotent – that
// is, it should have the same result when called multiple times. Note that
// Transaction.Get will append when unmarshalling slice fields, so it is not
// necessarily idempotent.
func (c *Client) RunInTransaction(ctx context.Context, f func(tx *Transaction) error, opts ...TransactionOption) (cmt *Commit, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/datastore.RunInTransaction")
	defer func() { trace.EndSpan(ctx, err) }()

	var tx *Transaction
	settings := newTransactionSettings(opts)
	retryer := gax.OnCodes(txnRetryCodes, txnBackoff)
	for n := 0; n < settings.attempts; n++ {
		tx, err = c.newTransactionWithRetry(ctx, settings)
		if err != nil {
			return nil, err
		}

		var retryErr error
		fRunErr := f(tx)
		if fRunErr == nil {
			// Commit transaction
			cmt, commitErr := tx.Commit()
			if commitErr == nil {
				// Commit successful
				return cmt, nil
			}

			// Commit failed
			retryErr = commitErr
			err = commitErr

			// Commit() returns 'ErrConcurrentTransaction' when Aborted.
			// Convert 'ErrConcurrentTransaction' back to Aborted to check retryable error code
			if retryErr == ErrConcurrentTransaction {
				retryErr = status.Error(codes.Aborted, commitErr.Error())
			}
		} else {
			// Failure while running user provided function f
			retryErr = fRunErr
			err = fRunErr
		}

		// Rollback the transaction on commit failure or f failure
		rollbackErr := tx.rollbackWithRetry()
		if rollbackErr != nil {
			// Do not restart transaction if rollback failed
			return nil, err
		}

		// If this is the last attempt, exit without delaying.
		if n+1 == settings.attempts {
			return nil, err
		}

		// Check if error should be retried
		code, errConvert := grpcStatusCode(retryErr)
		if errConvert != nil && code == codes.ResourceExhausted {
			// ResourceExhausted error should be retried with max backoff
			if sleepErr := gaxSleep(ctx, txnBackoff.Max); sleepErr != nil {
				return nil, err
			}
		} else {
			// Check whether error other than ResourceExhausted should be retried
			backoffErr := backoffBeforeRetry(ctx, retryer, retryErr)
			if backoffErr != nil {
				return nil, err
			}
		}
		// Pass this transaction's ID to the retry transaction to preserve
		// transaction priority.
		if !settings.readOnly {
			settings.prevID = tx.id
		}
	}
	return nil, err
}

// grpcStatusCodeCode extracts the canonical error code from a GRPC status error.
func grpcStatusCode(err error) (codes.Code, error) {
	s, ok := status.FromError(err)
	if !ok {
		return codes.Unknown, errors.New("Not a status error")
	}
	return s.Code(), nil
}

// Commit applies the enqueued operations atomically.
func (t *Transaction) Commit() (c *Commit, err error) {
	t.ctx = trace.StartSpan(t.ctx, "cloud.google.com/go/datastore.Transaction.Commit")
	defer func() { trace.EndSpan(t.ctx, err) }()

	if t.state == transactionStateExpired {
		return nil, errExpiredTransaction
	}

	err = t.beginLaterTransaction()
	if err != nil {
		return nil, err
	}

	req := &pb.CommitRequest{
		ProjectId:           t.client.dataset,
		DatabaseId:          t.client.databaseID,
		TransactionSelector: &pb.CommitRequest_Transaction{Transaction: t.id},
		Mutations:           t.mutations,
		Mode:                pb.CommitRequest_TRANSACTIONAL,
	}
	resp, err := t.client.client.Commit(t.ctx, req)
	if status.Code(err) == codes.Aborted {
		return nil, ErrConcurrentTransaction
	}
	if err != nil {
		return nil, err
	}

	t.state = transactionStateExpired

	c = &Commit{}
	// Copy any newly minted keys into the returned keys.
	for i, p := range t.pending {
		if i >= len(resp.MutationResults) || resp.MutationResults[i].Key == nil {
			return nil, errors.New("datastore: internal error: server returned the wrong mutation results")
		}
		key, err := protoToKey(resp.MutationResults[i].Key)
		if err != nil {
			return nil, errors.New("datastore: internal error: server returned an invalid key")
		}
		p.key = key
		p.commit = c
	}

	return c, nil
}

// rollbackWithRetry runs rollback with retries
// Returns last attempt rollback error if rollback fails even after retries
func (t *Transaction) rollbackWithRetry() error {
	var rollbackErr error
	retryer := gax.OnCodes(rollbackRetryCodes, txnBackoff)
	for rollbackAttempt := 0; rollbackAttempt < maxIndividualReqTxnRetry; rollbackAttempt++ {
		rollbackErr = t.Rollback()
		if rollbackErr == nil {
			return nil
		}

		// Check if Rollback should be retried
		if backoffErr := backoffBeforeRetry(t.ctx, retryer, rollbackErr); backoffErr != nil {
			return backoffErr
		}
	}
	return rollbackErr
}

// Rollback abandons a pending transaction.
func (t *Transaction) Rollback() (err error) {
	t.ctx = trace.StartSpan(t.ctx, "cloud.google.com/go/datastore.Transaction.Rollback")
	defer func() { trace.EndSpan(t.ctx, err) }()

	if t.state == transactionStateExpired {
		return errExpiredTransaction
	}

	if t.id == nil && t.state == transactionStateNotStarted {
		// no transaction to rollback
		return nil
	}

	err = t.beginLaterTransaction()
	if err != nil {
		return err
	}

	_, err = t.client.client.Rollback(t.ctx, &pb.RollbackRequest{
		ProjectId:   t.client.dataset,
		DatabaseId:  t.client.databaseID,
		Transaction: t.id,
	})
	if err != nil {
		return err
	}

	t.state = transactionStateExpired
	return nil
}

func (t *Transaction) parseReadOptions() (*pb.ReadOptions, error) {
	var opts *pb.ReadOptions
	switch t.state {
	case transactionStateExpired:
		return nil, errExpiredTransaction
	case transactionStateInProgress:
		opts = &pb.ReadOptions{
			// Use existing transaction id for this request
			ConsistencyType: &pb.ReadOptions_Transaction{Transaction: t.id},
		}
	case transactionStateNotStarted:
		tOptionsPb, _ := t.parseTransactionOptions()
		opts = &pb.ReadOptions{
			// Begin a new transaction for this request
			ConsistencyType: &pb.ReadOptions_NewTransaction{NewTransaction: tOptionsPb},
		}
	}
	return opts, nil
}

func (t *Transaction) get(spanName string, keys []*Key, dst interface{}) (err error) {
	t.ctx = trace.StartSpan(t.ctx, spanName)
	defer func() { trace.EndSpan(t.ctx, err) }()

	if t != nil {
		defer t.stateLockDeferUnlock()()
	}

	opts, err := t.parseReadOptions()
	if err != nil {
		return err
	}

	txnID, err := t.client.get(t.ctx, keys, dst, opts)

	if txnID != nil && err == nil {
		t.setToInProgress(txnID)
	}
	return t.client.processFieldMismatchError(err)
}

// Get is the transaction-specific version of the package function Get.
// All reads performed during the transaction will come from a single consistent
// snapshot. Furthermore, if the transaction is set to a serializable isolation
// level, another transaction cannot concurrently modify the data that is read
// or modified by this transaction.
func (t *Transaction) Get(key *Key, dst interface{}) (err error) {
	err = t.get("cloud.google.com/go/datastore.Transaction.Get", []*Key{key}, []interface{}{dst})
	if me, ok := err.(MultiError); ok {
		return t.client.processFieldMismatchError(me[0])
	}
	return t.client.processFieldMismatchError(err)
}

// GetMulti is a batch version of Get.
func (t *Transaction) GetMulti(keys []*Key, dst interface{}) (err error) {
	return t.get("cloud.google.com/go/datastore.Transaction.GetMulti", keys, dst)
}

// Put is the transaction-specific version of the package function Put.
//
// Put returns a PendingKey which can be resolved into a Key using the
// return value from a successful Commit. If key is an incomplete key, the
// returned pending key will resolve to a unique key generated by the
// datastore.
func (t *Transaction) Put(key *Key, src interface{}) (*PendingKey, error) {
	h, err := t.PutMulti([]*Key{key}, []interface{}{src})
	if err != nil {
		if me, ok := err.(MultiError); ok {
			return nil, me[0]
		}
		return nil, err
	}
	return h[0], nil
}

// PutMulti is a batch version of Put. One PendingKey is returned for each
// element of src in the same order.
// TODO(jba): rewrite in terms of Mutate.
func (t *Transaction) PutMulti(keys []*Key, src interface{}) (ret []*PendingKey, err error) {
	if t.state == transactionStateExpired {
		return nil, errExpiredTransaction
	}
	mutations, err := putMutations(keys, src)
	if err != nil {
		return nil, err
	}
	origin := len(t.mutations)
	t.mutations = append(t.mutations, mutations...)

	// Prepare the returned handles, pre-populating where possible.
	ret = make([]*PendingKey, len(keys))
	for i, key := range keys {
		p := &PendingKey{}
		if key.Incomplete() {
			// This key will be in the final commit result.
			t.pending[origin+i] = p
		} else {
			p.key = key
		}
		ret[i] = p
	}

	return ret, nil
}

// Delete is the transaction-specific version of the package function Delete.
// Delete enqueues the deletion of the entity for the given key, to be
// committed atomically upon calling Commit.
func (t *Transaction) Delete(key *Key) error {
	err := t.DeleteMulti([]*Key{key})
	if me, ok := err.(MultiError); ok {
		return me[0]
	}
	return err
}

// DeleteMulti is a batch version of Delete.
// TODO(jba): rewrite in terms of Mutate.
func (t *Transaction) DeleteMulti(keys []*Key) (err error) {
	if t.state == transactionStateExpired {
		return errExpiredTransaction
	}
	mutations, err := deleteMutations(keys)
	if err != nil {
		return err
	}
	t.mutations = append(t.mutations, mutations...)
	return nil
}

// Mutate adds the mutations to the transaction. They will all be applied atomically
// upon calling Commit. Mutate returns a PendingKey for each Mutation in the argument
// list, in the same order. PendingKeys for Delete mutations are always nil.
//
// If any of the mutations are invalid, Mutate returns a MultiError with the errors.
// Mutate returns a MultiError in this case even if there is only one Mutation.
//
// For an example, see Client.Mutate.
func (t *Transaction) Mutate(muts ...*Mutation) ([]*PendingKey, error) {
	if t.state == transactionStateExpired {
		return nil, errExpiredTransaction
	}
	pmuts, err := mutationProtos(muts)
	if err != nil {
		return nil, err
	}
	origin := len(t.mutations)
	t.mutations = append(t.mutations, pmuts...)
	// Prepare the returned handles, pre-populating where possible.
	ret := make([]*PendingKey, len(muts))
	for i, mut := range muts {
		if mut.isDelete() {
			continue
		}
		p := &PendingKey{}
		if mut.key.Incomplete() {
			// This key will be in the final commit result.
			t.pending[origin+i] = p
		} else {
			p.key = mut.key
		}
		ret[i] = p
	}
	return ret, nil
}

// Commit represents the result of a committed transaction.
type Commit struct{}

// Key resolves a pending key handle into a final key.
func (c *Commit) Key(p *PendingKey) *Key {
	if p == nil { // if called on a *PendingKey from a Delete mutation
		return nil
	}
	// If p.commit is nil, the PendingKey did not come from an incomplete key,
	// so p.key is valid.
	if p.commit != nil && c != p.commit {
		panic("PendingKey was not created by corresponding transaction")
	}
	return p.key
}

// PendingKey represents the key for newly-inserted entity. It can be
// resolved into a Key by calling the Key method of Commit.
type PendingKey struct {
	key    *Key
	commit *Commit
}
