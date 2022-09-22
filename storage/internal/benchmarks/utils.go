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
	crand "crypto/rand"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/uuid"
	"google.golang.org/api/option"
	htransport "google.golang.org/api/transport/http"
	"google.golang.org/grpc"
)

const (
	kib          = 1024
	bucketPrefix = "golang-grpc-test-" // needs to be this for GRPC for now
	objectPrefix = "benchmark-obj-"
)

func randomBool() bool {
	return rand.Intn(2) == 0
}

// randomOf3 returns 2 negative and one positive bool, randomly assigning the
// position of the positive return value.
func randomOf3() (bool, bool, bool) {
	r := rand.Intn(3)
	return r == 0, r == 1, r == 2
}

// randomInt64 returns a value in the closed interval [min, max].
// That is, the endpoints are possible return values.
func randomInt64(min, max int64) int64 {
	if min > max {
		log.Fatalf("min cannot be larger than max; min: %d max: %d", min, max)
	}
	return rand.Int63n(max-min+1) + min
}

// randomInt returns a value in the closed interval [min, max].
// That is, the endpoints are possible return values.
func randomInt(min, max int) int {
	if min > max {
		log.Fatalf("min cannot be larger than max; min: %d max: %d", min, max)
	}
	return rand.Intn(max-min+1) + min
}

func randomName(prefix string) string {
	var sb strings.Builder

	sb.WriteString(prefix)
	sb.WriteString(uuid.New().String())
	return sb.String()
}

// createBenchmarkBucket creates a bucket and returns a function to delete it.
func createBenchmarkBucket(bucketName string, opts *benchmarkOptions) func() {
	ctx := context.Background()

	// Create a bucket for the tests. We do not need to benchmark this.
	c, err := storage.NewClient(ctx, option.WithCredentialsFile(credentialsFile))
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}
	err = c.Bucket(bucketName).Create(ctx, projectID, &storage.BucketAttrs{
		Location:     opts.region,
		StorageClass: "STANDARD",
	})
	if err != nil {
		log.Fatalf("bucket.Create: %v", err)
	}

	return func() {
		if err := c.Bucket(bucketName).Delete(context.Background()); err != nil {
			log.Fatalf("bucket delete: %v", err)
		}
	}
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

// generateRandomFile creates a temp file on disk and fills it with size random bytes.
func generateRandomFile(size int64) (string, error) {
	f, err := os.CreateTemp("", objectPrefix)
	if err != nil {
		return "", fmt.Errorf("error creating file: %v", err)
	}
	defer f.Close()

	_, err = io.CopyN(f, crand.Reader, size)

	return f.Name(), err
}

// If the option is specified, run a garbage collector before collecting
// memory statistics and starting the timer on the benchmark. This can be
// used to compare between running each benchmark "on a blank slate" vs organically.
func forceGarbageCollection(run bool) {
	if run {
		runtime.GC()
		// debug.FreeOSMemory()
	}
}

func canUseClientPool(opts *benchmarkOptions) bool {
	return opts.useDefaults || (opts.maxReadSize == opts.minReadSize && opts.maxWriteSize == opts.minWriteSize)
}

func getClient(ctx context.Context, opts *benchmarkOptions, br benchmarkResult) (*storage.Client, error) {
	if canUseClientPool(opts) {
		if br.params.api == grpcAPI {
			return gRPCClients.Get().(*storage.Client), nil
		}
		return httpClients.Get().(*storage.Client), nil
	}

	// if necessary, create a client
	if br.params.api == grpcAPI {
		c, err := initializeGRPCClient(ctx, br.params.appBufferSize, br.params.appBufferSize, opts.connPoolSize, false)
		if err != nil {
			return nil, fmt.Errorf("initializeGRPCClient: %w", err)
		}
		return c, nil
	}
	c, err := initializeHTTPClient(ctx, br.params.appBufferSize, br.params.appBufferSize, false)
	if err != nil {
		return nil, fmt.Errorf("initializeHTTPClient: %w", err)
	}
	return c, nil
}
