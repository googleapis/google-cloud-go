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
	"google.golang.org/api/option"
	htransport "google.golang.org/api/transport/http"
	"google.golang.org/grpc"
)

type clientPool struct {
	New     func() *storage.Client
	clients []*storage.Client
}

func (p *clientPool) Get() *storage.Client {
	if p.clients == nil {
		p.clients = make([]*storage.Client, 0, opts.numWorkers)
	}

	if len(p.clients) > 0 {
		c := p.clients[0]
		p.clients = p.clients[1:]
		return c
	}

	return p.New()
}

func (p *clientPool) Put(c *storage.Client) {
	p.clients = append(p.clients, c)
}

// we can share clients as long as the app buffer sizes are constant
var httpClients, gRPCClients *clientPool

var nonBenchmarkingClients = clientPool{
	New: func() *storage.Client {
		// we don't care if it's grpc or http, so we don't need mutex
		client, err := storage.NewClient(context.Background())
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

func canUseClientPool(opts *benchmarkOptions) bool {
	return opts.useDefaults || (opts.maxReadSize == opts.minReadSize && opts.maxWriteSize == opts.minWriteSize)
}

func getClient(ctx context.Context, opts *benchmarkOptions, br benchmarkResult) (*storage.Client, func() error, error) {
	noOp := func() error { return nil }
	if canUseClientPool(opts) {
		if br.params.api == grpcAPI {
			return gRPCClients.Get(), noOp, nil
		}
		return httpClients.Get(), noOp, nil
	}

	// if necessary, create a client
	if br.params.api == grpcAPI {
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
