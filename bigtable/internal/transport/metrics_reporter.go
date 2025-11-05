// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"context"
	"log"
	"sync"
	"time"

	btopt "cloud.google.com/go/bigtable/internal/option"
	"go.opentelemetry.io/otel/metric"
)

// MetricsReporter periodically collects and reports metrics for the connection pool.
type MetricsReporter struct {
	config   btopt.MetricsReporterConfig
	pool     *BigtableChannelPool
	ticker   *time.Ticker
	done     chan struct{}
	stopOnce sync.Once
}

func NewMetricsReporter(config btopt.MetricsReporterConfig, pool *BigtableChannelPool, logger *log.Logger) *MetricsReporter {
	if pool.meterProvider != nil && config.Enabled {
		meter := pool.meterProvider.Meter("bigtable.googleapis.com/internal/client/")
		var err error
		pool.outstandingRPCsHistogram, err = meter.Float64Histogram("connection_pool/outstanding_rpcs", metric.WithDescription("Distribution of outstanding RPCs per connection."), metric.WithUnit("1"))
		if err != nil {
			btopt.Debugf(logger, "bigtable_connpool: failed to create outstanding_rpcs histogram: %v\n", err)
		}
		pool.perConnectionErrorCountHistogram, err = meter.Float64Histogram("per_connection_error_count", metric.WithDescription("Distribution of errors per connection per minute."), metric.WithUnit("1"))
		if err != nil {
			btopt.Debugf(logger, "bigtable_connpool: failed to create per_connection_error_count histogram: %v\n", err)
		}
	}
	return &MetricsReporter{
		config: config,
		pool:   pool,
		done:   make(chan struct{}),
	}
}

func (mr *MetricsReporter) Start(ctx context.Context) {
	if !mr.config.Enabled || (mr.pool.outstandingRPCsHistogram == nil && mr.pool.perConnectionErrorCountHistogram == nil) {
		return
	}
	mr.ticker = time.NewTicker(mr.config.ReportingInterval)
	go func() {
		defer mr.ticker.Stop()
		for {
			select {
			case <-mr.ticker.C:
				mr.pool.snapshotAndRecordMetrics(ctx)
			case <-mr.done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (mr *MetricsReporter) Stop() {
	mr.stopOnce.Do(func() {
		if mr.config.Enabled {
			close(mr.done)
		}
	})
}
