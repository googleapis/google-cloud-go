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
	"net/url"
	"os"
	"reflect"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	btopt "cloud.google.com/go/bigtable/internal/option"
	btransport "cloud.google.com/go/bigtable/internal/transport"
	"cloud.google.com/go/internal/trace"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// Client is a client for reading and writing data to tables in an instance.
//
// A Client is safe to use concurrently, except for its Close method.
type Client struct {
	classicPool                managedChannelPool
	client                     btpb.BigtableClient
	sessionClient              btpb.BigtableClient
	project, instance          string
	appProfile                 string
	metricsTracerFactory       *builtinMetricsTracerFactory
	disableRetryInfo           bool
	retryOption                gax.CallOption
	executeQueryRetryOption    gax.CallOption
	featureFlagsMD             metadata.MD // Pre-computed feature flags metadata to be sent with each request.
	configManager              *btransport.ClientConfigurationManager
	sessionMgr                 *SessionManager
	diverter                   *btransport.Diverter
	config                     ClientConfig
	backgroundCtx              context.Context
	backgroundCancel           context.CancelFunc
}

// ClientConfig has configurations for the client.
type ClientConfig struct {
	// The id of the app profile to associate with all data operations sent from this client.
	// If unspecified, the default app profile for the instance will be used.
	AppProfile string

	// If not set or set to nil, client side metrics will be collected and exported
	//
	// To disable client side metrics, set 'MetricsProvider' to 'NoopMetricsProvider'
	//
	// TODO: support user provided meter provider
	MetricsProvider MetricsProvider

	// DisableDynamicChannelPool disables the dynamic channel resizing based on load
	// Dynamic channel resizing  is enabled by default to resize based on load and avoid queuing of requests.
	DisableDynamicChannelPool bool

	// DisableConnectionRecycler disables the automatic preemptive refresh of connection.
	// Preemptive connection is default to true
	DisableConnectionRecycler bool

	DisableDirectAccess bool

	// EnableSessionPool enables the dedicated session pool infrastructure for vRPC operations.
	EnableSessionPool bool

	// SessionPoolMin configures the minimum number of sessions in the pool.
	SessionPoolMin int

	// SessionPoolMax configures the maximum number of sessions in the pool.
	SessionPoolMax int
}

// MetricsProvider is a wrapper for built in metrics meter provider
type MetricsProvider interface {
	isMetricsProvider()
}

// NoopMetricsProvider can be used to disable built in metrics
type NoopMetricsProvider struct{}

func (NoopMetricsProvider) isMetricsProvider() {}

// NewClient creates a new Client for a given project and instance.
// The default ClientConfig will be used.
func NewClient(ctx context.Context, project, instance string, opts ...option.ClientOption) (*Client, error) {
	return NewClientWithConfig(ctx, project, instance, ClientConfig{}, opts...)
}

