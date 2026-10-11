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
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"cloud.google.com/go/auth"
	gcemetadata "cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/iam/apiv1/iampb"
	"cloud.google.com/go/storage/internal"
	mexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
)

const (
	customMetricPrefix = "custom.googleapis.com/"
)

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
	gfeHeaderMissing          metric.Int64Counter
	dnsLookupDuration         metric.Float64Histogram
	tcpConnectDuration        metric.Float64Histogram
	tlsHandshakeDuration      metric.Float64Histogram
	gfeDuration               metric.Float64Histogram
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

// newMetricsGCMExporter creates a Google Cloud Monitoring exporter.
func newMetricsGCMExporter(ctx context.Context, projectID string) (sdkmetric.Exporter, error) {
	exporter, err := mexporter.New(
		mexporter.WithProjectID(projectID),
		mexporter.WithMetricDescriptorTypeFormatter(func(m metricdata.Metrics) string {
			return formatMetricWithPrefix(m, customMetricPrefix)
		}),
		// The OTel GCP exporter drops any resource attributes that don't map to the
		// target MonitoredResource (currently generic_node).
		// We use WithFilteredResourceAttributes returning true to ensure that ANY
		// resource attributes not in the MonitoredResource schema (like gcp.client.*,
		// or gcp detector attributes not supported by generic_node) are preserved
		// as metric labels instead of being dropped.
		//
		// TODO: When storage_client node is allowlisted in Monarch
		// (google3/configs/monitoring/cloud_pulse_monarch/storage/storage_client.proto),
		// we should uncomment the following options to export to the custom resource:
		// mexporter.WithCreateServiceTimeSeries(),
		// mexporter.WithMonitoredResourceDescription("storage.googleapis.com/Client", []string{"project_id", "location", "cloud_platform", "host_id", "instance_id", "api"}),
		mexporter.WithFilteredResourceAttributes(func(kv attribute.KeyValue) bool {
			key := string(kv.Key)
			// Keep our custom gcp.client.* attributes as labels
			if strings.HasPrefix(key, "gcp.client.") {
				return true
			}
			// Keep attributes that can later be mapped to the storage.googleapis.com/Client
			// MonitoredResource schema in Monarch.
			switch key {
			case "cloud.platform",
				"cloud.region",
				"cloud.availability_zone",
				"host.id":
				return true
			}
			// Drop the rest to avoid hitting the Cloud Monitoring label limit.
			return false
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("storage: creating GCM exporter: %w", err)
	}
	return exporter, nil
}

// detectMetricsProjectID returns the project to export metrics to when it
// could not be derived from the client credentials. It checks the
// GOOGLE_CLOUD_PROJECT environment variable and then the GCE metadata server.
func detectMetricsProjectID(ctx context.Context) string {
	if p := os.Getenv("GOOGLE_CLOUD_PROJECT"); p != "" {
		return p
	}
	if gcemetadata.OnGCE() {
		if p, err := gcemetadata.ProjectIDWithContext(ctx); err == nil {
			return p
		}
	}
	return ""
}

// initMetrics initializes clientMetrics with a meter provider and registered exporter.
func initMetrics(ctx context.Context, projectID string, config *storageConfig) (*clientMetrics, func(), error) {
	var provider *sdkmetric.MeterProvider
	var ownProvider bool

	if config.meterProvider != nil {
		provider = config.meterProvider
	} else {
		var exporter sdkmetric.Exporter
		var err error
		if config.metricExporter != nil {
			exporter = *config.metricExporter
		} else {
			if projectID == "" {
				projectID = detectMetricsProjectID(ctx)
			}
			if projectID == "" {
				return nil, nil, errors.New("storage: client metrics are enabled but the project ID for Cloud Monitoring export could not be determined " +
					"(for example when using option.WithHTTPClient or credentials without a project); set GOOGLE_CLOUD_PROJECT or " +
					"provide experimental.WithMetricExporter or experimental.WithMeterProvider")
			}
			exporter, err = newMetricsGCMExporter(ctx, projectID)
			if err != nil {
				return nil, nil, err
			}
		}

		interval := time.Minute
		if config.metricInterval > 0 {
			interval = config.metricInterval
		}

		reader := sdkmetric.NewPeriodicReader(&exporterLogSuppressor{Exporter: exporter}, sdkmetric.WithInterval(interval))

		// Static common attributes are defined as Resource Attributes.
		res, err := resource.New(ctx,
			resource.WithDetectors(gcp.NewDetector()),
			resource.WithAttributes(
				attribute.String("gcp.client.version", internal.Version),
				attribute.String("gcp.client.service", "storage"),
				attribute.String("gcp.client.artifact", "cloud.google.com/go/storage"),
			),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("storage: creating metrics resource: %w", err)
		}

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
					sdkmetric.Instrument{Name: "gcp.storage.client.operation.ttfb", Kind: sdkmetric.InstrumentKindHistogram},
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
					sdkmetric.Instrument{Name: "gcp.storage.client.gfe.duration", Kind: sdkmetric.InstrumentKindHistogram},
					sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: latencyHistogramBoundaries()}},
				),
				sdkmetric.NewView(
					sdkmetric.Instrument{Name: "gcp.storage.client.auth.credential_refresh.duration", Kind: sdkmetric.InstrumentKindHistogram},
					sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: latencyHistogramBoundaries()}},
				),
			),
		)
		ownProvider = true
	}

	meter := provider.Meter("cloud.google.com/go/storage")

	rpcDuration, err := meter.Float64Histogram(
		"rpc.client.call.duration",
		metric.WithDescription("Duration of one gRPC request. Retries not included (Otel)"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, nil, err
	}

	httpDuration, err := meter.Float64Histogram(
		"http.client.request.duration",
		metric.WithDescription("Duration of one HTTP client request. Retried not included (Otel)"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, nil, err
	}

	duration, err := meter.Float64Histogram(
		"gcp.client.request.duration",
		metric.WithDescription("Latency of a client operation"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, nil, err
	}

	operations, err := meter.Int64Counter(
		"gcp.storage.client.operations",
		metric.WithDescription("Number of GCS client operations"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, err
	}

	attempts, err := meter.Int64Counter(
		"gcp.storage.client.attempts",
		metric.WithDescription("Number of GCS client attempts (individual HTTP requests or gRPC calls), including retries, resumable upload chunks and list pages."),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, err
	}

	requestBodySize, err := meter.Int64Histogram(
		"gcp.storage.client.request.body.size",
		metric.WithDescription("Number of object bytes written by an upload operation (recorded once per operation, including empty objects)."),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, nil, err
	}

	responseBodySize, err := meter.Int64Histogram(
		"gcp.storage.client.response.body.size",
		metric.WithDescription("Number of object bytes delivered to the application by a download operation (recorded once per operation)."),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, nil, err
	}

	ttfb, err := meter.Float64Histogram(
		"gcp.storage.client.operation.ttfb",
		metric.WithDescription("Time from the start of an attempt until the first byte of the response was received. Not recorded for attempts that received no response."),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, nil, err
	}

	errors, err := meter.Int64Counter(
		"gcp.storage.client.errors",
		metric.WithDescription("Number of failed GCS client attempts, by error.type."),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, nil, err
	}

	var activeRequests metric.Int64UpDownCounter
	var gfeHeaderMissing metric.Int64Counter
	var dnsLookupDuration metric.Float64Histogram
	var tcpConnectDuration metric.Float64Histogram
	var tlsHandshakeDuration metric.Float64Histogram
	var gfeDuration metric.Float64Histogram
	var credentialRefreshDuration metric.Float64Histogram

	var networkBytesSent metric.Int64Counter
	var networkBytesReceived metric.Int64Counter
	var stallDuration metric.Float64Histogram

	if isOtelDebugMetricsEnabled(config) {
		networkBytesSent, err = meter.Int64Counter(
			"gcp.storage.client.network.bytes.sent",
			metric.WithDescription("Total physical bytes sent over the wire socket (gRPC only)."),
			metric.WithUnit("By"),
		)
		if err != nil {
			return nil, nil, err
		}

		networkBytesReceived, err = meter.Int64Counter(
			"gcp.storage.client.network.bytes.received",
			metric.WithDescription("Total physical bytes received over the wire socket (gRPC only)."),
			metric.WithUnit("By"),
		)
		if err != nil {
			return nil, nil, err
		}

		stallDuration, err = meter.Float64Histogram(
			"gcp.storage.client.stall.duration",
			metric.WithDescription("Stall timeout after which a read attempt was aborted while waiting for the initial response (response headers for HTTP, first response message for gRPC), or a write attempt was aborted due to lack of chunk progress."),
			metric.WithUnit("s"),
		)
		if err != nil {
			return nil, nil, err
		}

		credentialRefreshDuration, err = meter.Float64Histogram(
			"gcp.storage.client.auth.credential_refresh.duration",
			metric.WithDescription("Time a request was blocked obtaining an access token (credential refreshes and failures). Tokens served from the credential cache are not recorded."),
			metric.WithUnit("s"),
		)
		if err != nil {
			return nil, nil, err
		}
		activeRequests, err = meter.Int64UpDownCounter(
			"gcp.storage.client.request.active",
			metric.WithDescription("Number of active GCS client requests"),
			metric.WithUnit("1"),
		)
		if err != nil {
			return nil, nil, err
		}

		gfeHeaderMissing, err = meter.Int64Counter(
			"gcp.storage.client.gfe.header_missing",
			metric.WithDescription("Number of GCS attempts without an X-Goog-Gfe-Service-Time response header, including attempts that failed before a response was received (see error.type). DirectPath responses, which bypass the GFE, are not counted."),
			metric.WithUnit("1"),
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
			metric.WithDescription("Time taken to perform a TLS handshake. For gRPC this is the connection setup time after the TCP connection is established (TLS or ALTS handshake)."),
			metric.WithUnit("s"),
		)
		if err != nil {
			return nil, nil, err
		}

		gfeDuration, err = meter.Float64Histogram(
			"gcp.storage.client.gfe.duration",
			metric.WithDescription("GFE proxy processing time"),
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
		gfeHeaderMissing:          gfeHeaderMissing,
		dnsLookupDuration:         dnsLookupDuration,
		tcpConnectDuration:        tcpConnectDuration,
		tlsHandshakeDuration:      tlsHandshakeDuration,
		gfeDuration:               gfeDuration,
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

// Values of the error.type attribute that are not derived from a gRPC status
// or an HTTP status code. They describe failures that happen on the client or
// on the network before the server produced a response.
const (
	errorTypeOK                  = "OK"
	errorTypeCancelled           = "CANCELLED"
	errorTypeTimeout             = "TIMEOUT"
	errorTypeDNSFailure          = "DNS_FAILURE"
	errorTypeConnectionError     = "CONNECTION_ERROR"
	errorTypeTLSFailure          = "TLS_FAILURE"
	errorTypeAuthenticationError = "AUTHENTICATION_ERROR"
	errorTypeChecksumMismatch    = "CHECKSUM_MISMATCH"
	errorTypeUnknown             = "UNKNOWN"
)

// credentialError marks an error returned while obtaining an access token so
// that it can be classified as AUTHENTICATION_ERROR regardless of the
// underlying cause (for example a metadata server connection failure).
// It is transparent to callers: Error() and Unwrap() expose the original error.
type credentialError struct{ err error }

func (e *credentialError) Error() string { return e.err.Error() }
func (e *credentialError) Unwrap() error { return e.err }

// computeErrorType maps the request result to the standard error.type values.
//
// The classification is based on error types and status codes. Server
// responses (gRPC status or HTTP status code) always take precedence over the
// error message, so that object or bucket names that appear in an error
// message can never change the label. Message inspection is only used for
// transport-level failures that gRPC reports as flattened status strings.
//
// TIMEOUT is used for client-side deadlines (context deadline or network
// timeouts) while DEADLINE_EXCEEDED is used when the server or the gRPC
// transport reports that status.
func computeErrorType(err error, isHTTP bool, statusCode int64) string {
	// A bare io.EOF signals the successful end of a response body or stream.
	if err == nil || err == io.EOF {
		if isHTTP && statusCode >= 400 {
			return mapHTTPStatusCode(int(statusCode))
		}
		return errorTypeOK
	}

	if errors.Is(err, context.Canceled) {
		return errorTypeCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errorTypeTimeout
	}
	var credErr *credentialError
	if errors.As(err, &credErr) {
		return errorTypeAuthenticationError
	}
	if isChecksumError(err) {
		return errorTypeChecksumMismatch
	}

	if !isHTTP {
		if st, ok := status.FromError(err); ok && st.Code() != codes.OK {
			return classifyGRPCStatus(st)
		}
	} else {
		var apiErr *googleapi.Error
		if errors.As(err, &apiErr) && apiErr.Code != 0 {
			if apiErr.Code == http.StatusBadRequest && isChecksumMessage(apiErr.Message) {
				return errorTypeChecksumMismatch
			}
			return mapHTTPStatusCode(apiErr.Code)
		}
	}

	if t := classifyTransportError(err); t != "" {
		return t
	}

	if isHTTP && statusCode >= 400 {
		return mapHTTPStatusCode(int(statusCode))
	}
	return errorTypeUnknown
}

// classifyGRPCStatus maps a non-OK gRPC status to an error.type value. Failures
// that happen before a response is received (DNS, TCP, TLS, per-RPC
// credentials) are surfaced by grpc-go as UNAVAILABLE/UNKNOWN/INTERNAL/
// UNAUTHENTICATED statuses whose message carries the underlying cause.
func classifyGRPCStatus(st *status.Status) string {
	code := st.Code()
	msg := strings.ToLower(st.Message())
	switch code {
	case codes.Unavailable, codes.Unknown, codes.Internal, codes.Unauthenticated:
		if strings.Contains(msg, "per-rpc creds failed") || strings.Contains(msg, "per-rpc credentials") {
			return errorTypeAuthenticationError
		}
		if code != codes.Unauthenticated {
			if t := classifyTransportMessage(msg); t != "" {
				return t
			}
		}
	case codes.InvalidArgument, codes.DataLoss:
		if isChecksumMessage(msg) {
			return errorTypeChecksumMismatch
		}
	}
	return grpcCodeToString(code)
}

// classifyTransportError classifies network, TLS and credential errors based
// on their types, falling back to well-known transport error messages.
// It returns "" if the error is not recognized.
func classifyTransportError(err error) string {
	var authErr *auth.Error
	if errors.As(err, &authErr) {
		return errorTypeAuthenticationError
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return errorTypeDNSFailure
	}
	var (
		certVerifyErr *tls.CertificateVerificationError
		recordErr     tls.RecordHeaderError
		alertErr      tls.AlertError
		unknownAuth   x509.UnknownAuthorityError
		hostnameErr   x509.HostnameError
		certInvalid   x509.CertificateInvalidError
	)
	if errors.As(err, &certVerifyErr) || errors.As(err, &recordErr) || errors.As(err, &alertErr) ||
		errors.As(err, &unknownAuth) || errors.As(err, &hostnameErr) || errors.As(err, &certInvalid) {
		return errorTypeTLSFailure
	}
	// A failure to establish a connection is a connectivity problem even when
	// it manifests as a dial timeout (e.g. packets dropped by a firewall).
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return errorTypeConnectionError
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return errorTypeTimeout
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNABORTED) || errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, net.ErrClosed) || opErr != nil {
		return errorTypeConnectionError
	}
	return classifyTransportMessage(strings.ToLower(err.Error()))
}

// classifyTransportMessage recognizes transport-level failure messages
// produced by the Go standard library and grpc-go. The input must be lower
// case. It returns "" if the message is not recognized.
func classifyTransportMessage(msg string) string {
	switch {
	case strings.Contains(msg, "no such host"), strings.Contains(msg, "server misbehaving"),
		strings.Contains(msg, "produced zero addresses"):
		return errorTypeDNSFailure
	case strings.Contains(msg, "x509:"), strings.Contains(msg, "tls:"),
		strings.Contains(msg, "authentication handshake failed"):
		return errorTypeTLSFailure
	case strings.Contains(msg, "connection refused"), strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "broken pipe"), strings.Contains(msg, "unexpected eof"),
		strings.Contains(msg, "error while dialing"), strings.Contains(msg, "no route to host"),
		strings.Contains(msg, "network is unreachable"), strings.Contains(msg, "use of closed network connection"),
		strings.Contains(msg, "client connection lost"), strings.Contains(msg, "server sent goaway"):
		return errorTypeConnectionError
	}
	return ""
}

// isChecksumError reports whether err is an integrity check failure detected
// by the client (CRC32C/MD5 validation of downloaded or uploaded data).
func isChecksumError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "storage: bad CRC on") ||
		strings.Contains(msg, "storage: object checksum mismatch") ||
		strings.Contains(msg, "does not match the expected CRC32C")
}

// isChecksumMessage reports whether a server error message describes a
// checksum validation failure of uploaded data.
func isChecksumMessage(msg string) bool {
	m := strings.ToLower(msg)
	return (strings.Contains(m, "crc32c") || strings.Contains(m, "md5")) && strings.Contains(m, "match")
}

// mapHTTPStatusCode converts an HTTP status code to a canonical API error
// string following the HTTP to gRPC status mapping. Status codes without a
// canonical mapping are returned as numeric strings.
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

// isStreamingRPC reports whether the gRPC method streams object data. Unary
// methods record time to first byte as the full attempt latency.
func isStreamingRPC(methodName string) bool {
	switch methodName {
	case "ReadObject", "WriteObject", "BidiReadObject", "BidiWriteObject":
		return true
	}
	return false
}

// recordRPC records the metrics of a finished gRPC attempt. responded reports
// whether any response (headers, a message or trailers) was received from the
// server; time to first byte is not recorded for attempts that never received
// a response (for example DNS, connection or TLS failures).
func (cm *clientMetrics) recordRPC(ctx context.Context, fullMethod, target string, duration float64, err error, responded bool) {
	code := codes.OK
	if err != nil && err != io.EOF {
		code = status.Code(err)
	}
	methodName := getLogicalMethod(fullMethod)
	errorType := refineCancelled(ctx, computeErrorType(err, false, 0))
	server := stripPort(target)
	statusAttr := attribute.String("rpc.status_code", grpcCodeToString(code))

	// rpc.client.call.duration follows the OpenTelemetry RPC semantic
	// conventions: fully-qualified rpc.method and string rpc.status_code.
	attrs := []attribute.KeyValue{
		attribute.String("rpc.system.name", "grpc"),
		attribute.String("rpc.method", strings.TrimPrefix(fullMethod, "/")),
		statusAttr,
		attribute.String("server.address", server),
		attribute.String("error.type", errorType),
	}
	cm.rpcClientCallDuration.Record(ctx, duration, metric.WithAttributes(injectAPIMethod(ctx, attrs)...))

	state := metricsStateFromContext(ctx)
	logicalMethod := methodName
	if state != nil {
		logicalMethod = state.method
		state.setTarget(target)
	}
	cm.recordAttempt(ctx, "grpc", logicalMethod, server, errorType, statusAttr)

	if !isStreamingRPC(methodName) && responded {
		ttfbAttrs := []attribute.KeyValue{attribute.String("rpc.system.name", "grpc"), attribute.String("rpc.method", logicalMethod), attribute.String("server.address", server)}
		cm.ttfb.Record(ctx, duration, metric.WithAttributes(injectAPIMethod(ctx, ttfbAttrs)...))
	}
}

// recordAttempt records gcp.storage.client.attempts for every attempt and
// gcp.storage.client.errors for failed attempts.
func (cm *clientMetrics) recordAttempt(ctx context.Context, system, method, server, errorType string, statusAttr attribute.KeyValue) {
	base := []attribute.KeyValue{
		attribute.String("rpc.system.name", system),
		attribute.String("rpc.method", method),
		attribute.String("server.address", server),
		attribute.String("error.type", errorType),
	}
	attemptAttrs := make([]attribute.KeyValue, 0, len(base)+1)
	attemptAttrs = append(attemptAttrs, base...)
	attemptAttrs = append(attemptAttrs, statusAttr)
	cm.attempts.Add(ctx, 1, metric.WithAttributes(injectAPIMethod(ctx, attemptAttrs)...))
	if errorType != errorTypeOK {
		cm.errors.Add(ctx, 1, metric.WithAttributes(injectAPIMethod(ctx, base)...))
	}
}

// recordHTTP records the metrics of a finished HTTP attempt (after the
// response body was fully read or closed, or the round trip failed).
func (cm *clientMetrics) recordHTTP(ctx context.Context, req *http.Request, resp *http.Response, duration float64, errorType string) {
	statusCode := int64(0)
	if resp != nil {
		statusCode = int64(resp.StatusCode)
	}
	server := stripPort(req.URL.Host)

	// http.client.request.duration follows the OpenTelemetry HTTP semantic
	// conventions.
	attrs := []attribute.KeyValue{
		attribute.String("rpc.system.name", "http"),
		attribute.String("http.request.method", req.Method),
		attribute.String("url.template", computeURLTemplate(req.URL.Path, req.URL.Host)),
		attribute.Int64("http.response.status_code", statusCode),
		attribute.String("server.address", server),
		attribute.String("error.type", errorType),
	}
	cm.httpClientRequestDuration.Record(ctx, duration, metric.WithAttributes(injectAPIMethod(ctx, attrs)...))

	cm.recordAttempt(ctx, "http", httpLogicalMethod(ctx), server, errorType,
		attribute.Int64("http.response.status_code", statusCode))
}

// httpLogicalMethod returns the logical operation name of the request in ctx.
func httpLogicalMethod(ctx context.Context) string {
	if state := metricsStateFromContext(ctx); state != nil {
		return state.method
	}
	return "Unknown"
}

// computeHTTPAttemptErrorType classifies the outcome of a single HTTP
// round trip. Unlike response body reads, a round trip that fails with io.EOF
// means the connection was closed before a response was received.
func computeHTTPAttemptErrorType(err error, statusCode int64) string {
	if err == io.EOF {
		return errorTypeConnectionError
	}
	return computeErrorType(err, true, statusCode)
}

// refineCancelled labels attempts that the client aborted because of the
// dynamic read stall timeout as TIMEOUT rather than CANCELLED, which would
// otherwise suggest that the application cancelled the request.
func refineCancelled(ctx context.Context, errorType string) string {
	if errorType == errorTypeCancelled && errors.Is(context.Cause(ctx), errReadStallTimeout) {
		return errorTypeTimeout
	}
	return errorType
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
// The attribute therefore identifies the configured Cloud Storage endpoint and
// not the resolver or the path (DirectPath or CloudPath) used to reach it.
func stripPort(host string) string {
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
		if j := strings.Index(host, "/"); j >= 0 {
			host = host[j+1:]
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
	cm := rt.metrics
	if cm == nil {
		return rt.base.RoundTrip(req)
	}
	ctx := req.Context()
	logicalMethod := httpLogicalMethod(ctx)
	host := stripPort(req.URL.Host)
	rpcAttrs := metric.WithAttributes(
		attribute.String("rpc.method", logicalMethod),
		attribute.String("rpc.system.name", "http"),
		attribute.String("server.address", host),
	)
	if cm.activeRequests != nil {
		cm.activeRequests.Add(ctx, 1, rpcAttrs)
	}

	// Time to first byte is the time from sending the request until the
	// first byte of the response arrives, as reported by httptrace. This is
	// the same point curl (time_starttransfer), otelhttptrace and the AWS
	// SDK's TimeToFirstByte measure, and it is independent of when the
	// application starts reading the response body.
	var firstByteNanos atomic.Int64
	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() {
			firstByteNanos.CompareAndSwap(0, time.Now().UnixNano())
		},
	}
	if cm.dnsLookupDuration != nil || cm.tcpConnectDuration != nil || cm.tlsHandshakeDuration != nil {
		rt.addNetworkTrace(ctx, trace, metric.WithAttributes(
			attribute.String("rpc.system.name", "http"),
			attribute.String("server.address", host),
		))
	}
	req = req.WithContext(httptrace.WithClientTrace(ctx, trace))

	startTime := time.Now()
	resp, err := rt.base.RoundTrip(req)

	if state := metricsStateFromContext(ctx); state != nil {
		state.setTarget(req.URL.Host)
	}

	if err != nil {
		errorType := refineCancelled(ctx, computeHTTPAttemptErrorType(err, 0))
		cm.recordHTTPGFEMetrics(ctx, nil, logicalMethod, host, errorType, rpcAttrs)
		cm.recordHTTP(ctx, req, nil, time.Since(startTime).Seconds(), errorType)
		if cm.activeRequests != nil {
			cm.activeRequests.Add(ctx, -1, rpcAttrs)
		}
		return nil, err
	}

	// Attempts that received no response (err != nil above) record no TTFB.
	// If the trace hook did not fire (custom transports that do not support
	// httptrace), fall back to the time RoundTrip returned, which is after the
	// response headers were read.
	ttfb := time.Since(startTime)
	if n := firstByteNanos.Load(); n != 0 {
		ttfb = time.Unix(0, n).Sub(startTime)
	}
	ttfbAttrs := []attribute.KeyValue{attribute.String("rpc.system.name", "http"), attribute.String("rpc.method", logicalMethod), attribute.String("server.address", host)}
	cm.ttfb.Record(ctx, ttfb.Seconds(), metric.WithAttributes(injectAPIMethod(ctx, ttfbAttrs)...))
	cm.recordHTTPGFEMetrics(ctx, resp, logicalMethod, host, computeHTTPAttemptErrorType(nil, int64(resp.StatusCode)), rpcAttrs)

	body := &wrappedResponseBody{
		ReadCloser: resp.Body,
		startTime:  startTime,
		ctx:        ctx,
		req:        req,
		resp:       resp,
		metrics:    cm,
		rpcAttrs:   rpcAttrs,
	}
	if resp.Body == nil || resp.Body == http.NoBody {
		// Nothing to read: the attempt is complete.
		body.record(nil)
		return resp, nil
	}
	resp.Body = body
	return resp, nil
}

// addNetworkTrace adds DNS, TCP and TLS timing hooks to trace.
func (rt *metricsRoundTripper) addNetworkTrace(ctx context.Context, trace *httptrace.ClientTrace, netAttrs metric.MeasurementOption) {
	cm := rt.metrics
	var mu sync.Mutex
	var dnsStart, tlsStart time.Time
	tcpStarts := map[string]time.Time{}
	trace.DNSStart = func(httptrace.DNSStartInfo) {
		mu.Lock()
		dnsStart = time.Now()
		mu.Unlock()
	}
	trace.DNSDone = func(httptrace.DNSDoneInfo) {
		mu.Lock()
		start := dnsStart
		mu.Unlock()
		if cm.dnsLookupDuration != nil && !start.IsZero() {
			cm.dnsLookupDuration.Record(ctx, time.Since(start).Seconds(), netAttrs)
		}
	}
	trace.ConnectStart = func(network, addr string) {
		mu.Lock()
		tcpStarts[addr] = time.Now()
		mu.Unlock()
	}
	trace.ConnectDone = func(network, addr string, err error) {
		mu.Lock()
		start, ok := tcpStarts[addr]
		delete(tcpStarts, addr)
		mu.Unlock()
		if err == nil && ok && cm.tcpConnectDuration != nil {
			cm.tcpConnectDuration.Record(ctx, time.Since(start).Seconds(), netAttrs)
		}
	}
	trace.TLSHandshakeStart = func() {
		mu.Lock()
		tlsStart = time.Now()
		mu.Unlock()
	}
	trace.TLSHandshakeDone = func(_ tls.ConnectionState, err error) {
		mu.Lock()
		start := tlsStart
		mu.Unlock()
		if err == nil && cm.tlsHandshakeDuration != nil && !start.IsZero() {
			cm.tlsHandshakeDuration.Record(ctx, time.Since(start).Seconds(), netAttrs)
		}
	}
}

// recordHTTPGFEMetrics records the GFE service time reported in the
// X-Goog-Gfe-Service-Time response header, or counts a missing header. A nil
// response (the request never reached a server) is counted as missing.
func (cm *clientMetrics) recordHTTPGFEMetrics(ctx context.Context, resp *http.Response, logicalMethod, host, errorType string, rpcAttrs metric.MeasurementOption) {
	if cm.gfeHeaderMissing == nil {
		return
	}
	headerVal := ""
	if resp != nil {
		headerVal = resp.Header.Get("X-Goog-Gfe-Service-Time")
	}
	if headerVal == "" {
		cm.gfeHeaderMissing.Add(ctx, 1, metric.WithAttributes(
			attribute.String("rpc.method", logicalMethod),
			attribute.String("rpc.system.name", "http"),
			attribute.String("server.address", host),
			attribute.String("error.type", errorType),
		))
		return
	}
	if cm.gfeDuration != nil {
		if ms, err := strconv.ParseFloat(headerVal, 64); err == nil {
			cm.gfeDuration.Record(ctx, ms/1000.0, rpcAttrs)
		}
	}
}

// wrappedResponseBody records the attempt metrics when the response body has
// been fully read (io.EOF), failed, or was closed.
type wrappedResponseBody struct {
	io.ReadCloser
	startTime time.Time
	ctx       context.Context
	req       *http.Request
	resp      *http.Response
	metrics   *clientMetrics
	rpcAttrs  metric.MeasurementOption
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
	if !w.recorded.CompareAndSwap(false, true) {
		return
	}
	duration := time.Since(w.startTime).Seconds()
	w.metrics.recordHTTP(w.ctx, w.req, w.resp, duration, refineCancelled(w.ctx, computeErrorType(err, true, int64(w.resp.StatusCode))))
	if w.metrics.activeRequests != nil {
		w.metrics.activeRequests.Add(w.ctx, -1, w.rpcAttrs)
	}
}

func getLogicalMethod(method string) string {
	if idx := strings.LastIndex(method, "/"); idx != -1 {
		return method[idx+1:]
	}
	return method
}

// DirectPath traffic is served from these address ranges and bypasses the
// GFE, so responses do not carry the X-Goog-Gfe-Service-Time header.
var (
	directPathIPv4Range = netip.MustParsePrefix("34.126.0.0/18")
	directPathIPv6Range = netip.MustParsePrefix("2001:4860:8040::/42")
)

// isDirectPathPeer reports whether the connection peer is a DirectPath address.
func isDirectPathPeer(p *peer.Peer) bool {
	if p == nil || p.Addr == nil {
		return false
	}
	ip, err := netip.ParseAddr(stripPort(p.Addr.String()))
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	return directPathIPv4Range.Contains(ip) || directPathIPv6Range.Contains(ip)
}

// grpcLogicalMethod returns the logical storage operation for a gRPC call so
// that all gcp.storage.client.* metrics use the same rpc.method values
// regardless of the underlying RPC (e.g. ReadObject for BidiReadObject).
func grpcLogicalMethod(ctx context.Context, fullMethod string) string {
	if state := metricsStateFromContext(ctx); state != nil {
		return state.method
	}
	return getLogicalMethod(fullMethod)
}

// isClientSideErrorType reports whether errorType describes a failure that
// happened before any response was received from the server.
func isClientSideErrorType(errorType string) bool {
	switch errorType {
	case errorTypeCancelled, errorTypeTimeout, errorTypeDNSFailure, errorTypeConnectionError,
		errorTypeTLSFailure, errorTypeAuthenticationError:
		return true
	}
	return false
}

// grpcResponded reports whether a gRPC attempt received a response from the server.
func grpcResponded(err error, headerMD, trailerMD metadata.MD, p *peer.Peer) bool {
	if err == nil || err == io.EOF || len(headerMD) > 0 || len(trailerMD) > 0 {
		return true
	}
	return p != nil && p.Addr != nil && !isClientSideErrorType(computeErrorType(err, false, 0))
}

// recordGFEMetrics records the GFE service time from the
// x-goog-gfe-service-time response metadata, or counts a missing header.
// DirectPath responses never carry the header and are not counted as missing.
func (cm *clientMetrics) recordGFEMetrics(ctx context.Context, headerMD, trailerMD metadata.MD, err error, logicalMethod, target string, p *peer.Peer) {
	if cm.gfeHeaderMissing == nil {
		return
	}
	headerVals := headerMD.Get("x-goog-gfe-service-time")
	if len(headerVals) == 0 {
		headerVals = trailerMD.Get("x-goog-gfe-service-time")
	}
	headerVal := ""
	if len(headerVals) > 0 {
		headerVal = headerVals[0]
	}
	server := stripPort(target)
	if headerVal == "" {
		if isDirectPathPeer(p) {
			return
		}
		cm.gfeHeaderMissing.Add(ctx, 1, metric.WithAttributes(
			attribute.String("rpc.method", logicalMethod),
			attribute.String("rpc.system.name", "grpc"),
			attribute.String("server.address", server),
			attribute.String("error.type", computeErrorType(err, false, 0)),
		))
		return
	}
	if cm.gfeDuration != nil {
		if ms, parseErr := strconv.ParseFloat(headerVal, 64); parseErr == nil {
			cm.gfeDuration.Record(ctx, ms/1000.0, metric.WithAttributes(
				attribute.String("rpc.method", logicalMethod),
				attribute.String("rpc.system.name", "grpc"),
				attribute.String("server.address", server),
			))
		}
	}
}

// metricsInterceptors returns gRPC client interceptors.
func metricsInterceptors(cm *clientMetrics) (grpc.UnaryClientInterceptor, grpc.StreamClientInterceptor) {
	unary := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		target := ""
		if cc != nil {
			target = cc.Target()
		}
		// Record the target up front: operation metrics may be recorded (e.g. on
		// Reader.Close) before an asynchronously finished stream is.
		metricsStateFromContext(ctx).setTarget(target)
		logicalMethod := grpcLogicalMethod(ctx, method)

		var rpcAttrs metric.MeasurementOption
		if cm.activeRequests != nil {
			rpcAttrs = metric.WithAttributes(
				attribute.String("rpc.method", logicalMethod),
				attribute.String("rpc.system.name", "grpc"),
				attribute.String("server.address", stripPort(target)),
			)
			cm.activeRequests.Add(ctx, 1, rpcAttrs)
			defer cm.activeRequests.Add(ctx, -1, rpcAttrs)
		}

		var headerMD, trailerMD metadata.MD
		var p peer.Peer
		opts = append(opts, grpc.Header(&headerMD), grpc.Trailer(&trailerMD), grpc.Peer(&p))

		startTime := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(startTime).Seconds()

		cm.recordGFEMetrics(ctx, headerMD, trailerMD, err, logicalMethod, target, &p)
		cm.recordRPC(ctx, method, target, duration, err, grpcResponded(err, headerMD, trailerMD, &p))
		return err
	}

	stream := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		target := ""
		if cc != nil {
			target = cc.Target()
		}
		// Record the target up front: operation metrics may be recorded (e.g. on
		// Reader.Close) before an asynchronously finished stream is.
		metricsStateFromContext(ctx).setTarget(target)
		logicalMethod := grpcLogicalMethod(ctx, method)

		var rpcAttrs metric.MeasurementOption
		if cm.activeRequests != nil {
			rpcAttrs = metric.WithAttributes(
				attribute.String("rpc.method", logicalMethod),
				attribute.String("rpc.system.name", "grpc"),
				attribute.String("server.address", stripPort(target)),
			)
			cm.activeRequests.Add(ctx, 1, rpcAttrs)
		}

		// OnFinish is invoked when the stream ends for any reason, including
		// cancellation by the caller (e.g. a Reader closed before EOF). Without
		// it, streams that are never drained are never recorded and leak the
		// in-flight gauge.
		finisher := &streamFinisher{}
		opts = append(opts, grpc.OnFinish(finisher.onFinish))

		startTime := time.Now()
		clientStream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			finisher.attach(nil)
			if cm.activeRequests != nil {
				cm.activeRequests.Add(ctx, -1, rpcAttrs)
			}
			duration := time.Since(startTime).Seconds()
			cm.recordGFEMetrics(ctx, nil, nil, err, logicalMethod, target, nil)
			cm.recordRPC(ctx, method, target, duration, err, false)
			return nil, err
		}

		w := &wrappedClientStream{
			ClientStream:  clientStream,
			startTime:     startTime,
			method:        method,
			logicalMethod: logicalMethod,
			target:        target,
			metrics:       cm,
			ctx:           ctx,
			rpcAttrs:      rpcAttrs,
			serverStreams: desc.ServerStreams,
			clientStreams: desc.ClientStreams,
		}
		finisher.attach(w)
		return w, nil
	}

	return unary, stream
}

