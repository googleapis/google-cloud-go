// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"time"

	"cloud.google.com/go/spanner"
	traceExporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

/*
**
Employees table schema:

CREATE TABLE Employees(

	ID int64,
	NAME STRING(50)

) PRIMARY KEY(ID)
*/
const (
	selectQuery  = "SELECT ID from EMPLOYEES WHERE ID = @p1"
	totalRecords = 100000
	tableName    = "EMPLOYEES"
)

type transactionType string

const (
	read  transactionType = "READ"
	query                 = "QUERY"
)

type cloudEnvironment string

const (
	production cloudEnvironment = "PRODUCTION"
	devel                       = "DEVEL"
)

var spannerHosts = map[cloudEnvironment]string{
	production: "spanner.googleapis.com:443",
	devel:      "staging-wrenchworks.sandbox.googleapis.com:443",
}

var monitoringHosts = map[cloudEnvironment]string{
	production: "monitoring.googleapis.com:443",
	devel:      "staging-monitoring.sandbox.googleapis.com:443",
}

var cloudTracingHosts = map[cloudEnvironment]string{
	production: "cloudtrace.googleapis.com:443",
	devel:      "staging-cloudtrace.sandbox.googleapis.com:443",
}

type benchmarkingConfiguration struct {
	warmUpTime            int8            // in minutes, default 7 minutes
	executionTime         int8            // in minutes, default 30 minutes
	waitBetweenRequests   int8            // in ms, 		 default 5 ms
	staleness             int8            // in seconds, default 15s
	parsedTransactionType transactionType // default read
	tracesEnabled         bool            // default false
	disableNativeMetrics  bool            // default false
}

func getDefaultBenchmarkingConfiguration() benchmarkingConfiguration {
	return benchmarkingConfiguration{warmUpTime: 7, executionTime: 30, waitBetweenRequests: 5, staleness: 15, parsedTransactionType: read, tracesEnabled: false, disableNativeMetrics: false}
}

func main() {
	ctx := context.Background()

	project := os.Getenv("SPANNER_CLIENT_BENCHMARK_GOOGLE_CLOUD_PROJECT")
	instance := os.Getenv("SPANNER_CLIENT_BENCHMARK_SPANNER_INSTANCE")
	database := os.Getenv("SPANNER_CLIENT_BENCHMARK_SPANNER_DATABASE")

	if project == "" || instance == "" || database == "" {
		fmt.Println(`You must set all the environment variables SPANNER_CLIENT_BENCHMARK_GOOGLE_CLOUD_PROJECT, 
			SPANNER_CLIENT_BENCHMARK_SPANNER_INSTANCE and SPANNER_CLIENT_BENCHMARK_SPANNER_DATABASE`)
		return
	}

	environment := parseCloudEnvironment(os.Getenv("SPANNER_CLIENT_BENCHMARK_CLOUD_ENVIRONMENT"))
	host := spannerHosts[environment]
	monitoringHost := monitoringHosts[environment]
	if err := setMonitoringHost(monitoringHost); err != nil {
		fmt.Println(err)
		return
	}

	bc := getDefaultBenchmarkingConfiguration()
	if err := parseCommandLineArguments(os.Args, &bc); err != nil {
		fmt.Println(err)
		return
	}

	db := fmt.Sprintf("projects/%v/instances/%v/databases/%v", project, instance, database)

	fmt.Printf("Running benchmark on %v\nEnvironment: %v\nWarm up time: %v mins\nExecution Time: %v mins\nWait Between Requests: %v ms\nStaleness: %v secs\nTraces Enabled: %v\nDisable Native Metrics: %v\nTransaction Type: %v\n\n", db, environment, bc.warmUpTime, bc.executionTime, bc.waitBetweenRequests, bc.staleness, bc.tracesEnabled, bc.disableNativeMetrics, bc.parsedTransactionType)

	if bc.tracesEnabled {
		enableTracingWithCloudTraceExporter(project, cloudTracingHosts[environment])
	}

	client, err := spanner.NewClientWithConfig(ctx, db, spanner.ClientConfig{DisableNativeMetrics: bc.disableNativeMetrics}, option.WithEndpoint(host))
	if err != nil {
		return
	}
	defer client.Close()

	err = warmUp(ctx, client, bc.warmUpTime, bc.staleness, bc.parsedTransactionType)
	if err != nil {
		fmt.Println(err)
		return
	}

	latencies, err := runBenchmark(ctx, client, bc.executionTime, bc.staleness, bc.waitBetweenRequests, bc.parsedTransactionType)
	if err != nil {
		fmt.Println(err)
		return
	}

	slices.Sort(latencies)

	fmt.Printf("\nResults\np50 %v\np95 %v\np99 %v\n", percentiles(0.5, latencies),
		percentiles(0.95, latencies), percentiles(0.99, latencies))
}

func warmUp(ctx context.Context, client *spanner.Client, warmupTime int8, staleness int8, transactionType transactionType) error {
	endTime := time.Now().Local().Add(time.Minute * time.Duration(warmupTime))

	go runTimer(endTime, "Remaining warmup time")
	for {
		if time.Now().Local().After(endTime) {
			break
		}
		_, err := execute(ctx, transactionType, client, staleness)
		if err != nil {
			fmt.Println(err)
			return err
		}
	}
	return nil
}

