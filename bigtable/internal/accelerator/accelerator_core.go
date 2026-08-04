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

// Package accelerator exposes an in-process grpc.ClientConnInterface that
// translates Bigtable V2 RPCs into proto-native session vRPCs handled by
// internal/session.
//
// Channel is scoped to one (project, instance, appProfile). It owns a
// session.Client for the daemon's lifetime; Close tears it down along with all
// its pools. This file establishes that lifecycle. The per-RPC dispatch that
// translates individual V2 calls into session vRPCs is layered on top: until
// it is wired in, Invoke and NewStream report Unimplemented.
package accelerator

import (
	"context"

	"cloud.google.com/go/bigtable/internal"
	metrics "cloud.google.com/go/bigtable/internal/metrics"
	btopt "cloud.google.com/go/bigtable/internal/option"
	"cloud.google.com/go/bigtable/internal/session"
	btransport "cloud.google.com/go/bigtable/internal/transport"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Default data-plane dial parameters. These mirror the unexported
// bigtable.prodAddr / mtlsProdAddr / Scope / clientUserAgent constants; they
// are duplicated here (rather than imported) to keep the daemon from pulling
// in the full bigtable client package, matching how internal/session already
// duplicates its feature-flag metadata to avoid the same import cycle.
const (
	prodAddr     = "bigtable.UNIVERSE_DOMAIN:443"
	mtlsProdAddr = "bigtable.mtls.googleapis.com:443"
	dataScope    = "https://www.googleapis.com/auth/bigtable.data"
)

var userAgent = "cbt-go-accelerator/v" + internal.Version

// Ensure Channel implements grpc.ClientConnInterface.
var _ grpc.ClientConnInterface = (*Channel)(nil)

// newSessionClient constructs the session.Client a Channel dials on
// construction. It is a package-level seam that tests override to inject a
// mock; package-level state mutation is not parallel-safe.
var newSessionClient = func(
	ctx context.Context,
	project, instance, appProfile string,
	opts ...option.ClientOption,
) (session.Client, error) {
	// Phase 1 keeps built-in client-side metrics on: a nil MetricsProvider is
	// what metrics.NewFactory treats as the default Cloud Monitoring exporter.
	var metricsProvider metrics.MetricsProvider

	// Mirror bigtable.NewClientWithConfig: derive the on-wire feature flags
	// from the REAL metrics state rather than assuming it. metrics.NewFactory
	// reports Enabled=false when the provider disables metrics or built-in
	// setup fails (client-UID / exporter init), so the advertised
	// ClientSideMetricsEnabled matches what this client can actually emit.
	//
	// session.NewClient builds the live factory from the same provider below;
	// this instance exists only to read .Enabled, so shut it down immediately.
	// The accelerator has no separate classic data path that would use a second
	// factory, and leaving it running would double-export session's metrics.
	factory, err := metrics.NewFactory(ctx, project, instance, appProfile, metricsProvider, opts...)
	if err != nil {
		return nil, err
	}
	metricsEnabled := factory.Enabled
	factory.Shutdown()

	// One FeatureFlags proto, from the same btransport source of truth the
	// classic client uses. session.NewClient reuses this single reference for
	// both the bigtable-features header and OpenSessionRequest.Flags, so the two
	// stay byte-identical (the server rejects OpenSession with INVALID_ARGUMENT
	// when they disagree).
	featureFlags := btransport.NewFeatureFlagsProto(btransport.FeatureFlagsInput{
		ClientSideMetricsEnabled: metricsEnabled,
		// Advertise direct access, matching the classic client's default
		// (isDirectAccessEnabled → !DisableDirectAccess, i.e. true).
		EnableDirectAccess: true,
	})

	return session.NewClient(ctx, project, instance, appProfile, metricsProvider, featureFlags, opts...)
}

// Channel is an in-process grpc.ClientConnInterface backed by
// internal/session. It owns a session.Client for its lifetime. One channel per
// (project, instance, appProfile).
//
// project and instance are retained so the per-RPC dispatch path (layered on
// top of this scaffold) can validate that an incoming V2 resource name targets
// this daemon's scope before stripping it to the leaf ID session.Client
// expects.
type Channel struct {
	sc       session.Client
	project  string
	instance string
}

// NewChannel constructs an Channel scoped to
// (project, instance, appProfile). It dials the underlying session.Client,
// which the Channel owns and closes via Close (below).
func NewChannel(
	ctx context.Context,
	project, instance, appProfile string,
	opts ...option.ClientOption,
) (*Channel, error) {
	// session.NewClient forwards opts straight to gtransport.Dial
	// without supplying a default endpoint, so without these the dial target
	// is empty ("received empty target in Build()"). Establish the standard
	// Bigtable data-plane endpoint, scope, and user agent first, then let the
	// caller's opts override. Mirrors bigtable.NewClient's use of
	// btopt.DefaultClientOptions.
	defaultOpts, err := btopt.DefaultClientOptions(prodAddr, mtlsProdAddr, dataScope, userAgent)
	if err != nil {
		return nil, err
	}
	opts = append(defaultOpts, opts...)

	sc, err := newSessionClient(ctx, project, instance, appProfile, opts...)
	if err != nil {
		return nil, err
	}
	return &Channel{
		sc:       sc,
		project:  project,
		instance: instance,
	}, nil
}

// Invoke implements grpc.ClientConnInterface for unary V2 RPCs. The per-method
// dispatch is layered on top of this scaffold; until then every method reports
// Unimplemented.
func (c *Channel) Invoke(ctx context.Context, method string, args, reply interface{}, _ ...grpc.CallOption) error {
	return status.Errorf(codes.Unimplemented, "accelerator: method %s not implemented", method)
}

// NewStream implements grpc.ClientConnInterface for streaming V2 RPCs. The
// per-method dispatch is layered on top of this scaffold; until then every
// streaming method reports Unimplemented.
func (c *Channel) NewStream(ctx context.Context, _ *grpc.StreamDesc, method string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, status.Errorf(codes.Unimplemented, "accelerator: streaming method %s not implemented", method)
}

// Close releases resources held by the channel by closing the underlying
// session.Client and all its pools.
func (c *Channel) Close() error {
	if c.sc == nil {
		return nil
	}
	return c.sc.Close()
}
