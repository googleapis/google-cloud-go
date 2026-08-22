// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build spanner_opaque

package spanner

import (
	"fmt"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal/opaquepb"
	"google.golang.org/protobuf/proto"
)

const recvMsgUnsupportedError = "this method is not implemented, use Recv"

type messageReceiver interface {
	RecvMsg(any) error
}

func recvMsgUnsupported(err error) bool {
	return err != nil && err.Error() == recvMsgUnsupportedError
}

func recvPartialResultSet(stream streamingReceiver) (*sppb.PartialResultSet, error) {
	receiver, ok := stream.(messageReceiver)
	if !ok {
		// Unit-test receivers and non-gRPC implementations can retain the open
		// receive method. Production gRPC streams implement RecvMsg.
		return stream.Recv()
	}

	result := new(opaquepb.PartialResultSet)
	if err := receiver.RecvMsg(result); err != nil {
		// Generated REST streams explicitly reject RecvMsg without consuming a
		// message. Preserve their existing typed receive behavior.
		if recvMsgUnsupported(err) {
			return stream.Recv()
		}
		return nil, err
	}
	return opaquePartialResultSetToOpen(result)
}

func opaquePartialResultSetToOpen(src *opaquepb.PartialResultSet) (*sppb.PartialResultSet, error) {
	dst := &sppb.PartialResultSet{
		// Values remain structpb.Value in phase 1. Reuse the decoded pointers so
		// the per-cell path performs no wire marshal or copy.
		Values:       src.GetValues(),
		ChunkedValue: src.GetChunkedValue(),
		ResumeToken:  src.GetResumeToken(),
		Last:         src.GetLast(),
	}

	// Metadata crosses to the public open type because RowIterator.Metadata,
	// Row field types, and transaction callbacks are public/open boundaries.
	if src.HasMetadata() {
		dst.Metadata = new(sppb.ResultSetMetadata)
		if err := copyWireCompatibleMessage(src.GetMetadata(), dst.Metadata); err != nil {
			return nil, fmt.Errorf("convert opaque result metadata: %w", err)
		}
	}
	// Stats crosses to the public open type because QueryPlan, QueryStats, and
	// row-count extraction remain public/open surfaces.
	if src.HasStats() {
		dst.Stats = new(sppb.ResultSetStats)
		if err := copyWireCompatibleMessage(src.GetStats(), dst.Stats); err != nil {
			return nil, fmt.Errorf("convert opaque result stats: %w", err)
		}
	}
	// Precommit tokens cross to the public open type consumed by transaction
	// state and commit request construction.
	if src.HasPrecommitToken() {
		dst.PrecommitToken = new(sppb.MultiplexedSessionPrecommitToken)
		if err := copyWireCompatibleMessage(src.GetPrecommitToken(), dst.PrecommitToken); err != nil {
			return nil, fmt.Errorf("convert opaque precommit token: %w", err)
		}
	}
	// Cache updates cross to the public open type consumed by location routing.
	if src.HasCacheUpdate() {
		dst.CacheUpdate = new(sppb.CacheUpdate)
		if err := copyWireCompatibleMessage(src.GetCacheUpdate(), dst.CacheUpdate); err != nil {
			return nil, fmt.Errorf("convert opaque cache update: %w", err)
		}
	}
	return dst, nil
}

func copyWireCompatibleMessage(src, dst proto.Message) error {
	wire, err := proto.Marshal(src)
	if err != nil {
		return err
	}
	return proto.Unmarshal(wire, dst)
}

// RecvMsg preserves location-aware stream side effects when the tagged receive
// path bypasses the generated typed Recv method.
func (s *affinityTrackingStream) RecvMsg(message any) error {
	err := s.inner.RecvMsg(message)
	if recvMsgUnsupported(err) {
		return err
	}
	if err != nil {
		return s.observeRecv(nil, err)
	}

	var result *sppb.PartialResultSet
	switch message := message.(type) {
	case *opaquepb.PartialResultSet:
		result, err = opaquePartialResultSetToOpen(message)
	case *sppb.PartialResultSet:
		result = message
	default:
		err = fmt.Errorf("unexpected PartialResultSet receive type %T", message)
	}
	if err != nil {
		return s.observeRecv(nil, err)
	}
	return s.observeRecv(result, nil)
}

// RecvMsg preserves dynamic-channel stream accounting when the tagged receive
// path bypasses the generated typed Recv method.
func (c *dcpExecuteStreamingSqlClient) RecvMsg(message any) error {
	err := c.Spanner_ExecuteStreamingSqlClient.RecvMsg(message)
	if err != nil && !recvMsgUnsupported(err) {
		c.ref.done(err)
	}
	return err
}

// RecvMsg preserves dynamic-channel stream accounting when the tagged receive
// path bypasses the generated typed Recv method.
func (c *dcpStreamingReadClient) RecvMsg(message any) error {
	err := c.Spanner_StreamingReadClient.RecvMsg(message)
	if err != nil && !recvMsgUnsupported(err) {
		c.ref.done(err)
	}
	return err
}