// streamFinisher connects the grpc.OnFinish callback, which may fire before
// the wrapped stream exists, to the wrappedClientStream.
type streamFinisher struct {
	mu       sync.Mutex
	attached bool
	w        *wrappedClientStream
	finished bool
	err      error
}

func (f *streamFinisher) onFinish(err error) {
	f.mu.Lock()
	w, attached := f.w, f.attached
	if !attached {
		f.finished, f.err = true, err
	}
	f.mu.Unlock()
	if w != nil {
		// grpc-go invokes OnFinish while holding the stream lock, so the
		// stream (Header, Trailer, Context) must be inspected asynchronously.
		go w.record(err)
	}
}

// attach associates the stream with the finisher. A nil w means stream
// creation failed and was recorded by the caller.
func (f *streamFinisher) attach(w *wrappedClientStream) {
	f.mu.Lock()
	f.attached, f.w = true, w
	finished, err := f.finished, f.err
	f.mu.Unlock()
	if finished && w != nil {
		w.record(err)
	}
}

type wrappedClientStream struct {
	grpc.ClientStream
	startTime     time.Time
	method        string
	logicalMethod string
	target        string
	metrics       *clientMetrics
	ctx           context.Context
	rpcAttrs      metric.MeasurementOption
	recorded      atomic.Bool
	serverStreams bool
	clientStreams bool
	recordedTTFB  atomic.Bool
}

