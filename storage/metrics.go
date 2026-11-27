// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/iam/apiv1/iampb"
	"cloud.google.com/go/storage/internal"
	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	mexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"golang.org/x/oauth2/google"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
)

const (
	customMetricPrefix   = "custom.googleapis.com/"
	storageOperationAttr = "gcp.storage.operation"
	networkPathAttr      = "network.path"
	networkPathDirect    = "directpath"
	networkPathCloud     = "cloudpath"
	networkPathUnknown   = "unknown"
)

func parseHostIP(addr string) netip.Addr {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		addr = h
	}
	ip, _ := netip.ParseAddr(strings.TrimSuffix(strings.TrimPrefix(addr, "["), "]"))
	return ip.Unmap()
}

func isDirectPathAddr(addr string) bool {
	ip := parseHostIP(addr)
	return directPathIPv4Range.Contains(ip) || directPathIPv6Range.Contains(ip)
}

func networkPathOfPeer(p *peer.Peer) string {
	if p == nil || p.Addr == nil {
		return networkPathUnknown
	}
	if isDirectPathPeer(p) {
		return networkPathDirect
	}
	return networkPathCloud
}

// clientMetrics contains the OpenTelemetry metric instruments to record client-side metrics.
type clientMetrics struct {
	provider                  *sdkmetric.MeterProvider
	rpcClientCallDuration     metric.Float64Histogram
	httpClientRequestDuration metric.Float64Histogram
	duration                  metric.Float64Histogram
	operations                metric.Int64Counter
	attempts                  metric.Int64Counter
	requestBodySize           metric.Int64Histogram
	responseBodySize          metric.Int64Histogram
	ttfb                      metric.Float64Histogram
	errors                    metric.Int64Counter
	activeRequests            metric.Int64UpDownCounter
	serverUnreached           metric.Int64Counter
	dnsLookupDuration         metric.Float64Histogram
	tcpConnectDuration        metric.Float64Histogram
	tlsHandshakeDuration      metric.Float64Histogram
	serverDuration            metric.Float64Histogram
	credentialRefreshDuration metric.Float64Histogram
	networkBytesSent          metric.Int64Counter
	networkBytesReceived      metric.Int64Counter
	stallDuration             metric.Float64Histogram
}

func formatMetricWithPrefix(m metricdata.Metrics, prefix string) string {
	return prefix + strings.ReplaceAll(string(m.Name), ".", "/")
}

// isOtelMetricsEnabled checks if Otel metrics are enabled.
// The environment variable GCP_STORAGE_GO_ENABLE_OTEL_METRICS takes precedence
// over the config option. Note that if metrics are completely disabled via
// disableClientMetrics, this will always return false regardless of the environment variable.
func isOtelMetricsEnabled(config *storageConfig) bool {
	if config.disableClientMetrics {
		return false
	}
	if valStr, present := os.LookupEnv(envOtelMetrics); present {
		v, err := strconv.ParseBool(valStr)
		if err == nil {
			return v
		}
	}
	return config.enableOtelMetrics
}

// isOtelDebugMetricsEnabled checks if debug Otel metrics are enabled.
// The environment variable GCP_STORAGE_GO_ENABLE_OTEL_DEBUG_METRICS takes precedence
// over the config option. Note that if metrics are completely disabled via
// disableClientMetrics, this will always return false regardless of the environment variable.
func isOtelDebugMetricsEnabled(config *storageConfig) bool {
	if config.disableClientMetrics {
		return false
	}
	if valStr, present := os.LookupEnv(envOtelDebugMetrics); present {
		v, err := strconv.ParseBool(valStr)
		if err == nil {
			return v
		}
	}
	return config.enableOtelDebugMetrics
}

// monitoringClientOptions extracts safe Cloud Monitoring client options from
// the parent Storage client's options. It returns ok=false when Cloud
// Monitoring export should be skipped (unauthenticated/API-key clients, custom
// HTTP/gRPC transports, endpoint-only non-googleapis.com configs without a
// configured universe domain, missing resolved credentials, or invalid options).
func monitoringClientOptions(clientOpts []option.ClientOption, authCreds *auth.Credentials, googleCreds *google.Credentials) (opts []option.ClientOption, ok bool) {
	res, err := internaloption.NewUnsafeResolver(clientOpts...)
	if err != nil {
		return nil, false
	}
	if res.ResolvedWithoutAuthentication() || res.ResolvedWithAPIKeyIsCustom() || res.ResolvedGRPCConnIsCustom() || res.ResolvedHTTPClientIsCustom() {
		return nil, false
	}
	if authCreds == nil && googleCreds == nil {
		return nil, false
	}

	universeDomain := res.ResolvedUniverseDomain()
	if universeDomain == "googleapis.com" {
		ep, err := res.ResolvedGRPCEndpoint()
		if err != nil {
			return nil, false
		}
		if host := endpointHost(ep); host != "" && host != "googleapis.com" && !strings.HasSuffix(host, ".googleapis.com") {
			return nil, false
		}
	}

	opts = append(opts, option.WithUniverseDomain(universeDomain))
	if authCreds != nil {
		// Clone the struct value so subsequent in-place wrapping on the parent
		// client's credentials does not affect the Cloud Monitoring exporter.
		credsCopy := *authCreds
		opts = append(opts, option.WithAuthCredentials(&credsCopy))
	} else {
		opts = append(opts, option.WithCredentials(googleCreds))
	}
	return opts, true
}

func endpointHost(ep string) string {
	if ep == "" {
		return ""
	}
	if strings.Contains(ep, "://") {
		if u, err := url.Parse(ep); err == nil {
			ep = u.Host
		}
	}
	if h, _, err := net.SplitHostPort(ep); err == nil {
		return h
	}
	return ep
}

// newMetricsGCMExporter creates a Google Cloud Monitoring exporter.
func newMetricsGCMExporter(ctx context.Context, projectID string, monitoringOpts ...option.ClientOption) (sdkmetric.Exporter, error) {
	exporter, err := mexporter.New(
		mexporter.WithProjectID(projectID),
		mexporter.WithMetricDescriptorTypeFormatter(func(m metricdata.Metrics) string {
			return formatMetricWithPrefix(m, customMetricPrefix)
		}),
		// The OTel GCP exporter drops resource attributes that don't map to the
		// target MonitoredResource (currently generic_node). WithFilteredResourceAttributes
		// keeps an allowlist of attributes (see keepFilteredResourceAttribute) as metric
		// labels and drops the rest to stay under the Cloud Monitoring label limit.
		//
		// TODO: When storage_client node is allowlisted in Monarch
		// (google3/configs/monitoring/cloud_pulse_monarch/storage/storage_client.proto),
		// we should uncomment the following options to export to the custom resource:
		// mexporter.WithCreateServiceTimeSeries(),
		// mexporter.WithMonitoredResourceDescription("storage.googleapis.com/Client", []string{"project_id", "location", "cloud_platform", "host_id", "instance_id", "api"}),
		mexporter.WithFilteredResourceAttributes(func(kv attribute.KeyValue) bool {
			return keepFilteredResourceAttribute(string(kv.Key))
		}),
		mexporter.WithMonitoringClientOptions(monitoringOpts...),
	)
	if err != nil {
		return nil, fmt.Errorf("storage: creating GCM exporter: %w", err)
	}
	return exporter, nil
}

// keepFilteredResourceAttribute reports whether an OpenTelemetry Resource
// attribute key should be retained by the GCM exporter (either to map onto the
// storage.googleapis.com/Client MonitoredResource or to be promoted to a base
// metric label via gcp.client.*).
func keepFilteredResourceAttribute(key string) bool {
	if strings.HasPrefix(key, "gcp.client.") {
		return true
	}
	switch key {
	case "cloud.platform", "cloud.region", "host.id":
		return true
	}
	return false
}

// metricBaseLabels returns the client identity resource attributes (including
// a per-client UUID) promoted to base metric labels by keepFilteredResourceAttribute.
func metricBaseLabels() []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("gcp.client.version", internal.Version),
		attribute.String("gcp.client.service", "storage"),
		attribute.String("gcp.client.artifact", "cloud.google.com/go/storage"),
		attribute.String("gcp.client.instance_id", uuid.New().String()),
	}
}

