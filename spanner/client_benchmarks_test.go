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
	"fmt"
	"math/rand"
	"reflect"
	"sync"
	"testing"
	"time"

	. "cloud.google.com/go/spanner/internal/testutil"
	"google.golang.org/api/iterator"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
)

const networkLatencyTime = 10 * time.Millisecond
const batchCreateSessionsMinTime = 10 * time.Millisecond
const batchCreateSessionsRndTime = 10 * time.Millisecond
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

func createBenchmarkServer(incStep uint64) (server *MockedSpannerInMemTestServer, client *Client, teardown func()) {
	t := &testing.T{}
	server, client, teardown = setupMockedTestServerWithConfig(t, ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     100,
			MaxOpened:     400,
			WriteSessions: 0.2,
			incStep:       incStep,
		},
	})
	server.TestSpanner.PutExecutionTime(MethodBatchCreateSession, SimulatedExecutionTime{
		MinimumExecutionTime: networkLatencyTime + batchCreateSessionsMinTime,
		RandomExecutionTime:  batchCreateSessionsRndTime,
	})
	server.TestSpanner.PutExecutionTime(MethodCreateSession, SimulatedExecutionTime{
		MinimumExecutionTime: networkLatencyTime + batchCreateSessionsMinTime,
		RandomExecutionTime:  batchCreateSessionsRndTime,
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
	// Wait until the session pool has been initialized.
	waitFor(t, func() error {
		if uint64(client.idleSessions.idleList.Len()+client.idleSessions.idleWriteList.Len()) == client.idleSessions.MinOpened {
			return nil
		}
		return fmt.Errorf("not yet initialized")
	})
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
				b.Fatal(err)
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
		var err error
		if _, err = client.ReadWriteTransaction(context.Background(), func(ctx context.Context, transaction *ReadWriteTransaction) error {
			if updateCount, err = transaction.Update(ctx, NewStatement(UpdateBarSetFoo)); err != nil {
				return err
			}
			return nil
		}); err != nil {
			b.Fatal(err)
		}
		results <- updateCount
	}
}

func Benchmark_Client_BurstRead_IncStep01(b *testing.B) {
	benchmarkClientBurstRead(b, 1)
}

func Benchmark_Client_BurstRead_IncStep10(b *testing.B) {
	benchmarkClientBurstRead(b, 10)
}

func Benchmark_Client_BurstRead_IncStep20(b *testing.B) {
	benchmarkClientBurstRead(b, 20)
}

func Benchmark_Client_BurstRead_IncStep25(b *testing.B) {
	benchmarkClientBurstRead(b, 25)
}

func Benchmark_Client_BurstRead_IncStep30(b *testing.B) {
	benchmarkClientBurstRead(b, 30)
}

func Benchmark_Client_BurstRead_IncStep40(b *testing.B) {
	benchmarkClientBurstRead(b, 40)
}

func Benchmark_Client_BurstRead_IncStep50(b *testing.B) {
	benchmarkClientBurstRead(b, 50)
}

func Benchmark_Client_BurstRead_IncStep100(b *testing.B) {
	benchmarkClientBurstRead(b, 100)
}

func benchmarkClientBurstRead(b *testing.B, incStep uint64) {
	for n := 0; n < b.N; n++ {
		server, client, teardown := createBenchmarkServer(incStep)
		sp := client.idleSessions
		if uint64(sp.idleList.Len()+sp.idleWriteList.Len()) != sp.MinOpened {
			b.Fatalf("session count mismatch\nGot: %d\nWant: %d", sp.idleList.Len()+sp.idleWriteList.Len(), sp.MinOpened)
		}

		totalQueries := int(sp.MaxOpened * 8)
		jobs := make(chan int, totalQueries)
		results := make(chan int, totalQueries)
		parallel := int(sp.MaxOpened * 2)

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
		reportBenchmark(b, sp, server)
		teardown()
	}
}

func Benchmark_Client_BurstWrite01(b *testing.B) {
	benchmarkClientBurstWrite(b, 1)
}