func (w *wrappedClientStream) RecvMsg(m interface{}) error {
	err := w.ClientStream.RecvMsg(m)
	if err == nil {
		w.recordTTFB()
	}
	// For client-streaming streams (like WriteObject), the single successful RecvMsg call
	// returns the response and nil error, which marks the completion of the stream.
	isClientStreaming := !w.serverStreams && w.clientStreams
	if err != nil || isClientStreaming {
		w.record(err)
	}
	return err
}

// record records the attempt metrics once. SendMsg errors are not recorded
// here: gRPC reports the stream status through RecvMsg or OnFinish (SendMsg
// returns io.EOF when the server terminated the stream).
func (w *wrappedClientStream) record(err error) {
	if !w.recorded.CompareAndSwap(false, true) {
		return
	}
	duration := time.Since(w.startTime).Seconds()
	headerMD, _ := w.ClientStream.Header()
	trailerMD := w.ClientStream.Trailer()
	p, _ := peer.FromContext(w.ClientStream.Context())

	if w.metrics.activeRequests != nil {
		w.metrics.activeRequests.Add(w.ctx, -1, w.rpcAttrs)
	}
	w.metrics.recordGFEMetrics(w.ctx, headerMD, trailerMD, err, w.logicalMethod, w.target, p)
	w.metrics.recordRPC(w.ctx, w.method, w.target, duration, err, grpcResponded(err, headerMD, trailerMD, p))
}

