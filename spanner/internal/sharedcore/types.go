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
	"time"
	"unsafe"

	"google.golang.org/api/option"
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
	Enabled                           bool
	InitialChannels                   int
	MinChannels                       int
	MaxChannels                       int
	MaxRPCPerChannel                  float64
	MinRPCPerChannel                  float64
	ErrorPenaltyStep                  int32
	ErrorPenaltyDuration              time.Duration
	ScaleDownCheckInterval            time.Duration
	ScaleUpCooldown                   time.Duration
	DownscaleConsecutiveLowLoadChecks int
	MaxScaleUpPercent                 int
	MaxRemoveChannels                 int
	DrainIdleGrace                    time.Duration
	PrimeTimeout                      time.Duration
	PrimeMaxAttempts                  int
	SelectionStrategy                 int // 0 = DCPPowerOfTwoLeastBusy, 1 = DCPRoundRobin
}

// ClientConfig configures a shared core gRPC client transport and connection pool.
type ClientConfig struct {
	// Endpoint overrides the default Spanner gRPC endpoint (e.g. "localhost:9010" or "spanner.googleapis.com:443").
	Endpoint string
	// UniverseDomain overrides the default universe domain when dialing a non-plaintext endpoint.
	UniverseDomain string
	// NumChannels sets the static pool size when > 0 (must match option.WithGRPCConnectionPool if both are set).
	// When <= 0, transport.DefaultNumChannels (4) is used unless overridden via ClientOptions.
	NumChannels int
	// Compression enables request/response compression when set to "gzip".
	Compression string
	// EnableDirectAccess enables DirectPath when allowed by credentials and environment variables.
	EnableDirectAccess bool
	// UserAgent sets an optional product/version token appended to x-goog-api-client.
	UserAgent string
	// PlainText dials without TLS or authentication.
	PlainText bool
	// DisableBuiltInAuth omits default Google credentials (for host bindings supplying per-RPC auth headers).
	DisableBuiltInAuth bool
	// DynamicChannelPoolConfig configures opt-in dynamic gRPC channel pooling.
	DynamicChannelPoolConfig DynamicChannelPoolConfig
	// LogFunc is an optional callback for transport log messages. Must be safe for concurrent calls.
	LogFunc func(level string, message string)
	// ClientOptions appends caller-provided google.golang.org/api/option options (higher precedence than defaults).
	// Do not combine option.WithGRPCConn or gtransport.WithConnPool with DynamicChannelPoolConfig.Enabled.
	ClientOptions []option.ClientOption
}

// RetrySettings configures total stream timeout and retry backoff parameters in milliseconds
// for ExecuteStreamingSql.
//
// Timeout modes controlled by (HasTimeout, TimeoutMs):
//   - HasTimeout == false: uses the caller context deadline if present; otherwise applies
//     the 60-minute GAPIC default streaming deadline.
//   - HasTimeout == true && TimeoutMs == 0: explicitly disables any default deadline
//     (context.WithCancel with no timeout added, e.g. when a host caller passes timeout: Infinity/None).
//   - HasTimeout == true && TimeoutMs > 0: applies an explicit total deadline of TimeoutMs milliseconds.
//   - HasTimeout == true && TimeoutMs < 0: fails immediately with codes.DeadlineExceeded.
//
// Per-attempt timeout settings and custom retryable error codes are intentionally omitted
// for ExecuteStreamingSql because a single stream deadline spans all resume_token resumption
// attempts (matching spanner.resumableStreamDecoder), and retryable status codes follow
// Spanner's stream-resumption rules (codes.Unavailable, known transient codes.Internal
// transport/RST_STREAM errors, and codes.ResourceExhausted when accompanied by RetryInfo
// or location-aware routing).
type RetrySettings struct {
	// TimeoutMs is the total stream deadline in milliseconds when HasTimeout is true
	// (0 with HasTimeout == true disables the default deadline; < 0 fails with DeadlineExceeded).
	TimeoutMs int64
	// HasTimeout indicates whether TimeoutMs was explicitly specified by the caller.
	HasTimeout bool
	// UntimedRetryBudgetMs optionally bounds consecutive retry elapsed time in milliseconds
	// when HasTimeout is false. When <= 0 (the default), no retry-budget cap is applied
	// and retryable interruptions continue until the context deadline.
	UntimedRetryBudgetMs int64
	// InitialRetryDelayMs is the initial retry backoff delay in milliseconds
	// (defaults to 20 ms from spanner.DefaultRetryBackoff when <= 0).
	InitialRetryDelayMs int64
	// MaxRetryDelayMs is the maximum retry backoff delay in milliseconds
	// (defaults to 32,000 ms from spanner.DefaultRetryBackoff when <= 0).
	MaxRetryDelayMs int64
	// RetryDelayMultiplier is the exponential backoff multiplier
	// (defaults to 1.3 from spanner.DefaultRetryBackoff when <= 0).
	RetryDelayMultiplier float64
}

// StreamBatchConfig configures row batching, chunk checkpointing, and arena buffer pooling.
type StreamBatchConfig struct {
	// BatchRowSize is the target number of rows per StreamBatch callback (defaults to 128 when <= 0).
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
