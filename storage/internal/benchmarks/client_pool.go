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

// clientPool pools a number of Storage clients for use without blocking, ie.
// a client that is received through Get may still be in use by one or more other
// calls.
type clientPool struct {
	clients     []*storage.Client
	clientQueue chan *storage.Client
}

// newClientPool initializes the pool with numClients clients initialized using
// the initializeClient func.
// Returns the client pool and a cleanup func to be called to close the pool.
func newClientPool(initializeClient func() (*storage.Client, error), numClients int) (*clientPool, func()) {
	p := &clientPool{
		clients:     make([]*storage.Client, numClients),
		clientQueue: make(chan *storage.Client, numClients),
	}

	for i := 0; i < numClients; i++ {
		var err error
		p.clients[i], err = initializeClient()
		if err != nil {
			log.Fatalf("initializeClient: %v", err)
		}

		// Fill the queue with clients as they are created
		p.clientQueue <- p.clients[i]
	}

	return p, func() {
		for _, c := range p.clients {
			c.Close()
		}
	}
}

// Get rotates through clients. This means the work may not be evenly distributed,
// particularly if using varying object sizes.
// The rotation is not 100% deterministic (ie. clients may swap places in the
// queue) when using multiple workers.
func (p *clientPool) Get() *storage.Client {
	client := <-p.clientQueue

	// return client to queue so that it will be used again without blocking
	p.clientQueue <- client

	return client
}

var httpClients, gRPCClients, nonBenchmarkingClients *clientPool

// initializeClientPools creates separate client pools for HTTP and gRPC, and only
// creates those if required. For example, if the input parameter `api` is set to
// `JSON`, the HTTP client pool is initialized but not the GRPC pool.
func initializeClientPools(ctx context.Context, opts *benchmarkOptions) func() {
	var closeNonBenchmarking, closeHTTP, closeGRPC func()

	nonBenchmarkingClients, closeNonBenchmarking = newClientPool(
		func() (*storage.Client, error) {
			return initializeHTTPClient(ctx, clientParams{customClient: false})

		},
		1,
	)

	if opts.api == mixedAPIs || opts.api == jsonAPI || opts.api == xmlAPI {
		httpClients, closeHTTP = newClientPool(
			func() (*storage.Client, error) {
				return initializeHTTPClient(ctx, clientParams{
					customClient:    opts.allowCustomClient,
					writeBufferSize: opts.writeBufferSize,
					readBufferSize:  opts.readBufferSize,
				})

			},
			opts.numClients,
		)
	}

	if opts.api == mixedAPIs || opts.api == grpcAPI || opts.api == directPath {
		gRPCClients, closeGRPC = newClientPool(
			func() (*storage.Client, error) {
				return initializeGRPCClient(context.Background(), clientParams{
					customClient:       opts.allowCustomClient,
					writeBufferSize:    opts.writeBufferSize,
					readBufferSize:     opts.readBufferSize,
					connectionPoolSize: opts.connPoolSize,
				})
			},
			opts.numClients,
		)

	}
	return func() {
		closeNonBenchmarking()

		if closeHTTP != nil {
			closeHTTP()
		}
		if closeGRPC != nil {
			closeGRPC()
		}
	}
}

// Rotate through clients. This may mean certain clients get a larger workload
// than others, if object sizes vary.
func getClient(ctx context.Context, api benchmarkAPI) *storage.Client {
	if api == grpcAPI || api == directPath {
		return gRPCClients.Get()
	}
	return httpClients.Get()
}

// mutex on starting a client so that we can set an env variable for GRPC clients
var clientMu sync.Mutex

type clientParams struct {
	customClient                    bool // if false, all other params are ignored
	writeBufferSize, readBufferSize int
	connectionPoolSize              int
}

func initializeHTTPClient(ctx context.Context, p clientParams) (*storage.Client, error) {
	if !p.customClient {
		clientMu.Lock()
		c, err := storage.NewClient(ctx)
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
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		WriteBufferSize:       p.writeBufferSize,
		ReadBufferSize:        p.readBufferSize,
	}

	http2Trans, err := http2.ConfigureTransports(base)
	if err == nil {
		http2Trans.ReadIdleTimeout = time.Second * 31
	}

	trans, err := htransport.NewTransport(ctx, base,
		option.WithScopes("https://www.googleapis.com/auth/devstorage.full_control"))
	if err != nil {
		return nil, err
	}

	clientMu.Lock()
	client, err := storage.NewClient(ctx, option.WithHTTPClient(&http.Client{Transport: trans}))
	clientMu.Unlock()

	return client, err
}

func initializeGRPCClient(ctx context.Context, p clientParams) (*storage.Client, error) {
	if !p.customClient {
		clientMu.Lock()
		os.Setenv("STORAGE_USE_GRPC", "true")
		c, err := storage.NewClient(ctx)
		os.Unsetenv("STORAGE_USE_GRPC")
		clientMu.Unlock()
		return c, err
	}

	clientMu.Lock()
	os.Setenv("STORAGE_USE_GRPC", "true")
	client, err := storage.NewClient(ctx,
		option.WithGRPCDialOption(grpc.WithReadBufferSize(p.readBufferSize)),
		option.WithGRPCDialOption(grpc.WithWriteBufferSize(p.writeBufferSize)),
		option.WithGRPCConnectionPool(p.connectionPoolSize))
	os.Unsetenv("STORAGE_USE_GRPC")
	clientMu.Unlock()

	return client, err
}
