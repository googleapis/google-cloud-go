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
	"crypto/tls"
	"log"
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

var xmlClients, jsonClients, gRPCClients, nonBenchmarkingClients *clientPool

// initializeClientPools creates separate client pools for XML, JSON, and gRPC,
// and only creates those if required. For example, if the input parameter `api`
// is set to `JSON`, the JSON pool is initialized but not the XML or GRPC pools.
func initializeClientPools(ctx context.Context, opts *benchmarkOptions) func() {
	var closeNonBenchmarking, closeXML, closeJSON, closeGRPC func()

	nonBenchmarkingClients, closeNonBenchmarking = newClientPool(
		func() (*storage.Client, error) {
			return initializeHTTPClient(ctx, useDefault, useDefault, false, opts.gcsFuse)
		},
		1,
	)

	// Init XML clients if necessary
	if opts.api == mixedAPIs || opts.api == xmlAPI {
		xmlClients, closeXML = newClientPool(
			func() (*storage.Client, error) {
				return initializeHTTPClient(ctx, opts.writeBufferSize, opts.readBufferSize, false, opts.gcsFuse)
			},
			opts.numClients,
		)
	}

	// Init JSON clients if necessary
	// There is no XML implementation for uploads, so we also initialize JSON
	// clients if given that value.
	if opts.api == mixedAPIs || opts.api == jsonAPI || opts.api == xmlAPI {
		jsonClients, closeJSON = newClientPool(
			func() (*storage.Client, error) {
				return initializeHTTPClient(ctx, opts.writeBufferSize, opts.readBufferSize, true, opts.gcsFuse)
			},
			opts.numClients,
		)
	}

	// Init GRPC clients if necessary
	if opts.api == mixedAPIs || opts.api == grpcAPI || opts.api == directPath {
		gRPCClients, closeGRPC = newClientPool(
			func() (*storage.Client, error) {
				return initializeGRPCClient(context.Background(), opts.writeBufferSize, opts.readBufferSize, opts.connPoolSize)
			},
			opts.numClients,
		)
	}

	return func() {
		closeNonBenchmarking()

		if closeXML != nil {
			closeXML()
		}
		if closeJSON != nil {
			closeJSON()
		}
		if closeGRPC != nil {
			closeGRPC()
		}
	}
}

// Rotate through clients. This may mean certain clients get a larger workload
// than others, if object sizes vary.
func getClient(ctx context.Context, api benchmarkAPI) *storage.Client {
	switch api {
	case grpcAPI, directPath:
		return gRPCClients.Get()
	case jsonAPI:
		return jsonClients.Get()
	case xmlAPI:
		return xmlClients.Get()
	}
	return nil
}

// mutex on starting a client so that we can set an env variable for GRPC clients
var clientMu sync.Mutex

func initializeHTTPClient(ctx context.Context, writeBufferSize, readBufferSize int, json bool, gcsFuse bool) (*storage.Client, error) {
	opts := []option.ClientOption{}

	if writeBufferSize != useDefault || readBufferSize != useDefault || gcsFuse {
		// We need to modify the underlying HTTP client
		base := http.DefaultTransport.(*http.Transport).Clone()

		if gcsFuse {
			base = &http.Transport{
				MaxConnsPerHost:     100,
				MaxIdleConnsPerHost: 100,
				// This disables HTTP/2 in transport.
				TLSNextProto: make(
					map[string]func(string, *tls.Conn) http.RoundTripper,
				),
			}
		}

		base.MaxIdleConnsPerHost = 100 // this is set in Storage as well
		base.WriteBufferSize = writeBufferSize
		base.ReadBufferSize = readBufferSize

		if !gcsFuse {
			http2Trans, err := http2.ConfigureTransports(base)
			if err == nil {
				http2Trans.ReadIdleTimeout = time.Second * 31
			}
		}

		trans, err := htransport.NewTransport(ctx, base,
			option.WithScopes("https://www.googleapis.com/auth/devstorage.full_control"))
		if err != nil {
			return nil, err
		}

		opts = append(opts, option.WithHTTPClient(&http.Client{Transport: trans}))
	}

	if json {
		opts = append(opts, storage.WithJSONReads())
	}

	// Init client
	clientMu.Lock()
	client, err := storage.NewClient(ctx, opts...)
	clientMu.Unlock()

	return client, err
}

func initializeGRPCClient(ctx context.Context, writeBufferSize, readBufferSize int, connectionPoolSize int) (*storage.Client, error) {
	opts := []option.ClientOption{option.WithGRPCConnectionPool(connectionPoolSize)}

	if writeBufferSize != useDefault {
		opts = append(opts, option.WithGRPCDialOption(grpc.WithWriteBufferSize(writeBufferSize)))
	}
	if readBufferSize != useDefault {
		opts = append(opts, option.WithGRPCDialOption(grpc.WithReadBufferSize(readBufferSize)))
	}

	clientMu.Lock()
	os.Setenv("STORAGE_USE_GRPC", "true")
	client, err := storage.NewClient(ctx, opts...)
	os.Unsetenv("STORAGE_USE_GRPC")
	clientMu.Unlock()

	return client, err
}
