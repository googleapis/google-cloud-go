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

package bigtable

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc/metadata"

	btopt "cloud.google.com/go/bigtable/internal/option"
	btransport "cloud.google.com/go/bigtable/internal/transport"
)

// managedChannelPool encapsulates a connection pool along with its lifecycle monitors.
type managedChannelPool struct {
	pool         gtransport.ConnPool
	dsm          *btransport.DynamicScaleMonitor
	connRecycler *btransport.ConnectionRecycler
}

// Close stops all associated monitors/recyclers and closes the underlying pool.
func (m managedChannelPool) Close() error {
	if m.dsm != nil {
		m.dsm.Stop()
	}
	if m.connRecycler != nil {
		m.connRecycler.Stop()
	}
	if m.pool != nil {
		return m.pool.Close()
	}
	return nil
}

// createAndStartManagedChannelPool initializes and starts the lifecycle monitors for a classic or session connection pool.
func createAndStartManagedChannelPool(
	ctx context.Context,
	project, instance string,
	config ClientConfig,
	metricsTracerFactory *builtinMetricsTracerFactory,
	o []option.ClientOption,
	directPathOptions []option.ClientOption,
	directAccessMD metadata.MD,
	clientCreationTimestamp time.Time,
	enableBigtableConnPool bool,
) (managedChannelPool, error) {
	var m managedChannelPool
	if !enableBigtableConnPool {
		var err error
		m.pool, err = gtransport.DialPool(ctx, o...)
		return m, err
	}

	pool, err := createBigtableChannelPool(ctx, project, instance, config, metricsTracerFactory, o, directPathOptions, directAccessMD, clientCreationTimestamp)
	if err != nil {
		return m, err
	}
	m.pool = pool

	// Validate dynamic config early if enabled
	if !config.DisableDynamicChannelPool {
		if err := btransport.ValidateDynamicConfig(btopt.DefaultDynamicChannelPoolConfig(), defaultBigtableConnPoolSize); err != nil {
			pool.Close()
			return m, fmt.Errorf("invalid DynamicChannelPoolConfig: %w", err)
		}

		m.dsm = btransport.NewDynamicScaleMonitor(btopt.DefaultDynamicChannelPoolConfig(), pool)
		m.dsm.Start(ctx)
	}

	// connection recycler
	if !config.DisableConnectionRecycler {
		m.connRecycler = btransport.NewConnectionRecycler(btopt.DefaultConnectionRecycleConfig(), pool)
		m.connRecycler.Start(ctx)
	}

	return m, nil
}

// createBigtableChannelPool is a helper function to initialize a separate BigtableChannelPool instance.
func createBigtableChannelPool(
	ctx context.Context,
	project, instance string,
	config ClientConfig,
	metricsTracerFactory *builtinMetricsTracerFactory,
	o []option.ClientOption,
	directPathOptions []option.ClientOption,
	directAccessMD metadata.MD,
	clientCreationTimestamp time.Time,
) (*btransport.BigtableChannelPool, error) {
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

	poolOpts := []btransport.BigtableChannelPoolOption{
		btransport.WithInstanceName(fullInstanceName),
		btransport.WithAppProfile(config.AppProfile),
		btransport.WithFeatureFlagsMetadata(directAccessMD),
		btransport.WithMetricsReporterConfig(btopt.DefaultMetricsReporterConfig()),
		btransport.WithMeterProvider(metricsTracerFactory.otelMeterProvider),
		btransport.WithDirectAccessFeatureFlagsMetadata(directAccessMD),
	}

	if isDirectAccessEnabled(config) {
		directAccessDialerOptions := make([]option.ClientOption, len(o))
		copy(directAccessDialerOptions, o)
		directAccessDialerOptions = append(directAccessDialerOptions, directPathOptions...)
		directAccessDialerOptions = append(directAccessDialerOptions, internaloption.AllowHardBoundTokens("ALTS"))

		directAccessDialer := func() (*btransport.BigtableConn, error) {
			grpcConn, err := gtransport.Dial(ctx, directAccessDialerOptions...)
			if err != nil {
				return nil, err
			}
			return btransport.NewBigtableConn(grpcConn), nil
		}
		poolOpts = append(poolOpts, btransport.WithDirectAccessDialer(directAccessDialer))
	}


return btransport.NewBigtableChannelPool(ctx,
		connPoolSize,
		btopt.BigtableLoadBalancingStrategy(),
		func() (*btransport.BigtableConn, error) {
			grpcConn, err := gtransport.Dial(ctx, o...)
			if err != nil {
				return nil, err
			}
			return btransport.NewBigtableConn(grpcConn), nil
		},
		clientCreationTimestamp,
		poolOpts...,
	)
}