func runBenchmark(ctx context.Context, client *spanner.Client, executionTime int8, staleness int8, waitBetweenRequests int8, transactionType transactionType) ([]int64, error) {
	endTime := time.Now().Local().Add(time.Minute * time.Duration(executionTime))

	go runTimer(endTime, "Remaining operation time")
	var latencies []int64
	for {
		if time.Now().Local().After(endTime) {
			break
		}
		duration, err := execute(ctx, transactionType, client, staleness)
		if err != nil {
			fmt.Println(err)
			return make([]int64, 0), err
		}
		latencies = append(latencies, duration.Microseconds())
		time.Sleep(time.Millisecond * getRandomWaitTime(waitBetweenRequests))
	}
	return latencies, nil
}

func execute(ctx context.Context, transactionType transactionType, client *spanner.Client, staleness int8) (time.Duration, error) {
	switch transactionType {
	case query:
		return executeQuery(ctx, client, staleness)
	case read:
		return executeRead(ctx, client, staleness)
	default:
		return 0, errors.New("invalid transaction type")
	}
}

func executeQuery(ctx context.Context, client *spanner.Client, staleness int8) (time.Duration, error) {
	start := time.Now()

	iter := client.Single().WithTimestampBound(spanner.ExactStaleness(time.Second*time.Duration(staleness))).Query(ctx, spanner.Statement{SQL: selectQuery, Params: map[string]interface{}{
		"p1": generateUniqueID(),
	}})
	for {
		row, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return time.Duration(0), err
		}

		var id int64
		if err := row.Columns(&id); err != nil {
			return time.Duration(0), err
		}
	}

	return time.Since(start), nil
}

func executeRead(ctx context.Context, client *spanner.Client, staleness int8) (time.Duration, error) {
	start := time.Now()

	iter := client.Single().WithTimestampBound(spanner.ExactStaleness(time.Second*time.Duration(staleness))).Read(ctx, tableName, spanner.Key{generateUniqueID()}, []string{"ID"})
	for {
		row, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return time.Duration(0), err
		}

		var id int64
		if err := row.Columns(&id); err != nil {
			return time.Duration(0), err
		}
	}

	return time.Since(start), nil
}

func enableTracingWithCloudTraceExporter(projectID string, cloudTracingHost string) {
	res, err := resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceName("Go-Benchmarking"),
			semconv.ServiceVersion("0.1.0"),
		))

	if err != nil {
		log.Fatal(err)
	}

	// Create a new cloud trace exporter
	exporter, err := traceExporter.New(traceExporter.WithProjectID(projectID), traceExporter.WithTraceClientOptions([]option.ClientOption{option.WithEndpoint(cloudTracingHost)}))
	if err != nil {
		log.Fatal(err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.1)),
	)

	otel.SetTracerProvider(tracerProvider)
}

func runTimer(endTime time.Time, text string) {
	for {
		var t time.Time
		t = t.Add(endTime.Sub(time.Now()))
		fmt.Printf("\r%v %v", text, t.Format(time.TimeOnly))
		time.Sleep(time.Second)
		if time.Now().Local().After(endTime) {
			break
		}
	}
}

func setMonitoringHost(monitoringHost string) error {
	if err := os.Setenv("SPANNER_MONITORING_HOST", monitoringHost); err != nil {
		return err
	}
	return nil
}

func parseCommandLineArguments(args []string, bc *benchmarkingConfiguration) error {
	if len(args)%2 == 0 {
		return errors.New("some configuration is missing")
	}

	index := 1
	for {
		if index >= len(args) {
			break
		}
		commandLineOption := args[index][1:]
		commandLineValue := args[index+1]
		switch commandLineOption {
		case "wu", "warmUpTime":
			val, err := strconv.ParseInt(commandLineValue, 10, 8)
			if err != nil {
				return err
			}
			bc.warmUpTime = int8(val)
		case "et", "executionTime":
			val, err := strconv.ParseInt(commandLineValue, 10, 8)
			if err != nil {
				return err
			}
			bc.executionTime = int8(val)
		case "wbr", "waitBetweenRequests":
			val, err := strconv.ParseInt(commandLineValue, 10, 8)
			if err != nil {
				return err
			}
			bc.waitBetweenRequests = int8(val)
		case "st", "staleness":
			val, err := strconv.ParseInt(commandLineValue, 10, 8)
			if err != nil {
				return err
			}
			bc.staleness = int8(val)
		case "transactionType":
			parsedTransactionType, err := parseTransactionType(commandLineValue)
			if err != nil {
				return err
			}
			bc.parsedTransactionType = parsedTransactionType
		case "te", "tracesEnabled":
			tracesEnabled, err := strconv.ParseBool(commandLineValue)
			if err != nil {
				return err
			}
			bc.tracesEnabled = tracesEnabled
		case "dnm", "disableNativeMetrics":
			disableNativeMetrics, err := strconv.ParseBool(commandLineValue)
			if err != nil {
				return err
			}
			bc.disableNativeMetrics = disableNativeMetrics
		default:
			return fmt.Errorf("invalid option %v", commandLineOption)
		}
		index += 2
	}

	return nil
}

func parseTransactionType(s string) (transactionType, error) {
	switch s {
	case "READ":
		return read, nil
	case "QUERY":
		return query, nil
	default:
		return query, errors.New("invalid transaction type")
	}
}

func parseCloudEnvironment(environment string) cloudEnvironment {
	switch environment {
	case "DEVEL":
		return devel
	default:
		return production
	}
}

func percentiles(percentile float32, latencies []int64) any {
	rank := (percentile * float32(len(latencies)-1)) + 1
	return latencies[uint(rank)]
}

func generateUniqueID() int64 {
	return rand.Int64N(totalRecords) + 1
}

func getRandomWaitTime(waitTime int8) time.Duration {
	return time.Duration(rand.Int64N(int64(2*waitTime-1)) + 1)
}
