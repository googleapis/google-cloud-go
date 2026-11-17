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

package sharedcore

import (
	"unsafe"

	"google.golang.org/grpc/metadata"
)

// ValueKind identifies the native representation of a decoded Spanner cell value
// for zero-parse transfer across the FFI boundary.
type ValueKind uint8

const (
	// KindNull represents a SQL NULL value.
	KindNull ValueKind = 0
	// KindBool represents a BOOL value.
	KindBool ValueKind = 1
	// KindFloat64 represents a FLOAT64 value (including NaN, +Inf, -Inf).
	KindFloat64 ValueKind = 2
	// KindString represents a UTF-8 string value (STRING, INT64, NUMERIC, DATE, JSON, INTERVAL, UUID, ENUM).
	// INT64 and ENUM are transmitted as decimal strings on the Spanner protobuf wire and kept as
	// KindString so host languages whose default number type is IEEE 754 double (such as JavaScript)
	// can wrap or parse them without 53-bit integer precision loss or extra int64-to-string formatting.
	KindString ValueKind = 3
	// KindBytes represents a pre-decoded binary value (BYTES, PROTO).
	KindBytes ValueKind = 4
	// KindTimestamp represents a pre-parsed TIMESTAMP value as Unix seconds (Int64Val) and nanoseconds (Int32Val).
	KindTimestamp ValueKind = 5
	// KindFloat32 represents a FLOAT32 value (including NaN, +Inf, -Inf).
	KindFloat32 ValueKind = 6
	// KindArray represents a recursive ARRAY of SpannerValue elements.
	KindArray ValueKind = 7
	// KindStruct represents a recursive STRUCT of SpannerValue fields.
	KindStruct ValueKind = 8
)

// SpannerValue holds a single decoded Spanner cell value ready to be packed
// into a contiguous C memory arena for FFI transfer.
// For KindTimestamp, Int64Val holds Unix seconds and Int32Val holds nanoseconds (0..999,999,999).
type SpannerValue struct {
	Kind       ValueKind
	BoolVal    bool
	Float64Val float64
	StrVal     string
	BytesVal   []byte
	Int64Val   int64
	Int32Val   int32
	Children   []SpannerValue
}

// DynamicChannelPoolConfig configures opt-in dynamic gRPC channel pooling,
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

// ClientConfig configures a Service client pool.
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
	// LogFunc is an optional callback for transport log messages. Must be safe for concurrent calls.
	LogFunc func(level string, message string)
}

// RetrySettings configures per-RPC timeout and retry backoff parameters in milliseconds.
type RetrySettings struct {
	// TimeoutMs is the per-RPC deadline in milliseconds when HasTimeout is true.
	TimeoutMs int64
	// HasTimeout indicates whether TimeoutMs should be applied as a deadline.
	HasTimeout bool
	// UntimedRetryBudgetMs bounds total retry elapsed time in milliseconds when HasTimeout is false (defaults to 60,000 ms when <= 0).
	UntimedRetryBudgetMs int64
	// InitialRetryDelayMs is the initial retry backoff delay in milliseconds (defaults to 200 ms when <= 0).
	InitialRetryDelayMs int64
	// MaxRetryDelayMs is the maximum retry backoff delay in milliseconds (defaults to 32,000 ms when <= 0).
	MaxRetryDelayMs int64
	// RetryDelayMultiplier is the exponential backoff multiplier (defaults to 1.3 when <= 0).
	RetryDelayMultiplier float64
}

// StreamBatchConfig configures row batching, chunk checkpointing, and arena buffer pooling.
type StreamBatchConfig struct {
	// BatchRowSize is the target number of rows per StreamBatch callback (defaults to 64 when <= 0).
	BatchRowSize int
	// FlushOnChunkBoundary flushes accumulated complete rows at the end of each PartialResultSet chunk.
	FlushOnChunkBoundary bool
	// MaxUncheckpointedChunks bounds how many chunks without a resume_token can be buffered before pausing row emission.
	MaxUncheckpointedChunks int
	// ReuseArenaBuffers pools and reuses backing slices for StreamBatch.Cells and
	// StreamBatch.ArenaBytes across batches; when true, callers must consume or copy
	// Cells and ArenaBytes before the onBatch callback returns.
	ReuseArenaBuffers bool
}

