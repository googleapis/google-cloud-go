/*
Copyright 2016 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bigtable

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/bigtable/bttest"
	btopt "cloud.google.com/go/bigtable/internal/option"
	"cloud.google.com/go/internal/testutil"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

var legacyUseProd string
var integrationConfig IntegrationTestConfig

func init() {
	c := &integrationConfig

	flag.BoolVar(&c.UseProd, "it.use-prod", false, "Use remote bigtable instead of local emulator")
	flag.StringVar(&c.AdminEndpoint, "it.admin-endpoint", "", "Admin api host and port")
	flag.StringVar(&c.DataEndpoint, "it.data-endpoint", "", "Data api host and port")
	flag.StringVar(&c.Project, "it.project", "", "Project to use for integration test")
	flag.StringVar(&c.Instance, "it.instance", "", "Bigtable instance to use")
	flag.StringVar(&c.Cluster, "it.cluster", "", "Bigtable cluster to use")
	flag.StringVar(&c.Table, "it.table", "", "Bigtable table to create")
	flag.BoolVar(&c.AttemptDirectPath, "it.attempt-directpath", false, "Attempt DirectPath")
	flag.BoolVar(&c.DirectPathIPV4Only, "it.directpath-ipv4-only", false, "Run DirectPath on a ipv4-only VM")

	// Backwards compat
	flag.StringVar(&legacyUseProd, "use_prod", "", `DEPRECATED: if set to "proj,instance,table", run integration test against production`)

}

// IntegrationTestConfig contains parameters to pick and setup a IntegrationEnv for testing
type IntegrationTestConfig struct {
	UseProd            bool
	AdminEndpoint      string
	DataEndpoint       string
	Project            string
	Instance           string
	Cluster            string
	Table              string
	AttemptDirectPath  bool
	DirectPathIPV4Only bool
}

// IntegrationEnv represents a testing environment.
// The environment can be implemented using production or an emulator
type IntegrationEnv interface {
	Config() IntegrationTestConfig
	NewAdminClient() (*AdminClient, error)
	// NewInstanceAdminClient will return nil if instance administration is unsupported in this environment
	NewInstanceAdminClient() (*InstanceAdminClient, error)
	NewClient() (*Client, error)
	Close()
	Peer() *peer.Peer
}

// NewIntegrationEnv creates a new environment based on the command line args
func NewIntegrationEnv() (IntegrationEnv, error) {
	c := integrationConfig

	if legacyUseProd != "" {
		fmt.Println("WARNING: using legacy commandline arg -use_prod, please switch to -it.*")
		parts := strings.SplitN(legacyUseProd, ",", 3)
		c.UseProd = true
		c.Project = parts[0]
		c.Instance = parts[1]
		c.Table = parts[2]
	}

	if integrationConfig.UseProd {
		return NewProdEnv(c)
	}
	return NewEmulatedEnv(c)
}

// EmulatedEnv encapsulates the state of an emulator
type EmulatedEnv struct {
	config IntegrationTestConfig
	server *bttest.Server
}

// NewEmulatedEnv builds and starts the emulator based environment
func NewEmulatedEnv(config IntegrationTestConfig) (*EmulatedEnv, error) {
	srv, err := bttest.NewServer("localhost:0", grpc.MaxRecvMsgSize(200<<20), grpc.MaxSendMsgSize(100<<20))
	if err != nil {
		return nil, err
	}

	if config.Project == "" {
		config.Project = "project"
	}
	if config.Instance == "" {
		config.Instance = "instance"
	}
	if config.Table == "" {
		config.Table = "mytable"
	}
	config.AdminEndpoint = srv.Addr
	config.DataEndpoint = srv.Addr

	env := &EmulatedEnv{
		config: config,
		server: srv,
	}
	return env, nil
}

func (e *EmulatedEnv) Peer() *peer.Peer {
	return nil
}

// Close stops & cleans up the emulator
func (e *EmulatedEnv) Close() {
	e.server.Close()
}

// Config gets the config used to build this environment
func (e *EmulatedEnv) Config() IntegrationTestConfig {
	return e.config
}

var headersInterceptor = testutil.DefaultHeadersEnforcer()

// NewAdminClient builds a new connected admin client for this environment
func (e *EmulatedEnv) NewAdminClient() (*AdminClient, error) {
	o, err := btopt.DefaultClientOptions(e.server.Addr, e.server.Addr, AdminScope, clientUserAgent)
	if err != nil {
		return nil, err
	}
	// Add gRPC client interceptors to supply Google client information.
	//
	// Inject interceptors from headersInterceptor, since they are used to verify
	// client requests under test.
	o = append(o, btopt.ClientInterceptorOptions(
		headersInterceptor.StreamInterceptors(),
		headersInterceptor.UnaryInterceptors())...)

	timeout := 20 * time.Second
	ctx, _ := context.WithTimeout(context.Background(), timeout)

	o = append(o, option.WithGRPCDialOption(grpc.WithBlock()))
	conn, err := gtransport.DialInsecure(ctx, o...)
	if err != nil {
		return nil, err
	}

	return NewAdminClient(ctx, e.config.Project, e.config.Instance,
		option.WithGRPCConn(conn))
}

// NewInstanceAdminClient returns nil for the emulated environment since the API is not implemented.
func (e *EmulatedEnv) NewInstanceAdminClient() (*InstanceAdminClient, error) {
	return nil, nil
}

// NewClient builds a new connected data client for this environment
func (e *EmulatedEnv) NewClient() (*Client, error) {
	o, err := btopt.DefaultClientOptions(e.server.Addr, e.server.Addr, Scope, clientUserAgent)
	if err != nil {
		return nil, err
	}
	// Add gRPC client interceptors to supply Google client information.
	//
	// Inject interceptors from headersInterceptor, since they are used to verify
	// client requests under test.
	o = append(o, btopt.ClientInterceptorOptions(
		headersInterceptor.StreamInterceptors(),
		headersInterceptor.UnaryInterceptors())...)

	timeout := 20 * time.Second
	ctx, _ := context.WithTimeout(context.Background(), timeout)

	o = append(o, option.WithGRPCDialOption(grpc.WithBlock()))
	o = append(o, option.WithGRPCDialOption(grpc.WithDefaultCallOptions(
		grpc.MaxCallSendMsgSize(100<<20), grpc.MaxCallRecvMsgSize(100<<20))))
	conn, err := gtransport.DialInsecure(ctx, o...)
	if err != nil {
		return nil, err
	}
	return NewClient(ctx, e.config.Project, e.config.Instance, option.WithGRPCConn(conn))
}

// ProdEnv encapsulates the state necessary to connect to the external Bigtable service
type ProdEnv struct {
	config   IntegrationTestConfig
	peerInfo *peer.Peer
}

// NewProdEnv builds the environment representation
func NewProdEnv(config IntegrationTestConfig) (*ProdEnv, error) {
	if config.Project == "" {
		return nil, errors.New("Project not set")
	}
	if config.Instance == "" {
		return nil, errors.New("Instance not set")
	}
	if config.Table == "" {
		return nil, errors.New("Table not set")
	}

	env := &ProdEnv{
		config:   config,
		peerInfo: &peer.Peer{},
	}
	return env, nil
}

func (e *ProdEnv) Peer() *peer.Peer {
	return e.peerInfo
}

// Close is a no-op for production environments
func (e *ProdEnv) Close() {}

// Config gets the config used to build this environment
func (e *ProdEnv) Config() IntegrationTestConfig {
	return e.config
}

// NewAdminClient builds a new connected admin client for this environment
func (e *ProdEnv) NewAdminClient() (*AdminClient, error) {
	clientOpts := headersInterceptor.CallOptions()
	if endpoint := e.config.AdminEndpoint; endpoint != "" {
		clientOpts = append(clientOpts, option.WithEndpoint(endpoint))
	}
	return NewAdminClient(context.Background(), e.config.Project, e.config.Instance, clientOpts...)
}

// NewInstanceAdminClient returns a new connected instance admin client for this environment
func (e *ProdEnv) NewInstanceAdminClient() (*InstanceAdminClient, error) {
	clientOpts := headersInterceptor.CallOptions()
	if endpoint := e.config.AdminEndpoint; endpoint != "" {
		clientOpts = append(clientOpts, option.WithEndpoint(endpoint))
	}
	return NewInstanceAdminClient(context.Background(), e.config.Project, clientOpts...)
}

// NewClient builds a connected data client for this environment
func (e *ProdEnv) NewClient() (*Client, error) {
	clientOpts := headersInterceptor.CallOptions()
	if endpoint := e.config.DataEndpoint; endpoint != "" {
		clientOpts = append(clientOpts, option.WithEndpoint(endpoint))
	}

	if e.config.AttemptDirectPath {
		// TODO(mohanli): Move the EnableDirectPath internal option to bigtable.go after e2e tests are done.
		clientOpts = append(clientOpts, internaloption.EnableDirectPath(true))
		// For DirectPath tests, we need to add an interceptor to check the peer IP.
		clientOpts = append(clientOpts, option.WithGRPCDialOption(grpc.WithDefaultCallOptions(grpc.Peer(e.peerInfo))))
	}

	return NewClient(context.Background(), e.config.Project, e.config.Instance, clientOpts...)
}
