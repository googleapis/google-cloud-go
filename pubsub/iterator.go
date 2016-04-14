// Copyright 2016 Google Inc. All Rights Reserved.
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

package pubsub

import (
	"io"
	"sync"
	"time"

	"golang.org/x/net/context"
)

type Iterator struct {
	// The context to use for acking messages and extending message deadlines.
	ctx context.Context

	// kaTicker controls how often we send an ack deadline extension request.
	kaTicker *time.Ticker
	// ackTicker controls how often we acknowledge a batch of messages.
	ackTicker *time.Ticker

	ka     *keepAlive
	acker  *acker
	puller *puller

	mu     sync.Mutex
	closed bool
}

// newIterator starts a new Iterator.  Stop must be called on the Iterator
// when it is no longer needed.
// subName is the full name of the subscription to pull messages from.
// ctx is the context to use for acking messages and extending message deadlines.
func (c *Client) newIterator(ctx context.Context, subName string, po *pullOptions) *Iterator {
	// TODO: make kaTicker frequency more configurable.
	// (ackDeadline - 5s) is a reasonable default for now, because the minimum ack period is 10s.  This gives us 5s grace.
	keepAlivePeriod := po.ackDeadline - 5*time.Second
	kaTicker := time.NewTicker(keepAlivePeriod) // Stopped in it.Stop

	// TODO: make ackTicker more configurable.  Something less than
	// kaTicker is a reasonable default (there's no point extending
	// messages when they could be acked instead).
	ackTicker := time.NewTicker(keepAlivePeriod / 2) // Stopped in it.Stop

	ka := &keepAlive{
		Client:        c,
		Ctx:           ctx,
		Sub:           subName,
		ExtensionTick: kaTicker.C,
		Deadline:      po.ackDeadline,
		MaxExtension:  po.maxExtension,
	}

	ack := &acker{
		Client:  c,
		Ctx:     ctx,
		Sub:     subName,
		AckTick: ackTicker.C,
		Notify:  ka.Remove,
	}

	pull := &puller{
		Client:    c,
		Sub:       subName,
		BatchSize: int64(po.maxPrefetch),
		Notify:    ka.Add,
	}

	ka.Start()
	ack.Start()
	return &Iterator{
		ctx:       ctx,
		kaTicker:  kaTicker,
		ackTicker: ackTicker,
		ka:        ka,
		acker:     ack,
		puller:    pull,
	}
}

// Next returns the next Message to be processed.  The caller must call Done on
// the returned Message when finished with it.
// Once Stop has been called, subsequent calls to Next will return io.EOF.
func (it *Iterator) Next() (*Message, error) {
	it.mu.Lock()
	defer it.mu.Unlock()
	if it.closed {
		return nil, io.EOF
	}

	select {
	case <-it.ctx.Done():
		return nil, it.ctx.Err()
	default:
	}

	// Note: this is the only place where messages are added to keepAlive,
	// and this code is protected by mu. This means once an iterator starts
	// being closed down, no more messages will be added to keepalive.
	m, err := it.puller.Next(it.ctx)
	if err != nil {
		return nil, err
	}
	m.it = it
	return m, nil
}

// Client code must call Stop on an Iterator when finished with it.
// Stop will block until Done has been called on all Messages that have been
// returned by Next, or until the context with which the Iterator was created
// is cancelled or exceeds its deadline.
// Stop need only be called once, but may be called multiple times from
// multiple goroutines.
func (it *Iterator) Stop() {
	// TODO: test calling from multiple goroutines.
	it.mu.Lock()
	defer it.mu.Unlock()
	if it.closed {
		// early return ensures that it.ka.Stop is only called once.
		return
	}
	it.closed = true

	// Remove messages that are being kept alive, but have not been
	// supplied to the caller yet.  Then the only messages being kept alive
	// will be those that have been supplied to the caller but have not yet
	// had their Done method called.
	for _, m := range it.puller.Pending() {
		it.ka.Remove(m.AckID)
	}

	// Start acking messages as they arrive, ignoring ackTicker.  This will
	// result in it.ka.Stop, below, returning as soon as possible.
	it.acker.FastMode()

	// This will block until
	//   (a) it.Ctx is done, or
	//   (b) all messages have been removed from keepAlive.
	// (b) will happen once all outstanding messages have been either ACKed or NACKed.
	it.ka.Stop()

	// There are no more live messages that we care about, so kill off the acker.
	it.acker.Stop()

	it.kaTicker.Stop()
	it.ackTicker.Stop()
}

func (it *Iterator) done(ackID string, ack bool) {
	// NOTE: this method does not lock mu, because it's fine for done to be
	// called while the iterator is in the process of being closed.  In
	// fact, this is the only way to drain oustanding messages.
	if ack {
		it.acker.Ack(ackID)
		// There's no need to call it.ka.Remove here, as acker will
		// call it via its Notify function.
	} else {
		it.ka.Remove(ackID)
	}
}
