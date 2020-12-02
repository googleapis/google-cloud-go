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
	"errors"
	"fmt"
	"math"

	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

var (
	errTokenCounterBytesNegative    = errors.New("pubsublite: received messages that account for more bytes than were requested")
	errTokenCounterMessagesNegative = errors.New("pubsublite: received more messages than were requested")
	errOutOfOrderMessages           = errors.New("pubsublite: server delivered messages out of order")
)

type flowControlTokens struct {
	Bytes    int64
	Messages int64
}

// A tokenCounter stores the amount of outstanding byte and message flow control
// tokens that the client believes exists for the stream.
type tokenCounter struct {
	Bytes    int64
	Messages int64
}

func saturatedAdd(sum, delta int64) int64 {
	remainder := math.MaxInt64 - sum
	if delta >= remainder {
		return math.MaxInt64
	}
	return sum + delta
}

func (tc *tokenCounter) Add(delta flowControlTokens) {
	tc.Bytes = saturatedAdd(tc.Bytes, delta.Bytes)
	tc.Messages = saturatedAdd(tc.Messages, delta.Messages)
}

func (tc *tokenCounter) Sub(delta flowControlTokens) error {
	if delta.Bytes > tc.Bytes {
		return errTokenCounterBytesNegative
	}
	if delta.Messages > tc.Messages {
		return errTokenCounterMessagesNegative
	}
	tc.Bytes -= delta.Bytes
	tc.Messages -= delta.Messages
	return nil
}

func (tc *tokenCounter) Reset() {
	tc.Bytes = 0
	tc.Messages = 0
}

func (tc *tokenCounter) ToFlowControlRequest() *pb.FlowControlRequest {
	if tc.Bytes <= 0 && tc.Messages <= 0 {
		return nil
	}
	return &pb.FlowControlRequest{
		AllowedBytes:    tc.Bytes,
		AllowedMessages: tc.Messages,
	}
}

// flowControlBatcher tracks flow control tokens and manages batching of flow
// control requests to avoid overwhelming the server. It is only accessed by
// the subscribeStream.
type flowControlBatcher struct {
	// The current amount of outstanding byte and message flow control tokens.
	clientTokens tokenCounter
	// The pending batch flow control request that needs to be sent to the stream.
	pendingTokens tokenCounter
}

const expediteBatchRequestRatio = 0.5

func exceedsExpediteRatio(pending, client int64) bool {
	return client > 0 && (float64(pending)/float64(client)) >= expediteBatchRequestRatio
}

// OnClientFlow increments flow control tokens. This occurs when:
// - Initialization from ReceiveSettings.
// - The user acks messages.
func (fc *flowControlBatcher) OnClientFlow(tokens flowControlTokens) {
	fc.clientTokens.Add(tokens)
	fc.pendingTokens.Add(tokens)
}

// OnMessages decrements flow control tokens when messages are received from the
// server.
func (fc *flowControlBatcher) OnMessages(msgs []*pb.SequencedMessage) error {
	var totalBytes int64
	for _, msg := range msgs {
		totalBytes += msg.GetSizeBytes()
	}
	return fc.clientTokens.Sub(flowControlTokens{Bytes: totalBytes, Messages: int64(len(msgs))})
}

// RequestForRestart returns a FlowControlRequest that should be sent when a new
// subscriber stream is connected. May return nil.
func (fc *flowControlBatcher) RequestForRestart() *pb.FlowControlRequest {
	fc.pendingTokens.Reset()
	return fc.clientTokens.ToFlowControlRequest()
}

// ReleasePendingRequest returns a non-nil request when there is a batch
// FlowControlRequest to send to the stream.
func (fc *flowControlBatcher) ReleasePendingRequest() *pb.FlowControlRequest {
	req := fc.pendingTokens.ToFlowControlRequest()
	fc.pendingTokens.Reset()
	return req
}

// ShouldExpediteBatchRequest returns true if a batch FlowControlRequest should
// be sent ASAP to avoid starving the client of messages. This occurs when the
// client is rapidly acking messages.
func (fc *flowControlBatcher) ShouldExpediteBatchRequest() bool {
	if exceedsExpediteRatio(fc.pendingTokens.Bytes, fc.clientTokens.Bytes) {
		return true
	}
	if exceedsExpediteRatio(fc.pendingTokens.Messages, fc.clientTokens.Messages) {
		return true
	}
	return false
}

// subscriberOffsetTracker tracks the expected offset of the next message
// received from the server. It is only accessed by the subscribeStream.
type subscriberOffsetTracker struct {
	minNextOffset int64
}

// RequestForRestart returns the seek request to send when a new subscribe
// stream reconnects. Returns nil if the subscriber has just started, in which
// case the server returns the offset of the last committed cursor.
func (ot *subscriberOffsetTracker) RequestForRestart() *pb.SeekRequest {
	if ot.minNextOffset <= 0 {
		return nil
	}
	return &pb.SeekRequest{
		Target: &pb.SeekRequest_Cursor{
			Cursor: &pb.Cursor{Offset: ot.minNextOffset},
		},
	}
}

// OnMessages verifies that messages are delivered in order and updates the next
// expected offset.
func (ot *subscriberOffsetTracker) OnMessages(msgs []*pb.SequencedMessage) error {
	nextOffset := ot.minNextOffset
	for i, msg := range msgs {
		offset := msg.GetCursor().GetOffset()
		if offset < nextOffset {
			if i == 0 {
				return fmt.Errorf("pubsublite: server delivered messages with start offset = %d, expected >= %d", offset, ot.minNextOffset)
			}
			return errOutOfOrderMessages
		}
		nextOffset = offset + 1
	}
	ot.minNextOffset = nextOffset
	return nil
}
