// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package transport provides internal gRPC connection pool, GAPIC client,
// DirectPath/fallback, and Dynamic Channel Pool primitives shared across Spanner packages.
package transport

import (
	"context"
	"fmt"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/grpc-gcp-go/grpcgcp"
	"github.com/googleapis/gax-go/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	vkit "cloud.google.com/go/spanner/apiv1"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal"
	"cloud.google.com/go/spanner/internal/requestid"
)

const (
	// ResourcePrefixHeader is the gRPC metadata key for the database or instance resource name.
	ResourcePrefixHeader = "google-cloud-resource-prefix"
	// RequestsCompressionHeader is the gRPC metadata key requesting compressed responses.
	RequestsCompressionHeader = "x-response-encoding"
	// RouteToLeaderHeader is the gRPC metadata key enabling leader-aware routing.
	RouteToLeaderHeader = "x-goog-spanner-route-to-leader"
	// EndToEndTracingHeader is the gRPC metadata key enabling server-side end-to-end tracing.
	EndToEndTracingHeader = "x-goog-spanner-end-to-end-tracing"
	// AFEMetricHeader is the gRPC metadata key enabling AFE server-timing headers.
	AFEMetricHeader = "x-goog-spanner-enable-afe-server-timing"
	// ServerTimingHeaderKey is the response metadata key carrying GFE/AFE latencies.
	ServerTimingHeaderKey = "server-timing"
	// GFETimingHeader is the server-timing metric name for Google Frontend latency.
	GFETimingHeader = "gfet4t7"
	// AFETimingHeader is the server-timing metric name for Spanner AFE latency.
	AFETimingHeader = "afe"
	// DefaultNumChannels is the default number of gRPC channels in a static connection pool.
	DefaultNumChannels = 4
)

// Logf logs the formatted message to logger, or to the standard logger if logger is nil.
func Logf(logger *log.Logger, format string, v ...any) {
	if logger == nil {
		log.Printf(format, v...)
	} else {
		logger.Printf(format, v...)
	}
}

// StripEndpointScheme removes leading passthrough:///, https://, and http:// schemes
// (including chained passthrough:///http:// prefixes) from an endpoint string.
func StripEndpointScheme(endpoint string) string {
	s := strings.TrimPrefix(endpoint, "passthrough:///")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	return strings.TrimPrefix(s, "passthrough:///")
}

// EmulatorClientOptions returns the ClientOptions needed to connect to a Spanner
// emulator or plaintext endpoint.
func EmulatorClientOptions(emulatorAddr string) []option.ClientOption {
	schemeRemoved := StripEndpointScheme(emulatorAddr)
	opts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
		internaloption.SkipDialSettingsValidation(),
	}
	if schemeRemoved != "" {
		opts = append([]option.ClientOption{option.WithEndpoint("passthrough:///" + schemeRemoved)}, opts...)
	}
	return opts
}

// ResolveDirectPathEnabled resolves whether DirectPath is enabled by checking the
// GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS environment variable (which overrides configEnableDirectAccess).
func ResolveDirectPathEnabled(configEnableDirectAccess bool) bool {
	if envVal := os.Getenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS"); envVal != "" {
		enableDirectPath, _ := strconv.ParseBool(envVal)
		return enableDirectPath
	}
	return configEnableDirectAccess
}

// ResolveGCPFallbackEnabled resolves whether DirectPath-to-CloudPath fallback is enabled
// via GOOGLE_SPANNER_ENABLE_GCP_FALLBACK.
func ResolveGCPFallbackEnabled() bool {
	if envVal := os.Getenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK"); envVal != "" {
		enabled, err := strconv.ParseBool(envVal)
		if err == nil {
			return enabled
		}
	}
	return false
}