// recordTTFB records the time to the first response message of the stream,
// whether it contains metadata, persisted size or object data.
func (w *wrappedClientStream) recordTTFB() {
	if !w.recordedTTFB.CompareAndSwap(false, true) {
		return
	}
	duration := time.Since(w.startTime).Seconds()
	w.metrics.ttfb.Record(w.ctx, duration, metric.WithAttributes(injectAPIMethod(w.ctx, []attribute.KeyValue{
		attribute.String("rpc.system.name", "grpc"),
		attribute.String("rpc.method", w.logicalMethod),
		attribute.String("server.address", stripPort(w.target)),
	})...))
}

type metricsKey struct{}

type apiMethodKey struct{}

func injectAPIMethod(ctx context.Context, attrs []attribute.KeyValue) []attribute.KeyValue {
	if apiMethod, ok := ctx.Value(apiMethodKey{}).(string); ok {
		return append(attrs, attribute.String("gcp.client.method", apiMethod))
	}
	return attrs
}

type metricsState struct {
	target    atomic.Pointer[string]
	method    string
	startTime time.Time
	metrics   *clientMetrics
	isHTTP    bool
	record    func(error)
	sizeOnce  sync.Once
	// composite marks an operation that is implemented with other client
	// operations (e.g. a parallel composite upload that uploads parts and
	// composes them). Operations started within it are not recorded as
	// operations of their own; their attempts are still recorded.
	composite bool
	// parent is the composite operation this operation belongs to.
	parent *metricsState
}

