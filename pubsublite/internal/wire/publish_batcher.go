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
	"sort"

	"google.golang.org/api/support/bundler"
	"google.golang.org/protobuf/proto"

	pb "cloud.google.com/go/pubsublite/apiv1/pubsublitepb"
)

var errPublishQueueEmpty = errors.New("pubsublite: received publish response from server with no batches in flight")
var errInvalidCursorRanges = errors.New("pubsublite: server sent invalid cursor ranges in message publish response")

// publisherClientID is a 16-byte identifier for a publisher session and if
// specified, will enable publish idempotency. The same client id must be set
// for all partitions.
type publisherClientID []byte

// publishSequenceNumber uniquely identifies messages in a single publisher
// session, for implementing publish idempotency.
type publishSequenceNumber int64

// PublishResultFunc receives the result of a publish.
type PublishResultFunc func(*MessageMetadata, error)

type publishResult struct {
	Metadata *MessageMetadata
	OnResult PublishResultFunc
}

// messageHolder stores a message to be published, with associated metadata.
type messageHolder struct {
	seqNum   publishSequenceNumber
	msg      *pb.PubSubMessage
	size     int
	onResult PublishResultFunc
}

// publishBatch holds messages that are published in the same
// MessagePublishRequest.
type publishBatch struct {
	clientID   publisherClientID
	msgHolders []*messageHolder
	totalSize  int
}

func (b *publishBatch) ToPublishRequest() *pb.PublishRequest {
	msgs := make([]*pb.PubSubMessage, len(b.msgHolders))
	for i, holder := range b.msgHolders {
		msgs[i] = holder.msg
	}

	var firstSeqNum int64
	if len(b.clientID) > 0 && len(b.msgHolders) > 0 {
		firstSeqNum = int64(b.msgHolders[0].seqNum)
	}

	return &pb.PublishRequest{
		RequestType: &pb.PublishRequest_MessagePublishRequest{
			MessagePublishRequest: &pb.MessagePublishRequest{
				Messages:            msgs,
				FirstSequenceNumber: firstSeqNum,
			},
		},
	}
}

// publishMessageBatcher manages batching of messages, as well as in-flight
// published batches. It is owned by singlePartitionPublisher.
type publishMessageBatcher struct {
	partition int
	// The sequence number to assign to the next message.
	nextSequence publishSequenceNumber
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

func newPublishMessageBatcher(settings *PublishSettings, clientID publisherClientID,
	initialSeqNum publishSequenceNumber, partition int, onNewBatch func(*publishBatch)) *publishMessageBatcher {
	batcher := &publishMessageBatcher{
		partition:            partition,
		nextSequence:         initialSeqNum,
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
		batch := &publishBatch{clientID: clientID, msgHolders: msgs}
		for _, msg := range batch.msgHolders {
			batch.totalSize += msg.size
		}
		onNewBatch(batch)
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
		return fmt.Errorf("pubsublite: serialized message size is %d bytes: %w", msgSize, ErrOversizedMessage)
	case msgSize > b.availableBufferBytes:
		return ErrOverflow
	}

	holder := &messageHolder{seqNum: b.nextSequence, msg: msg, size: msgSize, onResult: onResult}
	b.nextSequence++
	if err := b.msgBundler.Add(holder, msgSize); err != nil {
		// As we've already checked the size of the message and overflow, the
		// bundler should not return an error.
		return fmt.Errorf("pubsublite: failed to batch message: %w", err)
	}
	b.availableBufferBytes -= msgSize
	return nil
}

func (b *publishMessageBatcher) AddBatch(batch *publishBatch) {
	b.publishQueue.PushBack(batch)
}

type byRangeStartIndex []*pb.MessagePublishResponse_CursorRange

func (m byRangeStartIndex) Len() int      { return len(m) }
func (m byRangeStartIndex) Swap(i, j int) { m[i], m[j] = m[j], m[i] }
func (m byRangeStartIndex) Less(i, j int) bool {
	return m[i].GetStartIndex() < m[j].GetStartIndex()
}

func (b *publishMessageBatcher) OnPublishResponse(response *pb.MessagePublishResponse) ([]*publishResult, error) {
	frontElem := b.publishQueue.Front()
	if frontElem == nil {
		return nil, errPublishQueueEmpty
	}

	// Ensure cursor ranges are sorted by increasing message batch index.
	sort.Sort(byRangeStartIndex(response.CursorRanges))

	batch, _ := frontElem.Value.(*publishBatch)
	var results []*publishResult
	var rIdx int
	ranges := response.GetCursorRanges()
	for msgIdx, msgHolder := range batch.msgHolders {
		if rIdx < len(ranges) && ranges[rIdx].GetEndIndex() <= int32(msgIdx) {
			rIdx++
			if rIdx < len(ranges) && ranges[rIdx].GetStartIndex() < ranges[rIdx-1].GetEndIndex() {
				return nil, errInvalidCursorRanges
			}
		}

		offset := int64(-1)
		if rIdx < len(ranges) && msgIdx >= int(ranges[rIdx].GetStartIndex()) && msgIdx < int(ranges[rIdx].GetEndIndex()) {
			offsetInRange := int64(msgIdx) - int64(ranges[rIdx].GetStartIndex())
			offset = ranges[rIdx].GetStartCursor().GetOffset() + offsetInRange
			if offset < b.minExpectedNextOffset {
				return nil, fmt.Errorf("pubsublite: received publish response with offset %d, expected at least %d", offset, b.minExpectedNextOffset)
			}
			b.minExpectedNextOffset = offset + 1
		}
		results = append(results, &publishResult{
			Metadata: &MessageMetadata{Partition: b.partition, Offset: offset},
			OnResult: msgHolder.onResult,
		})
	}

	b.availableBufferBytes += batch.totalSize
	b.publishQueue.Remove(frontElem)
	return results, nil
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
	for elem := b.publishQueue.Front(); elem != nil; {
		batch := elem.Value.(*publishBatch)
		if elem.Prev() != nil {
			// Merge current batch with previous if within max bytes and count limits.
			prevBatch := elem.Prev().Value.(*publishBatch)
			totalSize := prevBatch.totalSize + batch.totalSize
			totalLen := len(prevBatch.msgHolders) + len(batch.msgHolders)
			if totalSize <= MaxPublishRequestBytes && totalLen <= MaxPublishRequestCount {
				prevBatch.totalSize = totalSize
				prevBatch.msgHolders = append(prevBatch.msgHolders, batch.msgHolders...)
				removeElem := elem
				elem = elem.Next()
				b.publishQueue.Remove(removeElem)
				continue
			}
		}
		batches = append(batches, batch)
		elem = elem.Next()
	}
	return batches
}

func (b *publishMessageBatcher) Flush() {
	b.msgBundler.Flush()
}

func (b *publishMessageBatcher) InFlightBatchesEmpty() bool {
	return b.publishQueue.Len() == 0
}

func (b *publishMessageBatcher) NextSequenceNumber() publishSequenceNumber {
	return b.nextSequence
}
