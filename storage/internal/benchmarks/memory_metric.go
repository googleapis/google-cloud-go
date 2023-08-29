// Copyright 2023 Google LLC
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

package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// startRecordingMemory writes memory stats to w every interval, until the
// returned function is called.
func startRecordingMemory(w io.Writer, oType outputType, interval time.Duration) (cancel func()) {
	var memStats *runtime.MemStats = &runtime.MemStats{}
	done := make(chan bool)

	if oType == outputCSV {
		writeMemoryHeader(w)
	}

	go func() {
		ticker := time.NewTicker(interval)

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				runtime.ReadMemStats(memStats)

				if oType == outputCSV {
					writeMemStatsAsCSV(w, *memStats)
				} else if oType == outputCloudMonitoring {
					writeMemStatsAsCloudMonitoring(w, *memStats)
				}
			}
		}
	}()

	return func() { done <- true }
}

func writeMemoryHeader(w io.Writer) {
	cw := csv.NewWriter(w)
	if err := cw.Write(csvMemoryHeader); err != nil {
		log.Fatalf("error writing csv header: %v", err)
	}
	cw.Flush()
}

var csvMemoryHeader = []string{
	"TimeUs", "HeapAlloc", "HeapSys", "HeapInuse", "HeapObjects", "StackInuse",
	"NumWorkers", "NumClients", "CodeVersion", "BucketName",
}

func writeMemStatsAsCSV(w io.Writer, stats runtime.MemStats) {
	cw := csv.NewWriter(w)
	record := make(map[string]string)

	record["TimeUs"] = strconv.FormatInt(time.Now().Unix(), 10)
	record["HeapSys"] = strconv.Itoa(int(stats.HeapSys))
	record["HeapAlloc"] = strconv.Itoa(int(stats.HeapAlloc))
	record["HeapInuse"] = strconv.Itoa(int(stats.HeapInuse))
	record["HeapObjects"] = strconv.Itoa(int(stats.HeapObjects))
	record["StackInuse"] = strconv.Itoa(int(stats.StackInuse))
	record["NumWorkers"] = strconv.Itoa(opts.numWorkers)
	record["NumClients"] = strconv.Itoa(opts.numClients)
	record["CodeVersion"] = codeVersion
	record["BucketName"] = opts.bucket

	var result []string

	for _, h := range csvMemoryHeader {
		result = append(result, record[h])
	}

	if err := cw.Write(result); err != nil {
		log.Fatalf("error writing csv: %v", err)
	}
	cw.Flush()
}

func writeMemStatsAsCloudMonitoring(w io.Writer, stats runtime.MemStats) {
	var sb strings.Builder
	sb.Grow(380)

	sb.WriteString("heapsys{")
	addCloudMonitoringMetadata(&sb)
	sb.WriteString("} ")
	sb.WriteString(strconv.Itoa(int(stats.HeapSys)))
	sb.WriteByte('\n')

	sb.WriteString("heapalloc{")
	addCloudMonitoringMetadata(&sb)
	sb.WriteString("} ")
	sb.WriteString(strconv.Itoa(int(stats.HeapAlloc)))
	sb.WriteByte('\n')

	sb.WriteString("heapinuse{")
	addCloudMonitoringMetadata(&sb)
	sb.WriteString("} ")
	sb.WriteString(strconv.Itoa(int(stats.HeapInuse)))
	sb.WriteByte('\n')

	sb.WriteString("heapobjects{")
	addCloudMonitoringMetadata(&sb)
	sb.WriteString("} ")
	sb.WriteString(strconv.Itoa(int(stats.HeapObjects)))
	sb.WriteByte('\n')

	sb.WriteString("stackinuse{")
	addCloudMonitoringMetadata(&sb)
	sb.WriteString("} ")
	sb.WriteString(strconv.Itoa(int(stats.StackInuse)))
	sb.WriteByte('\n')

	_, err := w.Write([]byte(sb.String()))
	if err != nil {
		log.Fatalf("cloud monitoring w.Write: %v", err)
	}
}

func addCloudMonitoringMetadata(sb *strings.Builder) {
	// Cloud monitoring only allows letters, numbers and underscores
	sanitizeKey := func(key string) string {
		key = strings.Replace(key, ".", "", -1)
		return strings.Replace(key, "/", "_", -1)
	}

	// For values of type string
	makeStringQuoted := func(parameter string, value any) string {
		return fmt.Sprintf("%s=\"%v\"", parameter, value)
	}
	// For values of type int, bool
	makeStringUnquoted := func(parameter string, value any) string {
		return fmt.Sprintf("%s=%v", parameter, value)
	}

	sb.WriteString(makeStringQuoted("library", "go"))
	sb.WriteString(",")

	sb.WriteString(makeStringUnquoted("workers", opts.numWorkers))
	sb.WriteString(",")

	sb.WriteString(makeStringQuoted("bucket_name", opts.bucket))
	sb.WriteString(",")

	sb.WriteString(makeStringQuoted("clients", opts.numClients))
	sb.WriteString(",")

	if opts.api == directPath || opts.api == grpcAPI {
		sb.WriteString(makeStringUnquoted("connection_pool_size", opts.connPoolSize))
		sb.WriteString(",")
	}

	if len(opts.endpoint) > 0 {
		sb.WriteString(makeStringQuoted("endpoint", opts.endpoint))
		sb.WriteString(",")
	}

	sb.WriteString(makeStringQuoted("code_version", codeVersion))
	sb.WriteString(",")
	sb.WriteString(makeStringQuoted("go_version", goVersion))
	for dep, ver := range dependencyVersions {
		sb.WriteString(",")
		sb.WriteString(makeStringQuoted(sanitizeKey(dep), ver))
	}
}
