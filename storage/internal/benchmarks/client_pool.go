// Copyright 2022 Google LLC
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

package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"golang.org/x/net/http2"
	"google.golang.org/api/option"
	htransport "google.golang.org/api/transport/http"
	"google.golang.org/grpc"
)

// clientPool functions much like a sync Pool (https://pkg.go.dev/sync#Pool),
// except it does not automatically remove items stored in the clientPool.
// Re-using the clients rather than creating a new one each time reduces overhead
// (such as re-creating the underlying HTTP client and opening credential files),
// and is the intended way to use Storage clients.
//
// There is no limit to how many clients will be created, but it should be around
// the order of 5 * min(workers, max_samples).
type clientPool struct {
	New     func() *storage.Client
	clients []*storage.Client
}

func (p *clientPool) Get() *storage.Client {
	// Create the slice if not already created
	if p.clients == nil {
		p.clients = make([]*storage.Client, 0)
	}

	// If there is an unused client, return it
	if len(p.clients) > 0 {
		c := p.clients[0]
		p.clients = p.clients[1:]
		return c
	}

	// Otherwise, create a new client and return it
	return p.New()
}

func (p *clientPool) Put(c *storage.Client) {
	p.clients = append(p.clients, c)
}

// we can share clients as long as the app buffer sizes are constant
var httpClients, gRPCClients *clientPool

var nonBenchmarkingClients = clientPool{
	New: func() *storage.Client {
		// For debuggability's sake, these are HTTP
		clientMu.Lock()
		client, err := storage.NewClient(context.Background())
		clientMu.Unlock()
		if err != nil {
			log.Fatalf("storage.NewClient: %v", err)
		}

		return client
	},
}

func initializeClientPools(opts *benchmarkOptions) func() {
	httpClients = &clientPool{
		New: func() *storage.Client {
			client, err := initializeHTTPClient(context.Background(), opts.minWriteSize, opts.maxReadSize, opts.useDefaults)
			if err != nil {
				log.Fatalf("initializeHTTPClient: %v", err)
			}

			return client
		},
	}

	gRPCClients = &clientPool{
		New: func() *storage.Client {
			client, err := initializeGRPCClient(context.Background(), opts.minWriteSize, opts.maxReadSize, opts.connPoolSize, opts.useDefaults)
			if err != nil {
				log.Fatalf("initializeGRPCClient: %v", err)
			}
			return client
		},
	}

	return func() {
		for _, c := range httpClients.clients {
			c.Close()
		}
		for _, c := range gRPCClients.clients {
			c.Close()
		}
	}
}

// We can't pool storage clients if we need to change parameters at the HTTP or GRPC client level,
// since we can't access those after creation as it is set up now.
// If we are using defaults (ie. not creating an underlying HTTP client ourselves), or if
// we are only interested in one app buffer size at a time, we don't need to change anything on the underlying
// client and can re-use it (and therefore the storage client) for other benchmark runs.
func canUseClientPool(opts *benchmarkOptions) bool {
	return opts.useDefaults || (opts.maxReadSize == opts.minReadSize && opts.maxWriteSize == opts.minWriteSize)
}

func getClient(ctx context.Context, opts *benchmarkOptions, br benchmarkResult) (*storage.Client, func() error, error) {
	noOp := func() error { return nil }
	grpc := br.params.api == grpcAPI || br.params.api == directPath
	if canUseClientPool(opts) {
		if grpc {
			c := gRPCClients.Get()
			return c, func() error { gRPCClients.Put(c); return nil }, nil
		}
		c := httpClients.Get()
		return c, func() error { httpClients.Put(c); return nil }, nil
	}

	// if necessary, create a client
	if grpc {
		c, err := initializeGRPCClient(ctx, br.params.appBufferSize, br.params.appBufferSize, opts.connPoolSize, false)
		if err != nil {
			return nil, noOp, fmt.Errorf("initializeGRPCClient: %w", err)
		}
		return c, c.Close, nil
	}
	c, err := initializeHTTPClient(ctx, br.params.appBufferSize, br.params.appBufferSize, false)
	if err != nil {
		return nil, noOp, fmt.Errorf("initializeHTTPClient: %w", err)
	}
	return c, c.Close, nil
}

// mutex on starting a client so that we can set an env variable for GRPC clients
var clientMu sync.Mutex

func initializeHTTPClient(ctx context.Context, writeBufferSize, readBufferSize int, useDefaults bool) (*storage.Client, error) {
	if useDefaults {
		clientMu.Lock()
		c, err := storage.NewClient(ctx, option.WithCredentialsFile(credentialsFile))
		clientMu.Unlock()
		return c, err
	}

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	// These are the default parameters with write and read buffer sizes modified
	base := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		WriteBufferSize:       writeBufferSize,
		ReadBufferSize:        readBufferSize,
	}

	http2Trans, err := http2.ConfigureTransports(base)
	if err == nil {
		http2Trans.ReadIdleTimeout = time.Second * 31
	}

	trans, err := htransport.NewTransport(ctx, base,
		option.WithScopes("https://www.googleapis.com/auth/devstorage.full_control"),
		option.WithCredentialsFile(credentialsFile))
	if err != nil {
		return nil, err
	}

	clientMu.Lock()
	client, err := storage.NewClient(ctx, option.WithHTTPClient(&http.Client{Transport: trans}))
	clientMu.Unlock()

	return client, err
}

func initializeGRPCClient(ctx context.Context, writeBufferSize, readBufferSize int, connPoolSize int, useDefaults bool) (*storage.Client, error) {
	if useDefaults {
		clientMu.Lock()
		os.Setenv("STORAGE_USE_GRPC", "true")
		c, err := storage.NewClient(ctx, option.WithCredentialsFile(credentialsFile))
		os.Unsetenv("STORAGE_USE_GRPC")
		clientMu.Unlock()
		return c, err
	}

	clientMu.Lock()
	os.Setenv("STORAGE_USE_GRPC", "true")
	client, err := storage.NewClient(ctx, option.WithCredentialsFile(credentialsFile),
		option.WithGRPCDialOption(grpc.WithReadBufferSize(readBufferSize)),
		option.WithGRPCDialOption(grpc.WithWriteBufferSize(writeBufferSize)),
		option.WithGRPCConnectionPool(connPoolSize))
	os.Unsetenv("STORAGE_USE_GRPC")
	clientMu.Unlock()

	return client, err
}