// recordResponseBodySize records the number of object bytes delivered to the
// application by the operation. It records at most once per operation and
// includes zero-byte reads.
func (s *metricsState) recordResponseBodySize(ctx context.Context, n int64) {
	if s == nil || s.metrics == nil {
		return
	}
	s.recordBodySize(ctx, s.metrics.responseBodySize, n)
}

// recordRequestBodySize records the number of object bytes written by the
// operation. It records at most once per operation and includes zero-byte
// objects.
func (s *metricsState) recordRequestBodySize(ctx context.Context, n int64) {
	if s == nil || s.metrics == nil {
		return
	}
	s.recordBodySize(ctx, s.metrics.requestBodySize, n)
}

func (s *metricsState) recordBodySize(ctx context.Context, h metric.Int64Histogram, n int64) {
	if h == nil || s.parent != nil {
		return
	}
	s.sizeOnce.Do(func() {
		h.Record(ctx, n, metric.WithAttributes(injectAPIMethod(ctx, []attribute.KeyValue{
			attribute.String("rpc.system.name", s.getSystemName()),
			attribute.String("rpc.method", s.method),
			attribute.String("server.address", stripPort(s.getTarget())),
		})...))
	})
}

func (s *metricsState) setTarget(t string) {
	if s == nil {
		return
	}
	s.target.Store(&t)
	if s.parent != nil {
		s.parent.setTarget(t)
	}
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

func (cm *clientMetrics) startOperation(ctx context.Context, method string, isHTTP bool) (context.Context, func(error)) {
	if cm == nil {
		return ctx, func(error) {}
	}
	if parent := metricsStateFromContext(ctx); parent != nil && parent.composite {
		// Part of a composite operation: attribute attempts to this call but
		// record the operation only once, for the composite operation.
		child := &metricsState{method: method, startTime: time.Now(), metrics: cm, isHTTP: isHTTP, parent: parent}
		child.record = func(error) {}
		return contextWithMetricsState(ctx, child), child.record
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

			attrs := []attribute.KeyValue{
				attribute.String("rpc.system.name", state.getSystemName()),
				attribute.String("rpc.method", method),
				attribute.String("server.address", stripPort(state.getTarget())),
				attribute.String("error.type", errorType),
			}
			opts := metric.WithAttributes(injectAPIMethod(ctx, attrs)...)
			cm.duration.Record(ctx, duration, opts)
			cm.operations.Add(ctx, 1, opts)
		})
	}
	state.record = record

	ctx = contextWithMetricsState(ctx, state)
	return ctx, record
}

