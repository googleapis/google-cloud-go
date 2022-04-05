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
	"math/rand"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

const (
	alphabet                   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	lowercaseLettersAndNumbers = "abcdefghijklmnopqrstuvwxyz0123456789"
	uppercaseLetters           = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	ASCIIchars                 = " !\"#$%&\\'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~"
)

// returns a value in range [min, max]
// includes endpoints in possible values to return
func randomValue(min, max int64) int64 {
	return rand.Int63n(max-min+1) + min
}

func randomBucketName(prefix string) string {
	var sb strings.Builder
	// The total length of the bucket name must be <= 63 characters
	maxLen := 63
	date := time.Now().Format("06-01-02-1504")

	sb.WriteString(prefix)
	sb.WriteRune('-')
	sb.WriteString(date)
	sb.WriteRune('_')

	maxRandomChars := maxLen - sb.Len()
	sb.WriteString(randomString(maxRandomChars, lowercaseLettersAndNumbers))

	return sb.String()
}

func randomObjectName() string {
	// GCS accepts object name up to 1024 characters, but 128 seems long enough to
	// avoid collisions.
	maxLen := 128

	return randomString(maxLen, lowercaseLettersAndNumbers+uppercaseLetters)
}

// random string between 1 and maxLen
func randomString(maxLen int, allowedChars string) string {
	var sb strings.Builder
	length := rand.Intn(maxLen) + 1 // random length in (1 ... maxLen)
	sb.Grow(length)

	for i := 0; i < length; i++ {
		sb.WriteByte(allowedChars[rand.Intn(len(allowedChars))])
	}
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

func initializeClient(ctx context.Context, api benchmarkAPI) (*storage.Client, benchmarkAPI, benchmarkAPI, error) {
	var readAPI, writeAPI benchmarkAPI
	var client *storage.Client
	var err error

	if api == Random {
		if rand.Intn(2) == 0 {
			api = XML
		} else {
			api = GRPC
		}
	}

	switch api {
	case XML, JSON:
		client, err = storage.NewClient(ctx, option.WithCredentialsFile(credentialsFile))
		readAPI = JSON
		writeAPI = XML
	case GRPC:
		client, err = storage.NewHybridClient(ctx, &storage.HybridClientOptions{
			HTTPOpts: []option.ClientOption{option.WithCredentialsFile(credentialsFile)},
			GRPCOpts: []option.ClientOption{option.WithCredentialsFile(credentialsFile)},
		})
		readAPI = GRPC
		writeAPI = GRPC
	default:
		log.Fatalf("%s API not supported.\n", opts.api)
	}
	return client, readAPI, writeAPI, err
}