func Benchmark_Client_BurstWrite10(b *testing.B) {
	benchmarkClientBurstWrite(b, 10)
}

func Benchmark_Client_BurstWrite20(b *testing.B) {
	benchmarkClientBurstWrite(b, 20)
}

func Benchmark_Client_BurstWrite25(b *testing.B) {
	benchmarkClientBurstWrite(b, 25)
}

func Benchmark_Client_BurstWrite30(b *testing.B) {
	benchmarkClientBurstWrite(b, 30)
}

func Benchmark_Client_BurstWrite40(b *testing.B) {
	benchmarkClientBurstWrite(b, 40)
}

func Benchmark_Client_BurstWrite50(b *testing.B) {
	benchmarkClientBurstWrite(b, 50)
}

func Benchmark_Client_BurstWrite100(b *testing.B) {
	benchmarkClientBurstWrite(b, 100)
}

func benchmarkClientBurstWrite(b *testing.B, incStep uint64) {
	for n := 0; n < b.N; n++ {
		server, client, teardown := createBenchmarkServer(incStep)
		sp := client.idleSessions
		if uint64(sp.idleList.Len()+sp.idleWriteList.Len()) != sp.MinOpened {
			b.Fatalf("session count mismatch\nGot: %d\nWant: %d", sp.idleList.Len()+sp.idleWriteList.Len(), sp.MinOpened)
		}

		totalUpdates := int(sp.MaxOpened * 8)
		jobs := make(chan int, totalUpdates)
		results := make(chan int64, totalUpdates)
		parallel := int(sp.MaxOpened * 2)

		for w := 0; w < parallel; w++ {
			go writeWorker(client, b, jobs, results)
		}
		for j := 0; j < totalUpdates; j++ {
			jobs <- j
		}
		close(jobs)
		totalRows := int64(0)
		for a := 0; a < totalUpdates; a++ {
			totalRows = totalRows + <-results
		}
		reportBenchmark(b, sp, server)
		teardown()
	}
}

func Benchmark_Client_BurstReadAndWrite01(b *testing.B) {
	benchmarkClientBurstReadAndWrite(b, 1)
}

func Benchmark_Client_BurstReadAndWrite10(b *testing.B) {
	benchmarkClientBurstReadAndWrite(b, 10)
}

func Benchmark_Client_BurstReadAndWrite20(b *testing.B) {
	benchmarkClientBurstReadAndWrite(b, 20)
}

func Benchmark_Client_BurstReadAndWrite25(b *testing.B) {
	benchmarkClientBurstReadAndWrite(b, 25)
}

func Benchmark_Client_BurstReadAndWrite30(b *testing.B) {
	benchmarkClientBurstReadAndWrite(b, 30)
}

func Benchmark_Client_BurstReadAndWrite40(b *testing.B) {
	benchmarkClientBurstReadAndWrite(b, 40)
}

func Benchmark_Client_BurstReadAndWrite50(b *testing.B) {
	benchmarkClientBurstReadAndWrite(b, 50)
}

func Benchmark_Client_BurstReadAndWrite100(b *testing.B) {
	benchmarkClientBurstReadAndWrite(b, 100)
}

func benchmarkClientBurstReadAndWrite(b *testing.B, incStep uint64) {
	for n := 0; n < b.N; n++ {
		server, client, teardown := createBenchmarkServer(incStep)
		sp := client.idleSessions
		if uint64(sp.idleList.Len()+sp.idleWriteList.Len()) != sp.MinOpened {
			b.Fatalf("session count mismatch\nGot: %d\nWant: %d", sp.idleList.Len()+sp.idleWriteList.Len(), sp.MinOpened)
		}

		totalUpdates := int(sp.MaxOpened * 4)
		writeJobs := make(chan int, totalUpdates)
		writeResults := make(chan int64, totalUpdates)
		parallelWrites := int(sp.MaxOpened)

		totalQueries := int(sp.MaxOpened * 4)
		readJobs := make(chan int, totalQueries)
		readResults := make(chan int, totalQueries)
		parallelReads := int(sp.MaxOpened)

		for w := 0; w < parallelWrites; w++ {
			go writeWorker(client, b, writeJobs, writeResults)
		}
		for j := 0; j < totalUpdates; j++ {
			writeJobs <- j
		}
		for w := 0; w < parallelReads; w++ {
			go readWorker(client, b, readJobs, readResults)
		}
		for j := 0; j < totalQueries; j++ {
			readJobs <- j
		}

		close(writeJobs)
		close(readJobs)

		totalUpdatedRows := int64(0)
		for a := 0; a < totalUpdates; a++ {
			totalUpdatedRows = totalUpdatedRows + <-writeResults
		}
		totalReadRows := 0
		for a := 0; a < totalQueries; a++ {
			totalReadRows = totalReadRows + <-readResults
		}
		reportBenchmark(b, sp, server)
		teardown()
	}
}