// startCompositeOperation starts an operation that is implemented with other
// client operations. It is recorded when the returned state's record function
// is called (for writers, in Writer.Close); operations started with the
// returned context are recorded as attempts only.
func (cm *clientMetrics) startCompositeOperation(ctx context.Context, method string, isHTTP bool) context.Context {
	ctx, _ = cm.startOperation(ctx, method, isHTTP)
	if state := metricsStateFromContext(ctx); state != nil {
		state.composite = true
	}
	return ctx
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

// grpcNetworkMetricsDialOptions returns dial options that instrument TCP and TLS handshake metrics.
func grpcNetworkMetricsDialOptions(host string, metrics *clientMetrics) []option.ClientOption {
	if metrics == nil || (metrics.tcpConnectDuration == nil && metrics.tlsHandshakeDuration == nil && metrics.networkBytesSent == nil && metrics.networkBytesReceived == nil) {
		return nil
	}
	var dialTimes sync.Map

	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		var netAttrs metric.MeasurementOption
		if metrics.tcpConnectDuration != nil {
			netAttrs = metric.WithAttributes(
				attribute.String("rpc.system.name", "grpc"),
				attribute.String("server.address", host),
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
				dialTimes.Store(conn.LocalAddr().String(), dialInfo{doneTime: time.Now(), host: host})
			}
		}

		return conn, err
	}

	sh := &grpcMetricsStatsHandler{
		metrics:   metrics,
		dialTimes: &dialTimes,
		host:      host,
	}

	return []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithContextDialer(dialer)),
		option.WithGRPCDialOption(grpc.WithStatsHandler(sh)),
	}
}

