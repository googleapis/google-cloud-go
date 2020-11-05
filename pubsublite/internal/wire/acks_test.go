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

import "testing"

func TestAckConsumerAck(t *testing.T) {
	numAcks := 0
	onAck := func(ac *ackConsumer) {
		numAcks++
	}
	ackConsumer := newAckConsumer(0, 0, onAck)

	// Test duplicate acks.
	for i := 0; i < 3; i++ {
		ackConsumer.Ack()

		if got, want := ackConsumer.IsAcked(), true; got != want {
			t.Errorf("ackConsumer.IsAcked() got %v, want %v", got, want)
		}
		if got, want := numAcks, 1; got != want {
			t.Errorf("onAck func called %v times, expected %v call", got, want)
		}
	}
}

func TestAckConsumerClear(t *testing.T) {
	onAck := func(ac *ackConsumer) {
		t.Error("onAck func should not have been called")
	}
	ackConsumer := newAckConsumer(0, 0, onAck)
	ackConsumer.Clear()
	ackConsumer.Ack()

	if got, want := ackConsumer.IsAcked(), true; got != want {
		t.Errorf("ackConsumer.IsAcked() got %v, want %v", got, want)
	}
}

func TestAckTrackerProcessing(t *testing.T) {
	ackTracker := newAckTracker()

	// No messages received yet.
	if got, want := ackTracker.CommitOffset(), nilCursorOffset; got != want {
		t.Errorf("ackTracker.CommitOffset() got %v, want %v", got, want)
	}

	onAck := func(ac *ackConsumer) {
		// Nothing to do.
	}
	ack1 := newAckConsumer(1, 0, onAck)
	ack2 := newAckConsumer(2, 0, onAck)
	ack3 := newAckConsumer(3, 0, onAck)
	if err := ackTracker.Push(ack1); err != nil {
		t.Errorf("ackTracker.Push() got err %v", err)
	}
	if err := ackTracker.Push(ack2); err != nil {
		t.Errorf("ackTracker.Push() got err %v", err)
	}
	if err := ackTracker.Push(ack3); err != nil {
		t.Errorf("ackTracker.Push() got err %v", err)
	}

	// All messages unacked.
	if got, want := ackTracker.CommitOffset(), nilCursorOffset; got != want {
		t.Errorf("ackTracker.CommitOffset() got %v, want %v", got, want)
	}

	ack1.Ack()
	if got, want := ackTracker.CommitOffset(), int64(2); got != want {
		t.Errorf("ackTracker.CommitOffset() got %v, want %v", got, want)
	}

	// Skipped ack2, so the commit offset should not have been updated.
	ack3.Ack()
	if got, want := ackTracker.CommitOffset(), int64(2); got != want {
		t.Errorf("ackTracker.CommitOffset() got %v, want %v", got, want)
	}

	// Both ack2 and ack3 should be removed from the outstanding acks queue.
	ack2.Ack()
	if got, want := ackTracker.CommitOffset(), int64(4); got != want {
		t.Errorf("ackTracker.CommitOffset() got %v, want %v", got, want)
	}

	// Newly received message.
	ack4 := newAckConsumer(4, 0, onAck)
	if err := ackTracker.Push(ack4); err != nil {
		t.Errorf("ackTracker.Push() got err %v", err)
	}
	ack4.Ack()
	if got, want := ackTracker.CommitOffset(), int64(5); got != want {
		t.Errorf("ackTracker.CommitOffset() got %v, want %v", got, want)
	}
}

func TestAckTrackerRelease(t *testing.T) {
	ackTracker := newAckTracker()
	onAck := func(ac *ackConsumer) {
		t.Error("onAck should not be called")
	}
	ack1 := newAckConsumer(1, 0, onAck)
	ack2 := newAckConsumer(2, 0, onAck)
	ack3 := newAckConsumer(3, 0, onAck)

	if err := ackTracker.Push(ack1); err != nil {
		t.Errorf("ackTracker.Push() got err %v", err)
	}
	if err := ackTracker.Push(ack2); err != nil {
		t.Errorf("ackTracker.Push() got err %v", err)
	}
	if err := ackTracker.Push(ack3); err != nil {
		t.Errorf("ackTracker.Push() got err %v", err)
	}

	// After clearing outstanding acks, onAck should not be called.
	ackTracker.Release()
	ack1.Ack()
	ack2.Ack()
	ack3.Ack()
}

