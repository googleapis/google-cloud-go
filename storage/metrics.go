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

package storage

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/storage/internal"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	m "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
)

var meter = otel.Meter("cloud-storage")

// type MetricDescriptor struct {
// 	Type string
// }
// type Metric string
// type Metrics []Metric
// type registryMetrics struct {
// 	intCounts map[*MetricDescriptor]metric.Int64Counter
// }

// func (rm *registryMetrics) registerMetrics(metrics *Metrics, meter metric.Meter) {
// 	rm.intCounts = make(map[*MetricDescriptor]metric.Int64Counter)

// 	for metric := range metrics.Metrics() {
// 		desc := DescriptorForMetric(metric)
// 		if desc == nil {
// 			// Either the metric was per call or the metric is not registered.
// 			// Thus, if this component ever receives the desc as a handle in
// 			// record it will be a no-op.
// 			continue
// 		}
// 		switch desc.Type {
// 		case estats.MetricTypeIntCount:
// 			rm.intCounts[desc] = createInt64Counter(metrics.Metrics(), desc.Name, meter, otelmetric.WithUnit(desc.Unit), otelmetric.WithDescription(desc.Description))
// 		}
// 	}
// }

//	func createInt64Gauge(setOfMetrics map[Metric]bool, metricName Metric, meter Meter, options ...Int64GaugeOption) Int64Gauge {
//		if _, ok := setOfMetrics[metricName]; !ok {
//			return noop.Int64Gauge{}
//		}
//		ret, err := meter.Int64Gauge(string(metricName), options...)
//		if err != nil {
//			logger.Errorf("failed to register metric \"%v\", will not record: %v", metricName, err)
//			return noop.Int64Gauge{}
//		}
//		return ret
//	}
var wg sync.WaitGroup

func initializeMetrics() {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	exp, err := stdoutmetric.New(
		stdoutmetric.WithEncoder(enc),
		stdoutmetric.WithoutTimestamps(),
	)
	if err != nil {
		panic(err)
	}

	// Register the exporter with an SDK via a periodic reader.
	sdk := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(exp)),
	)
	sdkMeter := sdk.Meter("gcloud-go", m.WithInstrumentationVersion(internal.Version))
	ctx := context.Background()
	apiCounter, err := sdkMeter.Int64Counter(
		"api.counter",
		m.WithDescription("Number of API calls."),
		// Unit name using human readable string uses {} around it.
		// https://screenshot.googleplex.com/9ugGQzKthzw6wms
		m.WithUnit("{call}"),
	)
	if err != nil {
		panic(err)
	}

	// Start 100 goroutines
	for i := 0; i < 100; i++ {
		wg.Add(1) // Increment the WaitGroup counter

		go func(id int) {
			defer wg.Done() // Decrement the WaitGroup counter when the goroutine finishes
			time.Sleep(2*time.Second + time.Duration(i))
			apiCounter.Add(context.TODO(), 1)
		}(i)
	}

	wg.Wait() // Wait for all goroutines to finish

	// time.Sleep(60 * time.Second)
	// Ensure the periodic reader is cleaned up by shutting down the sdk.
	_ = sdk.Shutdown(ctx)
}
