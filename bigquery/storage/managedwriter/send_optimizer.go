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
	"hash/crc32"
	"time"

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
	optimizeSend(arc storagepb.BigQueryWrite_AppendRowsClient, pw *pendingWrite) error
}

// passthroughOptimizer is an optimizer that doesn't modify requests.
type passthroughOptimizer struct {
}

func (po *passthroughOptimizer) signalReset() {
	// we don't care, just here to satisfy the interface.
}

func (po *passthroughOptimizer) optimizeSend(arc storagepb.BigQueryWrite_AppendRowsClient, pw *pendingWrite) error {
	return arc.Send(pw.request)
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

func (eo *simplexOptimizer) optimizeSend(arc storagepb.BigQueryWrite_AppendRowsClient, pw *pendingWrite) error {
	var err error
	if eo.haveSent {
		// subsequent send, clone and redact.
		cp := proto.Clone(pw.request).(*storagepb.AppendRowsRequest)
		cp.WriteStream = ""
		if pr := cp.GetProtoRows(); pr != nil {
			pr.WriterSchema = nil
		}
		cp.TraceId = ""
		err = arc.Send(cp)
	} else {
		// first request, send unmodified.
		err = arc.Send(pw.request)
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
	prevStream            string
	prevDescriptorVersion *descriptorVersion
}

func (mo *multiplexOptimizer) signalReset() {
	mo.prevStream = ""
	mo.prevDescriptorVersion = nil
}

func (mo *multiplexOptimizer) optimizeSend(arc storagepb.BigQueryWrite_AppendRowsClient, pw *pendingWrite) error {
	var err error
	if mo.prevStream == "" {
		// startup case, send it all unmodified.
		err = arc.Send(pw.request)
		if err == nil {
			mo.prevStream = pw.request.GetWriteStream()
			mo.prevDescriptorVersion = pw.descVersion
		}
	} else {
		// we have a previous send.  make a copy as we'll modify it.
		curStream := pw.request.GetWriteStream()
		cp := proto.Clone(pw.request).(*storagepb.AppendRowsRequest)
		cp.TraceId = ""
		if mo.prevStream == curStream {
			// same stream
			swapOnSuccess := false
			if mo.prevDescriptorVersion != nil {
				if mo.prevDescriptorVersion.eqVersion(pw.descVersion) {
					// same, redact schema
					if pr := cp.GetProtoRows(); pr != nil {
						pr.WriterSchema = nil
					}
				} else {
					swapOnSuccess = true
				}
			}
			err = arc.Send(cp)
			if err != nil {
				// On send error, return to cleared state.
				mo.signalReset()
			} else {
				if swapOnSuccess {
					mo.prevDescriptorVersion = pw.descVersion
				}
			}
		} else {
			// different stream, but after the first append.
			// Send without further modification.
			err = arc.Send(cp)
			if err != nil {
				// On send error, return to cleared state.
				mo.signalReset()
			} else {
				mo.prevStream = curStream
				mo.prevDescriptorVersion = pw.descVersion
			}
		}
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

type descriptorVersion struct {
	versionTime     time.Time
	descriptorProto *descriptorpb.DescriptorProto
	hashVal         uint32
}

func newDescriptorVersion(in *descriptorpb.DescriptorProto) *descriptorVersion {
	var hashVal uint32
	if b, err := proto.Marshal(in); err == nil {
		hashVal = crc32.ChecksumIEEE(b)
	}
	return &descriptorVersion{
		versionTime:     time.Now(),
		descriptorProto: proto.Clone(in).(*descriptorpb.DescriptorProto),
		hashVal:         hashVal,
	}
}

func (dv *descriptorVersion) eqVersion(other *descriptorVersion) bool {
	if dv == nil || other == nil {
		return false
	}
	if dv.versionTime != other.versionTime {
		return false
	}
	if dv.hashVal == 0 || other.hashVal == 0 {
		return false
	}
	if dv.hashVal != other.hashVal {
		return false
	}
	// The expensive-but-unlikely case here is two schemas that share the same time and hash,
	// but are two slightly different descriptors.  We indicate true here as a speed compromise.
	return true
}

func (dv *descriptorVersion) isNewer(other *descriptorVersion) bool {
	return dv.versionTime.UnixNano() > other.versionTime.UnixNano()
}