// initMetrics initializes clientMetrics with a meter provider and registered exporter.
func initMetrics(ctx context.Context, projectID string, config *storageConfig) (*clientMetrics, func(), error) {
	var provider *sdkmetric.MeterProvider
	var ownProvider bool

	if config.meterProvider != nil {
		provider = config.meterProvider
	} else {
		res, err := resource.New(ctx,
			resource.WithDetectors(gcp.NewDetector()),
			resource.WithAttributes(metricBaseLabels()...),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("storage: creating metrics resource: %w", err)
		}

		var exporter sdkmetric.Exporter
		if config.metricExporter != nil {
			exporter = *config.metricExporter
		} else {
			if projectID == "" {
				if p, ok := res.Set().Value("cloud.account.id"); ok {
					projectID = p.AsString()
				}
			}
			if projectID == "" {
				return nil, nil, errors.New("storage: google cloud project is required to start client-side metrics")
			}
			exporter, err = newMetricsGCMExporter(ctx, projectID, config.monitoringOpts...)
			if err != nil {
				return nil, nil, err
			}
		}

		interval := time.Minute
		if config.metricInterval > 0 {
			interval = config.metricInterval
		}

		reader := sdkmetric.NewPeriodicReader(&exporterLogSuppressor{Exporter: exporter}, sdkmetric.WithInterval(interval))

		provider = sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(reader),
			sdkmetric.WithResource(res),
			sdkmetric.WithExemplarFilter(exemplar.TraceBasedFilter),
			sdkmetric.WithView(
				sdkmetric.NewView(
					sdkmetric.Instrument{Name: "rpc.client.call.duration", Kind: sdkmetric.InstrumentKindHistogram},
					sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: latencyHistogramBoundaries()}},
				),
				sdkmetric.NewView(
					sdkmetric.Instrument{Name: "http.client.request.duration", Kind: sdkmetric.InstrumentKindHistogram},
					sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: latencyHistogramBoundaries()}},
				),
				sdkmetric.NewView(
					sdkmetric.Instrument{Name: "gcp.client.request.duration", Kind: sdkmetric.InstrumentKindHistogram},
					sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: latencyHistogramBoundaries()}},
				),
				sdkmetric.NewView(
					sdkmetric.Instrument{Name: "gcp.storage.client.ttfb", Kind: sdkmetric.InstrumentKindHistogram},
					sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: latencyHistogramBoundaries()}},
				),
				sdkmetric.NewView(
					sdkmetric.Instrument{Name: "gcp.storage.client.request.body.size", Kind: sdkmetric.InstrumentKindHistogram},
					sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: sizeHistogramBoundaries()}},
				),
				sdkmetric.NewView(
					sdkmetric.Instrument{Name: "gcp.storage.client.response.body.size", Kind: sdkmetric.InstrumentKindHistogram},
					sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: sizeHistogramBoundaries()}},
				),
				sdkmetric.NewView(
					sdkmetric.Instrument{Name: "gcp.storage.client.network.dns.lookup.duration", Kind: sdkmetric.InstrumentKindHistogram},
					sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: latencyHistogramBoundaries()}},
				),
				sdkmetric.NewView(
					sdkmetric.Instrument{Name: "gcp.storage.client.network.tcp.connect.duration", Kind: sdkmetric.InstrumentKindHistogram},
					sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: latencyHistogramBoundaries()}},
				),
				sdkmetric.NewView(
					sdkmetric.Instrument{Name: "gcp.storage.client.network.tls.handshake.duration", Kind: sdkmetric.InstrumentKindHistogram},
					sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: latencyHistogramBoundaries()}},
				),
				sdkmetric.NewView(
					sdkmetric.Instrument{Name: "gcp.storage.client.server.duration", Kind: sdkmetric.InstrumentKindHistogram},
					sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: latencyHistogramBoundaries()}},
				),
				sdkmetric.NewView(
					sdkmetric.Instrument{Name: "gcp.storage.client.auth.credential_refresh.duration", Kind: sdkmetric.InstrumentKindHistogram},
					sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: latencyHistogramBoundaries()}},
				),
				sdkmetric.NewView(
					sdkmetric.Instrument{Name: "gcp.storage.client.stall.duration", Kind: sdkmetric.InstrumentKindHistogram},
					sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: latencyHistogramBoundaries()}},
				),
			),
		)
		ownProvider = true
	}

	meter := provider.Meter("cloud.google.com/go/storage")
	// Standard and debug metrics are enabled independently. When only debug
	// metrics are enabled, the standard instruments are no-ops so nothing is
	// recorded or exported for them.
	stdMeter := meter
	if !isOtelMetricsEnabled(config) {
		stdMeter = noop.Meter{}
	}

	rpcDuration, err := stdMeter.Float64Histogram(
		"rpc.client.call.duration",
		metric.WithDescription("Duration of one gRPC request. Retries not included (Otel)"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, nil, err
	}

	httpDuration, err := stdMeter.Float64Histogram(
		"http.client.request.duration",
		metric.WithDescription("Duration of one HTTP client request. Retried not included (Otel)"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, nil, err
	}

	duration, err := stdMeter.Float64Histogram(
		"gcp.client.request.duration",
		metric.WithDescription("Latency of a client operation"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, nil, err
	}

	operations, err := stdMeter.Int64Counter(
		"gcp.storage.client.operations",
		metric.WithDescription("Number of GCS client operations"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		return nil, nil, err
	}

	attempts, err := stdMeter.Int64Counter(
		"gcp.storage.client.attempts",
		metric.WithDescription("Number of GCS client attempts"),
		metric.WithUnit("{attempt}"),
	)
	if err != nil {
		return nil, nil, err
	}

	requestBodySize, err := stdMeter.Int64Histogram(
		"gcp.storage.client.request.body.size",
		metric.WithDescription("Size of GCS client request body"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, nil, err
	}

	responseBodySize, err := stdMeter.Int64Histogram(
		"gcp.storage.client.response.body.size",
		metric.WithDescription("Size of GCS client response body"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, nil, err
	}

	ttfb, err := stdMeter.Float64Histogram(
		"gcp.storage.client.ttfb",
		metric.WithDescription("Time to first byte of GCS client operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, nil, err
	}

	errors, err := stdMeter.Int64Counter(
		"gcp.storage.client.errors",
		metric.WithDescription("Number of GCS client errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return nil, nil, err
	}

	serverUnreached, err := stdMeter.Int64Counter(
		"gcp.storage.client.server.unreached",
		metric.WithDescription("Number of GCS requests that did not reach a Cloud Storage server"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, nil, err
	}

	var activeRequests metric.Int64UpDownCounter
	var dnsLookupDuration metric.Float64Histogram
	var tcpConnectDuration metric.Float64Histogram
	var tlsHandshakeDuration metric.Float64Histogram
	var serverDuration metric.Float64Histogram
	var credentialRefreshDuration metric.Float64Histogram

	var networkBytesSent metric.Int64Counter
	var networkBytesReceived metric.Int64Counter
	var stallDuration metric.Float64Histogram

	if isOtelDebugMetricsEnabled(config) {
		networkBytesSent, err = meter.Int64Counter(
			"gcp.storage.client.network.bytes.sent",
			metric.WithDescription("Total physical bytes sent over the wire socket (gRPC only)"),
			metric.WithUnit("By"),
		)
		if err != nil {
			return nil, nil, err
		}

		networkBytesReceived, err = meter.Int64Counter(
			"gcp.storage.client.network.bytes.received",
			metric.WithDescription("Total physical bytes received over the wire socket (gRPC only)"),
			metric.WithUnit("By"),
		)
		if err != nil {
			return nil, nil, err
		}

		stallDuration, err = meter.Float64Histogram(
			"gcp.storage.client.stall.duration",
			metric.WithDescription("Stall timeout duration before a read or write attempt was aborted"),
			metric.WithUnit("s"),
		)
		if err != nil {
			return nil, nil, err
		}

		credentialRefreshDuration, err = meter.Float64Histogram(
			"gcp.storage.client.auth.credential_refresh.duration",
			metric.WithDescription("Time taken to refresh access credentials"),
			metric.WithUnit("s"),
		)
		if err != nil {
			return nil, nil, err
		}
		activeRequests, err = meter.Int64UpDownCounter(
			"gcp.storage.client.request.active",
			metric.WithDescription("Number of active GCS client requests"),
			metric.WithUnit("{request}"),
		)
		if err != nil {
			return nil, nil, err
		}

		dnsLookupDuration, err = meter.Float64Histogram(
			"gcp.storage.client.network.dns.lookup.duration",
			metric.WithDescription("Time taken for DNS lookup"),
			metric.WithUnit("s"),
		)
		if err != nil {
			return nil, nil, err
		}

		tcpConnectDuration, err = meter.Float64Histogram(
			"gcp.storage.client.network.tcp.connect.duration",
			metric.WithDescription("Time taken for TCP connection"),
			metric.WithUnit("s"),
		)
		if err != nil {
			return nil, nil, err
		}

		tlsHandshakeDuration, err = meter.Float64Histogram(
			"gcp.storage.client.network.tls.handshake.duration",
			metric.WithDescription("Time taken to perform a TLS handshake"),
			metric.WithUnit("s"),
		)
		if err != nil {
			return nil, nil, err
		}

		serverDuration, err = meter.Float64Histogram(
			"gcp.storage.client.server.duration",
			metric.WithDescription("Server-reported request processing time"),
			metric.WithUnit("s"),
		)
		if err != nil {
			return nil, nil, err
		}
	}

	cm := &clientMetrics{
		provider:                  provider,
		rpcClientCallDuration:     rpcDuration,
		httpClientRequestDuration: httpDuration,
		duration:                  duration,
		operations:                operations,
		attempts:                  attempts,
		requestBodySize:           requestBodySize,
		responseBodySize:          responseBodySize,
		ttfb:                      ttfb,
		errors:                    errors,
		activeRequests:            activeRequests,
		serverUnreached:           serverUnreached,
		dnsLookupDuration:         dnsLookupDuration,
		tcpConnectDuration:        tcpConnectDuration,
		tlsHandshakeDuration:      tlsHandshakeDuration,
		serverDuration:            serverDuration,
		credentialRefreshDuration: credentialRefreshDuration,
		networkBytesSent:          networkBytesSent,
		networkBytesReceived:      networkBytesReceived,
		stallDuration:             stallDuration,
	}

	var cleanup func()
	if ownProvider {
		cleanup = func() {
			shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			provider.Shutdown(shutdownCtx)
		}
	}

	return cm, cleanup, nil
}

// grpcCodeToString maps a gRPC status code to its screaming-snake-case protocol name.
func grpcCodeToString(code codes.Code) string {
	switch code {
	case codes.OK:
		return "OK"
	case codes.Canceled:
		return "CANCELLED"
	case codes.Unknown:
		return "UNKNOWN"
	case codes.InvalidArgument:
		return "INVALID_ARGUMENT"
	case codes.DeadlineExceeded:
		return "DEADLINE_EXCEEDED"
	case codes.NotFound:
		return "NOT_FOUND"
	case codes.AlreadyExists:
		return "ALREADY_EXISTS"
	case codes.PermissionDenied:
		return "PERMISSION_DENIED"
	case codes.ResourceExhausted:
		return "RESOURCE_EXHAUSTED"
	case codes.FailedPrecondition:
		return "FAILED_PRECONDITION"
	case codes.Aborted:
		return "ABORTED"
	case codes.OutOfRange:
		return "OUT_OF_RANGE"
	case codes.Unimplemented:
		return "UNIMPLEMENTED"
	case codes.Internal:
		return "INTERNAL"
	case codes.Unavailable:
		return "UNAVAILABLE"
	case codes.DataLoss:
		return "DATA_LOSS"
	case codes.Unauthenticated:
		return "UNAUTHENTICATED"
	default:
		return "UNKNOWN"
	}
}

// isServerChecksumError reports whether err is the service rejecting a
// request because a CRC32C or MD5 checksum did not match, for example
// "Object crc32c mismatch: ..." (gRPC InvalidArgument) or
// "Provided CRC32C \"...\" doesn't match calculated CRC32C \"...\"" (HTTP 400).
func isServerChecksumError(err error, errStr string) bool {
	if !(strings.Contains(errStr, "crc32c") || strings.Contains(errStr, "md5")) || !strings.Contains(errStr, "match") {
		return false
	}
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) && apiErr.Code == http.StatusBadRequest {
		return true
	}
	if st, ok := status.FromError(err); ok {
		return st.Code() == codes.InvalidArgument || st.Code() == codes.DataLoss
	}
	return false
}

// credentialError marks an error returned while obtaining an access token
// (when metrics are enabled) so that it can be classified as
// AUTHENTICATION_ERROR regardless of the underlying cause (for example a
// metadata server connection failure). It is transparent to callers: Error(),
// Unwrap(), Timeout(), and Temporary() expose the original error's behavior.
type credentialError struct{ err error }

func (e *credentialError) Error() string { return e.err.Error() }
func (e *credentialError) Unwrap() error { return e.err }

func (e *credentialError) Timeout() bool {
	var ne net.Error
	return errors.As(e.err, &ne) && ne.Timeout()
}

func (e *credentialError) Temporary() bool {
	var ne net.Error
	return errors.As(e.err, &ne) && ne.Temporary()
}

// computeErrorType maps the request result to the standard error.type values.
func computeErrorType(err error, isHTTP bool, statusCode int64) string {
	// io.EOF signals the normal end of a gRPC stream or an HTTP response body
	// (where statusCode > 0), whereas an HTTP request that fails with io.EOF
	// before receiving a response (statusCode == 0) is a dropped connection.
	if err == nil || (errors.Is(err, io.EOF) && (!isHTTP || statusCode > 0)) {
		if isHTTP && statusCode >= 400 {
			return mapHTTPStatusCode(int(statusCode))
		}
		return "OK"
	}

	errStr := strings.ToLower(err.Error())
	// Match on the innermost error: *url.Error embeds the request URL, whose
	// bucket and object names must not influence classification.
	var uerr *url.Error
	if errors.As(err, &uerr) && uerr.Err != nil {
		errStr = strings.ToLower(uerr.Err.Error())
	}

	if errors.Is(err, context.Canceled) || strings.Contains(errStr, "context canceled") {
		return "CANCELLED"
	}

	var credErr *credentialError
	if errors.As(err, &credErr) {
		return "AUTHENTICATION_ERROR"
	}

	// Check *net.DNSError and *net.OpError{Op: "dial"} before
	// context.DeadlineExceeded so that DNS and TCP connect failures (including
	// dial timeouts) are classified as DNS_FAILURE and CONNECTION_ERROR rather
	// than generic TIMEOUT, allowing server.unreached to count them as
	// pre-server failures.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "DNS_FAILURE"
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return "CONNECTION_ERROR"
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errStallTimeout) {
		return "TIMEOUT"
	}

	// The not-found sentinels are returned without an underlying status, for
	// example by the HTTP XML reader on a 404 response.
	if errors.Is(err, ErrObjectNotExist) || errors.Is(err, ErrBucketNotExist) {
		return "NOT_FOUND"
	}

	if errors.Is(err, errChecksumMismatch) || isServerChecksumError(err, errStr) {
		return "CHECKSUM_MISMATCH"
	}

	// Prefer the status returned by the service over matching the error text,
	// which can contain resource names such as "my-key" or "geofence.json".
	if isHTTP {
		var apiErr *googleapi.Error
		if errors.As(err, &apiErr) && apiErr.Code > 0 {
			return mapHTTPStatusCode(apiErr.Code)
		}
		if statusCode >= 400 {
			return mapHTTPStatusCode(int(statusCode))
		}
	}
	var grpcType string
	if !isHTTP {
		if st, ok := status.FromError(err); ok && st.Code() != codes.OK {
			// gRPC reports a client-side deadline as a DeadlineExceeded status with
			// this message; other DeadlineExceeded statuses come from the service.
			if st.Code() == codes.DeadlineExceeded && strings.Contains(errStr, "context deadline exceeded") {
				return "TIMEOUT"
			}
			grpcType = grpcCodeToString(st.Code())
			// Unknown and Unavailable are also used for client-side and
			// network failures, which the message describes more precisely.
			if st.Code() != codes.Unknown && st.Code() != codes.Unavailable {
				return grpcType
			}
		}
	}

	if errType := errorTypeFromMessage(errStr); errType != "" {
		return errType
	}
	if grpcType != "" {
		return grpcType
	}
	return "UNKNOWN"
}

// errorTypeFromMessage classifies client-side and network errors by their
// lowercase error text. It returns "" if the text matches no known class.
func errorTypeFromMessage(errStr string) string {
	switch {
	case strings.Contains(errStr, "no such host") || strings.Contains(errStr, "no address"):
		return "DNS_FAILURE"
	case strings.Contains(errStr, "tls") || strings.Contains(errStr, "certificate") || strings.Contains(errStr, "x509") || strings.Contains(errStr, "authentication handshake failed"):
		return "TLS_FAILURE"
	case strings.Contains(errStr, "per-rpc creds"):
		return "AUTHENTICATION_ERROR"
	case strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "connection reset") || strings.Contains(errStr, "dial tcp") || strings.Contains(errStr, "broken pipe") || strings.Contains(errStr, "eof") || strings.Contains(errStr, "closed network connection"):
		return "CONNECTION_ERROR"
	case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "context deadline exceeded"):
		return "TIMEOUT"
	case strings.Contains(errStr, "auth") || strings.Contains(errStr, "credentials"):
		return "AUTHENTICATION_ERROR"
	}
	return ""
}

// refineCancelled labels attempts that the client aborted because of a dynamic
// read stall timeout or write chunk transfer timeout as TIMEOUT rather than
// CANCELLED, which would otherwise suggest that the application cancelled the
// request. Operation-level metrics and spans (startOperation and
// errorAttributes) do not need refineCancelled because executeWithReadStallTimeout
// and writeLoop return context.DeadlineExceeded and *stallTimeoutError directly.
func refineCancelled(ctx context.Context, errorType string) string {
	if errorType == "CANCELLED" {
		cause := context.Cause(ctx)
		if errors.Is(cause, errReadStallTimeout) || errors.Is(cause, errStallTimeout) {
			return "TIMEOUT"
		}
	}
	return errorType
}

// mapHTTPStatusCode converts an HTTP status code to a canonical API error string.
// If there is no direct mapping, it returns the numeric string.
func mapHTTPStatusCode(code int) string {
	switch code {
	case 400:
		return "INVALID_ARGUMENT"
	case 401:
		return "UNAUTHENTICATED"
	case 403:
		return "PERMISSION_DENIED"
	case 404:
		return "NOT_FOUND"
	case 408:
		return "DEADLINE_EXCEEDED"
	case 409:
		return "ABORTED"
	case 412:
		return "FAILED_PRECONDITION"
	case 416:
		return "OUT_OF_RANGE"
	case 429:
		return "RESOURCE_EXHAUSTED"
	case 499:
		return "CANCELLED"
	case 500:
		return "INTERNAL"
	case 501:
		return "UNIMPLEMENTED"
	case 502, 503:
		return "UNAVAILABLE"
	case 504:
		return "DEADLINE_EXCEEDED"
	default:
		return strconv.Itoa(code)
	}
}

func (cm *clientMetrics) recordTTFB(ctx context.Context, system, op, host string, seconds float64) {
	if cm == nil || cm.ttfb == nil {
		return
	}
	cm.ttfb.Record(ctx, seconds, metric.WithAttributes(
		attribute.String("rpc.system.name", system),
		attribute.String(storageOperationAttr, op),
		attribute.String("server.address", stripPort(host)),
	))
}

func (cm *clientMetrics) recordRPC(ctx context.Context, method, target string, duration float64, err error) string {
	code := codes.OK
	if err != nil && !errors.Is(err, io.EOF) {
		code = status.Code(err)
	}
	statusStr := grpcCodeToString(code)
	rpcMethod := fullGRPCMethodName(method)

	errorType := refineCancelled(ctx, computeErrorType(err, false, int64(code)))
	host := stripPort(target)

	attrs := []attribute.KeyValue{
		attribute.String("rpc.system.name", "grpc"),
		attribute.String("rpc.method", rpcMethod),
		attribute.String("rpc.response.status_code", statusStr),
		attribute.String("server.address", host),
		attribute.String("error.type", errorType),
	}

	cm.rpcClientCallDuration.Record(ctx, duration, metric.WithAttributes(attrs...))

	// Record standard attempt metric: gcp.storage.client.attempts.
	logicalMethod := operationName(ctx, method)
	attemptAttrs := []attribute.KeyValue{
		attribute.String("rpc.system.name", "grpc"),
		attribute.String(storageOperationAttr, logicalMethod),
		attribute.String("rpc.response.status_code", statusStr),
		attribute.String("server.address", host),
		attribute.String("error.type", errorType),
	}
	cm.attempts.Add(ctx, 1, metric.WithAttributes(attemptAttrs...))

	// Record standard error metric: gcp.storage.client.errors.
	if err != nil && !errors.Is(err, io.EOF) {
		errorAttrs := []attribute.KeyValue{
			attribute.String("rpc.system.name", "grpc"),
			attribute.String(storageOperationAttr, logicalMethod),
			attribute.String("server.address", host),
			attribute.String("error.type", errorType),
		}
		cm.errors.Add(ctx, 1, metric.WithAttributes(errorAttrs...))
	}

	return errorType
}

func (cm *clientMetrics) recordHTTP(ctx context.Context, req *http.Request, resp *http.Response, duration float64, err error) {
	statusCode := int64(0)
	if resp != nil {
		statusCode = int64(resp.StatusCode)
	}

	urlTemplate := computeURLTemplate(req.URL.Path, req.URL.Host)
	errorType := refineCancelled(ctx, computeErrorType(err, true, statusCode))

	attrs := []attribute.KeyValue{
		attribute.String("rpc.system.name", "http"),
		attribute.String("http.request.method", req.Method),
		attribute.String("url.template", urlTemplate),
		attribute.Int64("http.response.status_code", statusCode),
		attribute.String("server.address", stripPort(req.URL.Host)),
		attribute.String("error.type", errorType),
	}

	cm.httpClientRequestDuration.Record(ctx, duration, metric.WithAttributes(attrs...))
}

// computeURLTemplate extracts a parameterized template path for a given GCS HTTP request URL path.
func computeURLTemplate(path, host string) string {
	// Check for XML host-style: {bucket}.storage.googleapis.com.
	if strings.HasSuffix(host, ".storage.googleapis.com") && host != "storage.googleapis.com" {
		if path == "/" || path == "" {
			return "/"
		}
		return "/{object}"
	}

	// Check for XML path-style or JSON API.
	if !strings.HasPrefix(path, "/storage/") && !strings.HasPrefix(path, "/upload/") && !strings.HasPrefix(path, "/batch") {
		p := strings.TrimPrefix(path, "/")
		parts := strings.SplitN(p, "/", 2)
		if len(parts) == 1 {
			if parts[0] == "" {
				return "/"
			}
			return "/{bucket}"
		}
		return "/{bucket}/{object}"
	}

	// JSON API: /storage/v1/b/bucket-name/o/object-name etc.
	bIdx := strings.Index(path, "/b/")
	if bIdx == -1 {
		return path
	}
	prefix := path[:bIdx+3]
	rest := path[bIdx+3:]

	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 1 {
		return prefix + "{bucket}"
	}

	oRest := parts[1]
	if oRest == "o" {
		return prefix + "{bucket}/o"
	}
	if strings.HasPrefix(oRest, "o/") {
		return prefix + "{bucket}/o/{object}"
	}

	return prefix + "{bucket}/" + oRest
}

// stripPort returns the host name of an HTTP host ("host:port") or a gRPC
// target ("dns:///host:port", "google-c2p:///host", "dns://authority/host"),
// without scheme, authority or port, for use as the server.address attribute.
func stripPort(host string) string {
	if _, rest, ok := strings.Cut(host, "://"); ok {
		host = rest
		if _, afterSlash, ok := strings.Cut(host, "/"); ok {
			host = afterSlash
		}
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
}

// metricsRoundTripper is an http.RoundTripper that wraps an underlying transport.
type metricsRoundTripper struct {
	base    http.RoundTripper
	metrics *clientMetrics
}

func (rt *metricsRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	logicalMethod := operationName(req.Context(), "Unknown")

	var decrementActiveRequests bool
	var host string
	var netAttrs, rpcAttrs metric.MeasurementOption
	if rt.metrics != nil {
		host = stripPort(req.URL.Host)
		if state := metricsStateFromContext(req.Context()); state != nil {
			state.setTarget(req.URL.Host)
		}
		netAttrs = metric.WithAttributes(
			attribute.String("rpc.system.name", "http"),
			attribute.String("server.address", host),
		)
		if rt.metrics.activeRequests != nil || rt.metrics.serverDuration != nil {
			rpcAttrs = metric.WithAttributes(
				attribute.String(storageOperationAttr, logicalMethod),
				attribute.String("rpc.system.name", "http"),
				attribute.String("server.address", host),
			)
		}
		if rt.metrics.activeRequests != nil {
			rt.metrics.activeRequests.Add(req.Context(), 1, rpcAttrs)
			decrementActiveRequests = true
		}
	}
	defer func() {
		if decrementActiveRequests && rt.metrics != nil && rt.metrics.activeRequests != nil {
			rt.metrics.activeRequests.Add(req.Context(), -1, rpcAttrs)
		}
	}()

	var getConnTime, firstByteTime, wroteRequestTime atomic.Pointer[time.Time]
	if rt.metrics != nil {
		var dnsStart, tlsStart time.Time
		var tcpStarts sync.Map

		trace := &httptrace.ClientTrace{
			GetConn: func(hostPort string) {
				// Clear any timestamps recorded by a preceding hop sharing req's
				// context (for example a synchronous auth token refresh or transport
				// retry) and stamp getConnTime when the GCS connection attempt starts.
				t := time.Now()
				getConnTime.Store(&t)
				wroteRequestTime.Store(nil)
				firstByteTime.Store(nil)
			},
			WroteRequest: func(info httptrace.WroteRequestInfo) {
				if info.Err == nil {
					t := time.Now()
					wroteRequestTime.Store(&t)
				} else {
					wroteRequestTime.Store(nil)
				}
			},
			GotFirstResponseByte: func() {
				t := time.Now()
				firstByteTime.Store(&t)
			},
		}
		if rt.metrics.dnsLookupDuration != nil || rt.metrics.tcpConnectDuration != nil || rt.metrics.tlsHandshakeDuration != nil {
			trace.DNSStart = func(info httptrace.DNSStartInfo) {
				dnsStart = time.Now()
			}
			trace.DNSDone = func(info httptrace.DNSDoneInfo) {
				if rt.metrics.dnsLookupDuration != nil {
					start := dnsStart
					if !start.IsZero() {
						duration := time.Since(start).Seconds()
						rt.metrics.dnsLookupDuration.Record(req.Context(), duration, netAttrs)
					}
				}
			}
			trace.ConnectStart = func(network, addr string) {
				tcpStarts.Store(addr, time.Now())
			}
			trace.ConnectDone = func(network, addr string, err error) {
				if err == nil && rt.metrics.tcpConnectDuration != nil {
					if startV, ok := tcpStarts.LoadAndDelete(addr); ok {
						duration := time.Since(startV.(time.Time)).Seconds()
						rt.metrics.tcpConnectDuration.Record(req.Context(), duration, netAttrs)
					}
				}
			}
			trace.TLSHandshakeStart = func() {
				tlsStart = time.Now()
			}
			trace.TLSHandshakeDone = func(state tls.ConnectionState, err error) {
				if err == nil && rt.metrics.tlsHandshakeDuration != nil {
					start := tlsStart
					if !start.IsZero() {
						duration := time.Since(start).Seconds()
						rt.metrics.tlsHandshakeDuration.Record(req.Context(), duration, netAttrs)
					}
				}
			}
		}
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	}

	startTime := time.Now()
	resp, err := rt.base.RoundTrip(req)

	statusCode := int64(0)
	if resp != nil {
		statusCode = int64(resp.StatusCode)
	}
	errorType := refineCancelled(req.Context(), computeErrorType(err, true, statusCode))

	if rt.metrics != nil {
		// Record attempt.
		attemptAttrs := []attribute.KeyValue{
			attribute.String("rpc.system.name", "http"),
			attribute.String(storageOperationAttr, logicalMethod),
			attribute.Int64("http.response.status_code", statusCode),
			attribute.String("server.address", host),
			attribute.String("error.type", errorType),
		}
		rt.metrics.attempts.Add(req.Context(), 1, metric.WithAttributes(attemptAttrs...))

		// Record error if failed.
		if err != nil || (resp != nil && resp.StatusCode >= 400) {
			errorAttrs := []attribute.KeyValue{
				attribute.String("rpc.system.name", "http"),
				attribute.String(storageOperationAttr, logicalMethod),
				attribute.String("server.address", host),
				attribute.String("error.type", errorType),
			}
			rt.metrics.errors.Add(req.Context(), 1, metric.WithAttributes(errorAttrs...))
		}

		rt.metrics.recordHTTPServerMetrics(req.Context(), resp, logicalMethod, host, errorType, rpcAttrs)

		// Record TTFB only when a server response was received. For object
		// payload uploads, measure from WroteRequest to GotFirstResponseByte so
		// TTFB excludes client upload transmission; for reads and metadata calls
		// (including those with a JSON body), measure from GetConn (excluding
		// preceding auth token refreshes) to GotFirstResponseByte.
		if err == nil && resp != nil {
			gc, fb, wr := getConnTime.Load(), firstByteTime.Load(), wroteRequestTime.Load()
			end := time.Now()
			if fb != nil {
				end = *fb
			}
			start := startTime
			if gc != nil {
				start = *gc
			}
			recordTTFB := true
			if isHTTPPayloadUpload(req, logicalMethod) {
				if wr != nil {
					start = *wr
				} else {
					// The server responded before the body was fully written (for
					// example an early 412), or WroteRequest never fired, so there
					// is no request-sent anchor and the elapsed time reflects upload
					// or framing rather than server TTFB.
					recordTTFB = false
				}
			}
			if recordTTFB {
				// WroteRequest runs in a deferred call on the transport's write
				// goroutine and can be stamped just after GotFirstResponseByte.
				ttfbSeconds := max(end.Sub(start).Seconds(), 0)
				rt.metrics.recordTTFB(req.Context(), "http", logicalMethod, host, ttfbSeconds)
			}
		}
	}

	if err != nil {
		if rt.metrics != nil {
			duration := time.Since(startTime).Seconds()
			rt.metrics.recordHTTP(req.Context(), req, nil, duration, err)
		}
		return nil, err
	}

	if resp.Body != nil {
		decrementActiveRequests = false
		resp.Body = &wrappedResponseBody{
			ReadCloser: resp.Body,
			startTime:  startTime,
			req:        req,
			resp:       resp,
			metrics:    rt.metrics,
		}
	} else {
		if rt.metrics != nil {
			duration := time.Since(startTime).Seconds()
			rt.metrics.recordHTTP(req.Context(), req, resp, duration, nil)
		}
	}
	return resp, nil
}

// isHTTPPayloadUpload reports whether req carries object data: a PUT or POST
// with a body that belongs to a write operation or targets the upload
// endpoint. The resumable session-initiation POST only carries JSON metadata,
// so it is treated like any other metadata call.
func isHTTPPayloadUpload(req *http.Request, logicalMethod string) bool {
	if (req.Method != http.MethodPut && req.Method != http.MethodPost) || req.Body == nil || req.Body == http.NoBody {
		return false
	}
	if logicalMethod != "WriteObject" && !strings.HasPrefix(req.URL.Path, "/upload/") {
		return false
	}
	q := req.URL.Query()
	return !(req.Method == http.MethodPost && q.Get("uploadType") == "resumable" && q.Get("upload_id") == "")
}

type wrappedResponseBody struct {
	io.ReadCloser
	startTime time.Time
	req       *http.Request
	resp      *http.Response
	metrics   *clientMetrics
	recorded  atomic.Bool
}

func (w *wrappedResponseBody) Read(p []byte) (n int, err error) {
	n, err = w.ReadCloser.Read(p)
	if err != nil {
		w.record(err)
	}
	return n, err
}

func (w *wrappedResponseBody) Close() error {
	err := w.ReadCloser.Close()
	w.record(err)
	return err
}

func (w *wrappedResponseBody) record(err error) {
	if w.recorded.CompareAndSwap(false, true) {
		duration := time.Since(w.startTime).Seconds()
		w.metrics.recordHTTP(w.req.Context(), w.req, w.resp, duration, err)

		if w.metrics.activeRequests != nil {
			logicalMethod := operationName(w.req.Context(), "Unknown")
			w.metrics.activeRequests.Add(w.req.Context(), -1, metric.WithAttributes(
				attribute.String(storageOperationAttr, logicalMethod),
				attribute.String("rpc.system.name", "http"),
				attribute.String("server.address", stripPort(w.req.URL.Host)),
			))
		}
	}
}

func getLogicalMethod(method string) string {
	if idx := strings.LastIndex(method, "/"); idx != -1 {
		method = method[idx+1:]
	}
	switch method {
	case "BidiReadObject":
		return "ReadObject"
	case "BidiWriteObject", "StartResumableWrite", "QueryWriteStatus", "CancelResumableWrite":
		return "WriteObject"
	}
	return method
}

// operationName returns the logical operation for ctx, falling back to the
// operation derived from fallback.
func operationName(ctx context.Context, fallback string) string {
	if state := metricsStateFromContext(ctx); state != nil && state.method != "" {
		return state.method
	}
	return getLogicalMethod(fallback)
}

// uploadIDHeader is set on every HTTP and gRPC response served by Cloud
// Storage via GFE (for all operations, not only uploads). metadata.MD.Get
// lowercases its key lookup automatically, and http.Header.Get canonicalizes it.
const uploadIDHeader = "X-GUploader-UploadID"

var (
	directPathIPv4Range = netip.MustParsePrefix("34.126.0.0/18")
	directPathIPv6Range = netip.MustParsePrefix("2001:4860:8040::/42")
)

func isDirectPathPeer(p *peer.Peer) bool {
	if p == nil || p.Addr == nil {
		return false
	}
	var ip netip.Addr
	if tcp, ok := p.Addr.(*net.TCPAddr); ok {
		ip, _ = netip.AddrFromSlice(tcp.IP)
	} else {
		ip, _ = netip.ParseAddr(stripPort(p.Addr.String()))
	}
	ip = ip.Unmap()
	return directPathIPv4Range.Contains(ip) || directPathIPv6Range.Contains(ip)
}

func isAmbiguousClientAbort(errorType string) bool {
	switch errorType {
	case "CANCELLED", "TIMEOUT", "DEADLINE_EXCEEDED":
		return true
	}
	return false
}

func isPreServerErrorType(errorType string) bool {
	switch errorType {
	case "AUTHENTICATION_ERROR", "TLS_FAILURE", "DNS_FAILURE", "CONNECTION_ERROR":
		return true
	}
	return false
}

// recordUnreached increments gcp.storage.client.server.unreached for an
// attempt that is positively known not to have reached a Cloud Storage server.
func (cm *clientMetrics) recordUnreached(ctx context.Context, system, method, host, errType string) {
	cm.serverUnreached.Add(ctx, 1, metric.WithAttributes(
		attribute.String(storageOperationAttr, method),
		attribute.String("rpc.system.name", system),
		attribute.String("server.address", host),
		attribute.String("error.type", errType),
	))
}

func (cm *clientMetrics) recordHTTPServerMetrics(ctx context.Context, resp *http.Response, logicalMethod, host, errorType string, rpcAttrs metric.MeasurementOption) {
	if cm.serverUnreached == nil {
		return
	}
	var gfeTime string
	var reached bool
	if resp != nil {
		gfeTime = resp.Header.Get("X-Goog-Gfe-Service-Time")
		reached = gfeTime != "" || resp.Header.Get(uploadIDHeader) != ""
	}
	if !reached {
		if resp == nil && isAmbiguousClientAbort(errorType) {
			return
		}
		cm.recordUnreached(ctx, "http", logicalMethod, host, errorType)
		return
	}
	if cm.serverDuration != nil && gfeTime != "" {
		if ms, err := strconv.ParseFloat(gfeTime, 64); err == nil {
			cm.serverDuration.Record(ctx, ms/1000.0, rpcAttrs)
		}
	}
}

// grpcServerElapsed decodes the server elapsed time from the
// grpc-server-stats-bin trailer: version byte 0, field ID 0, then the
// elapsed time in nanoseconds as a little-endian uint64.
func grpcServerElapsed(md metadata.MD) (time.Duration, bool) {
	vals := md.Get("grpc-server-stats-bin")
	if len(vals) == 0 {
		return 0, false
	}
	b := []byte(vals[0])
	if len(b) != 10 || b[0] != 0 || b[1] != 0 {
		return 0, false
	}
	return time.Duration(binary.LittleEndian.Uint64(b[2:])), true
}

func (cm *clientMetrics) recordServerMetrics(ctx context.Context, headerMD, trailerMD metadata.MD, err error, logicalMethod, target string, rpcAttrs metric.MeasurementOption, p *peer.Peer) {
	if cm.serverUnreached == nil {
		return
	}

	headerVals := headerMD.Get("x-goog-gfe-service-time")
	if len(headerVals) == 0 {
		headerVals = trailerMD.Get("x-goog-gfe-service-time")
	}
	var seconds float64
	var ok bool
	if len(headerVals) > 0 && headerVals[0] != "" {
		if ms, parseErr := strconv.ParseFloat(headerVals[0], 64); parseErr == nil {
			seconds, ok = ms/1000.0, true
		}
	}
	if !ok {
		if d, found := grpcServerElapsed(trailerMD); found {
			seconds, ok = d.Seconds(), true
		}
	}

	hasGFEUploadID := len(headerMD.Get(uploadIDHeader)) > 0 || len(trailerMD.Get(uploadIDHeader)) > 0
	directPath := isDirectPathPeer(p)
	var reached bool
	if directPath {
		reached = ok || len(headerMD) > 0 || len(trailerMD) > 0 || err == nil || errors.Is(err, io.EOF)
	} else {
		reached = ok || hasGFEUploadID
	}

	if !reached {
		// On DirectPath, an error without server metadata is only proven
		// unreached for pre-server transport/auth failures; timeouts,
		// cancellations, or bare UNAVAILABLE ("transport is closing") are
		// ambiguous and skipped. On CloudPath, GFE always attaches uploadIDHeader
		// when reached, so only client-side aborts/timeouts are ambiguous.
		errType := refineCancelled(ctx, computeErrorType(err, false, int64(status.Code(err))))
		if directPath {
			if !isPreServerErrorType(errType) {
				return
			}
		} else if isAmbiguousClientAbort(errType) {
			return
		}
		cm.recordUnreached(ctx, "grpc", logicalMethod, stripPort(target), errType)
		return
	}
	if ok && cm.serverDuration != nil {
		cm.serverDuration.Record(ctx, seconds, rpcAttrs)
	}
}

// metricsInterceptors returns gRPC client interceptors.
func metricsInterceptors(cm *clientMetrics) (grpc.UnaryClientInterceptor, grpc.StreamClientInterceptor) {
	unary := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		logicalMethod := operationName(ctx, method)

		target := ""
		if cc != nil {
			target = cc.Target()
		}
		if state := metricsStateFromContext(ctx); state != nil {
			state.setTarget(target)
			state.setRPCMethod(fullGRPCMethodName(method))
		}

		var rpcAttrs metric.MeasurementOption
		if cm.activeRequests != nil || cm.serverDuration != nil {
			rpcAttrs = metric.WithAttributes(
				attribute.String(storageOperationAttr, logicalMethod),
				attribute.String("rpc.system.name", "grpc"),
				attribute.String("server.address", stripPort(target)),
			)
		}

		if cm.activeRequests != nil {
			cm.activeRequests.Add(ctx, 1, rpcAttrs)
			defer cm.activeRequests.Add(ctx, -1, rpcAttrs)
		}

		var headerMD, trailerMD metadata.MD
		var p peer.Peer
		opts = append(opts, grpc.Header(&headerMD), grpc.Trailer(&trailerMD), grpc.Peer(&p))

		startTime := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)

		cm.recordServerMetrics(ctx, headerMD, trailerMD, err, logicalMethod, target, rpcAttrs, &p)

		responded := err == nil || len(headerMD) > 0 || len(trailerMD) > 0
		duration := time.Since(startTime).Seconds()
		errorType := cm.recordRPC(ctx, method, target, duration, err)
		// For unary calls that received a server response, record TTFB equal to
		// the attempt latency. Calls that failed with a transport, auth, timeout,
		// or cancel error after response headers arrived are skipped because their
		// duration reflects time to failure rather than first response.
		if responded && !isPreServerErrorType(errorType) && errorType != "TIMEOUT" && errorType != "CANCELLED" {
			cm.recordTTFB(ctx, "grpc", logicalMethod, target, duration)
		}
		return err
	}

	stream := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		logicalMethod := operationName(ctx, method)

		target := ""
		if cc != nil {
			target = cc.Target()
		}
		if state := metricsStateFromContext(ctx); state != nil {
			state.setTarget(target)
			state.setRPCMethod(fullGRPCMethodName(method))
		}

		var rpcAttrs metric.MeasurementOption
		if cm.activeRequests != nil || cm.serverDuration != nil {
			rpcAttrs = metric.WithAttributes(
				attribute.String(storageOperationAttr, logicalMethod),
				attribute.String("rpc.system.name", "grpc"),
				attribute.String("server.address", stripPort(target)),
			)
		}
		if cm.activeRequests != nil {
			cm.activeRequests.Add(ctx, 1, rpcAttrs)
		}

		startTime := time.Now()
		clientStream, err := streamer(ctx, desc, cc, method, opts...)

		if err != nil {
			if cm.activeRequests != nil {
				cm.activeRequests.Add(ctx, -1, rpcAttrs)
			}
			cm.recordServerMetrics(ctx, nil, nil, err, logicalMethod, target, rpcAttrs, nil)
			duration := time.Since(startTime).Seconds()
			cm.recordRPC(ctx, method, target, duration, err)
			return nil, err
		}

		return &wrappedClientStream{
			ClientStream:  clientStream,
			startTime:     startTime,
			method:        method,
			isWrite:       isWriteStreamMethod(method),
			target:        target,
			metrics:       cm,
			ctx:           ctx,
			serverStreams: desc.ServerStreams,
			clientStreams: desc.ClientStreams,
		}, nil
	}

	return unary, stream
}

type wrappedClientStream struct {
	grpc.ClientStream
	startTime        time.Time
	firstFlushSentAt atomic.Pointer[time.Time]
	method           string
	isWrite          bool
	target           string
	metrics          *clientMetrics
	ctx              context.Context
	recorded         atomic.Bool
	serverStreams    bool
	clientStreams    bool
	recordedTTFB     atomic.Bool
}

// isWriteStreamMethod reports whether method is a streaming object write RPC.
func isWriteStreamMethod(method string) bool {
	if idx := strings.LastIndex(method, "/"); idx != -1 {
		method = method[idx+1:]
	}
	return method == "WriteObject" || method == "BidiWriteObject"
}

// isWriteFlushMsg reports whether a write request elicits a server response:
// finish_write, state_lookup, or the opening message of an appendable
// BidiWriteObject stream (which always elicits an immediate server response,
// even when inline payload data is pipelined on the first message). Flush alone
// only persists data without eliciting a server response.
func isWriteFlushMsg(m interface{}) bool {
	switch req := m.(type) {
	case *storagepb.BidiWriteObjectRequest:
		if req.GetFinishWrite() || req.GetStateLookup() {
			return true
		}
		if req.GetAppendObjectSpec() != nil || req.GetWriteObjectSpec().GetAppendable() {
			return true
		}
	case *storagepb.WriteObjectRequest:
		if req.GetFinishWrite() {
			return true
		}
	}
	return false
}

func (w *wrappedClientStream) RecvMsg(m interface{}) error {
	err := w.ClientStream.RecvMsg(m)
	// Streams record TTFB on the first received message only. A stream that
	// ends with an error before any message has no first response to time.
	if err == nil {
		w.maybeRecordTTFB()
	}
	// For client-streaming streams (like WriteObject), the single successful RecvMsg call
	// returns the response and nil error, which marks the completion of the stream.
	isClientStreaming := !w.serverStreams && w.clientStreams
	if err != nil || isClientStreaming {
		w.record(err)
	}
	return err
}

// stampFirstFlush anchors write TTFB at the first request that elicits a
// server response. It stamps before send so a concurrent RecvMsg always sees an
// anchor, re-stamps once send returns, and clears the anchor if send fails.
// Later flushes never overwrite the first one. Note that SendMsg returns once
// the message is queued, so for requests that also carry data the anchor can
// include wire transmission of that message's payload.
func (w *wrappedClientStream) stampFirstFlush(send func() error) error {
	if !w.isWrite || w.recordedTTFB.Load() || w.firstFlushSentAt.Load() != nil {
		return send()
	}
	pre := time.Now()
	if !w.firstFlushSentAt.CompareAndSwap(nil, &pre) {
		return send()
	}
	err := send()
	if err != nil {
		w.firstFlushSentAt.CompareAndSwap(&pre, nil)
		return err
	}
	post := time.Now()
	w.firstFlushSentAt.CompareAndSwap(&pre, &post)
	return nil
}

func (w *wrappedClientStream) SendMsg(m interface{}) error {
	var err error
	if w.isWrite && isWriteFlushMsg(m) {
		err = w.stampFirstFlush(func() error { return w.ClientStream.SendMsg(m) })
	} else {
		err = w.ClientStream.SendMsg(m)
	}
	if err != nil {
		w.record(err)
	}
	return err
}

// CloseSend half-closes the stream, which also elicits the final response of a
// write that did not send finish_write.
func (w *wrappedClientStream) CloseSend() error {
	return w.stampFirstFlush(w.ClientStream.CloseSend)
}

func (w *wrappedClientStream) record(err error) {
	if w.recorded.CompareAndSwap(false, true) {
		duration := time.Since(w.startTime).Seconds()
		w.metrics.recordRPC(w.ctx, w.method, w.target, duration, err)

		logicalMethod := operationName(w.ctx, w.method)

		var rpcAttrs metric.MeasurementOption
		if w.metrics.activeRequests != nil || w.metrics.serverDuration != nil {
			rpcAttrs = metric.WithAttributes(
				attribute.String(storageOperationAttr, logicalMethod),
				attribute.String("rpc.system.name", "grpc"),
				attribute.String("server.address", stripPort(w.target)),
			)
		}

		if w.metrics.activeRequests != nil {
			w.metrics.activeRequests.Add(w.ctx, -1, rpcAttrs)
		}

		headerMD, _ := w.ClientStream.Header()
		trailerMD := w.ClientStream.Trailer()
		p, _ := peer.FromContext(w.ClientStream.Context())
		w.metrics.recordServerMetrics(w.ctx, headerMD, trailerMD, err, logicalMethod, w.target, rpcAttrs, p)
	}
}

func (w *wrappedClientStream) maybeRecordTTFB() {
	if w.recordedTTFB.Load() {
		return
	}

	// The first response from the server, whether it contains metadata,
	// persisted size, or actual content, indicates TTFB. For streaming writes,
	// measure from the first response-eliciting request (see isWriteFlushMsg and
	// CloseSend) so TTFB excludes client upload transmission and application
	// Write() intervals.
	if w.recordedTTFB.CompareAndSwap(false, true) {
		start := w.startTime
		if w.isWrite {
			flushPtr := w.firstFlushSentAt.Load()
			if flushPtr == nil {
				// The server responded before any response-eliciting write request
				// was sent (for example an early rejection while uploading chunks),
				// so elapsed time from stream open reflects client upload/idle time.
				return
			}
			start = *flushPtr
		}
		duration := time.Since(start).Seconds()
		logicalMethod := operationName(w.ctx, w.method)
		w.metrics.recordTTFB(w.ctx, "grpc", logicalMethod, w.target, duration)
	}
}

type metricsKey struct{}

type apiMethodKey struct{}

// withAPIMethod records the outermost public SDK entrypoint name in ctx if one
// has not already been recorded.
func withAPIMethod(ctx context.Context, name string) context.Context {
	if ctx.Value(apiMethodKey{}) == nil {
		return context.WithValue(ctx, apiMethodKey{}, name)
	}
	return ctx
}

// apiMethodNames maps legacy span names used on public API calls to the
// exported Type.Method names reported in gcp.client.method.
var apiMethodNames = map[string]string{
	"Object.Writer":                     "Object.NewWriter",
	"Object.WriterFromAppendableObject": "Object.NewWriterFromAppendableObject",
	"Object.Reader":                     "Object.NewRangeReader",
	"Object.MultiRangeDownloader":       "Object.NewMultiRangeDownloader",
	"storage.IAM.Get":                   "IAM.Policy",
	"storage.IAM.Set":                   "IAM.SetPolicy",
	"storage.IAM.Test":                  "IAM.TestPermissions",
}

// defaultClientMethods maps logical operations whose public entrypoints do not
// open an operation trace span to their exported Type.Method names.
var defaultClientMethods = map[string]string{
	"GetServiceAccount":         "Client.ServiceAccount",
	"ListBuckets":               "Client.Buckets",
	"LockBucketRetentionPolicy": "Bucket.LockRetentionPolicy",
	"ListObjects":               "Bucket.Objects",
	"RestoreObject":             "Object.Restore",
	"MoveObject":                "Object.Move",
	"GetHMACKey":                "HMACKey.Get",
	"DeleteHMACKey":             "HMACKey.Delete",
	"UpdateHMACKey":             "HMACKey.Update",
	"CreateHMACKey":             "Client.CreateHMACKey",
	"ListHMACKeys":              "Client.ListHMACKeys",
}

func injectAPIMethod(ctx context.Context, operation string, attrs []attribute.KeyValue) []attribute.KeyValue {
	apiMethod, _ := ctx.Value(apiMethodKey{}).(string)
	if mapped, ok := apiMethodNames[apiMethod]; ok {
		apiMethod = mapped
	} else if apiMethod == "" {
		apiMethod = defaultClientMethods[operation]
	}
	attrs = append(attrs, attribute.String("gcp.client.method", apiMethod))
	return slices.Clip(attrs)
}

type metricsState struct {
	target      atomic.Pointer[string]
	rpcMethod   atomic.Pointer[string]
	networkPath atomic.Int32
	method      string
	startTime   time.Time
	metrics     *clientMetrics
	isHTTP      bool
	record      func(error)
}

func (s *metricsState) setTarget(t string) {
	if s == nil {
		return
	}
	s.target.Store(&t)
}

const (
	networkPathUnknownKind int32 = iota
	networkPathDirectKind
	networkPathCloudKind
)

func (s *metricsState) setNetworkPath(path string) {
	if s == nil {
		return
	}
	switch path {
	case networkPathDirect:
		s.networkPath.Store(networkPathDirectKind)
	case networkPathCloud:
		s.networkPath.Store(networkPathCloudKind)
	}
}

func (s *metricsState) getNetworkPath() string {
	if s == nil {
		return networkPathUnknown
	}
	if s.isHTTP {
		return networkPathCloud
	}
	switch s.networkPath.Load() {
	case networkPathDirectKind:
		return networkPathDirect
	case networkPathCloudKind:
		return networkPathCloud
	default:
		return networkPathUnknown
	}
}

// setRPCMethod records the last underlying gRPC method executed by the operation.
func (s *metricsState) setRPCMethod(m string) {
	if s == nil {
		return
	}
	s.rpcMethod.Store(&m)
}

// getRPCMethod returns the last gRPC method executed by the operation, falling
// back to a v2-qualified name when no gRPC call ran (e.g. on HTTP).
func (s *metricsState) getRPCMethod() string {
	if s == nil {
		return ""
	}
	if p := s.rpcMethod.Load(); p != nil && *p != "" {
		return *p
	}
	return fullGRPCMethodName(s.method)
}

// bodySizeAttrs returns the attributes shared by the request and response
// body size metrics for the given logical operation.
func (s *metricsState) bodySizeAttrs(ctx context.Context, operation string) []attribute.KeyValue {
	return injectAPIMethod(ctx, operation, []attribute.KeyValue{
		attribute.String("rpc.system.name", s.getSystemName()),
		attribute.String(storageOperationAttr, operation),
		attribute.String("server.address", stripPort(s.getTarget())),
	})
}

func (s *metricsState) getSystemName() string {
	if s == nil {
		return ""
	}
	if s.isHTTP {
		return "http"
	}
	return "grpc"
}

func (s *metricsState) getTarget() string {
	if s == nil {
		return ""
	}
	if p := s.target.Load(); p != nil {
		return *p
	}
	return ""
}

func contextWithMetricsState(ctx context.Context, state *metricsState) context.Context {
	return context.WithValue(ctx, metricsKey{}, state)
}

func metricsStateFromContext(ctx context.Context) *metricsState {
	if ctx == nil {
		return nil
	}
	if state, ok := ctx.Value(metricsKey{}).(*metricsState); ok {
		return state
	}
	return nil
}

func contextWithoutMetrics(ctx context.Context) context.Context {
	if ctx == nil {
		return nil
	}
	return context.WithValue(ctx, metricsKey{}, (*metricsState)(nil))
}

func (cm *clientMetrics) startOperation(ctx context.Context, method string, isHTTP bool) (context.Context, func(error)) {
	if cm == nil {
		return ctx, func(error) {}
	}
	state := &metricsState{
		method:    method,
		startTime: time.Now(),
		metrics:   cm,
		isHTTP:    isHTTP,
	}

	var recordOnce sync.Once
	record := func(err error) {
		recordOnce.Do(func() {
			duration := time.Since(state.startTime).Seconds()
			errorType := computeErrorType(err, isHTTP, 0)

			opAttrs := injectAPIMethod(ctx, method, []attribute.KeyValue{
				attribute.String("rpc.system.name", state.getSystemName()),
				attribute.String(storageOperationAttr, method),
				attribute.String("server.address", stripPort(state.getTarget())),
				attribute.String("error.type", errorType),
			})
			cm.operations.Add(ctx, 1, metric.WithAttributes(opAttrs...))

			durAttrs := append(opAttrs, attribute.String("rpc.method", state.getRPCMethod()))
			cm.duration.Record(ctx, duration, metric.WithAttributes(durAttrs...))
		})
	}
	state.record = record

	ctx = contextWithMetricsState(ctx, state)
	return ctx, record
}

// startMetricsOp starts a client operation if OpenTelemetry metrics are enabled in ctx.
// It returns the updated context containing metrics state and a recording closure.
func startMetricsOp(ctx context.Context, method string, isHTTP bool) (context.Context, func(error)) {
	if state := metricsStateFromContext(ctx); state != nil && state.metrics != nil {
		return state.metrics.startOperation(ctx, method, isHTTP)
	}
	return ctx, func(error) {}
}

// initClientMetrics initializes OpenTelemetry client metrics if enabled in config.
// It returns the metrics instance and its cleanup function, or nil if disabled or upon error.
func initClientMetrics(ctx context.Context, project string, config *storageConfig) (*clientMetrics, func()) {
	if !isOtelMetricsEnabled(config) && !isOtelDebugMetricsEnabled(config) {
		return nil, nil
	}
	cm, cleanup, err := initMetrics(ctx, project, config)
	if err != nil {
		log.Printf("Failed to enable metrics: %v", err)
		return nil, nil
	}
	return cm, cleanup
}

// metricsStorageClient wraps a storageClient and records client-level metrics.
type metricsStorageClient struct {
	storageClient
	metrics *clientMetrics
	isHTTP  bool
}

func (mc *metricsStorageClient) GetServiceAccount(ctx context.Context, project string, opts ...storageOption) (string, error) {
	ctx, record := mc.metrics.startOperation(ctx, "GetServiceAccount", mc.isHTTP)
	res, err := mc.storageClient.GetServiceAccount(ctx, project, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) CreateBucket(ctx context.Context, project, bucket string, attrs *BucketAttrs, enableObjectRetention *bool, opts ...storageOption) (*BucketAttrs, error) {
	ctx, record := mc.metrics.startOperation(ctx, "CreateBucket", mc.isHTTP)
	res, err := mc.storageClient.CreateBucket(ctx, project, bucket, attrs, enableObjectRetention, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) ListBuckets(ctx context.Context, project string, opts ...storageOption) *BucketIterator {
	ctx, _ = mc.metrics.startOperation(ctx, "ListBuckets", mc.isHTTP)
	return mc.storageClient.ListBuckets(ctx, project, opts...)
}

func (mc *metricsStorageClient) DeleteBucket(ctx context.Context, bucket string, conds *BucketConditions, opts ...storageOption) error {
	ctx, record := mc.metrics.startOperation(ctx, "DeleteBucket", mc.isHTTP)
	err := mc.storageClient.DeleteBucket(ctx, bucket, conds, opts...)
	record(err)
	return err
}

func (mc *metricsStorageClient) GetBucket(ctx context.Context, bucket string, conds *BucketConditions, opts ...storageOption) (*BucketAttrs, error) {
	ctx, record := mc.metrics.startOperation(ctx, "GetBucket", mc.isHTTP)
	res, err := mc.storageClient.GetBucket(ctx, bucket, conds, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) UpdateBucket(ctx context.Context, bucket string, uattrs *BucketAttrsToUpdate, conds *BucketConditions, opts ...storageOption) (*BucketAttrs, error) {
	ctx, record := mc.metrics.startOperation(ctx, "UpdateBucket", mc.isHTTP)
	res, err := mc.storageClient.UpdateBucket(ctx, bucket, uattrs, conds, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) LockBucketRetentionPolicy(ctx context.Context, bucket string, conds *BucketConditions, opts ...storageOption) error {
	ctx, record := mc.metrics.startOperation(ctx, "LockBucketRetentionPolicy", mc.isHTTP)
	err := mc.storageClient.LockBucketRetentionPolicy(ctx, bucket, conds, opts...)
	record(err)
	return err
}

func (mc *metricsStorageClient) ListObjects(ctx context.Context, bucket string, q *Query, opts ...storageOption) *ObjectIterator {
	ctx, _ = mc.metrics.startOperation(ctx, "ListObjects", mc.isHTTP)
	return mc.storageClient.ListObjects(ctx, bucket, q, opts...)
}

func (mc *metricsStorageClient) DeleteObject(ctx context.Context, bucket, object string, gen int64, conds *Conditions, opts ...storageOption) error {
	ctx, record := mc.metrics.startOperation(ctx, "DeleteObject", mc.isHTTP)
	err := mc.storageClient.DeleteObject(ctx, bucket, object, gen, conds, opts...)
	record(err)
	return err
}

func (mc *metricsStorageClient) GetObject(ctx context.Context, params *getObjectParams, opts ...storageOption) (*ObjectAttrs, error) {
	ctx, record := mc.metrics.startOperation(ctx, "GetObject", mc.isHTTP)
	res, err := mc.storageClient.GetObject(ctx, params, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) UpdateObject(ctx context.Context, params *updateObjectParams, opts ...storageOption) (*ObjectAttrs, error) {
	ctx, record := mc.metrics.startOperation(ctx, "UpdateObject", mc.isHTTP)
	res, err := mc.storageClient.UpdateObject(ctx, params, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) RestoreObject(ctx context.Context, params *restoreObjectParams, opts ...storageOption) (*ObjectAttrs, error) {
	ctx, record := mc.metrics.startOperation(ctx, "RestoreObject", mc.isHTTP)
	res, err := mc.storageClient.RestoreObject(ctx, params, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) MoveObject(ctx context.Context, params *moveObjectParams, opts ...storageOption) (*ObjectAttrs, error) {
	ctx, record := mc.metrics.startOperation(ctx, "MoveObject", mc.isHTTP)
	res, err := mc.storageClient.MoveObject(ctx, params, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) ComposeObject(ctx context.Context, req *composeObjectRequest, opts ...storageOption) (*ObjectAttrs, error) {
	ctx, record := mc.metrics.startOperation(ctx, "ComposeObject", mc.isHTTP)
	res, err := mc.storageClient.ComposeObject(ctx, req, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) RewriteObject(ctx context.Context, req *rewriteObjectRequest, opts ...storageOption) (*rewriteObjectResponse, error) {
	ctx, record := mc.metrics.startOperation(ctx, "RewriteObject", mc.isHTTP)
	res, err := mc.storageClient.RewriteObject(ctx, req, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) NewRangeReader(ctx context.Context, params *newRangeReaderParams, opts ...storageOption) (*Reader, error) {
	ctx, record := mc.metrics.startOperation(ctx, "ReadObject", mc.isHTTP)
	r, err := mc.storageClient.NewRangeReader(ctx, params, opts...)
	if err != nil {
		record(err)
		return nil, err
	}
	if state := metricsStateFromContext(ctx); state != nil {
		r.metricsState = state
	}
	return r, nil
}

func (mc *metricsStorageClient) OpenWriter(params *openWriterParams, opts ...storageOption) (internalWriter, error) {
	ctx, _ := mc.metrics.startOperation(params.ctx, "WriteObject", mc.isHTTP)
	params.ctx = ctx
	return mc.storageClient.OpenWriter(params, opts...)
}

func (mc *metricsStorageClient) NewMultiRangeDownloader(ctx context.Context, params *newMultiRangeDownloaderParams, opts ...storageOption) (*MultiRangeDownloader, error) {
	ctx, _ = mc.metrics.startOperation(ctx, "ReadObject", mc.isHTTP)
	return mc.storageClient.NewMultiRangeDownloader(ctx, params, opts...)
}

func (mc *metricsStorageClient) GetIamPolicy(ctx context.Context, resource string, version int32, opts ...storageOption) (*iampb.Policy, error) {
	ctx, record := mc.metrics.startOperation(ctx, "GetIamPolicy", mc.isHTTP)
	res, err := mc.storageClient.GetIamPolicy(ctx, resource, version, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) SetIamPolicy(ctx context.Context, resource string, policy *iampb.Policy, opts ...storageOption) error {
	ctx, record := mc.metrics.startOperation(ctx, "SetIamPolicy", mc.isHTTP)
	err := mc.storageClient.SetIamPolicy(ctx, resource, policy, opts...)
	record(err)
	return err
}

func (mc *metricsStorageClient) TestIamPermissions(ctx context.Context, resource string, permissions []string, opts ...storageOption) ([]string, error) {
	ctx, record := mc.metrics.startOperation(ctx, "TestIamPermissions", mc.isHTTP)
	res, err := mc.storageClient.TestIamPermissions(ctx, resource, permissions, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) GetHMACKey(ctx context.Context, project, accessID string, opts ...storageOption) (*HMACKey, error) {
	ctx, record := mc.metrics.startOperation(ctx, "GetHMACKey", mc.isHTTP)
	res, err := mc.storageClient.GetHMACKey(ctx, project, accessID, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) ListHMACKeys(ctx context.Context, project string, serviceAccountEmail string, showDeletedKeys bool, opts ...storageOption) *HMACKeysIterator {
	ctx, _ = mc.metrics.startOperation(ctx, "ListHMACKeys", mc.isHTTP)
	return mc.storageClient.ListHMACKeys(ctx, project, serviceAccountEmail, showDeletedKeys, opts...)
}

func (mc *metricsStorageClient) UpdateHMACKey(ctx context.Context, project, serviceAccountEmail, accessID string, attrs *HMACKeyAttrsToUpdate, opts ...storageOption) (*HMACKey, error) {
	ctx, record := mc.metrics.startOperation(ctx, "UpdateHMACKey", mc.isHTTP)
	res, err := mc.storageClient.UpdateHMACKey(ctx, project, serviceAccountEmail, accessID, attrs, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) CreateHMACKey(ctx context.Context, project, serviceAccountEmail string, opts ...storageOption) (*HMACKey, error) {
	ctx, record := mc.metrics.startOperation(ctx, "CreateHMACKey", mc.isHTTP)
	res, err := mc.storageClient.CreateHMACKey(ctx, project, serviceAccountEmail, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) DeleteHMACKey(ctx context.Context, project, accessID string, opts ...storageOption) error {
	ctx, record := mc.metrics.startOperation(ctx, "DeleteHMACKey", mc.isHTTP)
	err := mc.storageClient.DeleteHMACKey(ctx, project, accessID, opts...)
	record(err)
	return err
}

func (mc *metricsStorageClient) ListNotifications(ctx context.Context, bucket string, opts ...storageOption) (map[string]*Notification, error) {
	ctx, record := mc.metrics.startOperation(ctx, "ListNotifications", mc.isHTTP)
	res, err := mc.storageClient.ListNotifications(ctx, bucket, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) CreateNotification(ctx context.Context, bucket string, n *Notification, opts ...storageOption) (*Notification, error) {
	ctx, record := mc.metrics.startOperation(ctx, "CreateNotification", mc.isHTTP)
	res, err := mc.storageClient.CreateNotification(ctx, bucket, n, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) DeleteNotification(ctx context.Context, bucket string, id string, opts ...storageOption) error {
	ctx, record := mc.metrics.startOperation(ctx, "DeleteNotification", mc.isHTTP)
	err := mc.storageClient.DeleteNotification(ctx, bucket, id, opts...)
	record(err)
	return err
}

func (mc *metricsStorageClient) DeleteDefaultObjectACL(ctx context.Context, bucket string, entity ACLEntity, opts ...storageOption) error {
	ctx, record := mc.metrics.startOperation(ctx, "DeleteDefaultObjectACL", mc.isHTTP)
	err := mc.storageClient.DeleteDefaultObjectACL(ctx, bucket, entity, opts...)
	record(err)
	return err
}

func (mc *metricsStorageClient) ListDefaultObjectACLs(ctx context.Context, bucket string, opts ...storageOption) ([]ACLRule, error) {
	ctx, record := mc.metrics.startOperation(ctx, "ListDefaultObjectACLs", mc.isHTTP)
	res, err := mc.storageClient.ListDefaultObjectACLs(ctx, bucket, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) UpdateDefaultObjectACL(ctx context.Context, bucket string, entity ACLEntity, role ACLRole, opts ...storageOption) error {
	ctx, record := mc.metrics.startOperation(ctx, "UpdateDefaultObjectACL", mc.isHTTP)
	err := mc.storageClient.UpdateDefaultObjectACL(ctx, bucket, entity, role, opts...)
	record(err)
	return err
}

func (mc *metricsStorageClient) DeleteBucketACL(ctx context.Context, bucket string, entity ACLEntity, opts ...storageOption) error {
	ctx, record := mc.metrics.startOperation(ctx, "DeleteBucketACL", mc.isHTTP)
	err := mc.storageClient.DeleteBucketACL(ctx, bucket, entity, opts...)
	record(err)
	return err
}

func (mc *metricsStorageClient) ListBucketACLs(ctx context.Context, bucket string, opts ...storageOption) ([]ACLRule, error) {
	ctx, record := mc.metrics.startOperation(ctx, "ListBucketACLs", mc.isHTTP)
	res, err := mc.storageClient.ListBucketACLs(ctx, bucket, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) UpdateBucketACL(ctx context.Context, bucket string, entity ACLEntity, role ACLRole, opts ...storageOption) error {
	ctx, record := mc.metrics.startOperation(ctx, "UpdateBucketACL", mc.isHTTP)
	err := mc.storageClient.UpdateBucketACL(ctx, bucket, entity, role, opts...)
	record(err)
	return err
}

func (mc *metricsStorageClient) DeleteObjectACL(ctx context.Context, bucket, object string, entity ACLEntity, opts ...storageOption) error {
	ctx, record := mc.metrics.startOperation(ctx, "DeleteObjectACL", mc.isHTTP)
	err := mc.storageClient.DeleteObjectACL(ctx, bucket, object, entity, opts...)
	record(err)
	return err
}

func (mc *metricsStorageClient) ListObjectACLs(ctx context.Context, bucket, object string, opts ...storageOption) ([]ACLRule, error) {
	ctx, record := mc.metrics.startOperation(ctx, "ListObjectACLs", mc.isHTTP)
	res, err := mc.storageClient.ListObjectACLs(ctx, bucket, object, opts...)
	record(err)
	return res, err
}

func (mc *metricsStorageClient) UpdateObjectACL(ctx context.Context, bucket, object string, entity ACLEntity, role ACLRole, opts ...storageOption) error {
	ctx, record := mc.metrics.startOperation(ctx, "UpdateObjectACL", mc.isHTTP)
	err := mc.storageClient.UpdateObjectACL(ctx, bucket, object, entity, role, opts...)
	record(err)
	return err
}

func (mc *metricsStorageClient) Close() error {
	return mc.storageClient.Close()
}

func (mc *metricsStorageClient) fetchBucketMetadata(ctx context.Context, bucket string) (string, string, error) {
	return mc.storageClient.fetchBucketMetadata(ctx, bucket)
}

type dialInfo struct {
	doneTime time.Time
	host     string
}

type dialDoneContextKey struct{}

// grpcMetricsStatsHandler implements stats.Handler to capture TLS handshake duration.
type grpcMetricsStatsHandler struct {
	metrics   *clientMetrics
	dialTimes *sync.Map
	host      string
}

type contextKeyRPCTag struct{}

func (h *grpcMetricsStatsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	method := info.FullMethodName
	if idx := strings.LastIndex(method, "/"); idx != -1 {
		method = method[idx+1:]
	}
	return context.WithValue(ctx, contextKeyRPCTag{}, method)
}
func (h *grpcMetricsStatsHandler) HandleRPC(ctx context.Context, s stats.RPCStats) {
	if h.metrics == nil {
		return
	}
	method := ""
	if v := ctx.Value(contextKeyRPCTag{}); v != nil {
		method = v.(string)
	}
	attrs := metric.WithAttributes(
		attribute.String("rpc.system.name", "grpc"),
		attribute.String("rpc.method", method),
		attribute.String("server.address", h.host),
	)

	switch st := s.(type) {
	case *stats.OutHeader:
		if st.RemoteAddr != nil {
			metricsStateFromContext(ctx).setNetworkPath(networkPathOfPeer(&peer.Peer{Addr: st.RemoteAddr}))
		}
	case *stats.InPayload:
		if h.metrics.networkBytesReceived != nil {
			h.metrics.networkBytesReceived.Add(ctx, int64(st.WireLength), attrs)
		}
	case *stats.OutPayload:
		if h.metrics.networkBytesSent != nil {
			h.metrics.networkBytesSent.Add(ctx, int64(st.WireLength), attrs)
		}
	}
}
func (h *grpcMetricsStatsHandler) TagConn(ctx context.Context, info *stats.ConnTagInfo) context.Context {
	if info.LocalAddr != nil && h.dialTimes != nil {
		if val, ok := h.dialTimes.LoadAndDelete(info.LocalAddr.String()); ok {
			return context.WithValue(ctx, dialDoneContextKey{}, val)
		}
	}
	return ctx
}
func (h *grpcMetricsStatsHandler) HandleConn(ctx context.Context, s stats.ConnStats) {
	if _, ok := s.(*stats.ConnBegin); ok {
		if val := ctx.Value(dialDoneContextKey{}); val != nil {
			info := val.(dialInfo)
			if h.metrics != nil && h.metrics.tlsHandshakeDuration != nil {
				duration := time.Since(info.doneTime).Seconds()
				h.metrics.tlsHandshakeDuration.Record(context.Background(), duration, metric.WithAttributes(
					attribute.String("rpc.system.name", "grpc"),
					attribute.String("server.address", info.host),
				))
			}
		}
	}
}

var (
	isDefaultDNSLookupHost = true
	dnsLookupHost          = func(ctx context.Context, host string) ([]string, error) {
		return net.DefaultResolver.LookupHost(ctx, host)
	}
	dnsNow = time.Now
)

// grpcNetworkMetricsDialOptions returns dial options that instrument DNS, TCP and TLS handshake metrics.
func grpcNetworkMetricsDialOptions(host string, metrics *clientMetrics) []option.ClientOption {
	dialOpts := grpcNetworkMetricsRawDialOptions(host, metrics)
	if len(dialOpts) == 0 {
		return nil
	}
	opts := make([]option.ClientOption, 0, len(dialOpts))
	for _, do := range dialOpts {
		opts = append(opts, option.WithGRPCDialOption(do))
	}
	return opts
}

func grpcNetworkMetricsRawDialOptions(host string, metrics *clientMetrics) []grpc.DialOption {
	if metrics == nil || (metrics.dnsLookupDuration == nil && metrics.tcpConnectDuration == nil && metrics.tlsHandshakeDuration == nil && metrics.networkBytesSent == nil && metrics.networkBytesReceived == nil) {
		return nil
	}
	cleanHost := stripPort(host)
	var dialTimes sync.Map
	var lastDNSLookup atomic.Pointer[time.Time]

	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		// On CloudPath (non-DirectPath IP), sample DNS lookup duration for the
		// configured host asynchronously with a 60s cooldown so connection
		// bursts do not re-resolve on every socket dial and DNS probes never
		// delay the caller's TCP dial. Skip loopback dials when dnsLookupHost
		// is the default resolver so local test servers do not trigger external
		// DNS lookups.
		if metrics.dnsLookupDuration != nil && !isDirectPathAddr(addr) && cleanHost != "" && net.ParseIP(cleanHost) == nil && (!parseHostIP(addr).IsLoopback() || !isDefaultDNSLookupHost) {
			now := dnsNow()
			prev := lastDNSLookup.Load()
			if (prev == nil || now.Sub(*prev) >= time.Minute) && lastDNSLookup.CompareAndSwap(prev, &now) {
				go func() {
					lookupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					dnsStart := time.Now()
					if _, err := dnsLookupHost(lookupCtx, cleanHost); err == nil {
						metrics.dnsLookupDuration.Record(lookupCtx, time.Since(dnsStart).Seconds(), metric.WithAttributes(
							attribute.String("rpc.system.name", "grpc"),
							attribute.String("server.address", cleanHost),
						))
					}
				}()
			}
		}

		var netAttrs metric.MeasurementOption
		if metrics.tcpConnectDuration != nil {
			netAttrs = metric.WithAttributes(
				attribute.String("rpc.system.name", "grpc"),
				attribute.String("server.address", cleanHost),
			)
		}

		tcpStart := time.Now()
		var d net.Dialer
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err == nil {
			if metrics.tcpConnectDuration != nil {
				tcpDuration := time.Since(tcpStart).Seconds()
				metrics.tcpConnectDuration.Record(ctx, tcpDuration, netAttrs)
			}
			if conn != nil && metrics.tlsHandshakeDuration != nil {
				dialTimes.Store(conn.LocalAddr().String(), dialInfo{doneTime: time.Now(), host: cleanHost})
			}
		}

		return conn, err
	}

	sh := &grpcMetricsStatsHandler{
		metrics:   metrics,
		dialTimes: &dialTimes,
		host:      cleanHost,
	}

	return []grpc.DialOption{
		grpc.WithContextDialer(dialer),
		grpc.WithStatsHandler(sh),
	}
}

// metricsTokenProvider wraps an auth.TokenProvider and records
// gcp.storage.client.auth.credential_refresh.duration whenever a token fetch
// returns a new token or an error (ignoring repeated cached reads of the same
// token). Note: if the underlying TokenProvider refreshes tokens asynchronously
// in the background, the next Token() call returns the newly cached token in
// ~0s, so that rotation is recorded here with the fast cache-read duration
// rather than the background network latency.
type metricsTokenProvider struct {
	base    auth.TokenProvider
	metrics *clientMetrics
	last    atomic.Pointer[auth.Token]
}

func (p *metricsTokenProvider) Token(ctx context.Context) (*auth.Token, error) {
	if p.metrics == nil || p.metrics.credentialRefreshDuration == nil {
		tok, err := p.base.Token(ctx)
		if err != nil {
			return tok, &credentialError{err: err}
		}
		return tok, nil
	}
	start := time.Now()
	tok, err := p.base.Token(ctx)
	if err != nil {
		p.metrics.recordCredentialRefreshDuration(ctx, time.Since(start), err)
		return tok, &credentialError{err: err}
	}
	prev := p.last.Load()
	if (prev == nil || tok == nil || tok.Value != prev.Value || !tok.Expiry.Equal(prev.Expiry)) && (tok == nil || p.last.CompareAndSwap(prev, tok)) {
		p.metrics.recordCredentialRefreshDuration(ctx, time.Since(start), nil)
	}
	return tok, nil
}

// wrapAuthCredentials wraps an auth.Credentials object to track credential refresh durations.
// Note: We deliberately do not wrap legacy golang.org/x/oauth2/google.Credentials because
// it embeds unexported fields (e.g. universeDomain and its internal Mutex). Wrapping or copying
// it would either trigger go vet lock-copying errors or silently drop those unexported fields,
// which breaks Universe Domain resolution for legacy users.
func wrapAuthCredentials(c *auth.Credentials, m *clientMetrics) *auth.Credentials {
	if c == nil || c.TokenProvider == nil {
		return c
	}
	if mtp, ok := c.TokenProvider.(*metricsTokenProvider); ok {
		if m == nil || mtp.metrics == m {
			return c
		}
		clone := *c
		clone.TokenProvider = &metricsTokenProvider{base: mtp.base, metrics: m}
		return &clone
	}
	clone := *c
	clone.TokenProvider = &metricsTokenProvider{base: c.TokenProvider, metrics: m}
	return &clone
}

func (cm *clientMetrics) recordCredentialRefreshDuration(ctx context.Context, duration time.Duration, err error) {
	if cm == nil || cm.credentialRefreshDuration == nil {
		return
	}
	errorType := "OK"
	if err != nil {
		errorType = "AUTHENTICATION_ERROR"
		if errors.Is(err, context.Canceled) {
			errorType = "CANCELLED"
		} else if errors.Is(err, context.DeadlineExceeded) {
			errorType = "TIMEOUT"
		}
	}
	attrs := []attribute.KeyValue{attribute.String("error.type", errorType)}
	cm.credentialRefreshDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

func fullGRPCMethodName(method string) string {
	method = strings.TrimPrefix(method, "/")
	if strings.Contains(method, "/") {
		return method
	}
	return "google.storage.v2.Storage/" + method
}

func (cm *clientMetrics) recordStallDuration(ctx context.Context, duration time.Duration, method string, systemName string, target string) {
	if cm == nil || cm.stallDuration == nil {
		return
	}
	state := metricsStateFromContext(ctx)
	operation := operationName(ctx, method)
	rpcMethod := fullGRPCMethodName(method)
	networkPath := networkPathCloud
	if systemName == "grpc" {
		networkPath = state.getNetworkPath()
	}
	attrs := []attribute.KeyValue{
		attribute.String("rpc.system.name", systemName),
		attribute.String(storageOperationAttr, operation),
		attribute.String("rpc.method", rpcMethod),
		attribute.String("server.address", target),
		attribute.String(networkPathAttr, networkPath),
	}
	cm.stallDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}
