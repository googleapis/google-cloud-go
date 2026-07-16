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

package pubsub

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/pubsub/v2/pstest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func makeLatencyInterceptor(rpcAttempts *int64) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if strings.HasSuffix(method, "/Publish") {
			atomic.AddInt64(rpcAttempts, 1)
			p := rand.Float64()
			var delay time.Duration
			if p < 0.01 { // 1% of RPCs get 4-second delay
				delay = 4 * time.Second
			} else if p < 0.05 { // 4% of RPCs get 300ms delay
				delay = 300 * time.Millisecond
			} else {
				delay = 5 * time.Millisecond
			}

			if delay > 0 {
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func runHedgingSimulation(t *testing.T, modeName string, settings *HedgingSettings) {
	ctx := context.Background()
	srv := pstest.NewServer()
	defer srv.Close()

	var rpcAttempts int64
	client, err := NewClient(ctx, projName,
		option.WithEndpoint(srv.Addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(makeLatencyInterceptor(&rpcAttempts))),
		option.WithTelemetryDisabled(),
	)
	if err != nil {
		t.Fatalf("[%s] NewClient err: %v", modeName, err)
	}
	defer client.Close()

	topicName := fmt.Sprintf("projects/%s/topics/perf-topic-%s", testutil.ProjID(), modeName)
	publisher := mustCreateTopic(t, client, topicName)
	defer publisher.Stop()

	publisher.PublishSettings.HedgingSettings = settings
	// Keep batch delay small for QPS test
	publisher.PublishSettings.DelayThreshold = 1 * time.Millisecond
	publisher.PublishSettings.CountThreshold = 1

	const duration = 1 * time.Minute
	const qps = 100
	ticker := time.NewTicker(time.Second / time.Duration(qps))
	defer ticker.Stop()

	timer := time.NewTimer(duration)
	defer timer.Stop()

	var mu sync.Mutex
	var latencies []time.Duration
	var wg sync.WaitGroup

	var deadlineExceeded int

	t.Logf("Starting [%s] test at %d QPS for %v...", modeName, qps, duration)
	startTest := time.Now()
loop:
	for {
		select {
		case <-timer.C:
			break loop
		case <-ticker.C:
			wg.Add(1)
			go func() {
				defer wg.Done()
				t0 := time.Now()
				res := publisher.Publish(ctx, &Message{Data: []byte("perf test payload")})
				_, err := res.Get(ctx)
				if err != nil {
					if strings.Contains(err.Error(), "DeadlineExceeded") || strings.Contains(err.Error(), "context deadline exceeded") {
						mu.Lock()
						deadlineExceeded++
						mu.Unlock()
					}
					return
				}
				elapsed := time.Since(t0)
				mu.Lock()
				latencies = append(latencies, elapsed)
				mu.Unlock()
			}()
		}
	}
	wg.Wait()
	totalTime := time.Since(startTest)

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	n := len(latencies)
	if n == 0 {
		t.Fatalf("[%s] zero successful publishes recorded", modeName)
	}

	p99 := latencies[n*99/100]
	p999 := latencies[n*999/1000]
	p9999 := latencies[n*9999/10000]
	max := latencies[n-1]

	t.Logf("=== [%s] Results (Succ: %d, DL_Exceeded: %d, in %v) ===", modeName, n, deadlineExceeded, totalTime.Round(time.Millisecond))

	// Average attempts = Total RPCs / (Total Messages / CountThreshold)
	// Since CountThreshold is 1, total batches is n
	batches := float64(n)
	if batches == 0 {
		batches = 1 // avoid div zero
	}
	avgAttempts := float64(rpcAttempts) / batches

	t.Logf("  Avg Attempts/Req: %.3f", avgAttempts)
	t.Logf("  p99:    %v", p99.Round(time.Millisecond))
	t.Logf("  p99.9:  %v", p999.Round(time.Millisecond))
	t.Logf("  p99.99: %v", p9999.Round(time.Millisecond))
	t.Logf("  max:    %v", max.Round(time.Millisecond))
}

func TestPublishHedgingPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping hedging performance evaluation in short mode")
	}

	// MaxTokens impact under normal 5% tail latency (should be identical since none starve)
	t.Run("MaxTokens50", func(t *testing.T) {
		runHedgingSimulation(t, "MaxTokens50", &HedgingSettings{
			Delay:      50 * time.Millisecond,
			MaxTokens:  50,
			TokenRatio: 0.1,
		})
	})
	t.Run("MaxTokens100", func(t *testing.T) {
		runHedgingSimulation(t, "MaxTokens100", &HedgingSettings{
			Delay:      50 * time.Millisecond,
			MaxTokens:  100,
			TokenRatio: 0.1,
		})
	})
	t.Run("MaxTokens250", func(t *testing.T) {
		runHedgingSimulation(t, "MaxTokens250", &HedgingSettings{
			Delay:      50 * time.Millisecond,
			MaxTokens:  250,
			TokenRatio: 0.1,
		})
	})

	// Refill Ratio impact (0.05 should starve and expose 4s latency, 0.1 and 0.2 should protect completely)
	t.Run("Ratio0.05", func(t *testing.T) {
		runHedgingSimulation(t, "Ratio0.05", &HedgingSettings{
			Delay:      50 * time.Millisecond,
			MaxTokens:  100,
			TokenRatio: 0.05,
		})
	})
	t.Run("Ratio0.1", func(t *testing.T) {
		runHedgingSimulation(t, "Ratio0.1", &HedgingSettings{
			Delay:      50 * time.Millisecond,
			MaxTokens:  100,
			TokenRatio: 0.1,
		})
	})
	t.Run("Ratio0.2", func(t *testing.T) {
		runHedgingSimulation(t, "Ratio0.2", &HedgingSettings{
			Delay:      50 * time.Millisecond,
			MaxTokens:  100,
			TokenRatio: 0.2,
		})
	})
}

