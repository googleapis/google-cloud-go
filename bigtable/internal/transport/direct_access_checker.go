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

package internal

import (
	"context"
	"log"
	"net"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	bigtablepb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal/directaccess"
	btopt "cloud.google.com/go/bigtable/internal/option"
)

const defaultUniverseDomain = "googleapis.com"

// DirectAccessChecker decides whether the runtime environment is compatible
// with Direct Access (DirectPath / DirectPathXds) and provides the dialer used
// for direct-access connections once compatibility is confirmed.
//
// Implementations differ in HOW compatibility is determined:
//   - pingAndWarmDirectAccessChecker probes via a PingAndWarm call at startup
//     (used by the classic channel pool factory).
//   - sessionClientDirectAccessChecker probes via GetClientConfiguration at
//     startup (used by session-based channel pools).
type DirectAccessChecker interface {
	// CheckCompatibility reports whether Direct Access is compatible. When
	// compatible, conn is a primed connection the pool may adopt as its first
	// direct-access connection; otherwise conn is nil and the checker has
	// already closed any probe connection it created. Implementations are
	// responsible for recording the direct_access/compatible metric.
	CheckCompatibility(ctx context.Context) (conn *BigtableConn, compatible bool)

	// Dialer returns the dialer used to create direct-access connections
	// after CheckCompatibility has reported compatibility. Only consulted
	// when CheckCompatibility returned true.
	Dialer() func() (*BigtableConn, error)

	// LastProbeConfig returns the ClientConfiguration returned by the
	// most recent CheckCompatibility probe, or nil if the probe never
	// returned a body. Only the session-flavored checker
	// (sessionClientDirectAccessChecker) produces a non-nil value —
	// pingAndWarm probes a warm-only RPC with no ClientConfiguration
	// body, and the disabled stub never probes. Callers use it to seed
	// downstream state (e.g., ClientConfigurationManager) without an
	// extra RPC on client construction.
	LastProbeConfig() *bigtablepb.ClientConfiguration
}

// IsGCUEndpoint reports whether the gRPC endpoint and universe domain resolved
// from opts belong to the Google Cloud Universe ("googleapis.com"). Direct
// Access is only supported in GCU; callers targeting other universes (such as
// Trusted Partner Cloud) or custom non-googleapis endpoints must skip the
// Direct Access compatibility probe.
func IsGCUEndpoint(opts ...option.ClientOption) bool {
	uResolver, err := internaloption.NewUnsafeResolver(opts...)
	if err != nil {
		return false
	}
	endpoint, err := uResolver.ResolvedGRPCEndpoint()
	if err != nil || !isGCUEndpointHost(endpoint) {
		return false
	}

	// Also verify the configured universe domain (via WithUniverseDomain or
	// GOOGLE_CLOUD_UNIVERSE_DOMAIN) in case WithEndpoint overrode the host.
	udOpts := make([]option.ClientOption, 0, len(opts)+3)
	udOpts = append(udOpts, opts...)
	udOpts = append(udOpts,
		internaloption.WithDefaultEndpointTemplate("UNIVERSE_DOMAIN"),
		internaloption.WithDefaultMTLSEndpoint(defaultUniverseDomain),
		option.WithEndpoint(""),
	)
	udResolver, err := internaloption.NewUnsafeResolver(udOpts...)
	if err != nil {
		return false
	}
	ud, err := udResolver.ResolvedGRPCEndpoint()
	if err != nil {
		return false
	}
	return strings.TrimSuffix(strings.ToLower(ud), ".") == defaultUniverseDomain
}

