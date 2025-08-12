/*
Copyright 2025 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bigtable

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime/pprof"
	"sync"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/api/iterator"
)

/*
To run benchmark tests,
	go test -v -run=^$ -bench="BenchmarkReadRowsWithMetrics" -benchmem -memprofile=heap-metrics-enabled.prof .
	go test -v -run=^$ -bench="BenchmarkReadRowsWithoutMetrics" -benchmem -memprofile=heap-metrics-disabled.prof .


Compare Heap Allocation Profiles:
	To understand the impact of enabling metrics on heap allocations, compare the two profiles using pprof's -diff_base feature.

	Compare Total Bytes Allocated:
		go tool pprof -http=:5000 -sample_index=alloc_space -diff_base=heap-metrics-disabled.prof heap-metrics-enabled.prof
	The flame graph will highlight functions where the difference in total allocated bytes is significant.
	Positive values indicate more bytes allocated in heap-metrics-enabled.prof.

	Compare Total Number of Allocations:
		go tool pprof -http=:5001 -sample_index=alloc_objects -diff_base=heap-metrics-disabled.prof heap-metrics-enabled.prof
	This shows the difference in the number of allocations.


Compare CPU Profiles:
	This highlights the functions contributing most to the CPU overhead due to metrics being enabled.

	Generate a diff flame graph:
	This shows what's "new" or "more expensive" in the enabled profile
		go tool pprof -http=:5002 -diff_base=cpu-metrics-disabled.prof cpu-metrics-enabled.prof



View individual CPU profiles:
	go tool pprof -http=:5003 cpu-metrics-enabled.prof
	go tool pprof -http=:5004 cpu-metrics-disabled.prof
*/

const (
	project          = "my_project"
	instance         = "my_instance"
	tableNamePrefix  = "profile-test-"
	columnFamilyName = "cf1"
	columnName       = "col1"
	totalRows        = 10000000
	rowsPerApplyBulk = 100000
	numGoRoutines    = 100 // Number of concurrent readers
)

// setup performs the initial configuration for the benchmark, including client creation,
// table creation, and data population. It also handles the profiling setup and cleanup.
func setup(b *testing.B, metricsEnabled bool) (client *Client, tableName, rowKeyPrefix string, cleanup func()) {
	ctx := context.Background()
	b.Logf("Setting up for metrics enabled: %v", metricsEnabled)

	// 1. Create Admin Client
	adminClient, err := NewAdminClient(ctx, project, instance)
	if err != nil {
		b.Fatalf("Failed to create admin client: %v", err)
	}

	// 2. Create Table If Not Exists
	tableName = tableNamePrefix + uuid.New().String()
	b.Logf("Creating table: %s", tableName)
	if err := adminClient.CreateTable(ctx, tableName); err != nil {
		b.Fatalf("Failed to create table '%s': %v", tableName, err)
	}
	if err := adminClient.CreateColumnFamily(ctx, tableName, columnFamilyName); err != nil {
		b.Fatalf("Failed to create column family '%s': %v", columnFamilyName, err)
	}

	// Create Data Client for writing data
	writerClient, err := NewClient(ctx, project, instance)
	if err != nil {
		b.Fatalf("Failed to create writer data client: %v", err)
	}

	// 3. Write rows
	rowKeyPrefix = "row-" + uuid.New().String()
	b.Logf("Writing %d rows to table '%s' with prefix '%s'...", totalRows, tableName, rowKeyPrefix)
	for i := 0; i < totalRows; i += rowsPerApplyBulk {
		start := i
		end := i + rowsPerApplyBulk
		if end > totalRows {
			end = totalRows
		}
		writeBatch(b, writerClient, tableName, rowKeyPrefix, start, end)
	}
	b.Log("Finished writing data.")
	writerClient.Close()

	// 4. Create Data Client for benchmark
	clientConfig := ClientConfig{}
	if !metricsEnabled {
		clientConfig.MetricsProvider = NoopMetricsProvider{}
	}
	client, err = NewClientWithConfig(ctx, project, instance, clientConfig)
	if err != nil {
		b.Fatalf("Failed to create data client (metrics: %v): %v", metricsEnabled, err)
	}

	// Profiling setup
	profileSuffix := "disabled"
	if metricsEnabled {
		profileSuffix = "enabled"
	}
	cpuFile, err := os.Create(fmt.Sprintf("cpu-metrics-%s.prof", profileSuffix))
	if err != nil {
		b.Fatalf("could not create CPU profile: %v", err)
	}
	pprof.StartCPUProfile(cpuFile)

	cleanup = func() {
		b.Log("Running cleanup...")
		pprof.StopCPUProfile()
		cpuFile.Close()

		if err := adminClient.DeleteTable(ctx, tableName); err != nil {
			b.Logf("Warning: failed to delete table '%s': %v", tableName, err)
		}
		adminClient.Close()
		client.Close()
		b.Log("Cleanup complete.")
	}

	return client, tableName, rowKeyPrefix, cleanup
}

// writeBatch writes a batch of rows to the specified table.
func writeBatch(b *testing.B, client *Client, tableName, rowKeyPrefix string, start, end int) {
	muts := make([]*Mutation, end-start)
	rowKeys := make([]string, end-start)
	for i := 0; i < len(muts); i++ {
		muts[i] = NewMutation()
		muts[i].Set(columnFamilyName, columnName, Now(), []byte("p"))
		rowKeys[i] = fmt.Sprintf("%s-%010d", rowKeyPrefix, start+i)
	}

	errs, err := client.Open(tableName).ApplyBulk(context.Background(), rowKeys, muts)
	if err != nil {
		b.Fatalf("ApplyBulk failed: %v", err)
	}
	for _, err := range errs {
		if err != nil {
			b.Fatalf("An error occurred during ApplyBulk: %v", err)
		}
	}
}

// readRowsConcurrently simulates multiple clients reading from the table.
func readRowsConcurrently(b *testing.B, client *Client, tableName, rowKeyPrefix string) {
	var wg sync.WaitGroup
	wg.Add(numGoRoutines)

	rowsPerRoutine := totalRows / numGoRoutines

	for i := 0; i < numGoRoutines; i++ {
		startKey := fmt.Sprintf("%s-%010d", rowKeyPrefix, i*rowsPerRoutine)
		endKey := fmt.Sprintf("%s-%010d", rowKeyPrefix, (i+1)*rowsPerRoutine)

		go func(start, end string) {
			defer wg.Done()
			tbl := client.Open(tableName)
			err := tbl.ReadRows(context.Background(), NewRange(start, end), func(r Row) bool {
				// consume the row to simulate a real read.
				_ = r[columnFamilyName][0].Value
				return true
			})
			if err != nil && err != iterator.Done {
				b.Errorf("ReadRows failed for range %s-%s: %v", start, end, err)
			}
		}(startKey, endKey)
	}
	wg.Wait()
}

func BenchmarkReadRowsWithMetrics(b *testing.B) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	client, tableName, rowKeyPrefix, cleanup := setup(b, true)
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		readRowsConcurrently(b, client, tableName, rowKeyPrefix)
	}
	b.StopTimer()
}

func BenchmarkReadRowsWithoutMetrics(b *testing.B) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	client, tableName, rowKeyPrefix, cleanup := setup(b, false)
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		readRowsConcurrently(b, client, tableName, rowKeyPrefix)
	}
	b.StopTimer()
}
