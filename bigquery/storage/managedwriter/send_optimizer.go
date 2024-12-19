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
	"bytes"
	"encoding/binary"
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

	// isMultiplexing tracks if we've actually sent writes to more than a single stream on this connection.
	isMultiplexing() bool
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

func (vo *verboseOptimizer) isMultiplexing() bool {
	// we declare this no to ensure we always reconnect on schema changes.
	return false
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

func (so *simplexOptimizer) isMultiplexing() bool {
	// A simplex optimizer is not designed for multiplexing.
	return false
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
	prevStream       string
	prevTemplate     *versionedTemplate
	multiplexStreams bool
}

func (mo *multiplexOptimizer) signalReset() {
	mo.prevStream = ""
	mo.multiplexStreams = false
	mo.prevTemplate = nil
}

func (mo *multiplexOptimizer) optimizeSend(arc storagepb.BigQueryWrite_AppendRowsClient, pw *pendingWrite) error {
	var err error
	if mo.prevStream == "" {
		// startup case, send a full request (with traceID).
		req := pw.constructFullRequest(true)
		err = arc.Send(req)
		if err == nil {
			mo.prevStream = req.GetWriteStream()
			mo.prevTemplate = pw.reqTmpl
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
			if mo.prevTemplate != nil {
				if !mo.prevTemplate.Compatible(pw.reqTmpl) {
					swapOnSuccess = true
					req = pw.constructFullRequest(false) // full request minus traceID.
				}
			}
			err = arc.Send(req)
			if err == nil && swapOnSuccess {
				mo.prevTemplate = pw.reqTmpl
			}
		} else {
			// The previous send was for a different stream.  Send a full request, minus traceId.
			req := pw.constructFullRequest(false)
			err = arc.Send(req)
			if err == nil {
				// Send successful.  Update state to reflect this send is now the "previous" state.
				mo.prevStream = pw.writeStreamID
				mo.prevTemplate = pw.reqTmpl
			}
			// Also, note that we've sent traffic for multiple streams, which means the backend recognizes this
			// is a multiplex stream as well.
			mo.multiplexStreams = true
		}
	}
	return err
}

func (mo *multiplexOptimizer) isMultiplexing() bool {
	return mo.multiplexStreams
}

// versionedTemplate is used for faster comparison of the templated part of
// an AppendRowsRequest, which bears settings-like fields related to schema
// and default value configuration.  Direct proto comparison through something
// like proto.Equal is far too expensive, so versionTemplate leverages a faster
// hash-based comparison to avoid the deep equality checks.
type versionedTemplate struct {
	versionTime time.Time
	hashVal     uint32
	tmpl        *storagepb.AppendRowsRequest
}

func newVersionedTemplate() *versionedTemplate {
	vt := &versionedTemplate{
		versionTime: time.Now(),
		tmpl:        &storagepb.AppendRowsRequest{},
	}
	vt.computeHash()
	return vt
}

// computeHash is an internal utility function for calculating the hash value
// for faster comparison.
func (vt *versionedTemplate) computeHash() {
	buf := new(bytes.Buffer)
	if b, err := proto.Marshal(vt.tmpl); err == nil {
		buf.Write(b)
	} else {
		// if we fail to serialize the proto (unlikely), consume the timestamp for input instead.
		binary.Write(buf, binary.LittleEndian, vt.versionTime.UnixNano())
	}
	vt.hashVal = crc32.ChecksumIEEE(buf.Bytes())
}

type templateRevisionF func(m *storagepb.AppendRowsRequest)

// revise makes a new versionedTemplate from the existing template, applying any changes.
// The original revision is returned if there's no effective difference after changes are
// applied.
func (vt *versionedTemplate) revise(changes ...templateRevisionF) *versionedTemplate {
	before := vt
	if before == nil {
		before = newVersionedTemplate()
	}
	if len(changes) == 0 {
		// if there's no changes, return the base revision immediately.
		return before
	}
	out := &versionedTemplate{
		versionTime: time.Now(),
		tmpl:        proto.Clone(before.tmpl).(*storagepb.AppendRowsRequest),
	}
	for _, r := range changes {
		r(out.tmpl)
	}
	out.computeHash()
	if out.Compatible(before) {
		// The changes didn't yield an measured difference.  Return the base revision to avoid
		// possible connection churn from no-op revisions.
		return before
	}
	return out
}

// Compatible is effectively a fast equality check, that relies on the hash value
// and avoids the potentially very costly deep comparison of the proto message templates.
func (vt *versionedTemplate) Compatible(other *versionedTemplate) bool {
	if other == nil {
		return vt == nil
	}
	return vt.hashVal == other.hashVal
}

func reviseProtoSchema(newSchema *descriptorpb.DescriptorProto) templateRevisionF {
	return func(m *storagepb.AppendRowsRequest) {
		if m != nil {
			m.Rows = &storagepb.AppendRowsRequest_ProtoRows{
				ProtoRows: &storagepb.AppendRowsRequest_ProtoData{
					WriterSchema: &storagepb.ProtoSchema{
						ProtoDescriptor: proto.Clone(newSchema).(*descriptorpb.DescriptorProto),
					},
				},
			}
		}
	}
}

func reviseMissingValueInterpretations(vi map[string]storagepb.AppendRowsRequest_MissingValueInterpretation) templateRevisionF {
	return func(m *storagepb.AppendRowsRequest) {
		if m != nil {
			m.MissingValueInterpretations = vi
		}
	}
}

func reviseDefaultMissingValueInterpretation(def storagepb.AppendRowsRequest_MissingValueInterpretation) templateRevisionF {
	return func(m *storagepb.AppendRowsRequest) {
		if m != nil {
			m.DefaultMissingValueInterpretation = def
		}
	}
}