type metricsTokenProvider struct {
	base auth.TokenProvider
	// metrics may be attached after the provider has been installed in a
	// transport (see NewClient), hence the atomic pointer.
	metrics atomic.Pointer[clientMetrics]
}

func newMetricsTokenProvider(base auth.TokenProvider, m *clientMetrics) *metricsTokenProvider {
	p := &metricsTokenProvider{base: base}
	if m != nil {
		p.metrics.Store(m)
	}
	return p
}

func (p *metricsTokenProvider) Token(ctx context.Context) (*auth.Token, error) {
	start := time.Now()
	tok, err := p.base.Token(ctx)
	p.metrics.Load().recordCredentialRefreshDuration(ctx, time.Since(start), err)
	if err != nil {
		var credErr *credentialError
		if !errors.As(err, &credErr) {
			err = &credentialError{err: err}
		}
	}
	return tok, err
}

// deferredMetricsCredentials returns a copy of c whose token provider records
// credential refresh metrics once metrics are attached to the returned
// provider. It is used by NewClient, which must build the http.Client (and
// therefore install the credentials) before the metrics pipeline exists.
func deferredMetricsCredentials(c *auth.Credentials) (*auth.Credentials, *metricsTokenProvider) {
	if c == nil || c.TokenProvider == nil {
		return c, nil
	}
	if mtp, ok := c.TokenProvider.(*metricsTokenProvider); ok {
		return c, mtp
	}
	mtp := newMetricsTokenProvider(c.TokenProvider, nil)
	clone := *c
	clone.TokenProvider = mtp
	return &clone, mtp
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
		if m == nil || mtp.metrics.Load() == m {
			return c
		}
		// A provider installed by NewClient without metrics yet: attach them
		// in place so the already-built transport starts recording.
		if mtp.metrics.CompareAndSwap(nil, m) {
			return c
		}
		clone := *c
		clone.TokenProvider = newMetricsTokenProvider(mtp.base, m)
		return &clone
	}
	clone := *c
	clone.TokenProvider = newMetricsTokenProvider(c.TokenProvider, m)
	return &clone
}

