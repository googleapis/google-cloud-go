// Copyright 2026 Google LLC
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

package bigtable

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/api/option"
)

func TestReadSessionSandbox(t *testing.T) {
	// This test connects to the sandbox endpoint and performs a ReadRow via session-based transport.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	project := "autonomous-mote-782"
	instance := "test-sushanb"
	table := "sushanb"
	endpoint := "test-bigtable.sandbox.googleapis.com:443"

	cfg := ClientConfig{
		EnableSessionPool: true,
	}

	// Create client configured to use sessions & the sandbox endpoint
	client, err := NewClientWithConfig(ctx, project, instance, cfg, option.WithEndpoint(endpoint))
	if err != nil {
		t.Fatalf("failed to create bigtable client: %v", err)
	}
	defer client.Close()

	tbl := client.OpenTable(table)
	rowKey := "myrow-0"

	t.Logf("Waiting for session pool to warm up...")
	time.Sleep(3 * time.Second)

	t.Logf("Reading row %q from table %q via session pool before write...", rowKey, table)
	row, err := tbl.ReadRow(ctx, rowKey)
	if err != nil {
		t.Fatalf("ReadRow before write failed: %v", err)
	}
	if row != nil {
		t.Logf("Successfully read row %q before write:", rowKey)
		for fam, items := range row {
			for _, item := range items {
				t.Logf("  Family: %s, Column: %s, Value: %s", fam, item.Column, string(item.Value))
			}
		}
	}

	t.Logf("Applying mutation write via session pool to row %q...", rowKey)
	mut := NewMutation()
	mut.Set("cf12", "colq1", ServerTime, []byte("val-applied-vrpc"))
	if err := tbl.Apply(ctx, rowKey, mut); err != nil {
		t.Fatalf("Apply mutation failed: %v", err)
	}
	t.Logf("Successfully applied mutation write.")

	t.Logf("Reading row %q from table %q via session pool after write...", rowKey, table)
	rowAfter, err := tbl.ReadRow(ctx, rowKey)
	if err != nil {
		t.Fatalf("ReadRow after write failed: %v", err)
	}
	if rowAfter == nil {
		t.Fatalf("Row %q not found after write", rowKey)
	}

	t.Logf("Successfully read row %q after write:", rowKey)
	for fam, items := range rowAfter {
		for _, item := range items {
			t.Logf("  Family: %s, Column: %s, Value: %s", fam, item.Column, string(item.Value))
		}
	}
}

func TestHighQpsSessionSandbox(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 11*time.Minute)
	defer cancel()

	project := "autonomous-mote-782"
	instance := "test-sushanb"
	table := "sushanb"
	endpoint := "test-bigtable.sandbox.googleapis.com:443"

	cfg := ClientConfig{
		EnableSessionPool: true,
		SessionPoolMin:    3,
		SessionPoolMax:    5,
	}

	client, err := NewClientWithConfig(ctx, project, instance, cfg, option.WithEndpoint(endpoint))
	if err != nil {
		t.Fatalf("failed to create bigtable client: %v", err)
	}
	defer client.Close()

	tbl := client.OpenTable(table)

	t.Logf("Waiting for session pool to warm up before high QPS test...")
	time.Sleep(3 * time.Second)

	concurrency := 10
	testDuration := 10 * time.Minute
	endTime := time.Now().Add(testDuration)

	var successWrites int64
	var successReads int64
	var failedWrites int64
	var failedReads int64

	var wg sync.WaitGroup
	t.Logf("Starting 10-minute high QPS sandbox test with %d concurrent workers...", concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			counter := 0
			for time.Now().Before(endTime) {
				counter++
				rowKey := fmt.Sprintf("sandbox-%d-%d", workerID, counter)
				mut := NewMutation()
				mut.Set("cf12", "colq1", ServerTime, []byte(fmt.Sprintf("val-worker-%d-%d", workerID, counter)))

				// Perform write
				if err := tbl.Apply(ctx, rowKey, mut); err != nil {
					atomic.AddInt64(&failedWrites, 1)
					fmt.Printf(">>> ERROR [worker-%d]: write failed: %v <<<\n", workerID, err)
				} else {
					atomic.AddInt64(&successWrites, 1)
				}

				// Perform read
				_, err := tbl.ReadRow(ctx, rowKey)
				if err != nil {
					atomic.AddInt64(&failedReads, 1)
					fmt.Printf(">>> ERROR [worker-%d]: read failed: %v <<<\n", workerID, err)
				} else {
					atomic.AddInt64(&successReads, 1)
				}

				time.Sleep(50 * time.Millisecond)
			}
		}(i)
	}

	// Background stats logger
	doneChan := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		start := time.Now()

		for {
			select {
			case <-doneChan:
				return
			case <-ticker.C:
				sw := atomic.LoadInt64(&successWrites)
				sr := atomic.LoadInt64(&successReads)
				fw := atomic.LoadInt64(&failedWrites)
				fr := atomic.LoadInt64(&failedReads)
				elapsed := time.Since(start)
				qps := float64(sw+sr) / elapsed.Seconds()
				fmt.Printf(">>> STATS [%.1fs elapsed]: Success (W:%d, R:%d), Failed (W:%d, R:%d), QPS: %.2f <<<\n", elapsed.Seconds(), sw, sr, fw, fr, qps)
			}
		}
	}()

	wg.Wait()
	close(doneChan)

	finalSW := atomic.LoadInt64(&successWrites)
	finalSR := atomic.LoadInt64(&successReads)
	finalFW := atomic.LoadInt64(&failedWrites)
	finalFR := atomic.LoadInt64(&failedReads)
	t.Logf("10-minute high QPS test completed! Successful Writes: %d, Successful Reads: %d, Failed Writes: %d, Failed Reads: %d", finalSW, finalSR, finalFW, finalFR)
	if finalFW > 0 || finalFR > 0 {
		t.Errorf("Test had failed operations!")
	}
}

