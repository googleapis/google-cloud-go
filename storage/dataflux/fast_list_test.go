// Copyright 2024 Google LLC
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

package dataflux

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPrefixAdjustedOffsets(t *testing.T) {
	testcase := []struct {
		desc      string
		start     string
		end       string
		prefix    string
		wantStart string
		wantEnd   string
	}{
		// List all objects with the given prefix.
		{
			desc:      "start and end are empty",
			start:     "",
			end:       "",
			prefix:    "pre",
			wantStart: "",
			wantEnd:   "",
		},
		{
			desc:      "start is longer and lexicographically before prefix",
			start:     "abcqre",
			end:       "",
			prefix:    "pre",
			wantStart: "",
			wantEnd:   "",
		},
		{
			desc:      "start value same as prefix",
			start:     "pre",
			end:       "",
			prefix:    "pre",
			wantStart: "",
			wantEnd:   "",
		},
		{
			desc:      "lexicographically start comes before prefix and end after prefix",
			start:     "abc",
			end:       "xyz",
			prefix:    "pre",
			wantStart: "",
			wantEnd:   "",
		},
		// List bounded objects within the given prefix.
		{
			desc:      "start value contains prefix",
			start:     "pre_a",
			end:       "",
			prefix:    "pre",
			wantStart: "_a",
			wantEnd:   "",
		},
		{
			desc:      "end value contains prefix",
			start:     "",
			end:       "pre_x",
			prefix:    "pre",
			wantStart: "",
			wantEnd:   "_x",
		},
		// With empty prefix, start and end will not be affected.
		{
			desc:      "prefix is empty",
			start:     "abc",
			end:       "xyz",
			prefix:    "",
			wantStart: "abc",
			wantEnd:   "xyz",
		},
		{
			desc:      "start is lexicographically higher than end",
			start:     "xyz",
			end:       "abc",
			prefix:    "",
			wantStart: "xyz",
			wantEnd:   "abc",
		},
		// Cases where no objects will be listed when prefix is given.
		{
			desc:      "end is same as prefix",
			start:     "",
			end:       "pre",
			prefix:    "pre",
			wantStart: "pre",
			wantEnd:   "pre",
		},
		{
			desc:      "start is lexicographically higher than end with prefix",
			start:     "xyz",
			end:       "abc",
			prefix:    "pre",
			wantStart: "xyz",
			wantEnd:   "xyz",
		},
		{
			desc:      "start is lexicographically higher than prefix",
			start:     "xyz",
			end:       "",
			prefix:    "pre",
			wantStart: "xyz",
			wantEnd:   "xyz",
		},
	}

	for _, tc := range testcase {
		t.Run(tc.desc, func(t *testing.T) {
			gotStart, gotEnd := prefixAdjustedOffsets(tc.start, tc.end, tc.prefix)
			if gotStart != tc.wantStart || gotEnd != tc.wantEnd {
				t.Errorf("prefixAdjustedOffsets(%q, %q, %q) got = (%q, %q), want = (%q, %q)", tc.start, tc.end, tc.prefix, gotStart, gotEnd, tc.wantStart, tc.wantEnd)
			}
		})
	}
}

func TestNewLister(t *testing.T) {
	gcs := &storage.Client{}
	bucketName := "test-bucket"
	testcase := []struct {
		desc            string
		query           storage.Query
		parallelism     int
		wantStart       string
		wantEnd         string
		wantParallelism int
	}{
		{
			desc:            "start and end are empty",
			query:           storage.Query{Prefix: "pre"},
			parallelism:     1,
			wantStart:       "",
			wantEnd:         "",
			wantParallelism: 1,
		},
		{
			desc:            "start is longer than prefix",
			query:           storage.Query{Prefix: "pre", StartOffset: "pre_a"},
			parallelism:     1,
			wantStart:       "_a",
			wantEnd:         "",
			wantParallelism: 1,
		},
		{
			desc:            "start and end are empty",
			query:           storage.Query{Prefix: "pre"},
			parallelism:     0,
			wantStart:       "",
			wantEnd:         "",
			wantParallelism: 10 * runtime.NumCPU(),
		},
	}

	for _, tc := range testcase {
		t.Run(tc.desc, func(t *testing.T) {
			in := ListerInput{
				BucketName:  bucketName,
				BatchSize:   0,
				Query:       tc.query,
				Parallelism: tc.parallelism,
			}
			df := NewLister(gcs, &in)
			defer df.Close()
			if len(df.ranges) != 1 {
				t.Errorf("NewLister(%v, %v %v, %v) got len of ranges = %v, want = %v", bucketName, 1, 0, tc.query, len(df.ranges), 1)
			}
			ranges := <-df.ranges
			if df.method != open || df.pageToken != "" || ranges.startRange != tc.wantStart || ranges.endRange != tc.wantEnd || df.parallelism != tc.wantParallelism {
				t.Errorf("NewLister(%q, %d, %d, %v) got = (method: %v, token: %q,  start: %q, end: %q, parallelism: %d), want = (method: %v, token: %q,  start: %q, end: %q, parallelism: %d)", bucketName, 1, 0, tc.query, df.method, df.pageToken, ranges.startRange, ranges.endRange, df.parallelism, open, "", tc.wantStart, tc.wantEnd, tc.wantParallelism)
			}

		})
	}
}

func TestNextBatchContextCancelEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client *storage.Client) {

		bucketHandle := client.Bucket(bucket)
		if err := bucketHandle.Create(ctx, project, &storage.BucketAttrs{
			Name: bucket,
		}); err != nil {
			t.Fatal(err)
		}
		if err := createObject(ctx, bucketHandle, 2, ""); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}
		c := NewLister(client, &ListerInput{BucketName: bucket})
		defer c.Close()
		childCtx, cancel := context.WithCancel(ctx)
		cancel()
		result, err := c.NextBatch(childCtx)
		if err != nil && !(errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled) {
			t.Fatalf("NextBatch() failed with error: %v", err)
		}
		if err == nil {
			t.Errorf("NextBatch() expected to fail with %v, got nil", context.Canceled)
		}
		if len(result) > 0 {
			t.Errorf("NextBatch() got %v objects, want 0 objects", len(result))
		}
	})
}

func TestNextBatchEmulated(t *testing.T) {
	transportClientTest(skipGRPC("context cancel not supported"), t, func(t *testing.T, ctx context.Context, project, bucket string, client *storage.Client) {

		bucketHandle := client.Bucket(bucket)
		if err := bucketHandle.Create(ctx, project, &storage.BucketAttrs{
			Name: bucket,
		}); err != nil {
			t.Fatal(err)
		}
		numObject := 1500
		if err := createObject(ctx, bucketHandle, numObject, ""); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}

		testcase := []struct {
			desc   string
			method listingMethod
		}{
			{
				desc:   "sequential or worksteal listing",
				method: open,
			},
			{
				desc:   "sequential listing",
				method: sequential,
			}, {
				desc:   "worksteal listing",
				method: worksteal,
			}}

		for _, tc := range testcase {
			t.Run(tc.desc, func(t *testing.T) {
				c := NewLister(client, &ListerInput{BucketName: bucket})
				defer c.Close()
				c.method = tc.method
				result, err := c.NextBatch(ctx)
				if err != nil && err != iterator.Done {
					t.Fatalf("NextBatch() failed with error: %v", err)
				}
				if err == nil {
					t.Errorf("NextBatch() expected to return %v", iterator.Done)
				}
				if len(result) != numObject {
					t.Errorf("NextBatch() got %d objects, want %d objects", len(result), numObject)
				}
			})
		}

	})
}

