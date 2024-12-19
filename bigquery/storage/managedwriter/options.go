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
	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
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

	// Normalize the config to ensure we're dealing with sane values.
	if conf.useMultiplex {
		if conf.maxMultiplexPoolSize < 1 {
			conf.maxMultiplexPoolSize = 1
		}
	}
	if conf.defaultInflightBytes < 0 {
		conf.defaultInflightBytes = 0
	}
	if conf.defaultInflightRequests < 0 {
		conf.defaultInflightRequests = 0
	}
	return conf
}

// writerClientOption allows us to extend ClientOptions for client-specific needs.
type writerClientOption interface {
	option.ClientOption
	ApplyWriterOpt(*writerClientConfig)
}

// WithMultiplexing is an EXPERIMENTAL option that controls connection sharing
// when instantiating the Client.  Only writes to default streams can leverage the
// multiplex pool.  Internally, the client maintains a pool of connections per BigQuery
// destination region, and will grow the pool to it's maximum allowed size if there's
// sufficient traffic on the shared connection(s).
//
// This ClientOption is EXPERIMENTAL and subject to change.
func WithMultiplexing() option.ClientOption {
	return &enableMultiplexSetting{useMultiplex: true}
}

type enableMultiplexSetting struct {
	internaloption.EmbeddableAdapter
	useMultiplex bool
}

func (s *enableMultiplexSetting) ApplyWriterOpt(c *writerClientConfig) {
	c.useMultiplex = s.useMultiplex
}

// WithMultiplexPoolLimit is an EXPERIMENTAL option that sets the maximum
// shared multiplex pool size when instantiating the Client.  If multiplexing
// is not enabled, this setting is ignored.  By default, the limit is a single
// shared connection.  This limit is applied per destination region.
//
// This ClientOption is EXPERIMENTAL and subject to change.
func WithMultiplexPoolLimit(maxSize int) option.ClientOption {
	return &maxMultiplexPoolSizeSetting{maxSize: maxSize}
}

type maxMultiplexPoolSizeSetting struct {
	internaloption.EmbeddableAdapter
	maxSize int
}

func (s *maxMultiplexPoolSizeSetting) ApplyWriterOpt(c *writerClientConfig) {
	c.maxMultiplexPoolSize = s.maxSize
}

// WithDefaultInflightRequests is an EXPERIMENTAL ClientOption for controlling
// the default limit of how many individual AppendRows write requests can
// be in flight on a connection at a time.  This limit is enforced on all connections
// created by the instantiated Client.
//
// Note: the WithMaxInflightRequests WriterOption can still be used to control
// the behavior for individual ManagedStream writers when not using multiplexing.
//
// This ClientOption is EXPERIMENTAL and subject to change.
func WithDefaultInflightRequests(n int) option.ClientOption {
	return &defaultInflightRequestsSetting{maxRequests: n}
}

type defaultInflightRequestsSetting struct {
	internaloption.EmbeddableAdapter
	maxRequests int
}

func (s *defaultInflightRequestsSetting) ApplyWriterOpt(c *writerClientConfig) {
	c.defaultInflightRequests = s.maxRequests
}

// WithDefaultInflightBytes is an EXPERIMENTAL ClientOption for controlling
// the default byte limit for how many individual AppendRows write requests can
// be in flight on a connection at a time.  This limit is enforced on all connections
// created by the instantiated Client.
//
// Note: the WithMaxInflightBytes WriterOption can still be used to control
// the behavior for individual ManagedStream writers when not using multiplexing.
//
// This ClientOption is EXPERIMENTAL and subject to change.
func WithDefaultInflightBytes(n int) option.ClientOption {
	return &defaultInflightBytesSetting{maxBytes: n}
}

type defaultInflightBytesSetting struct {
	internaloption.EmbeddableAdapter
	maxBytes int
}

func (s *defaultInflightBytesSetting) ApplyWriterOpt(c *writerClientConfig) {
	c.defaultInflightBytes = s.maxBytes
}

