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
	"log"
	"os"
	"reflect"
	"strconv"
	"sync"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	metrics "cloud.google.com/go/bigtable/internal/metrics"
	btopt "cloud.google.com/go/bigtable/internal/option"
	btransport "cloud.google.com/go/bigtable/internal/transport"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// SessionClient is a session-only handle to a Bigtable instance. It owns
// its own gRPC channel pool, stub, and ClientConfigurationManager, and
// is independent of any classic Client the caller may also have open.
//
// This first slice provides construction, the running configuration
// manager, and teardown. Per-resource SessionTable factory methods
// land in a follow-up.
type SessionClient struct {
	project    string
	instance   string
	appProfile string

	mPool                btransport.ManagedChannelPool
	stub                 btpb.BigtableClient
	configManager        *btransport.ClientConfigurationManager
	metricsTracerFactory *metrics.Factory

	disableRetryInfo        bool
	retryOption             gax.CallOption
	executeQueryRetryOption gax.CallOption
	featureFlagsMD          metadata.MD

	closeOnce sync.Once
	closeErr  error
}

// NewSessionClient creates a new SessionClient for the given project and
// instance. The returned client owns a session-dedicated channel pool
// and a running ClientConfigurationManager that polls the control
// plane for server-driven configuration updates.
//
// The manager's polling loop lives past NewSessionClient's return —
// use SessionClient.Close to tear it down.
func NewSessionClient(ctx context.Context, project, instance string, config ClientConfig, opts ...option.ClientOption) (*SessionClient, error) {
	clientCreationTimestamp := time.Now()
	metricsProvider := config.MetricsProvider
	if emulatorAddr := os.Getenv("BIGTABLE_EMULATOR_HOST"); emulatorAddr != "" {
		metricsProvider = NoopMetricsProvider{}
	}

	metricsTracerFactory, err := metrics.NewFactory(ctx, project, instance, config.AppProfile, metricsProvider, opts...)
	if err != nil {
		return nil, err
	}

	o, err := btopt.DefaultClientOptions(prodAddr, mtlsProdAddr, Scope, clientUserAgent)
	if err != nil {
		return nil, err
	}
	if metricsTracerFactory.Enabled {
		if len(metricsTracerFactory.ClientOpts) > 0 {
			o = append(o, metricsTracerFactory.ClientOpts...)
		}
	}

	o = append(o, btopt.ClientInterceptorOptions(nil, nil)...)
	o = append(o, option.WithGRPCDialOption(grpc.WithStatsHandler(metrics.SharedStatsHandler)))
	o = append(o,
		option.WithGRPCConnectionPool(defaultBigtableConnPoolSize),
		option.WithGRPCDialOption(grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(1<<28), grpc.MaxCallRecvMsgSize(1<<28))),
	)

	directAccessOptions := []option.ClientOption{
		internaloption.EnableDirectPath(true),
		internaloption.EnableDirectPathXds(),
		internaloption.AllowHardBoundTokens("ALTS"),
	}

	o = append(o, internaloption.AllowNonDefaultServiceAccount(true))
	o = append(o, opts...)
	o = append(o, internaloption.EnableNewAuthLibrary())
	o = append(o, internaloption.EnableJwtWithScope())

	disableRetryInfo := os.Getenv("DISABLE_RETRY_INFO") == "1"
	retryOption := defaultRetryOption
	executeQueryRetryOption := defaultExecuteQueryRetryOption
	if disableRetryInfo {
		retryOption = clientOnlyRetryOption
		executeQueryRetryOption = clientOnlyExecuteQueryRetryOption
	}

	allowDirectAccess := isDirectAccessEnabled(config)
	featureFlagsMD := createFeatureFlagsMD(metricsTracerFactory.Enabled, disableRetryInfo, allowDirectAccess)

	enableBigtableConnPool := btopt.EnableBigtableConnectionPool()
	grpcConnOptType := reflect.TypeOf(option.WithGRPCConn(nil))
	for _, opt := range opts {
		if reflect.TypeOf(opt) == grpcConnOptType {
			enableBigtableConnPool = false
			break
		}
	}
	if !enableBigtableConnPool {
		if enabled, _ := strconv.ParseBool(os.Getenv(directAccessEnvVar)); enabled {
			o = append(o, directAccessOptions...)
		}
	}

	poolConfig := btransport.ChannelPoolConfig{
		AppProfile:                config.AppProfile,
		DisableDynamicChannelPool: config.DisableDynamicChannelPool,
		DisableConnectionRecycler: config.DisableConnectionRecycler,
		DisableDirectAccess:       config.DisableDirectAccess,
	}

	mPool, err := btransport.CreateAndStartManagedChannelPool(
		ctx,
		project,
		instance,
		poolConfig,
		metricsTracerFactory.OtelMeterProvider,
		o,
		directAccessOptions,
		featureFlagsMD,
		clientCreationTimestamp,
		enableBigtableConnPool,
	)
	if err != nil {
		metricsTracerFactory.Shutdown()
		return nil, err
	}

	stub := btpb.NewBigtableClient(mPool.Pool)

	// The manager's polling loop must outlive NewSessionClient's caller
	// context, so start it on a background parent. Close teardown flows
	// through configManager.Close(), not ctx cancellation.
	instanceName := fmt.Sprintf("projects/%s/instances/%s", project, instance)
	configManager := btransport.NewClientConfigurationManager(stub, instanceName, config.AppProfile, featureFlagsMD, log.Default())
	configManager.Start(context.Background())

	return &SessionClient{
		project:                 project,
		instance:                instance,
		appProfile:              config.AppProfile,
		mPool:                   mPool,
		stub:                    stub,
		configManager:           configManager,
		metricsTracerFactory:    metricsTracerFactory,
		disableRetryInfo:        disableRetryInfo,
		retryOption:             retryOption,
		executeQueryRetryOption: executeQueryRetryOption,
		featureFlagsMD:          featureFlagsMD,
	}, nil
}

// Project returns the GCP project the client was constructed with.
func (c *SessionClient) Project() string { return c.project }

// Instance returns the Bigtable instance the client was constructed with.
func (c *SessionClient) Instance() string { return c.instance }

// AppProfile returns the app profile the client was constructed with.
// Empty string means "use the instance's default app profile."
func (c *SessionClient) AppProfile() string { return c.appProfile }

// ConfigurationManager returns the running ClientConfigurationManager.
// Callers can register listeners for server-driven configuration
// updates via the manager's own API.
func (c *SessionClient) ConfigurationManager() *btransport.ClientConfigurationManager {
	return c.configManager
}

// Close stops the ClientConfigurationManager's polling loop, shuts
// down the metrics tracer factory, and tears down the channel pool.
// Manager first so no in-flight GetClientConfiguration lands on a
// closed pool.
//
// Idempotent — subsequent calls return the first call's error.
func (c *SessionClient) Close() error {
	c.closeOnce.Do(func() {
		if c.configManager != nil {
			c.configManager.Close()
		}
		if c.metricsTracerFactory != nil {
			c.metricsTracerFactory.Shutdown()
		}
		c.closeErr = c.mPool.Close()
	})
	return c.closeErr
}
