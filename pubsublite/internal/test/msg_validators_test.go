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

package test

import (
	"testing"
)

func TestOrderingSender(t *testing.T) {
	sender := NewOrderingSender()
	if got, want := sender.Next("prefix"), "prefix/1"; got != want {
		t.Errorf("OrderingSender.Next() got %q, want %q", got, want)
	}
	if got, want := sender.Next("prefix"), "prefix/2"; got != want {
		t.Errorf("OrderingSender.Next() got %q, want %q", got, want)
	}
	if got, want := sender.Next("foo"), "foo/3"; got != want {
		t.Errorf("OrderingSender.Next() got %q, want %q", got, want)
	}
}

func TestOrderingReceiver(t *testing.T) {
	receiver := NewOrderingReceiver()

	t.Run("Invalid message", func(t *testing.T) {
		if gotErr, wantMsg := receiver.Receive("invalid", "ignored"), "failed to parse index"; !ErrorHasMsg(gotErr, wantMsg) {
			t.Errorf("OrderingReceiver.Receive() got err: %v, want msg: %q", gotErr, wantMsg)
		}
	})

	t.Run("Key=foo", func(t *testing.T) {
		if gotErr := receiver.Receive("foo/1", "foo"); gotErr != nil {
			t.Errorf("OrderingReceiver.Receive() got err: %v", gotErr)
		}
		if gotErr := receiver.Receive("foo/3", "foo"); gotErr != nil {
			t.Errorf("OrderingReceiver.Receive() got err: %v", gotErr)
		}
		if gotErr := receiver.Receive("foo/4", "foo"); gotErr != nil {
			t.Errorf("OrderingReceiver.Receive() got err: %v", gotErr)
		}
		if gotErr, wantMsg := receiver.Receive("foo/4", "foo"), "expected message idx > 4, got 4"; !ErrorHasMsg(gotErr, wantMsg) {
			t.Errorf("OrderingReceiver.Receive() got err: %v, want msg: %q", gotErr, wantMsg)
		}
	})

	t.Run("Key=bar", func(t *testing.T) {
		if gotErr := receiver.Receive("bar/30", "bar"); gotErr != nil {
			t.Errorf("OrderingReceiver.Receive() got err: %v", gotErr)
		}
		if gotErr, wantMsg := receiver.Receive("bar/29", "bar"), "expected message idx > 30, got 29"; !ErrorHasMsg(gotErr, wantMsg) {
			t.Errorf("OrderingReceiver.Receive() got err: %v, want msg: %q", gotErr, wantMsg)
		}
	})
}

func TestDuplicateMsgDetector(t *testing.T) {
	t.Run("No duplicates", func(t *testing.T) {
		duplicateDetector := NewDuplicateMsgDetector()
		duplicateDetector.Receive("foo", 10)
		duplicateDetector.Receive("bar", 20)

		if got, want := duplicateDetector.duplicatePublishCount, int64(0); got != want {
			t.Errorf("DuplicateMsgDetector.duplicatePublishCount() got %v, want %v", got, want)
		}
		if got, want := duplicateDetector.duplicateReceiveCount, int64(0); got != want {
			t.Errorf("DuplicateMsgDetector.duplicateReceiveCount got %v, want %v", got, want)
		}
		if got, want := duplicateDetector.HasPublishDuplicates(), false; got != want {
			t.Errorf("DuplicateMsgDetector.HasPublishDuplicates() got %v, want %v", got, want)
		}
		if got, want := duplicateDetector.HasReceiveDuplicates(), false; got != want {
			t.Errorf("DuplicateMsgDetector.HasReceiveDuplicates() got %v, want %v", got, want)
		}
		if got, want := duplicateDetector.Status(), ""; got != want {
			t.Errorf("DuplicateMsgDetector.Status() got %q, want %q", got, want)
		}
	})

	t.Run("Duplicate publish", func(t *testing.T) {
		duplicateDetector := NewDuplicateMsgDetector()
		duplicateDetector.Receive("foo", 10)
		duplicateDetector.Receive("foo", 11)
		duplicateDetector.Receive("foo", 12)

		if got, want := duplicateDetector.duplicatePublishCount, int64(2); got != want {
			t.Errorf("DuplicateMsgDetector.duplicatePublishCount() got %v, want %v", got, want)
		}
		if got, want := duplicateDetector.duplicateReceiveCount, int64(0); got != want {
			t.Errorf("DuplicateMsgDetector.duplicateReceiveCount got %v, want %v", got, want)
		}
		if got, want := duplicateDetector.HasPublishDuplicates(), true; got != want {
			t.Errorf("DuplicateMsgDetector.HasPublishDuplicates() got %v, want %v", got, want)
		}
		if got, want := duplicateDetector.HasReceiveDuplicates(), false; got != want {
			t.Errorf("DuplicateMsgDetector.HasReceiveDuplicates() got %v, want %v", got, want)
		}
		if got := duplicateDetector.Status(); got == "" {
			t.Errorf("DuplicateMsgDetector.Status() got %q, want status string", got)
		}
	})

	t.Run("Duplicate receive", func(t *testing.T) {
		duplicateDetector := NewDuplicateMsgDetector()
		duplicateDetector.Receive("bar", 20)
		duplicateDetector.Receive("bar", 20)

		if got, want := duplicateDetector.duplicatePublishCount, int64(0); got != want {
			t.Errorf("DuplicateMsgDetector.duplicatePublishCount() got %v, want %v", got, want)
		}
		if got, want := duplicateDetector.duplicateReceiveCount, int64(1); got != want {
			t.Errorf("DuplicateMsgDetector.duplicateReceiveCount got %v, want %v", got, want)
		}
		if got, want := duplicateDetector.HasPublishDuplicates(), false; got != want {
			t.Errorf("DuplicateMsgDetector.HasPublishDuplicates() got %v, want %v", got, want)
		}
		if got, want := duplicateDetector.HasReceiveDuplicates(), true; got != want {
			t.Errorf("DuplicateMsgDetector.HasReceiveDuplicates() got %v, want %v", got, want)
		}
		if got := duplicateDetector.Status(); got == "" {
			t.Errorf("DuplicateMsgDetector.Status() got %q, want status string", got)
		}
	})
}
