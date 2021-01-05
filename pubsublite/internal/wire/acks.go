// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and

package wire

import (
	"container/list"
	"fmt"
	"sync"
)

// AckConsumer is the interface exported from this package for acking messages.
type AckConsumer interface {
	Ack()
}

// ackedFunc is invoked when a message has been acked by the user. Note: if the
// ackedFunc implementation calls any ackConsumer methods, it needs to run in a
// goroutine to avoid a deadlock.
type ackedFunc func(*ackConsumer)

// ackConsumer is used for handling message acks. It is attached to a Message
// and also stored within the ackTracker until the message has been acked by the
// user.
type ackConsumer struct {
	// The message offset.
	Offset int64
	// Bytes released to the flow controller once the message has been acked.
	MsgBytes int64

	// Guards access to fields below.
	mu    sync.Mutex
	acked bool
	onAck ackedFunc
}

func newAckConsumer(offset, msgBytes int64, onAck ackedFunc) *ackConsumer {
	return &ackConsumer{Offset: offset, MsgBytes: msgBytes, onAck: onAck}
}

func (ac *ackConsumer) Ack() {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.acked {
		return
	}
	ac.acked = true
	if ac.onAck != nil {
		// Not invoked in a goroutine here for ease of testing.
		ac.onAck(ac)
	}
}

func (ac *ackConsumer) IsAcked() bool {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return ac.acked
}

// Clear onAck when the ack can no longer be processed. The user's ack would be
// ignored.
func (ac *ackConsumer) Clear() {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.onAck = nil
}

// Represents an uninitialized cursor offset. A sentinel value is used instead
// of an optional to simplify cursor comparisons (i.e. -1 works without the need
// to check for nil and then convert to int64).
const nilCursorOffset int64 = -1

// ackTracker manages outstanding message acks, i.e. messages that have been
// delivered to the user, but not yet acked. It is used by the committer and
// subscribeStream, so requires its own mutex.
type ackTracker struct {
	// Guards access to fields below.
	mu sync.Mutex
	// All offsets before and including this prefix have been acked by the user.
	ackedPrefixOffset int64
	// Outstanding message acks, strictly ordered by increasing message offsets.
	outstandingAcks *list.List // Value = *ackConsumer
}

func newAckTracker() *ackTracker {
	return &ackTracker{
		ackedPrefixOffset: nilCursorOffset,
		outstandingAcks:   list.New(),
	}
}

// Push adds an outstanding ack to the tracker.
func (at *ackTracker) Push(ack *ackConsumer) error {
	at.mu.Lock()
	defer at.mu.Unlock()

	// These errors should not occur unless there is a bug in the client library
	// as message ordering should have been validated by subscriberOffsetTracker.
	if ack.Offset <= at.ackedPrefixOffset {
		return errOutOfOrderMessages
	}
	if elem := at.outstandingAcks.Back(); elem != nil {
		lastOutstandingAck, _ := elem.Value.(*ackConsumer)
		if ack.Offset <= lastOutstandingAck.Offset {
			return errOutOfOrderMessages
		}
	}

	at.outstandingAcks.PushBack(ack)
	return nil
}

// CommitOffset returns the cursor offset that should be committed. May return
// nilCursorOffset if no messages have been acked thus far.
func (at *ackTracker) CommitOffset() int64 {
	at.mu.Lock()
	defer at.mu.Unlock()

	at.unsafeProcessAcks()

	if at.ackedPrefixOffset == nilCursorOffset {
		return nilCursorOffset
	}
	// Convert from last acked to first unacked, which is the commit offset.
	return at.ackedPrefixOffset + 1
}

// Release clears and invalidates any outstanding acks. This should be called
// when the subscriber terminates.
func (at *ackTracker) Release() {
	at.mu.Lock()
	defer at.mu.Unlock()

	at.unsafeProcessAcks()

	for elem := at.outstandingAcks.Front(); elem != nil; elem = elem.Next() {
		ack, _ := elem.Value.(*ackConsumer)
		ack.Clear()
	}
	at.outstandingAcks.Init()
}

// Process outstanding acks and update `ackedPrefixOffset` until an unacked
// message is found.
func (at *ackTracker) unsafeProcessAcks() {
	for {
		elem := at.outstandingAcks.Front()
		if elem == nil {
			break
		}
		ack, _ := elem.Value.(*ackConsumer)
		if !ack.IsAcked() {
			break
		}
		at.ackedPrefixOffset = ack.Offset
		at.outstandingAcks.Remove(elem)
		ack.Clear()
	}
}

// Empty returns true if there are no outstanding acks.
func (at *ackTracker) Empty() bool {
	at.mu.Lock()
	defer at.mu.Unlock()
	return at.outstandingAcks.Front() == nil
}

// commitCursorTracker tracks pending and last successful committed offsets.
// It is only accessed by the committer.
type commitCursorTracker struct {
	// Used to obtain the desired commit offset based on messages acked by the
	// user.
	acks *ackTracker
	// Last offset for which the server confirmed (acknowledged) the commit.
	lastConfirmedOffset int64
	// Queue of committed offsets awaiting confirmation from the server.
	pendingOffsets *list.List // Value = int64
}

func newCommitCursorTracker(acks *ackTracker) *commitCursorTracker {
	return &commitCursorTracker{
		acks:                acks,
		lastConfirmedOffset: nilCursorOffset,
		pendingOffsets:      list.New(),
	}
}

func extractOffsetFromElem(elem *list.Element) int64 {
	if elem == nil {
		return nilCursorOffset
	}
	offset, _ := elem.Value.(int64)
	return offset
}

// NextOffset is the commit offset to be sent to the stream. Returns
// nilCursorOffset if the commit offset does not need to be updated.
func (ct *commitCursorTracker) NextOffset() int64 {
	desiredCommitOffset := ct.acks.CommitOffset()
	if desiredCommitOffset <= ct.lastConfirmedOffset {
		// The server has already confirmed the commit offset.
		return nilCursorOffset
	}
	if desiredCommitOffset <= extractOffsetFromElem(ct.pendingOffsets.Back()) {
		// The commit offset has already been sent to the commit stream and is
		// awaiting confirmation.
		return nilCursorOffset
	}
	return desiredCommitOffset
}

// AddPending adds a sent, but not yet confirmed, committed offset.
func (ct *commitCursorTracker) AddPending(offset int64) {
	ct.pendingOffsets.PushBack(offset)
}

// ClearPending discards old pending offsets. Should be called when the commit
// stream reconnects, as the server acknowledgments for these would not be
// received.
func (ct *commitCursorTracker) ClearPending() {
	ct.pendingOffsets.Init()
}

// ConfirmOffsets processes the server's acknowledgment of the first
// `numConfirmed` pending offsets.
func (ct *commitCursorTracker) ConfirmOffsets(numConfirmed int64) error {
	if numPending := int64(ct.pendingOffsets.Len()); numPending < numConfirmed {
		return fmt.Errorf("pubsublite: server acknowledged %d cursor commits, but only %d were sent", numConfirmed, numPending)
	}

	for i := int64(0); i < numConfirmed; i++ {
		front := ct.pendingOffsets.Front()
		ct.lastConfirmedOffset = extractOffsetFromElem(front)
		ct.pendingOffsets.Remove(front)
	}
	return nil
}

// UpToDate when the server has confirmed the desired commit offset.
func (ct *commitCursorTracker) UpToDate() bool {
	return ct.acks.CommitOffset() <= ct.lastConfirmedOffset
}