func TestCommitCursorTrackerProcessing(t *testing.T) {
	ackTracker := newAckTracker()
	commitTracker := newCommitCursorTracker(ackTracker)

	// No messages received yet.
	if got, want := commitTracker.NextOffset(), nilCursorOffset; got != want {
		t.Errorf("commitCursorTracker.NextOffset() got %v, want %v", got, want)
	}

	onAck := func(ac *ackConsumer) {
		// Nothing to do.
	}
	ack1 := newAckConsumer(1, 0, onAck)
	ack2 := newAckConsumer(2, 0, onAck)
	ack3 := newAckConsumer(3, 0, onAck)
	if err := ackTracker.Push(ack1); err != nil {
		t.Errorf("ackTracker.Push() got err %v", err)
	}
	if err := ackTracker.Push(ack2); err != nil {
		t.Errorf("ackTracker.Push() got err %v", err)
	}
	if err := ackTracker.Push(ack3); err != nil {
		t.Errorf("ackTracker.Push() got err %v", err)
	}

	// All messages unacked.
	if got, want := commitTracker.NextOffset(), nilCursorOffset; got != want {
		t.Errorf("commitCursorTracker.NextOffset() got %v, want %v", got, want)
	}

	// Msg1 acked and commit sent to stream.
	ack1.Ack()
	if got, want := commitTracker.NextOffset(), int64(2); got != want {
		t.Errorf("commitCursorTracker.NextOffset() got %v, want %v", got, want)
	}
	commitTracker.AddPending(commitTracker.NextOffset())
	if got, want := commitTracker.NextOffset(), nilCursorOffset; got != want {
		t.Errorf("commitCursorTracker.NextOffset() got %v, want %v", got, want)
	}

	// Msg 2 & 3 acked commit and sent to stream.
	ack2.Ack()
	if got, want := commitTracker.NextOffset(), int64(3); got != want {
		t.Errorf("commitCursorTracker.NextOffset() got %v, want %v", got, want)
	}
	ack3.Ack()
	if got, want := commitTracker.NextOffset(), int64(4); got != want {
		t.Errorf("commitCursorTracker.NextOffset() got %v, want %v", got, want)
	}
	commitTracker.AddPending(commitTracker.NextOffset())
	if got, want := commitTracker.NextOffset(), nilCursorOffset; got != want {
		t.Errorf("commitCursorTracker.NextOffset() got %v, want %v", got, want)
	}
	if got, want := commitTracker.Done(), false; got != want {
		t.Errorf("commitCursorTracker.Done() got %v, want %v", got, want)
	}

	// First 2 pending commits acknowledged.
	if got, want := commitTracker.lastConfirmedOffset, nilCursorOffset; got != want {
		t.Errorf("commitCursorTracker.lastConfirmedOffset got %v, want %v", got, want)
	}
	if err := commitTracker.ConfirmOffsets(2); err != nil {
		t.Errorf("commitCursorTracker.ConfirmOffsets() got err %v", err)
	}
	if got, want := commitTracker.lastConfirmedOffset, int64(4); got != want {
		t.Errorf("commitCursorTracker.lastConfirmedOffset got %v, want %v", got, want)
	}
	if got, want := commitTracker.Done(), true; got != want {
		t.Errorf("commitCursorTracker.Done() got %v, want %v", got, want)
	}
}

