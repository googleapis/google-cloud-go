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

// sendOptimizer handles the general task of optimizing AppendRowsRequest messages send to the backend.
//
// The general premise is that the ordering of AppendRowsRequests on a connection provides some opportunities
// to reduce payload size, thus potentially increasing throughput.  Care must be taken, however, as deep inspection
// of requests is potentially more costly (in terms of CPU usage) than gains from reducing request sizes.
type sendOptimizer interface {
	// signalReset is used to signal to the optimizer that the connection is freshly (re)opened, or that a previous
	// send yielded an error.
	signalReset()

	// optimizeSend handles possible manipulation of a request, and triggers the send.
	optimizeSend(arc storagepb.BigQueryWrite_AppendRowsClient, pw *pendingWrite) error
}

// verboseOptimizer is a primarily a testing optimizer that always sends the full request.
type verboseOptimizer struct {
}

func (vo *verboseOptimizer) signalReset() {
	// This optimizer is stateless.
}

// optimizeSend populates a full request every time.
func (vo *verboseOptimizer) optimizeSend(arc storagepb.BigQueryWrite_AppendRowsClient, pw *pendingWrite) error {
	return arc.Send(pw.constructFullRequest(true))
}

// simplexOptimizer is used for connections bearing AppendRowsRequest for only a single stream.
//
// The optimizations here are straightforward:
// * The first request on a connection is unmodified.
// * Subsequent requests can redact WriteStream, WriterSchema, and TraceID.
//
// Behavior of schema evolution differs based on the type of stream.
// * For an explicit stream, the connection must reconnect to signal schema change (handled in connection).
// * For default streams, the new descriptor (inside WriterSchema) can simply be sent.
type simplexOptimizer struct {
	haveSent bool
}

func (so *simplexOptimizer) signalReset() {
	so.haveSent = false
}

func (so *simplexOptimizer) optimizeSend(arc storagepb.BigQueryWrite_AppendRowsClient, pw *pendingWrite) error {
	var err error
	if so.haveSent {
		// subsequent send, we can send the request unmodified.
		err = arc.Send(pw.req)
	} else {
		// first request, build a full request.
		err = arc.Send(pw.constructFullRequest(true))
	}
	so.haveSent = err == nil
	return err
}

// multiplexOptimizer is used for connections where requests for multiple default streams are sent on a common
// connection.  Only default streams can currently be multiplexed.
//
// In this case, the optimizations are as follows:
// * We must send the WriteStream on all requests.
// * For sequential requests to the same stream, schema can be redacted after the first request.
// * Trace ID can be redacted from all requests after the first.
//
// Schema evolution is simply a case of sending the new WriterSchema as part of the request(s).  No explicit
// reconnection is necessary.
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
		// startup case, send a full request (with traceID).
		req := pw.constructFullRequest(true)
		err = arc.Send(req)
		if err == nil {
			mo.prevStream = req.GetWriteStream()
			mo.prevDescriptorVersion = pw.descVersion
		}
	} else {
		// We have a previous send.  Determine if it's the same stream or a different one.
		if mo.prevStream == pw.writeStreamID {
			// add the stream ID to the optimized request, as multiplex-optimization wants it present.
			if pw.req.GetWriteStream() == "" {
				pw.req.WriteStream = pw.writeStreamID
			}
			// swapOnSuccess tracks if we need to update schema versions on successful send.
			swapOnSuccess := false
			req := pw.req
			if mo.prevDescriptorVersion != nil {
				if !mo.prevDescriptorVersion.eqVersion(pw.descVersion) {
					swapOnSuccess = true
					req = pw.constructFullRequest(false) // full request minus traceID.
				}
			}
			err = arc.Send(req)
			if err == nil && swapOnSuccess {
				mo.prevDescriptorVersion = pw.descVersion
			}
		} else {
			// The previous send was for a different stream.  Send a full request, minus traceId.
			req := pw.constructFullRequest(false)
			err = arc.Send(req)
			if err == nil {
				// Send successful.  Update state to reflect this send is now the "previous" state.
				mo.prevStream = pw.writeStreamID
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

// descriptorVersion is used for faster comparisons of proto descriptors.  Deep equality comparisons
// of DescriptorProto can be very costly, so we use a simple versioning strategy based on
// time and a crc32 hash of the serialized proto bytes.
//
// The descriptorVersion is used for retaining schema, signalling schema change and optimizing requests.
type descriptorVersion struct {
	versionTime     time.Time
	descriptorProto *descriptorpb.DescriptorProto
	hashVal         uint32
}

func newDescriptorVersion(in *descriptorpb.DescriptorProto) *descriptorVersion {
	var hashVal uint32
	// It is a known issue that we may have non-deterministic serialization of a DescriptorProto
	// due to the nature of protobuf.  Our primary protection is the time-based version identifier,
	// this hashing is primarily for time collisions.
	if b, err := proto.Marshal(in); err == nil {
		hashVal = crc32.ChecksumIEEE(b)
	}
	return &descriptorVersion{
		versionTime:     time.Now(),
		descriptorProto: proto.Clone(in).(*descriptorpb.DescriptorProto),
		hashVal:         hashVal,
	}
}

// eqVersion is the fast equality comparison that uses the versionTime and crc32 hash
// in place of deep proto equality.
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
	return true
}

// isNewer reports whether the current schema bears a newer time (version)
// than the other.
func (dv *descriptorVersion) isNewer(other *descriptorVersion) bool {
	return dv.versionTime.UnixNano() > other.versionTime.UnixNano()
}
