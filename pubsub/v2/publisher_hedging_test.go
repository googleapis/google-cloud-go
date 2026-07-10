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
	publisher.PublishSettings.DelayThreshold = 5 * time.Millisecond
	publisher.PublishSettings.CountThreshold = 10

	const duration = 10 * time.Second
	const qps = 2000
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

	p50 := latencies[n*50/100]
	p95 := latencies[n*95/100]
	p995 := latencies[n*995/1000]
	p9999 := latencies[n*9999/10000]
	max := latencies[n-1]

	t.Logf("=== [%s] Results (Succ: %d, DL_Exceeded: %d, in %v) ===", modeName, n, deadlineExceeded, totalTime.Round(time.Millisecond))
	
	// Average attempts = Total RPCs / (Total Messages / CountThreshold)
	// Since CountThreshold is 10, total batches is n / 10
	batches := float64(n) / 10.0
	if batches == 0 {
		batches = 1 // avoid div zero
	}
	avgAttempts := float64(rpcAttempts) / batches

	t.Logf("  Avg Attempts/Req: %.3f", avgAttempts)
	t.Logf("  p50:    %v", p50.Round(time.Millisecond))
	t.Logf("  p95:    %v", p95.Round(time.Millisecond))
	t.Logf("  p99.5:  %v", p995.Round(time.Millisecond))
	t.Logf("  p99.99: %v", p9999.Round(time.Millisecond))
	t.Logf("  max:    %v", max.Round(time.Millisecond))
}

func TestPublishHedgingPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping hedging performance evaluation in short mode")
	}
	t.Run("NoHedging", func(t *testing.T) {
		runHedgingSimulation(t, "NoHedging", nil)
	})
	t.Run("SingleHedging", func(t *testing.T) {
		runHedgingSimulation(t, "SingleHedging", &HedgingSettings{
			Delay:             50 * time.Millisecond,
			MaxHedgedAttempts: 1,
		})
	})
	t.Run("MultiHedging-Tokens10", func(t *testing.T) {
		runHedgingSimulation(t, "MultiHedging-Tokens10", &HedgingSettings{
			Delay:             50 * time.Millisecond,
			MaxHedgedAttempts: 0,
			MaxTokens:         10,
		})
	})
	t.Run("MultiHedging-Tokens100", func(t *testing.T) {
		runHedgingSimulation(t, "MultiHedging-Tokens100", &HedgingSettings{
			Delay:             50 * time.Millisecond,
			MaxHedgedAttempts: 0,
			MaxTokens:         100,
		})
	})
	t.Run("MultiHedging-Tokens500", func(t *testing.T) {
		runHedgingSimulation(t, "MultiHedging-Tokens500", &HedgingSettings{
			Delay:             50 * time.Millisecond,
			MaxHedgedAttempts: 0,
			MaxTokens:         500,
		})
	})
}

