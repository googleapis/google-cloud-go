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
	"bytes"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/pubsublite/internal/test"
	"cloud.google.com/go/pubsublite/publish"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

// testPublishResultReceiver provides convenience methods for receiving and
// validating Publish results.
type testPublishResultReceiver struct {
	done   chan struct{}
	msg    string
	t      *testing.T
	got    *publish.Metadata
	gotErr error
}

func newTestPublishResultReceiver(t *testing.T, msg *pb.PubSubMessage) *testPublishResultReceiver {
	return &testPublishResultReceiver{
		t:    t,
		msg:  string(msg.Data),
		done: make(chan struct{}),
	}
}

func (r *testPublishResultReceiver) set(pm *publish.Metadata, err error) {
	r.got = pm
	r.gotErr = err
	close(r.done)
}

func (r *testPublishResultReceiver) wait() bool {
	select {
	case <-time.After(serviceTestWaitTimeout):
		r.t.Errorf("Publish(%q) result not available within %v", r.msg, serviceTestWaitTimeout)
		return false
	case <-r.done:
		return true
	}
}

func (r *testPublishResultReceiver) ValidateResult(wantPartition int, wantOffset int64) {
	if !r.wait() {
		return
	}
	if r.gotErr != nil {
		r.t.Errorf("Publish(%q) error: (%v), want: partition=%d,offset=%d", r.msg, r.gotErr, wantPartition, wantOffset)
	} else if r.got.Partition != wantPartition || r.got.Offset != wantOffset {
		r.t.Errorf("Publish(%q) got: partition=%d,offset=%d, want: partition=%d,offset=%d", r.msg, r.got.Partition, r.got.Offset, wantPartition, wantOffset)
	}
}

func (r *testPublishResultReceiver) ValidateError(wantErr error) {
	if !r.wait() {
		return
	}
	if !test.ErrorEqual(r.gotErr, wantErr) {
		r.t.Errorf("Publish(%q) error: (%v), want: (%v)", r.msg, r.gotErr, wantErr)
	}
}

func (r *testPublishResultReceiver) ValidateErrorCode(wantCode codes.Code) {
	if !r.wait() {
		return
	}
	if !test.ErrorHasCode(r.gotErr, wantCode) {
		r.t.Errorf("Publish(%q) error: (%v), want code: %v", r.msg, r.gotErr, wantCode)
	}
}

func (r *testPublishResultReceiver) ValidateErrorMsg(wantStr string) {
	if !r.wait() {
		return
	}
	if !test.ErrorHasMsg(r.gotErr, wantStr) {
		r.t.Errorf("Publish(%q) error: (%v), want msg: %q", r.msg, r.gotErr, wantStr)
	}
}

// testPublishBatchReceiver receives message batches from the Bundler.
type testPublishBatchReceiver struct {
	t        *testing.T
	batchesC chan *publishBatch
}

func newTestPublishBatchReceiver(t *testing.T) *testPublishBatchReceiver {
	return &testPublishBatchReceiver{
		t:        t,
		batchesC: make(chan *publishBatch, 3),
	}
}

func (br *testPublishBatchReceiver) onNewBatch(batch *publishBatch) {
	br.batchesC <- batch
}

func (br *testPublishBatchReceiver) ValidateBatches(want []*publishBatch) {
	var got []*publishBatch
	for count := 0; count < len(want); count++ {
		select {
		case <-time.After(serviceTestWaitTimeout):
			br.t.Errorf("Publish batches count: got %d, want %d", count, len(want))
		case batch := <-br.batchesC:
			got = append(got, batch)
		}
	}

	if !testutil.Equal(got, want, cmp.AllowUnexported(publishBatch{}, messageHolder{})) {
		br.t.Errorf("Batches got: %v\nwant: %v", got, want)
	}
}

func makeMsgHolder(msg *pb.PubSubMessage, receiver ...*testPublishResultReceiver) *messageHolder {
	h := &messageHolder{
		msg:  msg,
		size: proto.Size(msg),
	}
	if len(receiver) > 0 {
		h.onResult = receiver[0].set
	}
	return h
}

