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
	"os"
	"runtime/debug"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/google/uuid"
	"google.golang.org/api/option"
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

var goVersion string
var dependencyVersions = map[string]string{
	"cloud.google.com/go/storage": "",
	"google.golang.org/api":       "",
	"cloud.google.com/go":         "",
	"google.golang.org/grpc":      "",
}

func populateDependencyVersions() error {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return fmt.Errorf("binary not built with module support, cannot read build info")
	}

	goVersion = info.GoVersion
	for _, mod := range info.Deps {
		if _, ok := dependencyVersions[mod.Path]; ok {
			dependencyVersions[mod.Path] = mod.Version
		}
	}
	return nil
}
