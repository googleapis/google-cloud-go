// Copyright 2021 Google LLC
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
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/protobuf/types/descriptorpb"
)

// WriterOption are variadic options used to configure a ManagedStream instance.
type WriterOption func(*ManagedStream)

// WithType sets the stream type for the managed stream.
func WithType(st StreamType) WriterOption {
	return func(ms *ManagedStream) {
		ms.streamSettings.streamType = st
	}
}

// WithStreamName allows users to set the stream name this writer will
// append to explicitly.  By default, the managed client will create the
// stream when instantiated if necessary.
//
// Note:  Supplying this option causes other options which affect stream construction
// such as WithStreamType and WithDestinationTable to be ignored.
func WithStreamName(name string) WriterOption {
	return func(ms *ManagedStream) {
		ms.streamSettings.streamID = name
	}
}

// WithDestinationTable specifies the destination table to which a created
// stream will append rows.  Format of the table:
//
//   projects/{projectid}/datasets/{dataset}/tables/{table}
func WithDestinationTable(destTable string) WriterOption {
	return func(ms *ManagedStream) {
		ms.destinationTable = destTable
	}
}

// WithMaxInflightRequests bounds the inflight appends on the write connection.
func WithMaxInflightRequests(n int) WriterOption {
	return func(ms *ManagedStream) {
		ms.streamSettings.MaxInflightRequests = n
	}
}

// WithMaxInflightBytes bounds the inflight append request bytes on the write connection.
func WithMaxInflightBytes(n int) WriterOption {
	return func(ms *ManagedStream) {
		ms.streamSettings.MaxInflightBytes = n
	}
}

// WithTraceID allows instruments requests to the service with a custom trace prefix.
// This is generally for diagnostic purposes only.
func WithTraceID(traceID string) WriterOption {
	return func(ms *ManagedStream) {
		ms.streamSettings.TraceID = traceID
	}
}

// WithSchemaDescriptor describes the format of the serialized data being sent by
// AppendRows calls on the stream.
func WithSchemaDescriptor(dp *descriptorpb.DescriptorProto) WriterOption {
	return func(ms *ManagedStream) {
		ms.schemaDescriptor = dp
	}
}

// WithDataOrigin is used to attach an origin context to the instrumentation metrics
// emitted by the library.
func WithDataOrigin(dataOrigin string) WriterOption {
	return func(ms *ManagedStream) {
		ms.streamSettings.dataOrigin = dataOrigin
	}
}

// WithAppendRowsCallOption is used to supply additional call options to the ManagedStream when
// it opens the underlying append stream.
func WithAppendRowsCallOption(o gax.CallOption) WriterOption {
	return func(ms *ManagedStream) {
		ms.callOptions = append(ms.callOptions, o)
	}
}
