/*
Copyright 2020 Google LLC

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

package spanner

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	. "cloud.google.com/go/spanner/internal/testutil"
	"google.golang.org/api/iterator"
)

const networkLatencyTime = 10 * time.Millisecond
const createSessionsMinTime = 10 * time.Millisecond
const createSessionsRndTime = 10 * time.Millisecond
const beginTransactionMinTime = 1 * time.Millisecond
const beginTransactionRndTime = 1 * time.Millisecond
const commitTransactionMinTime = 5 * time.Millisecond
const commitTransactionRndTime = 5 * time.Millisecond
const executeStreamingSqlMinTime = 10 * time.Millisecond
const executeStreamingSqlRndTime = 10 * time.Millisecond
const executeSqlMinTime = 10 * time.Millisecond
const executeSqlRndTime = 10 * time.Millisecond

const holdSessionTime = 100
const rndWaitTimeBetweenRequests = 10

var mu sync.Mutex
var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

func createBenchmarkServer() (server *MockedSpannerInMemTestServer, client *Client, teardown func()) {
	t := &testing.T{}
	server, client, teardown = setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		// SessionPoolConfig is deprecated but kept for backward compatibility
		SessionPoolConfig: SessionPoolConfig{},
	})
	server.TestSpanner.PutExecutionTime(MethodCreateSession, SimulatedExecutionTime{
		MinimumExecutionTime: networkLatencyTime + createSessionsMinTime,
		RandomExecutionTime:  createSessionsRndTime,
	})
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql, SimulatedExecutionTime{
		MinimumExecutionTime: networkLatencyTime + executeStreamingSqlMinTime,
		RandomExecutionTime:  executeStreamingSqlRndTime,
	})
	server.TestSpanner.PutExecutionTime(MethodBeginTransaction, SimulatedExecutionTime{
		MinimumExecutionTime: networkLatencyTime + beginTransactionMinTime,
		RandomExecutionTime:  beginTransactionRndTime,
	})
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction, SimulatedExecutionTime{
		MinimumExecutionTime: networkLatencyTime + commitTransactionMinTime,
		RandomExecutionTime:  commitTransactionRndTime,
	})
	server.TestSpanner.PutExecutionTime(MethodExecuteSql, SimulatedExecutionTime{
		MinimumExecutionTime: networkLatencyTime + executeSqlMinTime,
		RandomExecutionTime:  executeSqlRndTime,
	})
	// Wait a moment for the multiplexed session to be ready
	time.Sleep(100 * time.Millisecond)
	return
}

func readWorker(client *Client, b *testing.B, jobs <-chan int, results chan<- int) {
	for range jobs {
		mu.Lock()
		d := time.Millisecond * time.Duration(rnd.Int63n(rndWaitTimeBetweenRequests))
		mu.Unlock()
		time.Sleep(d)
		iter := client.Single().Query(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		row := 0
		for {
			_, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				b.Error(err)
				return
			}
			row++
			if row == 1 {
				mu.Lock()
				d := time.Millisecond * time.Duration(rnd.Int63n(holdSessionTime))
				mu.Unlock()
				time.Sleep(d)
			}
		}
		iter.Stop()
		results <- row
	}
}

func writeWorker(client *Client, b *testing.B, jobs <-chan int, results chan<- int64) {
	for range jobs {
		mu.Lock()
		d := time.Millisecond * time.Duration(rnd.Int63n(rndWaitTimeBetweenRequests))
		mu.Unlock()
		time.Sleep(d)
		var updateCount int64
		_, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, txn *ReadWriteTransaction) error {
			stmt := Statement{
				SQL: UpdateBarSetFoo,
				Params: map[string]interface{}{
					"p1": 1,
				},
			}
			count, err := txn.Update(ctx, stmt)
			updateCount = count
			return err
		})
		if err != nil {
			b.Error(err)
			return
		}
		results <- updateCount
	}
}

func Benchmark_Client_BurstRead(b *testing.B) {
	benchmarkClientBurstRead(b)
}

func benchmarkClientBurstRead(b *testing.B) {
	for n := 0; n < b.N; n++ {
		_, client, teardown := createBenchmarkServer()

		totalQueries := 100
		jobs := make(chan int, totalQueries)
		results := make(chan int, totalQueries)
		parallel := 20

		for w := 0; w < parallel; w++ {
			go readWorker(client, b, jobs, results)
		}
		for j := 0; j < totalQueries; j++ {
			jobs <- j
		}
		close(jobs)
		totalRows := 0
		for a := 0; a < totalQueries; a++ {
			totalRows = totalRows + <-results
		}
		teardown()
	}
}

func Benchmark_Client_BurstWrite(b *testing.B) {
	benchmarkClientBurstWrite(b)
}

func benchmarkClientBurstWrite(b *testing.B) {
	for n := 0; n < b.N; n++ {
		_, client, teardown := createBenchmarkServer()

		totalUpdates := 100
		jobs := make(chan int, totalUpdates)
		results := make(chan int64, totalUpdates)
		parallel := 20

		for w := 0; w < parallel; w++ {
			go writeWorker(client, b, jobs, results)
		}
		for j := 0; j < totalUpdates; j++ {
			jobs <- j
		}
		close(jobs)
		var totalRowCount int64
		for a := 0; a < totalUpdates; a++ {
			totalRowCount = totalRowCount + <-results
		}
		teardown()
	}
}

func Benchmark_Client_BurstReadAndWrite(b *testing.B) {
	benchmarkClientBurstReadAndWrite(b)
}

func benchmarkClientBurstReadAndWrite(b *testing.B) {
	server, client, teardown := createBenchmarkServer()
	defer teardown()

	server.TestSpanner.PutStatementResult(UpdateBarSetFoo, &StatementResult{
		Type:        StatementResultUpdateCount,
		UpdateCount: 1,
	})

	for n := 0; n < b.N; n++ {
		totalQueries := 100
		jobs := make(chan int, totalQueries)
		results := make(chan int, totalQueries)

		parallel := 10

		for w := 0; w < parallel; w++ {
			go readWorker(client, b, jobs, results)
		}
		for j := 0; j < totalQueries; j++ {
			jobs <- j
		}
		close(jobs)
		totalRows := 0
		for a := 0; a < totalQueries; a++ {
			totalRows = totalRows + <-results
		}

		totalUpdates := 100
		wjobs := make(chan int, totalUpdates)
		wresults := make(chan int64, totalUpdates)

		for w := 0; w < parallel; w++ {
			go writeWorker(client, b, wjobs, wresults)
		}
		for j := 0; j < totalUpdates; j++ {
			wjobs <- j
		}
		close(wjobs)
		var totalRowCount int64
		for a := 0; a < totalUpdates; a++ {
			totalRowCount = totalRowCount + <-wresults
		}
	}
}

func Benchmark_Client_BurstReadAndWrite_Parallel(b *testing.B) {
	benchmarkClientBurstReadAndWriteParallel(b)
}

func benchmarkClientBurstReadAndWriteParallel(b *testing.B) {
	server, client, teardown := createBenchmarkServer()
	defer teardown()

	server.TestSpanner.PutStatementResult(UpdateBarSetFoo, &StatementResult{
		Type:        StatementResultUpdateCount,
		UpdateCount: 1,
	})

	for n := 0; n < b.N; n++ {
		totalOps := 100
		parallel := 10

		jobs := make(chan int, totalOps)
		results := make(chan int, totalOps)
		for w := 0; w < parallel; w++ {
			go readWorker(client, b, jobs, results)
		}

		wjobs := make(chan int, totalOps)
		wresults := make(chan int64, totalOps)
		for w := 0; w < parallel; w++ {
			go writeWorker(client, b, wjobs, wresults)
		}

		for j := 0; j < totalOps; j++ {
			jobs <- j
			wjobs <- j
		}
		close(jobs)
		close(wjobs)

		totalRows := 0
		var totalRowCount int64
		for a := 0; a < totalOps; a++ {
			totalRows = totalRows + <-results
			totalRowCount = totalRowCount + <-wresults
		}
	}
}