// DefaultClientOptions combines the generated GAPIC default options, Spanner client
// default options (connection pool size, user-agent, interceptors, keepalive, DirectPath,
// and gzip compressor), and userOpts in precedence order: userOpts > clientDefaultOpts > generatedDefaultOpts.
func DefaultClientOptions(
	numChannels int,
	compression string,
	enableDirectAccess bool,
	unaryInterceptors []grpc.UnaryClientInterceptor,
	streamInterceptors []grpc.StreamClientInterceptor,
	userOpts ...option.ClientOption,
) []option.ClientOption {
	generatedDefaultOpts := vkit.DefaultClientOptions()
	clientDefaultOpts := []option.ClientOption{
		option.WithGRPCConnectionPool(numChannels),
		option.WithUserAgent(fmt.Sprintf("spanner-go/v%s", internal.Version)),
	}
	if len(unaryInterceptors) > 0 {
		clientDefaultOpts = append(clientDefaultOpts, option.WithGRPCDialOption(grpc.WithChainUnaryInterceptor(unaryInterceptors...)))
	}
	if len(streamInterceptors) > 0 {
		clientDefaultOpts = append(clientDefaultOpts, option.WithGRPCDialOption(grpc.WithChainStreamInterceptor(streamInterceptors...)))
	}
	clientDefaultOpts = append(clientDefaultOpts,
		option.WithGRPCDialOption(grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: 120 * time.Second})),
	)

	if ResolveDirectPathEnabled(enableDirectAccess) {
		clientDefaultOpts = append(clientDefaultOpts, internaloption.AllowNonDefaultServiceAccount(true))
		clientDefaultOpts = append(clientDefaultOpts, internaloption.EnableDirectPath(true), internaloption.EnableDirectPathXds())
		if disableBoundToken, _ := strconv.ParseBool(os.Getenv("GOOGLE_SPANNER_DISABLE_DIRECT_ACCESS_BOUND_TOKEN")); !disableBoundToken {
			clientDefaultOpts = append(clientDefaultOpts, internaloption.AllowHardBoundTokens("ALTS"))
		}
	}
	if compression == gzip.Name {
		userOpts = append(userOpts, option.WithGRPCDialOption(grpc.WithDefaultCallOptions(
			grpc.UseCompressor(gzip.Name))))
	}
	allDefaultOpts := append(generatedDefaultOpts, clientDefaultOpts...)
	return append(allDefaultOpts, userOpts...)
}

// WithRequestIDInterceptors returns a copy of userOpts with stream and unary
// x-goog-spanner-request-id client interceptors appended.
func WithRequestIDInterceptors(augmentErr requestid.ErrorAugmenter, userOpts ...option.ClientOption) []option.ClientOption {
	reqIDInjector := requestid.NewHeaderInjector(augmentErr)
	opts := append([]option.ClientOption{}, userOpts...)
	return append(opts,
		option.WithGRPCDialOption(grpc.WithChainStreamInterceptor(reqIDInjector.InterceptStream)),
		option.WithGRPCDialOption(grpc.WithChainUnaryInterceptor(reqIDInjector.InterceptUnary)),
	)
}

// PoolConfig configures connection pool dialing for CreateFallbackConnPool and DialStaticPool.
type PoolConfig struct {
	NumChannels          int
	HasNumChannelsConfig bool
	Compression          string
	EnableDirectAccess   bool
	AugmentError         requestid.ErrorAugmenter
	MeterProvider        metric.MeterProvider
	UnaryInterceptors    []grpc.UnaryClientInterceptor
	StreamInterceptors   []grpc.StreamClientInterceptor
}

// FallbackPool wraps a grpcgcp.GCPFallback connection pool and its primary/fallback pools.
type FallbackPool struct {
	*grpcgcp.GCPFallback
	PrimaryConn  gtransport.ConnPool
	FallbackConn gtransport.ConnPool
}

// Conn returns nil because GCPFallback hides the underlying ClientConn.
func (fw *FallbackPool) Conn() *grpc.ClientConn {
	return nil
}

// Num returns the number of connections in the primary pool.
func (fw *FallbackPool) Num() int {
	return fw.PrimaryConn.Num()
}

