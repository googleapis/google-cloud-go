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
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"runtime/debug"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	kib          = 1024
	bucketPrefix = "golang-grpc-test-" // needs to be this for GRPC for now
	objectPrefix = "benchmark-obj-"
)

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
	c, err := storage.NewClient(ctx)
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
func generateRandomFile(path string, size int64) (string, error) {
	f, err := os.CreateTemp(path, objectPrefix)
	if err != nil {
		return "", fmt.Errorf("error creating file: %v", err)
	}
	defer f.Close()

	_, err = io.CopyN(f, crand.Reader, size)

	return f.Name(), err
}

// fillDirectory fills the directory with the number of different files
// specified on the command line. Each file created will contain random bytes,
// and will be of the size specified on the command line. No subdirectories are
// created.
// The number of bytes across all created files is returned.
func fillDirectory(dirPath string) (int64, error) {
	currNumBytes := int64(0)

	for i := opts.numObjectsPerDirectory; i > 0; i-- {
		size := opts.objectSize
		if size == 0 {
			// Choose a different random object size for each file
			size = randomInt64(opts.minObjectSize, opts.maxObjectSize)
		}

		if _, err := generateRandomFile(dirPath, size); err != nil {
			return 0, err
		}
		currNumBytes += size
	}

	return currNumBytes, nil
}

// generateDirInGCS generates a directory in GCS and fills it with the number of
// different files specified on the command line. Each file created will contain
// random bytes, and will be of the size specified on the command line.
// Only a single file size is supported.
// A list of object names is returned as a channel.
func generateDirInGCS(ctx context.Context, dirPath string, objectSize int64) (*chan string, error) {
	objectNames := make(chan string, opts.numObjectsPerDirectory)

	for i := opts.numObjectsPerDirectory; i > 0; i-- {
		object, err := generateRandomFileInGCS(ctx, dirPath, objectSize)
		if err != nil {
			return nil, err
		}
		objectNames <- object
	}

	return &objectNames, nil
}

// generateRandomFileInGCS creates a file in GCS and fills it with size random bytes.
func generateRandomFileInGCS(ctx context.Context, dir string, size int64) (string, error) {
	c := nonBenchmarkingClients.Get()
	name := randomName(dir)

	o := c.Bucket(opts.bucket).Object(name).Retryer(storage.WithPolicy(storage.RetryAlways))

	w := o.NewWriter(context.Background())

	if _, err := io.CopyN(w, crand.Reader, size); err != nil {
		w.Close()
		return "", err
	}

	return name, w.Close()
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

// errorIsDeadLineExceeded functions like errors.Is(err, context.DeadlineExceeded)
// Except, it unwraps the error to look for GRPC DeadlineExceeded errors.
func errorIsDeadLineExceeded(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	err = errors.Unwrap(err)
	for err != nil {
		if status.Code(err) == codes.DeadlineExceeded {
			return true
		}
		err = errors.Unwrap(err)
	}
	return false
}

// deleteDirectoryFromGCS deletes everything under the given root.
func deleteDirectoryFromGCS(bucketName, root string) error {
	// Delete uploaded objects
	c := nonBenchmarkingClients.Get()
	// List objects under root and delete all
	it := c.Bucket(bucketName).Objects(context.Background(), &storage.Query{
		Prefix:     root,
		Projection: storage.ProjectionNoACL,
	})

	attrs, err := it.Next()

	for err == nil {
		o := c.Bucket(bucketName).Object(attrs.Name).Retryer(storage.WithPolicy(storage.RetryAlways))
		if err := o.Delete(context.Background()); err != nil {
			return err
		}
		attrs, err = it.Next()
	}

	if err != iterator.Done {
		return fmt.Errorf("Bucket.Objects: %w", err)
	}
	return nil
}