func Benchmark_Client_SteadyIncrease01(b *testing.B) {
	benchmarkClientSteadyIncrease(b, 1)
}

func Benchmark_Client_SteadyIncrease10(b *testing.B) {
	benchmarkClientSteadyIncrease(b, 10)
}

func Benchmark_Client_SteadyIncrease20(b *testing.B) {
	benchmarkClientSteadyIncrease(b, 20)
}

func Benchmark_Client_SteadyIncrease25(b *testing.B) {
	benchmarkClientSteadyIncrease(b, 25)
}

func Benchmark_Client_SteadyIncrease30(b *testing.B) {
	benchmarkClientSteadyIncrease(b, 30)
}

func Benchmark_Client_SteadyIncrease40(b *testing.B) {
	benchmarkClientSteadyIncrease(b, 40)
}

func Benchmark_Client_SteadyIncrease50(b *testing.B) {
	benchmarkClientSteadyIncrease(b, 50)
}

func Benchmark_Client_SteadyIncrease100(b *testing.B) {
	benchmarkClientSteadyIncrease(b, 100)
}

func benchmarkClientSteadyIncrease(b *testing.B, incStep uint64) {
	for n := 0; n < b.N; n++ {
		server, client, teardown := createBenchmarkServer(incStep)
		sp := client.idleSessions
		if uint64(sp.idleList.Len()+sp.idleWriteList.Len()) != sp.MinOpened {
			b.Fatalf("session count mismatch\nGot: %d\nWant: %d", sp.idleList.Len()+sp.idleWriteList.Len(), sp.MinOpened)
		}

		transactions := make([]*ReadOnlyTransaction, sp.MaxOpened)
		for i := uint64(0); i < sp.MaxOpened; i++ {
			transactions[i] = client.ReadOnlyTransaction()
			transactions[i].Query(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		}
		for i := uint64(0); i < sp.MaxOpened; i++ {
			transactions[i].Close()
		}
		reportBenchmark(b, sp, server)
		teardown()
	}
}

func reportBenchmark(b *testing.B, sp *sessionPool, server *MockedSpannerInMemTestServer) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	requests := drainRequestsFromServer(server.TestSpanner)
	// TODO(loite): Use b.ReportMetric when Go1.13 is the minimum required.
	b.Logf("BatchCreateSessions: %d\t", countRequests(requests, reflect.TypeOf(&sppb.BatchCreateSessionsRequest{})))
	b.Logf("CreateSession: %d\t", countRequests(requests, reflect.TypeOf(&sppb.CreateSessionRequest{})))
	b.Logf("BeginTransaction: %d\t", countRequests(requests, reflect.TypeOf(&sppb.BeginTransactionRequest{})))
	b.Logf("Commit: %d\t", countRequests(requests, reflect.TypeOf(&sppb.CommitRequest{})))
	b.Logf("ReadSessions: %d\t", sp.idleList.Len())
	b.Logf("WriteSessions: %d\n", sp.idleWriteList.Len())
}

func countRequests(requests []interface{}, tp reflect.Type) (count int) {
	for _, req := range requests {
		if tp == reflect.TypeOf(req) {
			count++
		}
	}
	return count
}