func TestNextBatchWithQueryEmulated(t *testing.T) {
	transportClientTest(skipGRPC("context cancel not supported"), t, func(t *testing.T, ctx context.Context, project, bucket string, client *storage.Client) {

		bucketHandle := client.Bucket(bucket)
		if err := bucketHandle.Create(ctx, project, &storage.BucketAttrs{
			Name: bucket,
		}); err != nil {
			t.Fatal(err)
		}
		numObject := 100
		prefix := "prefix/"
		if err := createObject(ctx, bucketHandle, numObject, ""); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}
		if err := createObject(ctx, bucketHandle, numObject, prefix); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}
		input := &ListerInput{
			BucketName: bucket,
		}
		testcase := []struct {
			desc                 string
			skipDirectoryObjects bool
			query                storage.Query
			method               listingMethod
			want                 int
		}{
			{
				desc:                 "object at root level using worksteal lsiting",
				skipDirectoryObjects: false,
				query:                storage.Query{Delimiter: "/"},
				method:               worksteal,
				want:                 101,
			},
			{
				desc:                 "objects with prefix using worksteal listing",
				skipDirectoryObjects: false,
				query:                storage.Query{Prefix: prefix},
				method:               worksteal,
				want:                 100,
			},
			{
				desc:                 "object at root level",
				skipDirectoryObjects: false,
				query:                storage.Query{Delimiter: "/"},
				method:               open,
				want:                 101,
			},
			{
				desc:                 "objects with prefix",
				skipDirectoryObjects: false,
				query:                storage.Query{Prefix: prefix},
				method:               open,
				want:                 100,
			},
		}
		for _, tc := range testcase {
			t.Run(tc.desc, func(t *testing.T) {
				input.Query = tc.query
				input.SkipDirectoryObjects = tc.skipDirectoryObjects
				df := NewLister(client, input)
				defer df.Close()
				df.method = tc.method
				got, err := df.NextBatch(ctx)
				if err != nil && err != iterator.Done {
					t.Fatalf("NextBatch() for input %v failed: %v", *input, err)
				}
				if len(got) != tc.want || err != iterator.Done {
					t.Errorf("NextBatch(%v) got = (%d, %v), want (%d, %v)", *input, len(got), err, tc.want, iterator.Done)
				}
			})
		}
	})
}

var emulatorClients map[string]*storage.Client

type skipTransportTestKey string

func initEmulatorClients() func() error {
	noopCloser := func() error { return nil }

	if !isEmulatorEnvironmentSet() {
		return noopCloser
	}
	ctx := context.Background()

	grpcClient, err := storage.NewGRPCClient(ctx)
	if err != nil {
		log.Fatalf("Error setting up gRPC client for emulator tests: %v", err)
		return noopCloser
	}
	httpClient, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("Error setting up HTTP client for emulator tests: %v", err)
		return noopCloser
	}

	emulatorClients = map[string]*storage.Client{
		HTTP: httpClient,
		GRPC: grpcClient,
	}

	return func() error {
		gerr := grpcClient.Close()
		herr := httpClient.Close()

		if gerr != nil {
			return gerr
		}
		return herr
	}
}

// transportClienttest executes the given function with a sub-test, a project name
// based on the transport, a unique bucket name also based on the transport, and
// the transport-specific client to run the test with. It also checks the environment
// to ensure it is suitable for emulator-based tests, or skips.
func transportClientTest(ctx context.Context, t *testing.T, test func(*testing.T, context.Context, string, string, *storage.Client)) {
	checkEmulatorEnvironment(t)
	for transport, client := range emulatorClients {
		if reason := ctx.Value(skipTransportTestKey(transport)); reason != nil {
			t.Skip("transport", fmt.Sprintf("%q", transport), "explicitly skipped:", reason)
		}
		t.Run(transport, func(t *testing.T) {
			project := fmt.Sprintf("%s-project", transport)
			bucket := fmt.Sprintf("%s-bucket-%d", transport, time.Now().Nanosecond())
			test(t, ctx, project, bucket, client)
		})
	}
}

func skipGRPC(reason string) context.Context {
	return context.WithValue(context.Background(), skipTransportTestKey(GRPC), reason)
}

// checkEmulatorEnvironment skips the test if the emulator environment variables
// are not set.
func checkEmulatorEnvironment(t *testing.T) {
	if !isEmulatorEnvironmentSet() {
		t.Skip("Emulator tests skipped without emulator environment variables set")
	}
}

// isEmulatorEnvironmentSet checks if the emulator environment variables are set.
func isEmulatorEnvironmentSet() bool {
	return os.Getenv("STORAGE_EMULATOR_HOST_GRPC") != "" && os.Getenv("STORAGE_EMULATOR_HOST") != ""
}

// createObject creates given number of objects in the given bucket.
func createObject(ctx context.Context, bucket *storage.BucketHandle, numObjects int, prefix string) error {

	for i := 0; i < numObjects; i++ {
		// Generate a unique object name using UUIDs
		objectName := fmt.Sprintf("%s%s", prefix, uuid.New().String())
		// Create a writer for the object
		w := bucket.Object(objectName).NewWriter(ctx)

		// Close the writer to finalize the upload
		if err := w.Close(); err != nil {
			return fmt.Errorf("failed to close writer for object %q: %v", objectName, err)
		}
	}
	return nil
}