// NewClientWithConfig creates a new client with the given config.
func NewClientWithConfig(ctx context.Context, project, instance string, config ClientConfig, opts ...option.ClientOption) (*Client, error) {
	clientCreationTimestamp := time.Now()
	metricsProvider := config.MetricsProvider
	if emulatorAddr := os.Getenv("BIGTABLE_EMULATOR_HOST"); emulatorAddr != "" {
		// Do not emit metrics when emulator is being used
		metricsProvider = NoopMetricsProvider{}
	}

	// Create a OpenTelemetry metrics configuration
	metricsTracerFactory, err := newBuiltinMetricsTracerFactory(ctx, project, instance, config.AppProfile, metricsProvider, opts...)
	if err != nil {
		return nil, err
	}

	o, err := btopt.DefaultClientOptions(prodAddr, mtlsProdAddr, Scope, clientUserAgent)
	if err != nil {
		return nil, err
	}
	// for otel metrics
	if metricsTracerFactory.enabled {
		if len(metricsTracerFactory.clientOpts) > 0 {
			o = append(o, metricsTracerFactory.clientOpts...)
		}
	}

	// Add gRPC client interceptors to supply Google client information. No external interceptors are passed.
	o = append(o, btopt.ClientInterceptorOptions(nil, nil)...)
	o = append(o, option.WithGRPCDialOption(grpc.WithStatsHandler(sharedLatencyStatsHandler)))
	// Default to a small connection pool that can be overridden.
	o = append(o,
		option.WithGRPCConnectionPool(4),
		// Set the max size to correspond to server-side limits.
		option.WithGRPCDialOption(grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(1<<28), grpc.MaxCallRecvMsgSize(1<<28))),
	)

	var directPathOptions = []option.ClientOption{
		internaloption.EnableDirectPath(true),
		internaloption.EnableDirectPathXds(),
	}

	// Allow non-default service account in DirectPath.
	o = append(o, internaloption.AllowNonDefaultServiceAccount(true))
	o = append(o, opts...)

	// TODO(b/372244283): Remove after b/358175516 has been fixed
	o = append(o, internaloption.EnableAsyncRefreshDryRun(metricsTracerFactory.newAsyncRefreshErrHandler()))

	disableRetryInfo := false

	// If DISABLE_RETRY_INFO=1, library does not base retry decision and back off time on server returned RetryInfo value.
	disableRetryInfoEnv := os.Getenv("DISABLE_RETRY_INFO")
	disableRetryInfo = disableRetryInfoEnv == "1"
	retryOption := defaultRetryOption
	executeQueryRetryOption := defaultExecuteQueryRetryOption
	if disableRetryInfo {
		retryOption = clientOnlyRetryOption
		executeQueryRetryOption = clientOnlyExecuteQueryRetryOption
	}

	// Create the feature flags metadata with direct access enabled
	// setting feature flags for direct access is good
	// as CFE/GFE will call RLS with gslb target type
	// only TD calls the RLS with grpc target type
	// and we evaluate the directAccess option after that.
	directAccessMD := createFeatureFlagsMD(metricsTracerFactory.enabled, disableRetryInfo, true)

	enableBigtableConnPool := btopt.EnableBigtableConnectionPool()
	grpcConnOptType := reflect.TypeOf(option.WithGRPCConn(nil))
	for _, opt := range opts {
		if reflect.TypeOf(opt) == grpcConnOptType {
			enableBigtableConnPool = false
			break
		}
	}
	var classicManaged managedChannelPool
	var classicErr error
	classicManaged, classicErr = createAndStartManagedChannelPool(ctx, project, instance, config, metricsTracerFactory, o, directPathOptions, directAccessMD, clientCreationTimestamp, enableBigtableConnPool)
	if classicErr != nil {
		return nil, classicErr
	}
	btClient := btpb.NewBigtableClient(classicManaged.pool)

	var sessionManaged managedChannelPool
	var sessionClient btpb.BigtableClient
	if config.EnableSessionPool {
		var sessionErr error
		sessionManaged, sessionErr = createAndStartManagedChannelPool(ctx, project, instance, config, metricsTracerFactory, o, directPathOptions, directAccessMD, clientCreationTimestamp, enableBigtableConnPool)
		if sessionErr != nil {
			classicManaged.Close()
			return nil, fmt.Errorf("failed to create dedicated session pool: %w", sessionErr)
		}
		sessionClient = btpb.NewBigtableClient(sessionManaged.pool)
	}

	c := &Client{
		classicPool:             classicManaged,
		client:                  btClient,
		sessionClient:           sessionClient,
		project:                 project,
		instance:                instance,
		appProfile:              config.AppProfile,
		metricsTracerFactory:    metricsTracerFactory,
		disableRetryInfo:        disableRetryInfo,
		retryOption:             retryOption,
		executeQueryRetryOption: executeQueryRetryOption,
		featureFlagsMD:          directAccessMD,
		config:                  config,
		diverter:                btransport.NewDiverter(1.0),
	}
	c.backgroundCtx, c.backgroundCancel = context.WithCancel(context.Background())

	configMD := metadata.Join(metadata.Pairs(
		resourcePrefixHeader, c.fullInstanceName(),
		requestParamsHeader, c.reqParamsHeaderValInstance(),
	), c.featureFlagsMD)

	configManager := btransport.NewClientConfigurationManager(btClient, c.fullInstanceName(), config.AppProfile, configMD, nil)
	configManager.Start(ctx)
	c.configManager = configManager

	c.sessionMgr = NewSessionManager(
		config.EnableSessionPool,
		metricsTracerFactory.enabled,
		disableRetryInfo,
		directAccessMD,
		c.diverter,
		configManager,
		c.backgroundCtx,
		config.SessionPoolMin,
		config.SessionPoolMax,
		metricsTracerFactory.otelMeterProvider,
		sessionManaged,
	)

	return c, nil
}