// credentialCacheHitThreshold is the duration below which a call to
// TokenProvider.Token is considered to have been served from the token cache.
// Cached lookups take microseconds, whereas any refresh involves a network
// round trip (metadata server, STS or OAuth2 endpoint).
const credentialCacheHitThreshold = time.Millisecond

// recordCredentialRefreshDuration records the time a request was blocked while
// obtaining an access token. The auth libraries cache tokens and serve almost
// every call from memory; those cache hits are not recorded so that the
// histogram reflects actual credential refreshes (and failures) that added
// latency to requests. Background (asynchronous) refreshes that do not block a
// request are not observable here.
func (cm *clientMetrics) recordCredentialRefreshDuration(ctx context.Context, duration time.Duration, err error) {
	if cm == nil || cm.credentialRefreshDuration == nil {
		return
	}
	if err == nil && duration < credentialCacheHitThreshold {
		return
	}
	errorType := errorTypeOK
	if err != nil {
		errorType = errorTypeAuthenticationError
		if errors.Is(err, context.Canceled) {
			errorType = errorTypeCancelled
		} else if errors.Is(err, context.DeadlineExceeded) {
			errorType = errorTypeTimeout
		}
	}
	attrs := []attribute.KeyValue{attribute.String("error.type", errorType)}
	cm.credentialRefreshDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

func (cm *clientMetrics) recordStallDuration(ctx context.Context, duration time.Duration, method string, systemName string, target string) {
	if cm == nil || cm.stallDuration == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("rpc.system.name", systemName),
		attribute.String("rpc.method", method),
		attribute.String("server.address", target),
	}
	cm.stallDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}