// WithDefaultAppendRowsCallOption is an EXPERIMENTAL ClientOption for controlling
// the gax.CallOptions passed when opening the underlying AppendRows bidi
// stream connections used by this library to communicate with the BigQuery
// Storage service.  This option is propagated to all
// connections created by the instantiated Client.
//
// Note: the WithAppendRowsCallOption WriterOption can still be used to control
// the behavior for individual ManagedStream writers that don't participate
// in multiplexing.
//
// This ClientOption is EXPERIMENTAL and subject to change.
func WithDefaultAppendRowsCallOption(o gax.CallOption) option.ClientOption {
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
//
// Note: See the WithDefaultInflightRequests ClientOption for setting a default
// when instantiating a client, rather than setting this limit per-writer.
// This WriterOption is ignored for ManagedStreams that participate in multiplexing.
func WithMaxInflightRequests(n int) WriterOption {
	return func(ms *ManagedStream) {
		ms.streamSettings.MaxInflightRequests = n
	}
}

// WithMaxInflightBytes bounds the inflight append request bytes on the write connection.
//
// Note: See the WithDefaultInflightBytes ClientOption for setting a default
// when instantiating a client, rather than setting this limit per-writer.
// This WriterOption is ignored for ManagedStreams that participate in multiplexing.
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
		ms.curTemplate = ms.curTemplate.revise(reviseProtoSchema(dp))
	}
}

// WithMissingValueInterpretations controls how missing values are interpreted
// for individual columns.
//
// You must provide a map to indicate how to interpret missing value for some fields. Missing
// values are fields present in user schema but missing in rows. The key is
// the field name. The value is the interpretation of missing values for the
// field.
//
// For example, the following option would indicate that missing values in the "foo"
// column are interpreted as null, whereas missing values in the "bar" column are
// treated as the default value:
//
//	   WithMissingValueInterpretations(map[string]storagepb.AppendRowsRequest_MissingValueInterpretation{
//					"foo": storagepb.AppendRowsRequest_DEFAULT_VALUE,
//					"bar": storagepb.AppendRowsRequest_NULL_VALUE,
//		  })
//
// If a field is not in this map and has missing values, the missing values
// in this field are interpreted as NULL unless overridden with a default missing
// value interpretation.
//
// Currently, field name can only be top-level column name, can't be a struct
// field path like 'foo.bar'.
func WithMissingValueInterpretations(mvi map[string]storagepb.AppendRowsRequest_MissingValueInterpretation) WriterOption {
	return func(ms *ManagedStream) {
		ms.curTemplate = ms.curTemplate.revise(reviseMissingValueInterpretations(mvi))
	}
}

// WithDefaultMissingValueInterpretation controls how missing values are interpreted by
// for a given stream.  See WithMissingValueIntepretations for more information about
// missing values.
//
// WithMissingValueIntepretations set for individual colums can override the default chosen
// with this option.
//
// For example, if you want to write
// `NULL` instead of using default values for some columns, you can set
// `default_missing_value_interpretation` to `DEFAULT_VALUE` and at the same
// time, set `missing_value_interpretations` to `NULL_VALUE` on those columns.
func WithDefaultMissingValueInterpretation(def storagepb.AppendRowsRequest_MissingValueInterpretation) WriterOption {
	return func(ms *ManagedStream) {
		ms.curTemplate = ms.curTemplate.revise(reviseDefaultMissingValueInterpretation(def))
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
//
// Note: See the DefaultAppendRowsCallOption ClientOption for setting defaults
// when instantiating a client, rather than setting this limit per-writer.  This WriterOption
// is ignored for ManagedStream writers that participate in multiplexing.
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
		pw.reqTmpl = pw.reqTmpl.revise(reviseProtoSchema(schema))
	}
}

// UpdateMissingValueInterpretations updates the per-column missing-value intepretations settings,
// and is retained for subsequent writes.  See the WithMissingValueInterpretations WriterOption for
// more details.
func UpdateMissingValueInterpretations(mvi map[string]storagepb.AppendRowsRequest_MissingValueInterpretation) AppendOption {
	return func(pw *pendingWrite) {
		pw.reqTmpl = pw.reqTmpl.revise(reviseMissingValueInterpretations(mvi))
	}
}

// UpdateDefaultMissingValueInterpretation updates the default intepretations setting for the stream,
// and is retained for subsequent writes.  See the WithDefaultMissingValueInterpretations WriterOption for
// more details.
func UpdateDefaultMissingValueInterpretation(def storagepb.AppendRowsRequest_MissingValueInterpretation) AppendOption {
	return func(pw *pendingWrite) {
		pw.reqTmpl = pw.reqTmpl.revise(reviseDefaultMissingValueInterpretation(def))
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
