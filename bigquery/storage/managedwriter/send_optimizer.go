// Copyright 2023 Google LLC
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
// limitations under the License.

package managedwriter

import (
	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// optimizeAndSend handles the general task of optimizing AppendRowsRequest messages send to the backend.
//
// The basic premise is that by maintaining awareness of previous sends, individual messages can be made
// more efficient (smaller) by redacting redundant information.
type sendOptimizer interface {
	// signalReset is used to signal to the optimizer that the connection is freshly (re)opened.
	signalReset()

	// optimizeSend handles redactions for a given stream.
	optimizeSend(arc storagepb.BigQueryWrite_AppendRowsClient, req *storagepb.AppendRowsRequest) error
}

// passthroughOptimizer is an optimizer that doesn't modify requests.
type passthroughOptimizer struct {
}

func (po *passthroughOptimizer) signalReset() {
	// we don't care, just here to satisfy the interface.
}

func (po *passthroughOptimizer) optimizeSend(arc storagepb.BigQueryWrite_AppendRowsClient, req *storagepb.AppendRowsRequest) error {
	return arc.Send(req)
}

// simplexOptimizer is used for connections where there's only a single stream's data being transmitted.
//
// The optimizations here are straightforward: the first request on a stream is unmodified, all
// subsequent requests can redact WriteStream, WriterSchema, and TraceID.
//
// TODO: this optimizer doesn't do schema evolution checkes, but relies on existing behavior that triggers reconnect
// on schema change.  This should be revisited if and when explicit streams support multiplexing and schema change in-connection.
type simplexOptimizer struct {
	haveSent bool
}

func (eo *simplexOptimizer) signalReset() {
	eo.haveSent = false
}

func (eo *simplexOptimizer) optimizeSend(arc storagepb.BigQueryWrite_AppendRowsClient, req *storagepb.AppendRowsRequest) error {
	var err error
	if eo.haveSent {
		// subsequent send, clone and redact.
		cp := proto.Clone(req).(*storagepb.AppendRowsRequest)
		cp.WriteStream = ""
		if pr := cp.GetProtoRows(); pr != nil {
			pr.WriterSchema = nil
		}
		cp.TraceId = ""
		err = arc.Send(cp)
	} else {
		// first request, send unmodified.
		err = arc.Send(req)
	}
	eo.haveSent = err == nil
	return err
}

// multiplexOptimizer is used for connections where requests for multiple streams are sent on a common connection.
//
// In this case, the optimizations are as follows:
// * We **must** send the WriteStream on all requests.
// * For sequential requests to the same stream, schema can be redacted after the first request.
// * Trace ID can be redacted from all requests after the first.
type multiplexOptimizer struct {
	prev *storagepb.AppendRowsRequest
}

func (mo *multiplexOptimizer) signalReset() {
	mo.prev = nil
}

func (mo *multiplexOptimizer) optimizeSend(arc storagepb.BigQueryWrite_AppendRowsClient, req *storagepb.AppendRowsRequest) error {
	var err error
	// we'll need a copy
	cp := proto.Clone(req).(*storagepb.AppendRowsRequest)
	if mo.prev != nil {
		var swapOnSuccess bool
		// Clear trace ID.  We use the _presence_ of a previous request for reasoning about TraceID, we don't compare
		// it's value.
		cp.TraceId = ""
		// we have a previous send.
		if cp.GetWriteStream() != mo.prev.GetWriteStream() {
			// different stream, no further optimization.
			swapOnSuccess = true
		} else {
			// same stream
			if !proto.Equal(getDescriptorFromAppend(mo.prev), getDescriptorFromAppend(cp)) {
				swapOnSuccess = true
			} else {
				// the redaction case, where we won't swap.
				if pr := cp.GetProtoRows(); pr != nil {
					pr.WriterSchema = nil
				}
			}
		}
		err = arc.Send(cp)
		if err == nil && swapOnSuccess {
			if pr := cp.GetProtoRows(); pr != nil {
				pr.Rows = nil
			}
			cp.MissingValueInterpretations = nil
			mo.prev = cp
		}
		if err != nil {
			mo.prev = nil
		}
		return err
	}

	// no previous trace case.
	err = arc.Send(req)
	if err == nil {
		// copy the send as the previous.
		if pr := cp.GetProtoRows(); pr != nil {
			pr.Rows = nil
		}
		cp.MissingValueInterpretations = nil
		mo.prev = cp
	}
	return err
}

// getDescriptorFromAppend is a utility method for extracting the deeply nested schema
// descriptor from a request.  It returns a nil if the descriptor is not set.
func getDescriptorFromAppend(req *storagepb.AppendRowsRequest) *descriptorpb.DescriptorProto {
	if pr := req.GetProtoRows(); pr != nil {
		if ws := pr.GetWriterSchema(); ws != nil {
			return ws.GetProtoDescriptor()
		}
	}
	return nil
}