// Close closes the Client.
func (c *Client) Close() error {
	fmt.Printf("Closing the client for project %s and instance %s\n", c.project, c.instance)
	if c.backgroundCancel != nil {
		c.backgroundCancel()
	}
	if c.configManager != nil {
		c.configManager.Close()
	}
	if c.metricsTracerFactory != nil {
		c.metricsTracerFactory.shutdown()
	}

	var sessionErr error
	if c.sessionMgr != nil {
		sessionErr = c.sessionMgr.Close()
	}

	classicErr := c.classicPool.Close()
	if sessionErr != nil {
		return sessionErr
	}
	return classicErr
}

func (c *Client) fullInstanceName() string {
	return fmt.Sprintf("projects/%s/instances/%s", c.project, c.instance)
}

func (c *Client) fullTableName(table string) string {
	return fmt.Sprintf("projects/%s/instances/%s/tables/%s", c.project, c.instance, table)
}

func (c *Client) fullAuthorizedViewName(table string, authorizedView string) string {
	return fmt.Sprintf("projects/%s/instances/%s/tables/%s/authorizedViews/%s", c.project, c.instance, table, authorizedView)
}

func (c *Client) fullMaterializedViewName(materializedView string) string {
	return fmt.Sprintf("projects/%s/instances/%s/materializedViews/%s", c.project, c.instance, materializedView)
}

func (c *Client) reqParamsHeaderValTable(table string) string {
	return fmt.Sprintf("table_name=%s&app_profile_id=%s", url.QueryEscape(c.fullTableName(table)), url.QueryEscape(c.appProfile))
}

func (c *Client) reqParamsHeaderValInstance() string {
	return fmt.Sprintf("name=%s&app_profile_id=%s", url.QueryEscape(c.fullInstanceName()), url.QueryEscape(c.appProfile))
}



// PingAndWarm pings the server and warms up the connection.
func (c *Client) PingAndWarm(ctx context.Context) (err error) {
	md := metadata.Join(metadata.Pairs(
		resourcePrefixHeader, c.fullInstanceName(),
		requestParamsHeader, c.reqParamsHeaderValInstance(),
	), c.featureFlagsMD)

	ctx = mergeOutgoingMetadata(ctx, md)
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/bigtable/PingAndWarm")
	defer func() { trace.EndSpan(ctx, err) }()
	mt := c.newBuiltinMetricsTracer(ctx, "", false)
	defer mt.recordOperationCompletion()

	err = c.pingerWithMetadata(ctx, mt)
	statusCode, statusErr := convertToGrpcStatusErr(err)
	mt.currOp.setStatus(statusCode.String())
	return statusErr
}

func (c *Client) pingerWithMetadata(ctx context.Context, mt *builtinMetricsTracer) (err error) {
	req := &btpb.PingAndWarmRequest{
		Name:         c.fullInstanceName(),
		AppProfileId: c.appProfile,
	}
	err = gaxInvokeWithRecorder(ctx, mt, "PingAndWarm", func(ctx context.Context, headerMD, trailerMD *metadata.MD, _ gax.CallSettings) error {
		var err error
		_, err = c.client.PingAndWarm(ctx, req, grpc.Header(headerMD), grpc.Trailer(trailerMD))
		return err
	})

	return err

}

func (c *Client) newBuiltinMetricsTracer(ctx context.Context, table string, isStreaming bool) *builtinMetricsTracer {
	mt := c.metricsTracerFactory.createBuiltinMetricsTracer(ctx, table, isStreaming)
	return &mt
}