func TestCommitCursorTrackerStreamReconnects(t *testing.T) {
	ackTracker := newAckTracker()
	commitTracker := newCommitCursorTracker(ackTracker)

	onAck := func(ac *ackConsumer) {
		// Nothing to do.
	}
	ack1 := newAckConsumer(1, 0, onAck)
	ack2 := newAckConsumer(2, 0, onAck)
	ack3 := newAckConsumer(3, 0, onAck)
	if err := ackTracker.Push(ack1); err != nil {
		t.Errorf("ackTracker.Push() got err %v", err)
	}
	if err := ackTracker.Push(ack2); err != nil {
		t.Errorf("ackTracker.Push() got err %v", err)
	}
	if err := ackTracker.Push(ack3); err != nil {
		t.Errorf("ackTracker.Push() got err %v", err)
	}

	// All messages unacked.
	if got, want := commitTracker.NextOffset(), nilCursorOffset; got != want {
		t.Errorf("commitCursorTracker.NextOffset() got %v, want %v", got, want)
	}

	// Msg1 acked and commit sent to stream.
	ack1.Ack()
	if got, want := commitTracker.NextOffset(), int64(2); got != want {
		t.Errorf("commitCursorTracker.NextOffset() got %v, want %v", got, want)
	}
	commitTracker.AddPending(commitTracker.NextOffset())
	if got, want := commitTracker.NextOffset(), nilCursorOffset; got != want {
		t.Errorf("commitCursorTracker.NextOffset() got %v, want %v", got, want)
	}

	// Msg2 acked and commit sent to stream.
	ack2.Ack()
	if got, want := commitTracker.NextOffset(), int64(3); got != want {
		t.Errorf("commitCursorTracker.NextOffset() got %v, want %v", got, want)
	}
	commitTracker.AddPending(commitTracker.NextOffset())
	if got, want := commitTracker.NextOffset(), nilCursorOffset; got != want {
		t.Errorf("commitCursorTracker.NextOffset() got %v, want %v", got, want)
	}

	// Stream breaks and pending offsets are cleared.
	commitTracker.ClearPending()
	if got, want := commitTracker.Done(), false; got != want {
		t.Errorf("commitCursorTracker.Done() got %v, want %v", got, want)
	}
	// When the stream reconnects the next offset should be 3 (offset 2 skipped).
	if got, want := commitTracker.NextOffset(), int64(3); got != want {
		t.Errorf("commitCursorTracker.NextOffset() got %v, want %v", got, want)
	}
	commitTracker.AddPending(commitTracker.NextOffset())
	if got, want := commitTracker.NextOffset(), nilCursorOffset; got != want {
		t.Errorf("commitCursorTracker.NextOffset() got %v, want %v", got, want)
	}

	// Msg2 acked and commit sent to stream.
	ack3.Ack()
	if got, want := commitTracker.NextOffset(), int64(4); got != want {
		t.Errorf("commitCursorTracker.NextOffset() got %v, want %v", got, want)
	}
	commitTracker.AddPending(commitTracker.NextOffset())
	if got, want := commitTracker.NextOffset(), nilCursorOffset; got != want {
		t.Errorf("commitCursorTracker.NextOffset() got %v, want %v", got, want)
	}

	// Only 1 pending commit confirmed.
	if got, want := commitTracker.lastConfirmedOffset, nilCursorOffset; got != want {
		t.Errorf("commitCursorTracker.lastConfirmedOffset got %v, want %v", got, want)
	}
	if err := commitTracker.ConfirmOffsets(1); err != nil {
		t.Errorf("commitCursorTracker.ConfirmOffsets() got err %v", err)
	}
	if got, want := commitTracker.lastConfirmedOffset, int64(3); got != want {
		t.Errorf("commitCursorTracker.lastConfirmedOffset got %v, want %v", got, want)
	}
	if got, want := commitTracker.Done(), false; got != want {
		t.Errorf("commitCursorTracker.Done() got %v, want %v", got, want)
	}

	// Final pending commit confirmed.
	if err := commitTracker.ConfirmOffsets(1); err != nil {
		t.Errorf("commitCursorTracker.ConfirmOffsets() got err %v", err)
	}
	if got, want := commitTracker.lastConfirmedOffset, int64(4); got != want {
		t.Errorf("commitCursorTracker.lastConfirmedOffset got %v, want %v", got, want)
	}
	if got, want := commitTracker.Done(), true; got != want {
		t.Errorf("commitCursorTracker.Done() got %v, want %v", got, want)
	}

	// Note: Done() returns true even though there are unacked messages.
	ack4 := newAckConsumer(4, 0, onAck)
	if err := ackTracker.Push(ack4); err != nil {
		t.Errorf("ackTracker.Push() got err %v", err)
	}
	if got, want := commitTracker.Done(), true; got != want {
		t.Errorf("commitCursorTracker.Done() got %v, want %v", got, want)
	}
	if got, want := commitTracker.NextOffset(), nilCursorOffset; got != want {
		t.Errorf("commitCursorTracker.NextOffset() got %v, want %v", got, want)
	}
}