// Close closes the fallback wrapper and both underlying connection pools.
func (fw *FallbackPool) Close() error {
	fw.GCPFallback.Close()
	err1 := fw.PrimaryConn.Close()
	err2 := fw.FallbackConn.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

// CreateFallbackConnPool dials a DirectPath primary pool and a CloudPath fallback pool
// wrapped in a FallbackPool.
func CreateFallbackConnPool(
	ctx context.Context,
	cfg PoolConfig,
	userOpts ...option.ClientOption,
) (gtransport.ConnPool, []option.ClientOption, error) {
	opts := WithRequestIDInterceptors(cfg.AugmentError, userOpts...)
	allOpts := DefaultClientOptions(cfg.NumChannels, cfg.Compression, cfg.EnableDirectAccess, cfg.UnaryInterceptors, cfg.StreamInterceptors, opts...)
	primaryConn, err := gtransport.DialPool(ctx, allOpts...)
	if err != nil {
		return nil, nil, err
	}

	fallbackConnOpts := append(allOpts, internaloption.EnableDirectPath(false))
	fallbackConn, err := gtransport.DialPool(ctx, fallbackConnOpts...)
	if err != nil {
		primaryConn.Close()
		return nil, nil, err
	}

	if cfg.HasNumChannelsConfig && ((primaryConn.Num() != cfg.NumChannels) || (fallbackConn.Num() != cfg.NumChannels)) {
		primaryConn.Close()
		fallbackConn.Close()
		return nil, nil, status.Errorf(codes.InvalidArgument, "Connection pool mismatch: NumChannels=%v, primaryConn.Num()=%v, fallbackConn.Num()=%v", cfg.NumChannels, primaryConn.Num(), fallbackConn.Num())
	}

	fbOpts := grpcgcp.NewGCPFallbackOptions()
	fbOpts.EnableFallback = true
	fbOpts.ErrorRateThreshold = 1
	fbOpts.MinFailedCalls = 1
	fbOpts.Period = time.Minute * 3
	if cfg.MeterProvider != nil {
		fbOpts.MeterProvider = cfg.MeterProvider
	}

	gcpFallback, err := grpcgcp.NewGCPFallback(ctx, primaryConn, fallbackConn, fbOpts)
	if err != nil {
		primaryConn.Close()
		fallbackConn.Close()
		return nil, nil, err
	}

	return &FallbackPool{
		GCPFallback:  gcpFallback,
		PrimaryConn:  primaryConn,
		FallbackConn: fallbackConn,
	}, allOpts, nil
}

// DialStaticPool dials a static gtransport.ConnPool with x-goog-spanner-request-id
// interceptors and validates that NumChannels does not conflict with WithGRPCConnectionPool.
func DialStaticPool(
	ctx context.Context,
	cfg PoolConfig,
	userOpts ...option.ClientOption,
) (gtransport.ConnPool, []option.ClientOption, error) {
	opts := WithRequestIDInterceptors(cfg.AugmentError, userOpts...)
	allOpts := DefaultClientOptions(cfg.NumChannels, cfg.Compression, cfg.EnableDirectAccess, cfg.UnaryInterceptors, cfg.StreamInterceptors, opts...)
	pool, err := gtransport.DialPool(ctx, allOpts...)
	if err != nil {
		return nil, nil, err
	}
	if cfg.HasNumChannelsConfig && pool.Num() != cfg.NumChannels {
		pool.Close()
		return nil, nil, status.Errorf(codes.InvalidArgument, "Connection pool mismatch: NumChannels=%v, WithGRPCConnectionPool=%v. Only set one of these options, or set both to the same value.", cfg.NumChannels, pool.Num())
	}
	return pool, allOpts, nil
}

// MergeCallOptions merges two vkit.CallOptions structs, giving b precedence over a.
func MergeCallOptions(a *vkit.CallOptions, b *vkit.CallOptions) *vkit.CallOptions {
	res := &vkit.CallOptions{}
	resVal := reflect.ValueOf(res).Elem()
	aVal := reflect.ValueOf(a).Elem()
	bVal := reflect.ValueOf(b).Elem()

	t := aVal.Type()
	for i := 0; i < aVal.NumField(); i++ {
		fieldName := t.Field(i).Name
		aFieldVal := aVal.Field(i).Interface().([]gax.CallOption)
		bFieldVal := bVal.Field(i).Interface().([]gax.CallOption)
		merged := append([]gax.CallOption{}, aFieldVal...)
		merged = append(merged, bFieldVal...)
		resVal.FieldByName(fieldName).Set(reflect.ValueOf(merged))
	}
	return res
}

// NewGAPICClient constructs a new GAPIC vkit.Client, sets x-goog-api-client info
// ("gccl", internal.Version, and optional userAgent), and merges callOpts if non-nil.
func NewGAPICClient(ctx context.Context, userAgent string, callOpts *vkit.CallOptions, opts ...option.ClientOption) (*vkit.Client, error) {
	raw, err := vkit.NewClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	clientInfo := []string{"gccl", internal.Version}
	if userAgent != "" {
		agentWithVersion := strings.SplitN(userAgent, "/", 2)
		if len(agentWithVersion) == 2 {
			clientInfo = append(clientInfo, agentWithVersion[0], agentWithVersion[1])
		}
	}
	raw.SetGoogleClientInfo(clientInfo...)
	if callOpts != nil {
		raw.CallOptions = MergeCallOptions(raw.CallOptions, callOpts)
	}
	return raw, nil
}

// SetSpanAttributes dynamically sets OpenTelemetry span attributes based on the request type.
func SetSpanAttributes[T any](span oteltrace.Span, req T) {
	if !span.IsRecording() {
		return
	}
	var attrs []attribute.KeyValue
	if r, ok := any(req).(interface {
		GetRequestOptions() *spannerpb.RequestOptions
	}); ok {
		if tag := r.GetRequestOptions().GetTransactionTag(); tag != "" {
			attrs = append(attrs, attribute.String("transaction.tag", tag))
		}
		if tag := r.GetRequestOptions().GetRequestTag(); tag != "" {
			attrs = append(attrs, attribute.String("statement.tag", tag))
		}
	}
	if r, ok := any(req).(interface{ GetSql() string }); ok {
		if sql := r.GetSql(); sql != "" {
			attrs = append(attrs, attribute.String("db.statement", sql))
		}
	} else if r, ok := any(req).(interface {
		GetStatements() []*spannerpb.ExecuteBatchDmlRequest_Statement
	}); ok {
		if stmts := r.GetStatements(); len(stmts) > 0 {
			sqls := make([]string, len(stmts))
			for i, stmt := range stmts {
				sqls[i] = stmt.GetSql()
			}
			attrs = append(attrs, attribute.StringSlice("db.statement", sqls))
		}
	}
	if r, ok := any(req).(interface{ GetTable() string }); ok {
		if table := r.GetTable(); table != "" {
			attrs = append(attrs, attribute.String("db.table", table))
		}
	}
	span.SetAttributes(attrs...)
}

// SetGFEAndAFESpanAttributes records GFE and AFE latency attributes on span if recording.
func SetGFEAndAFESpanAttributes(span oteltrace.Span, latencyMap map[string]time.Duration) {
	if !span.IsRecording() {
		return
	}
	for t, v := range latencyMap {
		if t == GFETimingHeader || t == AFETimingHeader {
			span.SetAttributes(
				attribute.Float64(t[:3]+".latency_ms", float64(v.Nanoseconds())/1e6),
			)
		}
	}
}

// CachedExecuteStreamingSQLClient caches the response headers of an ExecuteStreamingSql
// stream so that server-timing capture and GFE latency sinks share a single Header() call.
type CachedExecuteStreamingSQLClient struct {
	spannerpb.Spanner_ExecuteStreamingSqlClient
	once sync.Once
	md   metadata.MD
	err  error
}

// Header returns the response headers of the stream, calling the underlying Header() at most once.
func (c *CachedExecuteStreamingSQLClient) Header() (metadata.MD, error) {
	c.once.Do(func() { c.md, c.err = c.Spanner_ExecuteStreamingSqlClient.Header() })
	return c.md, c.err
}

// ExecuteStreamingSQL invokes raw.ExecuteStreamingSql with peer info context and span attributes,
// wrapping the returned stream in a CachedExecuteStreamingSQLClient.
func ExecuteStreamingSQL(ctx context.Context, raw *vkit.Client, req *spannerpb.ExecuteSqlRequest, opts ...gax.CallOption) (*CachedExecuteStreamingSQLClient, error) {
	span := oteltrace.SpanFromContext(ctx)
	SetSpanAttributes(span, req)
	client, err := raw.ExecuteStreamingSql(peer.NewContext(ctx, &peer.Peer{}), req, opts...)
	if err != nil {
		return nil, err
	}
	return &CachedExecuteStreamingSQLClient{Spanner_ExecuteStreamingSqlClient: client}, nil
}