func TestSequentialReads(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	project := "autonomous-mote-782"
	instance := "test-sushanb"
	table := "sushanb"
	endpoint := "test-bigtable.sandbox.googleapis.com:443"

	cfg := ClientConfig{
		EnableSessionPool: true,
	}

	client, err := NewClientWithConfig(ctx, project, instance, cfg, option.WithEndpoint(endpoint))
	if err != nil {
		t.Fatalf("failed to nn bigtable client: %v", err)
	}
	defer client.Close()

	tbl := client.OpenTable(table)
	rowKey := "myrow-0"

	t.Logf("Waiting for session pool to warm up...")
	time.Sleep(3 * time.Second)

	for i := 0; i < 10; i++ {
		t.Logf("Sequential Read %d/10 for row %q...", i+1, rowKey)
		t.Logf("Sleeping for 1s...")
		time.Sleep(1 * time.Second)
		row, err := tbl.ReadRow(ctx, rowKey)
		if err != nil {
			t.Fatalf("Read %d failed: %v", i+1, err)
		}
		if row != nil {
			t.Logf("Read %d successfully. Columns read: %d", i+1, len(row["cf12"]))
		} else {
			t.Logf("Read %d: row %q not found", i+1, rowKey)
		}
	}
}

func TestDequentialReadsParallel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	project := "autonomous-mote-782"
	instance := "test-sushanb"
	table := "sushanb"
	endpoint := "test-bigtable.sandbox.googleapis.com:443"

	cfg := ClientConfig{
		EnableSessionPool: true,
	}

	client, err := NewClientWithConfig(ctx, project, instance, cfg, option.WithEndpoint(endpoint))
	if err != nil {
		t.Fatalf("failed to create bigtable client: %v", err)
	}
	defer client.Close()

	tbl := client.OpenTable(table)

	t.Logf("Waiting for session pool to warm up...")
	time.Sleep(3 * time.Second)

	var wg sync.WaitGroup
	for seed := 0; seed < 10; seed++ {
		wg.Add(1)
		go func(s int) {
			defer wg.Done()
			rowKey := fmt.Sprintf("myrow-%d", s)
			for i := 0; i < 10; i++ {
				t.Logf("Seed %d - Sequential Read %d/10 for row %q...", s, i+1, rowKey)
				row, err := tbl.ReadRow(ctx, rowKey)
				if err != nil {
					t.Errorf("Seed %d - Read %d failed: %v", s, i+1, err)
					return
				}
				if row != nil {
					t.Logf("Seed %d - Read %d successfully. Columns read: %d", s, i+1, len(row["cf12"]))
				} else {
					t.Logf("Seed %d - Read %d: row %q not found", s, i+1, rowKey)
				}
			}
		}(seed)
	}
	wg.Wait()
}
