/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Internal Use Only: Package sharedcore provides the internal Go interface layer between
// host language bindings (such as Node.js cgo/N-API) and the Cloud Spanner Go
// client library's transport and resumable stream decoding infrastructure.
// This package is intended only for Google Cloud client libraries and is not a supported public API.
package sharedcore

import "google.golang.org/grpc/metadata"

// Internal Use Only: ValueKind identifies the native representation of a decoded Spanner cell value
// for zero-parse transfer across the FFI boundary.
type ValueKind uint8

const (
	// Internal Use Only: KindNull represents a SQL NULL value.
	KindNull ValueKind = 0
	// Internal Use Only: KindBool represents a BOOL value.
	KindBool ValueKind = 1
	// Internal Use Only: KindFloat64 represents a FLOAT64 value (including NaN, +Inf, -Inf).
	KindFloat64 ValueKind = 2
	// Internal Use Only: KindString represents a UTF-8 string value (STRING, INT64, NUMERIC, DATE, JSON, INTERVAL, UUID, ENUM).
	KindString ValueKind = 3
	// Internal Use Only: KindBytes represents a pre-decoded binary value (BYTES, PROTO).
	KindBytes ValueKind = 4
	// Internal Use Only: KindTimestamp represents a pre-parsed TIMESTAMP value as Unix seconds and nanoseconds.
	KindTimestamp ValueKind = 5
	// Internal Use Only: KindFloat32 represents a FLOAT32 value (including NaN, +Inf, -Inf).
	KindFloat32 ValueKind = 6
	// Internal Use Only: KindArray represents a recursive ARRAY of SpannerValue elements.
	KindArray ValueKind = 7
	// Internal Use Only: KindStruct represents a recursive STRUCT of SpannerValue fields.
	KindStruct ValueKind = 8
)

// Internal Use Only: SpannerValue holds a single decoded Spanner cell value ready to be packed
// into a contiguous C memory arena for FFI transfer.
type SpannerValue struct {
	Kind       ValueKind
	BoolVal    bool
	Float64Val float64
	StrVal     string
	BytesVal   []byte
	SecVal     int64
	NanoVal    int32
	Children   []SpannerValue
}

// Internal Use Only: DynamicChannelPoolConfig configures opt-in dynamic gRPC channel pooling,
// mirroring spanner.DynamicChannelPoolConfig. Zero values use DefaultDynamicChannelPoolConfig
// when Enabled is true.
type DynamicChannelPoolConfig struct {
	Enabled           bool
	InitialChannels   int
	MinChannels       int
	MaxChannels       int
	MaxRPCPerChannel  float64
	MinRPCPerChannel  float64
	SelectionStrategy int // 0 = DCPPowerOfTwoLeastBusy, 1 = DCPRoundRobin
}

// Internal Use Only: ClientConfig configures a SharedCoreService client pool.
type ClientConfig struct {
	Endpoint                 string
	UniverseDomain           string
	RootCAPEM                string
	NumChannels              int
	Compression              string
	EnableDirectAccess       bool
	UserAgent                string
	PlainText                bool
	DisableBuiltInAuth       bool
	DynamicChannelPoolConfig DynamicChannelPoolConfig
	LogFunc                  func(level string, message string)
}

// Internal Use Only: StreamRequest encapsulates a single ExecuteStreamingSql invocation from a host language binding.
type StreamRequest struct {
	RoutingKey              string
	RequestID               string
	Metadata                map[string]string
	MetadataPairs           []string
	OutgoingMD              metadata.MD
	SkipTraceContext        bool
	RequestBytes            []byte
	SkipMetadata            bool
	ExpectedSchemaHash      uint32
	SkipTelemetry           bool
	SkipReadTimestamp       bool
	FlushOnChunkBoundary    bool
	MaxUncheckpointedChunks int
	ReuseArenaBuffers       bool
	TimeoutMs               int64
	HasTimeout              bool
	UntimedRetryBudgetMs    int64
	InitialRetryDelayMs     int64
	MaxRetryDelayMs         int64
	RetryDelayMultiplier    float64
	BatchRowSize            int
}

// Internal Use Only: PackedCell is a 24-byte, 8-byte-aligned, pointer-free value descriptor
// matching the exact C ABI layout of CSpannerCell. Because it contains no Go
// pointers, []PackedCell is allocated in a noscan span (zero GC mark overhead)
// and can be copied across the cgo boundary with a single memcpy.
type PackedCell struct {
	Kind    int32
	Flags   uint32
	NumBits uint64
	Offset  uint32
	Len     uint32
}

// Internal Use Only: StreamBatch represents a batch of decoded rows and associated stream metadata
// delivered to the host language binding callback.
type StreamBatch struct {
	Rows                [][]SpannerValue
	Cells               []PackedCell
	ArenaBytes          []byte
	NumRows             int
	NumCols             int
	SchemaHash          uint32
	MetadataBytes       []byte
	StatsBytes          []byte
	PrecommitTokenBytes []byte
	HasReadTimestamp    bool
	ReadTimestampSec    int64
	ReadTimestampNanos  int32
	Telemetry           StreamTelemetry
	IsLast              bool
}

// Internal Use Only: StreamTelemetry holds server-timing and retry attempt metrics collected during the stream.
type StreamTelemetry struct {
	ServerTiming   string
	GFELatencyMs   float64
	AFELatencyMs   float64
	HasGFELatency  bool
	HasAFELatency  bool
	AttemptCount   int64
	DirectPathUsed bool
	RequestID      string
	ErrorTrailers  map[string][]string
	AttemptsJSON   []byte
}
