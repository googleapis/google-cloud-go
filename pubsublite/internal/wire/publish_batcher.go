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
	"errors"
	"fmt"

	"cloud.google.com/go/pubsublite/publish"
	"golang.org/x/xerrors"
	"google.golang.org/api/support/bundler"
	"google.golang.org/protobuf/proto"

	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

var errPublishQueueEmpty = errors.New("pubsublite: received publish response from server with no batches in flight")

// PublishResultFunc receives the result of a publish.
type PublishResultFunc func(*publish.Metadata, error)

// messageHolder stores a message to be published, with associated metadata.
type messageHolder struct {
	msg      *pb.PubSubMessage
	size     int
	onResult PublishResultFunc
}

// publishBatch holds messages that are published in the same
// MessagePublishRequest.
type publishBatch struct {
	msgHolders []*messageHolder
}

func (b *publishBatch) ToPublishRequest() *pb.PublishRequest {
	msgs := make([]*pb.PubSubMessage, len(b.msgHolders))
	for i, holder := range b.msgHolders {
		msgs[i] = holder.msg
	}

	return &pb.PublishRequest{
		RequestType: &pb.PublishRequest_MessagePublishRequest{
			MessagePublishRequest: &pb.MessagePublishRequest{
				Messages: msgs,
			},
		},
	}
}

// publishMessageBatcher manages batching of messages, as well as in-flight
// published batches. It is owned by singlePartitionPublisher.
type publishMessageBatcher struct {
	partition int
	// Used to batch messages. Setting HandlerLimit=1 results in ordered batches.
	msgBundler *bundler.Bundler
	// FIFO queue of in-flight batches of published messages. Results have not yet
	// been received from the server.
	publishQueue *list.List // Value = *publishBatch
	// Used for error checking, to ensure the server returns increasing offsets
	// for published messages.
	minExpectedNextOffset int64
	// The available buffer size is managed by this batcher rather than the
	// Bundler due to the in-flight publish queue.
	availableBufferBytes int
}

func newPublishMessageBatcher(settings *PublishSettings, partition int, onNewBatch func(*publishBatch)) *publishMessageBatcher {
	batcher := &publishMessageBatcher{
		partition:            partition,
		publishQueue:         list.New(),
		availableBufferBytes: settings.BufferedByteLimit,
	}

	msgBundler := bundler.NewBundler(&messageHolder{}, func(item interface{}) {
		msgs, _ := item.([]*messageHolder)
		if len(msgs) == 0 {
			// This should not occur.
			return
		}
		// The publishMessageBatcher is accessed by the singlePartitionPublisher and
		// Bundler handler func (called in a goroutine).
		// singlePartitionPublisher.onNewBatch() receives the new batch from the
		// Bundler, which calls publishMessageBatcher.AddBatch(). Only the
		// publisher's mutex is required.
		onNewBatch(&publishBatch{msgHolders: msgs})
	})
	msgBundler.DelayThreshold = settings.DelayThreshold
	msgBundler.BundleCountThreshold = settings.CountThreshold
	msgBundler.BundleByteThreshold = settings.ByteThreshold   // Soft limit
	msgBundler.BundleByteLimit = MaxPublishRequestBytes       // Hard limit
	msgBundler.HandlerLimit = 1                               // Handle batches serially for ordering
	msgBundler.BufferedByteLimit = settings.BufferedByteLimit // Actually handled in the batcher

	batcher.msgBundler = msgBundler
	return batcher
}

func (b *publishMessageBatcher) AddMessage(msg *pb.PubSubMessage, onResult PublishResultFunc) error {
	msgSize := proto.Size(msg)
	switch {
	case msgSize > MaxPublishRequestBytes:
		return xerrors.Errorf("pubsublite: serialized message size is %d bytes: %w", msgSize, ErrOversizedMessage)
	case msgSize > b.availableBufferBytes:
		return ErrOverflow
	}

	holder := &messageHolder{msg: msg, size: msgSize, onResult: onResult}
	if err := b.msgBundler.Add(holder, msgSize); err != nil {
		// As we've already checked the size of the message and overflow, the
		// bundler should not return an error.
		return fmt.Errorf("pubsublite: failed to batch message: %v", err)
	}
	b.availableBufferBytes -= msgSize
	return nil
}

func (b *publishMessageBatcher) AddBatch(batch *publishBatch) {
	b.publishQueue.PushBack(batch)
}

func (b *publishMessageBatcher) OnPublishResponse(firstOffset int64) error {
	frontElem := b.publishQueue.Front()
	if frontElem == nil {
		return errPublishQueueEmpty
	}
	if firstOffset < b.minExpectedNextOffset {
		return fmt.Errorf("pubsublite: server returned publish response with inconsistent start offset = %d, expected >= %d", firstOffset, b.minExpectedNextOffset)
	}

	batch, _ := frontElem.Value.(*publishBatch)
	for i, msgHolder := range batch.msgHolders {
		// Messages are ordered, so the offset of each message is firstOffset + i.
		pm := &publish.Metadata{Partition: b.partition, Offset: firstOffset + int64(i)}
		msgHolder.onResult(pm, nil)
		b.availableBufferBytes += msgHolder.size
	}

	b.minExpectedNextOffset = firstOffset + int64(len(batch.msgHolders))
	b.publishQueue.Remove(frontElem)
	return nil
}

func (b *publishMessageBatcher) OnPermanentError(err error) {
	for elem := b.publishQueue.Front(); elem != nil; elem = elem.Next() {
		if batch, ok := elem.Value.(*publishBatch); ok {
			for _, msgHolder := range batch.msgHolders {
				msgHolder.onResult(nil, err)
			}
		}
	}
	b.publishQueue.Init()
}

func (b *publishMessageBatcher) InFlightBatches() []*publishBatch {
	var batches []*publishBatch
	for elem := b.publishQueue.Front(); elem != nil; elem = elem.Next() {
		if batch, ok := elem.Value.(*publishBatch); ok {
			batches = append(batches, batch)
		}
	}
	return batches
}

func (b *publishMessageBatcher) Flush() {
	b.msgBundler.Flush()
}

func (b *publishMessageBatcher) InFlightBatchesEmpty() bool {
	return b.publishQueue.Len() == 0
}
