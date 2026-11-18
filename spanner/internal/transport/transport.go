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
// DirectPath/fallback, and DynamicChannelPool primitives shared across Spanner
// packages.
package transport

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/grpc-gcp-go/grpcgcp"
	"github.com/googleapis/gax-go/v2"
	"go.opentelemetry.io/otel/metric"
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
	// DefaultNumChannels is the default number of gRPC channels in a static connection pool.
	DefaultNumChannels = 4
)

// StripEndpointScheme removes a single leading http://, https://, or passthrough:///
// prefix from endpoint, matching the historical client.go emulator scheme stripping.
func StripEndpointScheme(endpoint string) string {
	for _, prefix := range []string{"http://", "https://", "passthrough:///"} {
		if after, ok := strings.CutPrefix(endpoint, prefix); ok {
			return after
		}
	}
	return endpoint
}

// PlainTextClientOptions returns the ClientOptions needed to connect to a plaintext
// gRPC endpoint without authentication (insecure transport credentials, no auth,
// and dial settings validation skipped). Callers supply option.WithEndpoint separately.
func PlainTextClientOptions() []option.ClientOption {
	return []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
		internaloption.SkipDialSettingsValidation(),
	}
}

// EmulatorClientOptions returns the ClientOptions needed to connect to a Spanner
// emulator endpoint.
func EmulatorClientOptions(emulatorAddr string) []option.ClientOption {
	return append(
		[]option.ClientOption{option.WithEndpoint("passthrough:///" + StripEndpointScheme(emulatorAddr))},
		PlainTextClientOptions()...,
	)
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
// default options (connection pool size, user-agent, interceptors, keepalive, and
// DirectPath), and userOpts in order: generatedDefaultOpts, clientDefaultOpts, userOpts
// (so userOpts override earlier defaults), followed by the gzip compressor call option
// when compression == "gzip" (which therefore overrides userOpts).
//
// It reads GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS and GOOGLE_SPANNER_DISABLE_DIRECT_ACCESS_BOUND_TOKEN
// from the process environment when resolving DirectPath options.
func DefaultClientOptions(
	numChannels int,
	compression string,
	enableDirectAccess bool,
	unaryInterceptors []grpc.UnaryClientInterceptor,
	streamInterceptors []grpc.StreamClientInterceptor,
	userOpts ...option.ClientOption,
) []option.ClientOption {
	return defaultClientOptions(
		numChannels,
		compression,
		ResolveDirectPathEnabled(enableDirectAccess),
		unaryInterceptors,
		streamInterceptors,
		userOpts...,
	)
}

func defaultClientOptions(
	numChannels int,
	compression string,
	enableDirectPath bool,
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

	if enableDirectPath {
		clientDefaultOpts = append(clientDefaultOpts, internaloption.AllowNonDefaultServiceAccount(true))
		clientDefaultOpts = append(clientDefaultOpts, internaloption.EnableDirectPath(true), internaloption.EnableDirectPathXds())
		if disableBoundToken, _ := strconv.ParseBool(os.Getenv("GOOGLE_SPANNER_DISABLE_DIRECT_ACCESS_BOUND_TOKEN")); !disableBoundToken {
			clientDefaultOpts = append(clientDefaultOpts, internaloption.AllowHardBoundTokens("ALTS"))
		}
	}

	extra := 0
	if compression == gzip.Name {
		extra = 1
	}
	out := make([]option.ClientOption, 0, len(generatedDefaultOpts)+len(clientDefaultOpts)+len(userOpts)+extra)
	out = append(out, generatedDefaultOpts...)
	out = append(out, clientDefaultOpts...)
	out = append(out, userOpts...)
	if compression == gzip.Name {
		out = append(out, option.WithGRPCDialOption(grpc.WithDefaultCallOptions(
			grpc.UseCompressor(gzip.Name))))
	}
	return out
}

// WithRequestIDInterceptors returns a copy of userOpts with stream and unary
// x-goog-spanner-request-id client interceptors appended.
// augmentErr may be nil (see requestid.NewHeaderInjector for the ErrorAugmenter contract).
func WithRequestIDInterceptors(augmentErr requestid.ErrorAugmenter, userOpts ...option.ClientOption) []option.ClientOption {
	reqIDInjector := requestid.NewHeaderInjector(augmentErr)
	opts := make([]option.ClientOption, 0, len(userOpts)+2)
	opts = append(opts, userOpts...)
	return append(opts,
		option.WithGRPCDialOption(grpc.WithChainStreamInterceptor(reqIDInjector.InterceptStream)),
		option.WithGRPCDialOption(grpc.WithChainUnaryInterceptor(reqIDInjector.InterceptUnary)),
	)
}

// PoolConfig configures connection pool dialing and client option construction.
type PoolConfig struct {
	// NumChannels sets the target gRPC connection pool size. Zero passes
	// option.WithGRPCConnectionPool(0) (dialing 1 channel unless overridden by
	// userOpts); pass DefaultNumChannels explicitly for Spanner's 4-channel default.
	NumChannels int
	// HasNumChannelsConfig reports whether NumChannels was explicitly set by the
	// caller, enabling validation against conflicting WithGRPCConnectionPool userOpts.
	HasNumChannelsConfig bool
	// Compression enables request compression when set to "gzip".
	Compression string
	// EnableDirectAccess enables DirectPath unless overridden by GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS
	// or suppressed by DisableDirectPath.
	EnableDirectAccess bool
	// DisableDirectPath unconditionally suppresses DirectPath options even when
	// EnableDirectAccess or GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS=true is set (for example,
	// on plaintext or unauthenticated transports).
	DisableDirectPath bool
	// AugmentError optionally attaches x-goog-spanner-request-id to non-EOF RPC
	// errors. May be nil (headers are still injected without wrapping errors).
	AugmentError requestid.ErrorAugmenter
	// MeterProvider configures OpenTelemetry metrics on CreateFallbackConnPool
	// (unused by DialStaticPool and ClientOptions).
	MeterProvider metric.MeterProvider
	// UnaryInterceptors are prepended before userOpts unary client interceptors.
	UnaryInterceptors []grpc.UnaryClientInterceptor
	// StreamInterceptors are prepended before userOpts stream client interceptors.
	StreamInterceptors []grpc.StreamClientInterceptor
}

// ClientOptions builds the resolved slice of ClientOptions combining DefaultClientOptions,
// userOpts, and x-goog-spanner-request-id interceptors for cfg.
func (cfg PoolConfig) ClientOptions(userOpts ...option.ClientOption) []option.ClientOption {
	opts := WithRequestIDInterceptors(cfg.AugmentError, userOpts...)
	enableDirectPath := !cfg.DisableDirectPath && ResolveDirectPathEnabled(cfg.EnableDirectAccess)
	return defaultClientOptions(cfg.NumChannels, cfg.Compression, enableDirectPath, cfg.UnaryInterceptors, cfg.StreamInterceptors, opts...)
}

// PoolSizeMismatchError is returned when an explicit NumChannels configuration
// conflicts with option.WithGRPCConnectionPool.
type PoolSizeMismatchError struct {
	Msg string
}

func (e *PoolSizeMismatchError) Error() string {
	return e.Msg
}

// GRPCStatus implements the status.GRPCStatus interface with codes.InvalidArgument.
func (e *PoolSizeMismatchError) GRPCStatus() *status.Status {
	return status.New(codes.InvalidArgument, e.Msg)
}

// FallbackPool wraps a grpcgcp.GCPFallback connection pool and its primary/fallback pools.
type FallbackPool struct {
	*grpcgcp.GCPFallback
	primaryConn  gtransport.ConnPool
	fallbackConn gtransport.ConnPool
}

// Conn returns nil because GCPFallback hides the underlying ClientConn.
// The Spanner client handles this by using the interface methods (Invoke/NewStream).
func (fw *FallbackPool) Conn() *grpc.ClientConn {
	return nil
}

// Num returns the number of connections in the primary pool.
func (fw *FallbackPool) Num() int {
	return fw.primaryConn.Num()
}

// PrimaryConn returns the underlying DirectPath primary connection pool.
func (fw *FallbackPool) PrimaryConn() gtransport.ConnPool {
	return fw.primaryConn
}

// FallbackConn returns the underlying CloudPath fallback connection pool.
func (fw *FallbackPool) FallbackConn() gtransport.ConnPool {
	return fw.fallbackConn
}

// Close closes the fallback wrapper and both underlying connection pools.
func (fw *FallbackPool) Close() error {
	fw.GCPFallback.Close()
	err1 := fw.primaryConn.Close()
	err2 := fw.fallbackConn.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

// CreateFallbackConnPool dials a DirectPath primary pool and a CloudPath fallback pool
// wrapped in a FallbackPool, returning the pool and the resolved primary ClientOptions
// (cfg.ClientOptions(userOpts...)) for reuse by routed endpoint clients.
func CreateFallbackConnPool(
	ctx context.Context,
	cfg PoolConfig,
	userOpts ...option.ClientOption,
) (gtransport.ConnPool, []option.ClientOption, error) {
	allOpts := cfg.ClientOptions(userOpts...)
	primaryConn, err := gtransport.DialPool(ctx, allOpts...)
	if err != nil {
		return nil, nil, err
	}

	fallbackConnOpts := append(allOpts[:len(allOpts):len(allOpts)], internaloption.EnableDirectPath(false))
	fallbackConn, err := gtransport.DialPool(ctx, fallbackConnOpts...)
	if err != nil {
		primaryConn.Close()
		return nil, nil, err
	}

	if cfg.HasNumChannelsConfig && ((primaryConn.Num() != cfg.NumChannels) || (fallbackConn.Num() != cfg.NumChannels)) {
		primaryConn.Close()
		fallbackConn.Close()
		return nil, nil, &PoolSizeMismatchError{
			Msg: fmt.Sprintf("Connection pool mismatch: NumChannels=%v, primaryConn.Num()=%v, fallbackConn.Num()=%v", cfg.NumChannels, primaryConn.Num(), fallbackConn.Num()),
		}
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
		primaryConn:  primaryConn,
		fallbackConn: fallbackConn,
	}, allOpts, nil
}

// DialStaticPool dials a static gtransport.ConnPool with x-goog-spanner-request-id
// interceptors and validates that NumChannels does not conflict with WithGRPCConnectionPool.
// The second return value is the resolved ClientOption slice (cfg.ClientOptions(userOpts...)).
func DialStaticPool(
	ctx context.Context,
	cfg PoolConfig,
	userOpts ...option.ClientOption,
) (gtransport.ConnPool, []option.ClientOption, error) {
	allOpts := cfg.ClientOptions(userOpts...)
	pool, err := gtransport.DialPool(ctx, allOpts...)
	if err != nil {
		return nil, nil, err
	}
	if cfg.HasNumChannelsConfig && pool.Num() != cfg.NumChannels {
		pool.Close()
		return nil, nil, &PoolSizeMismatchError{
			Msg: fmt.Sprintf("Connection pool mismatch: NumChannels=%v, WithGRPCConnectionPool=%v. Only set one of these options, or set both to the same value.", cfg.NumChannels, pool.Num()),
		}
	}
	return pool, allOpts, nil
}

// NewGAPICClient constructs a new GAPIC vkit.Client and sets x-goog-api-client info
// ("gccl", internal.Version, and optional userAgent).
func NewGAPICClient(ctx context.Context, userAgent string, opts ...option.ClientOption) (*vkit.Client, error) {
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
	return raw, nil
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

// ExecuteStreamingSQL invokes raw.ExecuteStreamingSql with peer info context and wraps
// the returned stream in a CachedExecuteStreamingSQLClient.
//
// Note: This function does not inject x-goog-spanner-request-id CallOptions because
// stream iterators construct and attach per-attempt request-ID options themselves.
func ExecuteStreamingSQL(ctx context.Context, raw *vkit.Client, req *spannerpb.ExecuteSqlRequest, opts ...gax.CallOption) (*CachedExecuteStreamingSQLClient, error) {
	client, err := raw.ExecuteStreamingSql(peer.NewContext(ctx, &peer.Peer{}), req, opts...)
	if err != nil {
		return nil, err
	}
	return &CachedExecuteStreamingSQLClient{Spanner_ExecuteStreamingSqlClient: client}, nil
}
