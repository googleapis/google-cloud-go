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
	"fmt"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/metric"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc/metadata"

	btopt "cloud.google.com/go/bigtable/internal/option"
)

const (
	directpathEnvVar            = "CBT_ENABLE_DIRECTPATH"
	defaultBigtableConnPoolSize = 10
)

// ChannelPoolConfig has configurations for the channel pool.
type ChannelPoolConfig struct {
	AppProfile                string
	DisableDynamicChannelPool bool
	DisableConnectionRecycler bool
	DisableDirectAccess       bool
}

// ManagedChannelPool encapsulates a connection pool along with its lifecycle monitors.
type ManagedChannelPool struct {
	Pool         gtransport.ConnPool
	Dsm          *DynamicScaleMonitor
	ConnRecycler *ConnectionRecycler
}

// Close stops all associated monitors/recyclers and closes the underlying pool.
func (m ManagedChannelPool) Close() error {
	if m.Dsm != nil {
		m.Dsm.Stop()
	}
	if m.ConnRecycler != nil {
		m.ConnRecycler.Stop()
	}
	if m.Pool != nil {
		return m.Pool.Close()
	}
	return nil
}

// CreateAndStartManagedChannelPool initializes and starts the lifecycle monitors for a classic or session connection pool.
func CreateAndStartManagedChannelPool(
	ctx context.Context,
	project, instance string,
	config ChannelPoolConfig,
	otelMeterProvider metric.MeterProvider,
	o []option.ClientOption,
	directPathOptions []option.ClientOption,
	directAccessMD metadata.MD,
	clientCreationTimestamp time.Time,
	enableBigtableConnPool bool,
) (ManagedChannelPool, error) {
	var m ManagedChannelPool
	if !enableBigtableConnPool {
		var err error
		m.Pool, err = gtransport.DialPool(ctx, o...)
		return m, err
	}

	pool, err := CreateBigtableChannelPool(ctx, project, instance, config, otelMeterProvider, o, directPathOptions, directAccessMD, clientCreationTimestamp)
	if err != nil {
		return m, err
	}
	m.Pool = pool

	// Validate dynamic config early if enabled
	if !config.DisableDynamicChannelPool {
		if err := ValidateDynamicConfig(btopt.DefaultDynamicChannelPoolConfig(), defaultBigtableConnPoolSize); err != nil {
			pool.Close()
			return m, fmt.Errorf("invalid DynamicChannelPoolConfig: %w", err)
		}

		m.Dsm = NewDynamicScaleMonitor(btopt.DefaultDynamicChannelPoolConfig(), pool)
		m.Dsm.Start(ctx)
	}

	// connection recycler
	if !config.DisableConnectionRecycler {
		m.ConnRecycler = NewConnectionRecycler(btopt.DefaultConnectionRecycleConfig(), pool)
		m.ConnRecycler.Start(ctx)
	}

	return m, nil
}

// CreateBigtableChannelPool is a helper function to initialize a separate BigtableChannelPool instance.
func CreateBigtableChannelPool(
	ctx context.Context,
	project, instance string,
	config ChannelPoolConfig,
	otelMeterProvider metric.MeterProvider,
	o []option.ClientOption,
	directPathOptions []option.ClientOption,
	directAccessMD metadata.MD,
	clientCreationTimestamp time.Time,
) (*BigtableChannelPool, error) {
	uResolver, err := internaloption.NewUnsafeResolver(o...)
	var connPoolSize int
	if err != nil {
		connPoolSize = defaultBigtableConnPoolSize
	} else {
		connPoolSize = uResolver.ResolvedGRPCConnPoolSize()
		if connPoolSize == 0 {
			connPoolSize = defaultBigtableConnPoolSize
		}
	}

	fullInstanceName := fmt.Sprintf("projects/%s/instances/%s", project, instance)

	poolOpts := []BigtableChannelPoolOption{
		WithInstanceName(fullInstanceName),
		WithAppProfile(config.AppProfile),
		WithFeatureFlagsMetadata(directAccessMD),
		WithMetricsReporterConfig(btopt.DefaultMetricsReporterConfig()),
		WithMeterProvider(otelMeterProvider),
		WithDirectAccessFeatureFlagsMetadata(directAccessMD),
	}

	if isDirectAccessEnabled(config) {
		directAccessDialerOptions := make([]option.ClientOption, len(o))
		copy(directAccessDialerOptions, o)
		directAccessDialerOptions = append(directAccessDialerOptions, directPathOptions...)
		directAccessDialerOptions = append(directAccessDialerOptions, internaloption.AllowHardBoundTokens("ALTS"))

		directAccessDialer := func() (*BigtableConn, error) {
			grpcConn, err := gtransport.Dial(ctx, directAccessDialerOptions...)
			if err != nil {
				return nil, err
			}
			return NewBigtableConn(grpcConn), nil
		}
		poolOpts = append(poolOpts, WithDirectAccessDialer(directAccessDialer))
	}

	return NewBigtableChannelPool(ctx,
		connPoolSize,
		btopt.BigtableLoadBalancingStrategy(),
		func() (*BigtableConn, error) {
			grpcConn, err := gtransport.Dial(ctx, o...)
			if err != nil {
				return nil, err
			}
			return NewBigtableConn(grpcConn), nil
		},
		clientCreationTimestamp,
		poolOpts...,
	)
}

func isDirectAccessEnabled(config ChannelPoolConfig) bool {
	if os.Getenv(directpathEnvVar) == "" {
		return !config.DisableDirectAccess
	}
	res, _ := strconv.ParseBool(os.Getenv(directpathEnvVar))
	return res
}
