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
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/uuid"
	"google.golang.org/api/option"
	htransport "google.golang.org/api/transport/http"
)

const bucketPrefix = "golang-grpc-test-" // needs to be this for GRPC for now

func randomBool() bool {
	return rand.Intn(2) == 0
}

// returns a value in range [min, max]
// includes endpoints in possible values to return
func randomInt64(min, max int64) int64 {
	if min > max {
		log.Fatalf("min cannot be larger than max; min: %d max: %d", min, max)
	}
	return rand.Int63n(max-min+1) + min
}

// returns a value in range [min, max]
// includes endpoints in possible values to return
func randomInt(min, max int) int {
	if min > max {
		log.Fatalf("min cannot be larger than max; min: %d max: %d", min, max)
	}
	return rand.Intn(max-min+1) + min
}

func randomName(prefix string) string {
	var sb strings.Builder

	sb.WriteString(prefix)
	sb.WriteRune('-')
	sb.WriteString(uuid.New().String()) // 36 chars
	return sb.String()
}

// createBenchmarkBucket creates a bucket and returns a function to delete it
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

func initializeClient(ctx context.Context, api benchmarkAPI, writeBufferSize, readBufferSize int, connPoolSize int) (*storage.Client, benchmarkAPI, benchmarkAPI, error) {
	var readAPI, writeAPI benchmarkAPI
	var client *storage.Client
	var err error

	if api == mixed {
		if randomBool() {
			api = xml
		} else {
			api = grpc
		}
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
		WriteBufferSize:       int(writeBufferSize),
		ReadBufferSize:        int(readBufferSize),
	}
	trans, err := htransport.NewTransport(ctx, base,
		option.WithScopes("https://www.googleapis.com/auth/devstorage.full_control"),
		option.WithCredentialsFile(credentialsFile))
	if err != nil {
		return nil, "", "", err
	}

	c := http.Client{Transport: trans}

	switch api {
	case xml, json:
		client, err = storage.NewClient(ctx, option.WithHTTPClient(&c))
		readAPI = json
		writeAPI = xml
	case grpc:
		client, err = storage.NewHybridClient(ctx, &storage.HybridClientOptions{
			HTTPOpts: []option.ClientOption{option.WithCredentialsFile(credentialsFile)},
			GRPCOpts: []option.ClientOption{option.WithCredentialsFile(credentialsFile), option.WithGRPCConnectionPool(connPoolSize)},
		})
		readAPI = grpc
		writeAPI = grpc
	default:
		log.Fatalf("%s API not supported.\n", opts.api)
	}
	return client, readAPI, writeAPI, err
}

// generateRandomFile creates a file on disk and fills it with size random bytes
func generateRandomFile(fileName string, size int64) error {
	f, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("error creating file: %v", err)
	}
	defer f.Close()

	_, err = io.CopyN(f, crand.Reader, size)

	return err
}