// StreamOptions configures optional metadata, telemetry, and trace-context optimizations for a stream.
type StreamOptions struct {
	// SkipMetadata omits serialized ResultSetMetadataBytes when the schema hash matches ExpectedSchemaHash.
	SkipMetadata bool
	// ExpectedSchemaHash is the caller's cached FNV-1a 32-bit schema hash used when SkipMetadata is true.
	ExpectedSchemaHash uint32
	// SkipTelemetry omits per-attempt telemetry formatting when built-in metrics are disabled.
	SkipTelemetry bool
	// SkipReadTimestamp skips parsing read_timestamp from TransactionMetadata when not requested.
	SkipReadTimestamp bool
	// SkipTraceContext skips OpenTelemetry span context extraction when end-to-end tracing is disabled.
	SkipTraceContext bool
}

// StreamRequest encapsulates a single ExecuteStreamingSql invocation from a host language binding.
type StreamRequest struct {
	RoutingKey string
	RequestID  string
	// Metadata supplies per-request outgoing gRPC metadata headers as a string map.
	// When OutgoingMD is non-empty, OutgoingMD takes precedence over Metadata on the hot path.
	Metadata map[string]string
	// OutgoingMD optionally supplies pre-parsed, pre-lowercased gRPC metadata.MD that callers
	// can cache and reuse across requests sharing the same static headers.
	OutgoingMD   metadata.MD
	RequestBytes []byte
	Retry        RetrySettings
	Batch        StreamBatchConfig
	Options      StreamOptions
}

// PackedCell is a 24-byte, 8-byte-aligned, pointer-free value descriptor
// matching the exact C ABI layout of CSpannerCell. Because it contains no Go
// pointers, []PackedCell is allocated in a noscan span (zero GC mark overhead)
// and can be copied across the cgo boundary with a single memcpy.
//
// Per-kind field semantics:
//   - KindNull:      all fields zeroed.
//   - KindBool:      Flags = 1 (true) or 0 (false).
//   - KindFloat64:   NumBits = math.Float64bits(v) (matches CSpannerCell.num.f64_val).
//   - KindFloat32:   NumBits = math.Float64bits(v) (stored as IEEE 754 float64 bits to match CSpannerCell.num.f64_val).
//   - KindString:    Offset, Len = UTF-8 byte range in ArenaBytes.
//   - KindBytes:     Offset, Len = decoded binary byte range in ArenaBytes.
//   - KindTimestamp: NumBits = uint64(unixSeconds) (matches CSpannerCell.num.i64_val), Flags = uint32(nanos) (0..999,999,999).
//   - KindArray:     Offset = start index in Cells, Len = number of contiguous child cells.
//   - KindStruct:    Offset = start index in Cells, Len = number of contiguous child field cells.
type PackedCell struct {
	Kind    int32
	Flags   uint32
	NumBits uint64
	Offset  uint32
	Len     uint32
}

// Compile-time assertions that PackedCell is exactly 24 bytes and its field offsets match CSpannerCell across FFI memcpy.
var (
	_ [24 - unsafe.Sizeof(PackedCell{})]byte
	_ [unsafe.Sizeof(PackedCell{}) - 24]byte
	_ [0 - unsafe.Offsetof(PackedCell{}.Kind)]byte
	_ [4 - unsafe.Offsetof(PackedCell{}.Flags)]byte
	_ [unsafe.Offsetof(PackedCell{}.Flags) - 4]byte
	_ [8 - unsafe.Offsetof(PackedCell{}.NumBits)]byte
	_ [unsafe.Offsetof(PackedCell{}.NumBits) - 8]byte
	_ [16 - unsafe.Offsetof(PackedCell{}.Offset)]byte
	_ [unsafe.Offsetof(PackedCell{}.Offset) - 16]byte
	_ [20 - unsafe.Offsetof(PackedCell{}.Len)]byte
	_ [unsafe.Offsetof(PackedCell{}.Len) - 20]byte
)

// StreamBatch represents a batch of decoded rows and associated stream metadata
// delivered to the host language binding callback.
// When StreamBatchConfig.ReuseArenaBuffers is true, Cells and ArenaBytes alias pooled
// scratch buffers and are only valid until the onBatch callback returns.
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

// StreamTelemetry holds server-timing and retry attempt metrics collected during the stream.
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
