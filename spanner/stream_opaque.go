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
	proto3 "google.golang.org/protobuf/types/known/structpb"
)

const recvMsgUnsupportedError = "this method is not implemented, use Recv"

type messageReceiver interface {
	RecvMsg(any) error
}

// internalPartialResultSet keeps opaque cell values on the iterator path while
// retaining public control-plane messages at their existing API boundaries.
type internalPartialResultSet struct {
	Metadata       *sppb.ResultSetMetadata
	Values         []*opaquepb.Value
	ChunkedValue   bool
	ResumeToken    []byte
	Stats          *sppb.ResultSetStats
	PrecommitToken *sppb.MultiplexedSessionPrecommitToken
	Last           bool
	CacheUpdate    *sppb.CacheUpdate
	wireSize       int
}

func (r *internalPartialResultSet) GetLast() bool { return r != nil && r.Last }
func (r *internalPartialResultSet) GetPrecommitToken() *sppb.MultiplexedSessionPrecommitToken {
	if r == nil {
		return nil
	}
	return r.PrecommitToken
}

func recvMsgUnsupported(err error) bool {
	return err != nil && err.Error() == recvMsgUnsupportedError
}

func recvPartialResultSet(stream streamingReceiver) (*internalPartialResultSet, error) {
	receiver, ok := stream.(messageReceiver)
	if !ok {
		// Unit-test receivers and non-gRPC implementations can retain the open
		// receive method. Production gRPC streams implement RecvMsg.
		return openPartialResultSetToInternal(stream.Recv())
	}

	result := new(opaquepb.PartialResultSet)
	if err := receiver.RecvMsg(result); err != nil {
		// Generated REST streams explicitly reject RecvMsg without consuming a
		// message. Preserve their existing typed receive behavior.
		if recvMsgUnsupported(err) {
			return openPartialResultSetToInternal(stream.Recv())
		}
		return nil, err
	}
	return opaquePartialResultSetToInternal(result)
}

func opaquePartialResultSetToInternal(src *opaquepb.PartialResultSet) (*internalPartialResultSet, error) {
	dst := &internalPartialResultSet{
		Values:       src.GetValues(),
		ChunkedValue: src.GetChunkedValue(),
		ResumeToken:  src.GetResumeToken(),
		Last:         src.GetLast(),
		wireSize:     proto.Size(src),
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

func openPartialResultSetToInternal(src *sppb.PartialResultSet, err error) (*internalPartialResultSet, error) {
	if err != nil || src == nil {
		return nil, err
	}
	values := make([]*opaquepb.Value, len(src.Values))
	for i, value := range src.Values {
		values[i] = publicValueToInternal(value)
	}
	return &internalPartialResultSet{
		Metadata:       src.Metadata,
		Values:         values,
		ChunkedValue:   src.ChunkedValue,
		ResumeToken:    src.ResumeToken,
		Stats:          src.Stats,
		PrecommitToken: src.PrecommitToken,
		Last:           src.Last,
		CacheUpdate:    src.CacheUpdate,
		wireSize:       proto.Size(src),
	}, nil
}

func opaquePartialResultSetToOpen(src *opaquepb.PartialResultSet) (*sppb.PartialResultSet, error) {
	internal, err := opaquePartialResultSetToInternal(src)
	if err != nil {
		return nil, err
	}
	values := make([]*proto3.Value, len(internal.Values))
	for i, value := range internal.Values {
		values[i] = internalValueToPublic(value)
	}
	return &sppb.PartialResultSet{
		Metadata:       internal.Metadata,
		Values:         values,
		ChunkedValue:   internal.ChunkedValue,
		ResumeToken:    internal.ResumeToken,
		Stats:          internal.Stats,
		PrecommitToken: internal.PrecommitToken,
		Last:           internal.Last,
		CacheUpdate:    internal.CacheUpdate,
	}, nil
}

// opaquePartialResultSetControlToOpen converts only fields needed by stream
// observers. Cell values stay opaque and untouched.
func opaquePartialResultSetControlToOpen(src *opaquepb.PartialResultSet) (*sppb.PartialResultSet, error) {
	internal, err := opaquePartialResultSetToInternal(src)
	if err != nil {
		return nil, err
	}
	return &sppb.PartialResultSet{
		Metadata:    internal.Metadata,
		CacheUpdate: internal.CacheUpdate,
	}, nil
}

func copyWireCompatibleMessage(src, dst proto.Message) error {
	wire, err := proto.Marshal(src)
	if err != nil {
		return err
	}
	return proto.Unmarshal(wire, dst)
}

func partialResultSetSize(result *internalPartialResultSet) int {
	if result == nil {
		return 0
	}
	return result.wireSize
}

func internalPartialResultSetFromOpen(result *sppb.PartialResultSet) *internalPartialResultSet {
	converted, _ := openPartialResultSetToInternal(result, nil)
	return converted
}

func internalPartialResultSetToOpen(result *internalPartialResultSet) *sppb.PartialResultSet {
	if result == nil {
		return nil
	}
	values := make([]*proto3.Value, len(result.Values))
	for i, value := range result.Values {
		values[i] = internalValueToPublic(value)
	}
	return &sppb.PartialResultSet{
		Metadata:       result.Metadata,
		Values:         values,
		ChunkedValue:   result.ChunkedValue,
		ResumeToken:    result.ResumeToken,
		Stats:          result.Stats,
		PrecommitToken: result.PrecommitToken,
		Last:           result.Last,
		CacheUpdate:    result.CacheUpdate,
	}
}

func internalPartialResultSetsToOpen(results []*internalPartialResultSet) []*sppb.PartialResultSet {
	converted := make([]*sppb.PartialResultSet, len(results))
	for i, result := range results {
		converted[i] = internalPartialResultSetToOpen(result)
	}
	return converted
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
		result, err = opaquePartialResultSetControlToOpen(message)
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
