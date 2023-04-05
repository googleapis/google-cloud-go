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
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// encapsulates custom client-level config settings.
type writerClientConfig struct {
	useMultiplex                 bool
	maxMultiplexPoolSize         int
	defaultInflightRequests      int
	defaultInflightBytes         int
	defaultAppendRowsCallOptions []gax.CallOption
}

// newWriterClientConfig builds a client config based on package-specific custom ClientOptions.
func newWriterClientConfig(opts ...option.ClientOption) *writerClientConfig {
	conf := &writerClientConfig{}
	for _, opt := range opts {
		if wOpt, ok := opt.(writerClientOption); ok {
			wOpt.ApplyWriterOpt(conf)
		}
	}
	return conf
}

// writerClientOption allows us to extend ClientOptions for client-specific needs.
type writerClientOption interface {
	option.ClientOption
	ApplyWriterOpt(*writerClientConfig)
}

// enableMultiplex enables multiplex behavior in the client.
// maxSize indicates the maximum number of shared multiplex connections
// in a given location/region
//
// TODO: export this as part of the multiplex feature launch.
func enableMultiplex(enable bool, maxSize int) option.ClientOption {
	return &enableMultiplexSetting{useMultiplex: enable, maxSize: maxSize}
}

type enableMultiplexSetting struct {
	internaloption.EmbeddableAdapter
	useMultiplex bool
	maxSize      int
}

func (s *enableMultiplexSetting) ApplyWriterOpt(c *writerClientConfig) {
	c.useMultiplex = s.useMultiplex
	c.maxMultiplexPoolSize = s.maxSize
}

// defaultMaxInflightRequests sets the default flow controller limit for requests for
// all AppendRows connections created by this client.
//
// TODO: export this as part of the multiplex feature launch.
func defaultMaxInflightRequests(n int) option.ClientOption {
	return &defaultInflightRequestsSetting{maxRequests: n}
}

type defaultInflightRequestsSetting struct {
	internaloption.EmbeddableAdapter
	maxRequests int
}

func (s *defaultInflightRequestsSetting) ApplyWriterOpt(c *writerClientConfig) {
	c.defaultInflightRequests = s.maxRequests
}

// defaultMaxInflightBytes sets the default flow controller limit for bytes for
// all AppendRows connections created by this client.
//
// TODO: export this as part of the multiplex feature launch.
func defaultMaxInflightBytes(n int) option.ClientOption {
	return &defaultInflightBytesSetting{maxBytes: n}
}

type defaultInflightBytesSetting struct {
	internaloption.EmbeddableAdapter
	maxBytes int
}

func (s *defaultInflightBytesSetting) ApplyWriterOpt(c *writerClientConfig) {
	c.defaultInflightBytes = s.maxBytes
}

// defaultAppendRowsCallOptions sets a gax.CallOption passed when opening
// the AppendRows bidi connection.
//
// TODO: export this as part of the multiplex feature launch.
func defaultAppendRowsCallOption(o gax.CallOption) option.ClientOption {
	return &defaultAppendRowsCallOptionSetting{opt: o}
}

type defaultAppendRowsCallOptionSetting struct {
	internaloption.EmbeddableAdapter
	opt gax.CallOption
}

func (s *defaultAppendRowsCallOptionSetting) ApplyWriterOpt(c *writerClientConfig) {
	c.defaultAppendRowsCallOptions = append(c.defaultAppendRowsCallOptions, s.opt)
}

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
//	projects/{projectid}/datasets/{dataset}/tables/{table}
func WithDestinationTable(destTable string) WriterOption {
	return func(ms *ManagedStream) {
		ms.streamSettings.destinationTable = destTable
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
		ms.curDescVersion = newDescriptorVersion(dp)
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
		ms.streamSettings.appendCallOptions = append(ms.streamSettings.appendCallOptions, o)
	}
}

// EnableWriteRetries enables ManagedStream to automatically retry failed appends.
//
// Enabling retries is best suited for cases where users want to achieve at-least-once
// append semantics.  Use of automatic retries may complicate patterns where the user
// is designing for exactly-once append semantics.
func EnableWriteRetries(enable bool) WriterOption {
	return func(ms *ManagedStream) {
		if enable {
			ms.retry = newStatelessRetryer()
		}
	}
}

// AppendOption are options that can be passed when appending data with a managed stream instance.
type AppendOption func(*pendingWrite)

// UpdateSchemaDescriptor is used to update the descriptor message schema associated
// with a given stream.
func UpdateSchemaDescriptor(schema *descriptorpb.DescriptorProto) AppendOption {
	return func(pw *pendingWrite) {
		// create a new descriptorVersion and attach it to the pending write.
		pw.descVersion = newDescriptorVersion(schema)
	}
}

// WithOffset sets an explicit offset value for this append request.
func WithOffset(offset int64) AppendOption {
	return func(pw *pendingWrite) {
		pw.req.Offset = &wrapperspb.Int64Value{
			Value: offset,
		}
	}
}