func TestPublishBatcherAddMessage(t *testing.T) {
	const initAvailableBytes = MaxPublishRequestBytes
	settings := DefaultPublishSettings
	settings.BufferedByteLimit = initAvailableBytes

	receiver := newTestPublishBatchReceiver(t)
	batcher := newPublishMessageBatcher(&settings, 0, receiver.onNewBatch)

	if got, want := batcher.availableBufferBytes, initAvailableBytes; got != want {
		t.Errorf("availableBufferBytes: got %d, want %d", got, want)
	}

	t.Run("small messages", func(t *testing.T) {
		msg1 := &pb.PubSubMessage{Data: []byte("foo")}
		msgSize1 := proto.Size(msg1)
		if err := batcher.AddMessage(msg1, nil); err != nil {
			t.Errorf("AddMessage(%v) got err: %v", msg1, err)
		}
		if got, want := batcher.availableBufferBytes, initAvailableBytes-msgSize1; got != want {
			t.Errorf("availableBufferBytes: got %d, want %d", got, want)
		}

		msg2 := &pb.PubSubMessage{Data: []byte("hello world")}
		msgSize2 := proto.Size(msg2)
		if err := batcher.AddMessage(msg2, nil); err != nil {
			t.Errorf("AddMessage(%v) got err: %v", msg2, err)
		}
		if got, want := batcher.availableBufferBytes, initAvailableBytes-msgSize1-msgSize2; got != want {
			t.Errorf("availableBufferBytes: got %d, want %d", got, want)
		}
	})

	t.Run("oversized message", func(t *testing.T) {
		msg := &pb.PubSubMessage{Data: bytes.Repeat([]byte{'0'}, MaxPublishRequestBytes)}
		if gotErr, wantMsg := batcher.AddMessage(msg, nil), "MaxPublishRequestBytes"; !test.ErrorHasMsg(gotErr, wantMsg) {
			t.Errorf("AddMessage(%v) got err: %v, want err msg: %q", msg, gotErr, wantMsg)
		}
	})

	t.Run("buffer overflow", func(t *testing.T) {
		msg := &pb.PubSubMessage{Data: bytes.Repeat([]byte{'1'}, batcher.availableBufferBytes)}
		if gotErr, wantErr := batcher.AddMessage(msg, nil), ErrOverflow; !test.ErrorEqual(gotErr, wantErr) {
			t.Errorf("AddMessage(%v) got err: %v, want err: %v", msg, gotErr, wantErr)
		}
	})
}

func TestPublishBatcherBundlerCountThreshold(t *testing.T) {
	settings := DefaultPublishSettings
	settings.DelayThreshold = time.Minute // Batching delay disabled
	settings.CountThreshold = 2

	// Batch 1
	msg1 := &pb.PubSubMessage{Data: []byte{'1'}}
	msg2 := &pb.PubSubMessage{Data: []byte{'2'}}
	wantBatch1 := &publishBatch{
		[]*messageHolder{makeMsgHolder(msg1), makeMsgHolder(msg2)},
	}

	// Batch 2
	msg3 := &pb.PubSubMessage{Data: []byte{'3'}}
	msg4 := &pb.PubSubMessage{Data: []byte{'4'}}
	wantBatch2 := &publishBatch{
		[]*messageHolder{makeMsgHolder(msg3), makeMsgHolder(msg4)},
	}

	// Batch 3
	msg5 := &pb.PubSubMessage{Data: []byte{'5'}}
	wantBatch3 := &publishBatch{
		[]*messageHolder{makeMsgHolder(msg5)},
	}

	receiver := newTestPublishBatchReceiver(t)
	batcher := newPublishMessageBatcher(&settings, 0, receiver.onNewBatch)

	msgs := []*pb.PubSubMessage{msg1, msg2, msg3, msg4, msg5}
	for _, msg := range msgs {
		if err := batcher.AddMessage(msg, nil); err != nil {
			t.Errorf("AddMessage(%v) got err: %v", msg, err)
		}
	}
	batcher.Flush()

	receiver.ValidateBatches([]*publishBatch{wantBatch1, wantBatch2, wantBatch3})
}

