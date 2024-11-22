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
	"sync"

	"cloud.google.com/go/internal/trace"
	"google.golang.org/grpc/codes"
)

// TransactionPool is an experimental interface. This interface may be removed in a future
// version of this library.
//
// TransactionPool can be used to create a pool of prepared read/write transactions. These
// transactions can be used to execute transactions that cannot (efficiently) use the inline
// BeginTransaction option. This includes transactions that:
//  1. Only apply mutations. These transactions do not have any statements that can
//     include the BeginTransaction option, and require an explicit BeginTransaction RPC.
//  2. Execute multiple parallel read operations as the first statement. Only one of these
//     statements can include the BeginTransaction option, and all other statements must
//     wait for this statement to return the first result before they can proceed. This
//     reduces the degree of parallelism at the start of the transaction.
type TransactionPool interface {
	// RegisterPool registers the transaction pool with a client. The client will pick
	// transactions from this pool when ReadWriteTransaction is called.
	RegisterPool(client *Client) error

	// RunTransaction runs a transaction using a transaction from this pool.
	RunTransaction(ctx context.Context, f func(context.Context, *ReadWriteTransaction) error) (resp CommitResponse, err error)
}

type fixedSizePool struct {
	mu           sync.Mutex
	c            *Client
	size         int
	opts         TransactionOptions
	transactions []*preparedTransaction

	lastPrepareErr error
	numPrepareErrs int
	maxPrepareErrs int

	prepareFunc func()
}

// NewFixedSizeTransactionPool returns a transaction pool of fixed size. The pool will check out size sessions
// from the session pool and create a read/write transaction on these sessions. The pool is automatically
// back-filled when a transaction has been used.
func NewFixedSizeTransactionPool(client *Client, size int, opts TransactionOptions) (TransactionPool, error) {
	pool := &fixedSizePool{
		c:            client,
		size:         size,
		opts:         opts,
		transactions: make([]*preparedTransaction, 0, size),

		// TODO: Make configurable
		maxPrepareErrs: 1000,
	}
	pool.prepareFunc = pool.prepareTransaction
	pool.prepareTransactions(size)
	return pool, nil
}

func (p *fixedSizePool) RegisterPool(client *Client) error {
	return client.registerTransactionPool(p)
}

// RunTransaction runs a read/write transaction using a prepared transaction from the pool.
func (p *fixedSizePool) RunTransaction(ctx context.Context, f func(context.Context, *ReadWriteTransaction) error) (resp CommitResponse, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/spanner.fixedSizePool.RunTransaction")
	defer func() { trace.EndSpan(ctx, err) }()
	tx, err := p.get(ctx)
	if err != nil {
		return resp, err
	}
	return tx.run(ctx, f)
}

func (p *fixedSizePool) get(ctx context.Context) (*preparedTransaction, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	l := len(p.transactions)
	if l == 0 {
		if p.lastPrepareErr != nil && p.numPrepareErrs == p.maxPrepareErrs {
			return nil, p.lastPrepareErr
		}
		// TODO: Implement a waiting mechanism
		return nil, spannerErrorf(codes.ResourceExhausted, "transaction pool exhausted")
	}
	tx := p.transactions[l-1]
	p.transactions = p.transactions[:l-1]
	p.prepareTransactions(1)

	return tx, nil
}

func (p *fixedSizePool) prepareTransactions(numTransactions int) {
	for n := 0; n < numTransactions; n++ {
		go p.prepareFunc()
	}
}

func (p *fixedSizePool) prepareTransaction() {
	for {
		tx, err := p.c.prepareTransaction(context.Background(), p.opts)
		p.mu.Lock()
		if err != nil {
			p.lastPrepareErr = err
			p.numPrepareErrs++
			if p.numPrepareErrs > p.maxPrepareErrs {
				return
			}
		} else {
			p.transactions = append(p.transactions, tx)
		}
		p.mu.Unlock()
		break
	}
}