func makeCircuitBreakerInterceptor(rpcAttempts *int64, failRate float64, delay time.Duration) grpc.UnaryClientInterceptor {
	var reqCount int64
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		atomic.AddInt64(rpcAttempts, 1)
		id := atomic.AddInt64(&reqCount, 1)

		isFail := float64(id%100) < (failRate * 100.0)

		if isFail {
			time.Sleep(delay)
		} else {
			time.Sleep(2 * time.Millisecond) // fast success
		}

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func runCircuitBreakerSimulation(t *testing.T, modeName string, settings *HedgingSettings, failRate float64, delay time.Duration) {
	ctx := context.Background()
	srv := pstest.NewServer()
	defer srv.Close()

	var rpcAttempts int64

	client, err := NewClient(ctx, testutil.ProjID(),
		option.WithEndpoint(srv.Addr),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(makeCircuitBreakerInterceptor(&rpcAttempts, failRate, delay))),
		option.WithTelemetryDisabled(),
	)
	if err != nil {
		t.Fatalf("[%s] NewClient err: %v", modeName, err)
	}
	defer client.Close()

	topicName := fmt.Sprintf("projects/%s/topics/cb-topic-%s", testutil.ProjID(), modeName)
	publisher := mustCreateTopic(t, client, topicName)
	defer publisher.Stop()

	publisher.PublishSettings.HedgingSettings = settings
	publisher.PublishSettings.DelayThreshold = 5 * time.Millisecond
	publisher.PublishSettings.CountThreshold = 10

	const duration = 15 * time.Second
	const qps = 100 // Lower QPS
	ticker := time.NewTicker(time.Second / time.Duration(qps))
	defer ticker.Stop()

	timer := time.NewTimer(duration)
	defer timer.Stop()

	var wg sync.WaitGroup
	var publishes int64
	startTest := time.Now()

	testCtx, cancel := context.WithCancel(ctx)
loop:
	for {
		select {
		case <-timer.C:
			cancel()
			break loop
		case <-ticker.C:
			wg.Add(1)
			go func() {
				defer wg.Done()
				atomic.AddInt64(&publishes, 1)
				publisher.Publish(testCtx, &Message{Data: []byte("perf")}).Get(testCtx)
			}()
		}
	}
	wg.Wait()

	expectedBatches := float64(publishes) / 10.0
	actualRPCs := float64(atomic.LoadInt64(&rpcAttempts))

	t.Logf("\n=== [%s] Results (Succ: %d, in %v) ===", modeName, publishes, time.Since(startTest))
	t.Logf("  Attempts/Req: %.3f", actualRPCs/expectedBatches)
	t.Logf("  Net Extra RPCs: %.0f", actualRPCs-expectedBatches)
}

func TestCircuitBreaker(t *testing.T) {
	// 100% Outage (to exactly measure MaxTokens burst limit)
	t.Run("MaxTokens50", func(t *testing.T) {
		runCircuitBreakerSimulation(t, "MaxTokens50", &HedgingSettings{
			Delay:      50 * time.Millisecond,
			MaxTokens:  50,
			TokenRatio: 0.1,
		}, 1.0, 2*time.Second) // 100% failure
	})
	t.Run("MaxTokens100", func(t *testing.T) {
		runCircuitBreakerSimulation(t, "MaxTokens100", &HedgingSettings{
			Delay:      50 * time.Millisecond,
			MaxTokens:  100,
			TokenRatio: 0.1,
		}, 1.0, 2*time.Second) // 100% failure
	})
	t.Run("MaxTokens250", func(t *testing.T) {
		runCircuitBreakerSimulation(t, "MaxTokens250", &HedgingSettings{
			Delay:      50 * time.Millisecond,
			MaxTokens:  250,
			TokenRatio: 0.1,
		}, 1.0, 2*time.Second) // 100% failure
	})

	// 12.5% Outage (to show 0.2 goes blind, while 0.1 and 0.05 trip the breaker)
	t.Run("Ratio0.05", func(t *testing.T) {
		runCircuitBreakerSimulation(t, "Ratio0.05", &HedgingSettings{
			Delay:      50 * time.Millisecond,
			MaxTokens:  100,
			TokenRatio: 0.05,
		}, 0.125, 2*time.Second)
	})
	t.Run("Ratio0.1", func(t *testing.T) {
		runCircuitBreakerSimulation(t, "Ratio0.1", &HedgingSettings{
			Delay:      50 * time.Millisecond,
			MaxTokens:  100,
			TokenRatio: 0.1,
		}, 0.125, 2*time.Second)
	})
	t.Run("Ratio0.2", func(t *testing.T) {
		runCircuitBreakerSimulation(t, "Ratio0.2", &HedgingSettings{
			Delay:      50 * time.Millisecond,
			MaxTokens:  100,
			TokenRatio: 0.2,
		}, 0.125, 2*time.Second)
	})
}
