//go:build !spanner_vtproto

// Copyright 2026 Google LLC
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

package spanner

// This file is compiled without the spanner_vtproto build tag. Streams are
// decoded into *sppb.PartialResultSet, and rows keep normal Go ownership. See
// vtproto_stream.go for the build with the tag.

import (
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/protobuf/proto"
)

// vtStream is only implemented with the spanner_vtproto build tag.
type vtStream struct{}

// newVTStream returns nil, because the spanner_vtproto build tag is not set.
func newVTStream() *vtStream { return nil }

func (*vtStream) withCodec(rpc streamRPC) streamRPC { return rpc }

func (*vtStream) next(*RowIterator) (*Row, error) {
	panic("spanner: vtStream requires the spanner_vtproto build tag")
}

func (*vtStream) stop(*resumableStreamDecoder) {}

// recvPartialResultSet receives the next PartialResultSet of stream.
func recvPartialResultSet(_ *vtStream, stream streamingReceiver) (receivedPartialResultSet, error) {
	prs, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	return prs, nil
}

// drainStream receives the end of stream after its last PartialResultSet.
func drainStream(_ *vtStream, stream streamingReceiver) {
	_, _ = stream.Recv()
}

// partialResultSetSize returns the encoded size of r.
func partialResultSetSize(r receivedPartialResultSet) int {
	return proto.Size(r.(*sppb.PartialResultSet))
}

// releasePartialResultSet is called for a PartialResultSet that the iterator
// discards.
func releasePartialResultSet(receivedPartialResultSet) {}

// detachRow returns row. Rows returned by a RowIterator stay valid after the
// next call to Next or Stop without the spanner_vtproto build tag.
func detachRow(row *Row) *Row { return row }

// partialResultSetRouting returns the transaction ID and the cache update of
// the PartialResultSet m that was received with RecvMsg.
func partialResultSetRouting(m any) ([]byte, *sppb.CacheUpdate) {
	prs, ok := m.(*sppb.PartialResultSet)
	if !ok {
		return nil, nil
	}
	return prs.GetMetadata().GetTransaction().GetId(), prs.GetCacheUpdate()
}