func TestPublishBatcherBundlerBatchingDelay(t *testing.T) {
	settings := DefaultPublishSettings
	settings.DelayThreshold = 5 * time.Millisecond

	// Batch 1
	msg1 := &pb.PubSubMessage{Data: []byte{'1'}}
	wantBatch1 := &publishBatch{
		[]*messageHolder{makeMsgHolder(msg1)},
	}

	// Batch 2
	msg2 := &pb.PubSubMessage{Data: []byte{'2'}}
	wantBatch2 := &publishBatch{
		[]*messageHolder{makeMsgHolder(msg2)},
	}

	receiver := newTestPublishBatchReceiver(t)
	batcher := newPublishMessageBatcher(&settings, 0, receiver.onNewBatch)

	if err := batcher.AddMessage(msg1, nil); err != nil {
		t.Errorf("AddMessage(%v) got err: %v", msg1, err)
	}
	time.Sleep(settings.DelayThreshold * 2)
	if err := batcher.AddMessage(msg2, nil); err != nil {
		t.Errorf("AddMessage(%v) got err: %v", msg2, err)
	}
	batcher.Flush()

	receiver.ValidateBatches([]*publishBatch{wantBatch1, wantBatch2})
}

func TestPublishBatcherBundlerOnPermanentError(t *testing.T) {
	receiver := newTestPublishBatchReceiver(t)
	batcher := newPublishMessageBatcher(&DefaultPublishSettings, 0, receiver.onNewBatch)

	msg1 := &pb.PubSubMessage{Data: []byte{'1'}}
	msg2 := &pb.PubSubMessage{Data: []byte{'2'}}
	pubResult1 := newTestPublishResultReceiver(t, msg1)
	pubResult2 := newTestPublishResultReceiver(t, msg2)
	batcher.AddBatch(&publishBatch{
		[]*messageHolder{
			makeMsgHolder(msg1, pubResult1),
			makeMsgHolder(msg2, pubResult2),
		},
	})

	wantErr := status.Error(codes.FailedPrecondition, "failed")
	batcher.OnPermanentError(wantErr)
	pubResult1.ValidateError(wantErr)
	pubResult2.ValidateError(wantErr)
}

func TestPublishBatcherBundlerOnPublishResponse(t *testing.T) {
	const partition = 2
	receiver := newTestPublishBatchReceiver(t)
	batcher := newPublishMessageBatcher(&DefaultPublishSettings, partition, receiver.onNewBatch)

	t.Run("empty in-flight batches", func(t *testing.T) {
		if gotErr, wantErr := batcher.OnPublishResponse(0), errPublishQueueEmpty; !test.ErrorEqual(gotErr, wantErr) {
			t.Errorf("OnPublishResponse() got err: %v, want err: %v", gotErr, wantErr)
		}
	})

	t.Run("set publish results", func(t *testing.T) {
		// Batch 1
		msg1 := &pb.PubSubMessage{Data: []byte{'1'}}
		msg2 := &pb.PubSubMessage{Data: []byte{'2'}}

		// Batch 2
		msg3 := &pb.PubSubMessage{Data: []byte{'3'}}
		pubResult1 := newTestPublishResultReceiver(t, msg1)
		pubResult2 := newTestPublishResultReceiver(t, msg2)
		pubResult3 := newTestPublishResultReceiver(t, msg3)

		batcher.AddBatch(&publishBatch{
			[]*messageHolder{
				makeMsgHolder(msg1, pubResult1),
				makeMsgHolder(msg2, pubResult2),
			},
		})
		batcher.AddBatch(&publishBatch{
			[]*messageHolder{
				makeMsgHolder(msg3, pubResult3),
			},
		})
		if err := batcher.OnPublishResponse(70); err != nil {
			t.Errorf("OnPublishResponse() got err: %v", err)
		}
		if err := batcher.OnPublishResponse(80); err != nil {
			t.Errorf("OnPublishResponse() got err: %v", err)
		}

		pubResult1.ValidateResult(partition, 70)
		pubResult2.ValidateResult(partition, 71)
		pubResult3.ValidateResult(partition, 80)
	})

	t.Run("inconsistent offset", func(t *testing.T) {
		msg := &pb.PubSubMessage{Data: []byte{'4'}}
		pubResult := newTestPublishResultReceiver(t, msg)
		batcher.AddBatch(&publishBatch{
			[]*messageHolder{
				makeMsgHolder(msg, pubResult),
			},
		})

		if gotErr, wantMsg := batcher.OnPublishResponse(80), "inconsistent start offset = 80"; !test.ErrorHasMsg(gotErr, wantMsg) {
			t.Errorf("OnPublishResponse() got err: %v, want err msg: %q", gotErr, wantMsg)
		}
	})
}