func isGCUEndpointHost(endpoint string) bool {
	if endpoint == "" {
		return false
	}
	supportedScheme := !strings.Contains(endpoint, "://") || strings.HasPrefix(endpoint, "dns:///") || strings.HasPrefix(endpoint, "google-c2p:///")
	if !supportedScheme {
		return false
	}
	raw := endpoint
	if !strings.Contains(raw, "://") {
		raw = "//" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	target := u.Host
	if target == "" {
		target = strings.TrimPrefix(u.Path, "/")
	}
	if slashIdx := strings.Index(target, "/"); slashIdx != -1 {
		target = target[:slashIdx]
	}
	host := target
	if h, _, splitErr := net.SplitHostPort(target); splitErr == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return host == defaultUniverseDomain || strings.HasSuffix(host, "."+defaultUniverseDomain)
}

// newDirectAccessEligibleGauge constructs the direct_access/compatible gauge
// from the provided meter provider. Returns nil if meterProvider is nil or
// instrument creation fails — callers treat a nil gauge as a metric-disabled
// no-op so behavior matches the prior in-pool gauge construction.
func newDirectAccessEligibleGauge(meterProvider metric.MeterProvider, logger *log.Logger) metric.Int64Gauge {
	if meterProvider == nil {
		return nil
	}
	gauge, err := meterProvider.Meter(clientMeterName).Int64Gauge(
		"direct_access/compatible",
		metric.WithDescription("Reports 1 if the environment is eligible for DirectPath, 0 otherwise. Based on a connection attempt at startup."),
		metric.WithUnit("1"),
	)
	if err != nil {
		btopt.Debugf(logger, "bigtable_direct_access: failed to create direct_access/compatible metric: %v", err)
		return nil
	}
	return gauge
}

// pingAndWarmDirectAccessChecker probes Direct Access compatibility by
// dialing once and issuing PingAndWarm + ALTS inspection. On failure it
// kicks off an asynchronous investigation that records a more specific
// failure reason on the direct_access/compatible metric.
//
// The checker reuses the same ChannelPrimer that the pool's connection
// factory uses, so the exact PingAndWarm invocation (instance name, app
// profile, feature-flag metadata) stays in one place — both the
// compatibility probe and any single-endpoint investigation go through it.
type pingAndWarmDirectAccessChecker struct {
	dialer          func() (*BigtableConn, error)
	primer          ChannelPrimer
	daEligibleGauge metric.Int64Gauge
	logger          *log.Logger
}

// NewPingAndWarmDirectAccessChecker constructs the today-default checker.
// A nil meterProvider produces a checker that silently skips metric
// reporting. A nil primer produces a checker that skips the Prime step
// during CheckCompatibility — that's the mode session-based clients use,
// since they warm channels on-demand via OpenSession streams rather than
// eagerly at pool-init. The classic (pingAndWarm) channel pool factory
// constructs a single ChannelPrimer and shares it with both the pool
// (via WithChannelPrimer) and this checker.
func NewPingAndWarmDirectAccessChecker(
	dialer func() (*BigtableConn, error),
	primer ChannelPrimer,
	meterProvider metric.MeterProvider,
	logger *log.Logger,
) DirectAccessChecker {
	return &pingAndWarmDirectAccessChecker{
		dialer:          dialer,
		primer:          primer,
		daEligibleGauge: newDirectAccessEligibleGauge(meterProvider, logger),
		logger:          logger,
	}
}

// Dialer returns the configured direct-access dialer.
func (c *pingAndWarmDirectAccessChecker) Dialer() func() (*BigtableConn, error) {
	return c.dialer
}

// LastProbeConfig always returns nil for the pingAndWarm variant —
// PingAndWarm's response carries no ClientConfiguration body, so
// there's nothing to reuse.
func (c *pingAndWarmDirectAccessChecker) LastProbeConfig() *bigtablepb.ClientConfiguration {
	return nil
}

// CheckCompatibility opens a single probe connection, primes it, and decides
// whether Direct Access is usable. On compatible: the primed connection is
// returned so the pool can adopt it as its first connection (saving one
// redial). On incompatible: any probe connection is closed and an async
// investigation begins to report a specific failure reason.
func (c *pingAndWarmDirectAccessChecker) CheckCompatibility(ctx context.Context) (*BigtableConn, bool) {
	conn, err := c.dialer()
	if err != nil {
		btopt.Debugf(c.logger, "bigtable_direct_access: dial failed: %v", err)
		return nil, false
	}

	// TODO(sushanb): replace this nil-guard with a NoopChannelPrimer
	// (channel_primer.go). Nullable-field pattern here is the sharp
	// edge — passing a Noop impl at the session-client construction
	// site kills the branch and keeps the invariant "primer is always
	// non-nil". Deferred out of this PR per the PR-4 port scope.
	if c.primer != nil {
		err = c.primer.Prime(ctx, conn)
		if err != nil {
			// PermissionDenied is expected on probes that are otherwise healthy
			// (the bootstrap credentials may lack PingAndWarm), so fall through
			// to the ALTS check rather than failing fast.
			if status.Code(err) != codes.PermissionDenied {
				btopt.Debugf(c.logger, "bigtable_direct_access: Prime() failed during compatibility check: %v", err)
				conn.Close()
				go c.investigateFailure(err)
				return nil, false
			}
			btopt.Debugf(c.logger, "bigtable_direct_access: Prime() failed with PermissionDenied, continuing to ALTS check: %v", err)
		}
	}

	if conn.isALTSConn.Load() {
		c.reportSuccess(ctx, conn.ipProtocol())
		return conn, true
	}

	conn.Close()
	go c.investigateFailure(err)
	return nil, false
}

// reportSuccess records a direct_access/compatible=1 reading.
func (c *pingAndWarmDirectAccessChecker) reportSuccess(ctx context.Context, ipPreference string) {
	if c.daEligibleGauge == nil {
		return
	}
	c.daEligibleGauge.Record(ctx, 1, metric.WithAttributes(
		attribute.String("ip_preference", ipPreference),
		attribute.String("reason", ""),
	))
}

// reportFailure records a direct_access/compatible=0 reading with the given
// reason tag (e.g. "manually_disabled", "metadata_unreachable").
func (c *pingAndWarmDirectAccessChecker) reportFailure(reason string) {
	if c.daEligibleGauge == nil {
		return
	}
	c.daEligibleGauge.Record(context.Background(), 0, metric.WithAttributes(
		attribute.String("ip_preference", ""),
		attribute.String("reason", reason),
	))
}

// investigateFailure runs asynchronously after a failed compatibility check
// to determine why Direct Access was not usable, and reports the specific
// reason to the metric.
func (c *pingAndWarmDirectAccessChecker) investigateFailure(originalErr error) {
	investigateDirectAccessFailure(c.logger, c.reportFailure, originalErr)
}

// directAccessEnv groups the local host and GCE metadata checks consulted by
// investigateDirectAccessFailureWithEnv so unit tests can exercise every
// precondition branch hermetically.
type directAccessEnv struct {
	isRunningOnGCP            func() error
	checkMetadataReachability func() error
	fetchIPFromMetadata       func(addrFamily string) (*net.IP, error)
	checkLoopbackInterfaceUp  func() error
	checkLocalIPv4Loopback    func() error
	checkLocalIPv6Loopback    func() error
	checkLocalIPv4Addresses   func(ip *net.IP) (*net.Interface, error)
	checkLocalIPv6Addresses   func(ip *net.IP) (*net.Interface, error)
}

func defaultDirectAccessEnv() directAccessEnv {
	return directAccessEnv{
		isRunningOnGCP:            directaccess.IsRunningOnGCP,
		checkMetadataReachability: directaccess.CheckMetadataServerReachability,
		fetchIPFromMetadata:       directaccess.FetchIPFromMetadataServer,
		checkLoopbackInterfaceUp:  directaccess.CheckLoopbackInterfaceUp,
		checkLocalIPv4Loopback:    directaccess.CheckLocalIPv4LoopbackAddress,
		checkLocalIPv6Loopback:    directaccess.CheckLocalIPv6LoopbackAddress,
		checkLocalIPv4Addresses:   directaccess.CheckLocalIPv4Addresses,
		checkLocalIPv6Addresses:   directaccess.CheckLocalIPv6Addresses,
	}
}

// investigateDirectAccessFailure walks the local GCE-environment preconditions
// in order of cheapness, short-circuiting as soon as a failing precondition is
// found, and reports the first failing reason via reportFailure.
func investigateDirectAccessFailure(logger *log.Logger, reportFailure func(reason string), originalErr error) {
	investigateDirectAccessFailureWithEnv(logger, reportFailure, originalErr, defaultDirectAccessEnv())
}

func investigateDirectAccessFailureWithEnv(logger *log.Logger, reportFailure func(reason string), originalErr error, env directAccessEnv) {
	if err := env.isRunningOnGCP(); err != nil {
		btopt.Debugf(logger, "bigtable_direct_access: investigation: %v. Original error: %v", err, originalErr)
		reportFailure("not_in_gcp")
		return
	}

	if err := env.checkMetadataReachability(); err != nil {
		btopt.Debugf(logger, "bigtable_direct_access: investigation: Metadata unreachable: %v", err)
		reportFailure("metadata_unreachable")
		return
	}

	ipv4, errV4 := env.fetchIPFromMetadata("IPv4")
	ipv6, errV6 := env.fetchIPFromMetadata("IPv6")

	if errV4 != nil && errV6 != nil {
		btopt.Debugf(logger, "bigtable_direct_access: investigation: Neither IPv4 nor IPv6 assigned. v4Err: %v, v6Err: %v", errV4, errV6)
		reportFailure("no_ip_assigned")
		return
	}

	if err := env.checkLoopbackInterfaceUp(); err != nil {
		btopt.Debugf(logger, "bigtable_direct_access: investigation: Loopback interface down: %v", err)
		reportFailure("loopback_misconfigured")
		return
	}

	if ipv4 != nil {
		if err := env.checkLocalIPv4Loopback(); err != nil {
			btopt.Debugf(logger, "bigtable_direct_access: investigation: IPv4 loopback missing: %v", err)
			reportFailure("loopback_misconfigured_ipv4")
			return
		}
	}

	if ipv6 != nil {
		if err := env.checkLocalIPv6Loopback(); err != nil {
			btopt.Debugf(logger, "bigtable_direct_access: investigation: IPv6 loopback missing: %v", err)
			reportFailure("loopback_misconfigured_ipv6")
			return
		}
	}

	v4Plumbed, v6Plumbed := checkIPPlumbing(logger, ipv4, ipv6, env)
	if !v4Plumbed && !v6Plumbed {
		btopt.Debugf(logger, "bigtable_direct_access: investigation: Metadata IPs not plumbed to local interfaces (likely containerized).")
	}

	btopt.Debugf(logger, "bigtable_direct_access: investigation: All local preconditions passed but Direct Access originally failed. Original error: %v", originalErr)
	reportFailure("unknown")
}

// noopDirectAccessChecker skips Direct Access compatibility probing without
// recording metrics. It is wired when the target endpoint is outside the GCU
// ("googleapis.com") universe, where Direct Access is not applicable.
type noopDirectAccessChecker struct{}

// NewNoopDirectAccessChecker constructs a DirectAccessChecker that skips
// compatibility probing and emits no metrics.
func NewNoopDirectAccessChecker() DirectAccessChecker {
	return noopDirectAccessChecker{}
}

func (noopDirectAccessChecker) CheckCompatibility(context.Context) (*BigtableConn, bool) {
	return nil, false
}

func (noopDirectAccessChecker) Dialer() func() (*BigtableConn, error) {
	return nil
}

func (noopDirectAccessChecker) LastProbeConfig() *bigtablepb.ClientConfiguration {
	return nil
}

// disabledDirectAccessChecker refuses every compatibility check and records a
// single manually_disabled reading the first (and only) time CheckCompatibility
// runs. The factory wires it up when isDirectAccessEnabled returns false so the
// direct_access/compatible metric still surfaces the "off" state — keeping
// observability identical to the pre-modularization behavior in
// NewBigtableChannelPool.
type disabledDirectAccessChecker struct {
	daEligibleGauge metric.Int64Gauge
}

// newDisabledDirectAccessChecker constructs the always-disabled checker. The
// manually_disabled metric is recorded lazily inside CheckCompatibility so it
// uses the pool's ctx for propagation and keeps construction side-effect-free.
func newDisabledDirectAccessChecker(meterProvider metric.MeterProvider, logger *log.Logger) *disabledDirectAccessChecker {
	return &disabledDirectAccessChecker{
		daEligibleGauge: newDirectAccessEligibleGauge(meterProvider, logger),
	}
}

// NewDisabledDirectAccessChecker constructs the always-disabled checker for
// callers outside this package.
func NewDisabledDirectAccessChecker(meterProvider metric.MeterProvider, logger *log.Logger) DirectAccessChecker {
	return newDisabledDirectAccessChecker(meterProvider, logger)
}

// CheckCompatibility always returns (nil, false) and records the
// manually_disabled metric exactly once — the pool calls it exactly once at
// startup, with the pool ctx.
func (c *disabledDirectAccessChecker) CheckCompatibility(ctx context.Context) (*BigtableConn, bool) {
	if c.daEligibleGauge != nil {
		c.daEligibleGauge.Record(ctx, 0, metric.WithAttributes(
			attribute.String("ip_preference", ""),
			attribute.String("reason", "manually_disabled"),
		))
	}
	return nil, false
}

// Dialer returns nil; never consulted by the pool because CheckCompatibility
// returns false.
func (c *disabledDirectAccessChecker) Dialer() func() (*BigtableConn, error) {
	return nil
}

// LastProbeConfig always returns nil — the disabled checker never
// probes, so there's no response body to hand back.
func (c *disabledDirectAccessChecker) LastProbeConfig() *bigtablepb.ClientConfiguration {
	return nil
}

// checkIPPlumbing verifies whether the IPs assigned by the metadata server are
// actually plumbed onto a local network interface.
func checkIPPlumbing(logger *log.Logger, ipv4, ipv6 *net.IP, env directAccessEnv) (v4Plumbed, v6Plumbed bool) {
	if ipv4 != nil {
		if _, err := env.checkLocalIPv4Addresses(ipv4); err == nil {
			v4Plumbed = true
		} else {
			btopt.Debugf(logger, "bigtable_direct_access: investigation: IPv4 assigned by metadata but not found on NIC: %v", err)
		}
	}

	if ipv6 != nil {
		if _, err := env.checkLocalIPv6Addresses(ipv6); err == nil {
			v6Plumbed = true
		} else {
			btopt.Debugf(logger, "bigtable_direct_access: investigation: IPv6 assigned by metadata but not found on NIC: %v", err)
		}
	}

	return v4Plumbed, v6Plumbed
}
