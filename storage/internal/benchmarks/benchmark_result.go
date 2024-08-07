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
	"context"
	"fmt"
	"log"
	"runtime"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
)

type benchmarkResult struct {
	objectSize    int64
	directorySize int64 // if benchmark is on a directory, this will be > 0
	readOffset    int64
	params        randomizedParams
	isRead        bool
	readIteration int
	start         time.Time
	elapsedTime   time.Duration
	err           error
	timedOut      bool
	startMem      runtime.MemStats
	endMem        runtime.MemStats
	metricName    string // defaults to throughput
}

func (br *benchmarkResult) calculateThroughput() float64 {
	throughput := float64(br.objectSize) / float64(br.elapsedTime.Seconds())

	// Calculate throughput per entire directory
	if br.directorySize > 0 {
		throughput = float64(br.directorySize) / float64(br.elapsedTime.Seconds())
	}

	// Calculate throughput per range size instead of object size if only
	// downloading a range
	if br.isRead && opts.rangeSize > 0 {
		throughput = float64(opts.rangeSize) / float64(br.elapsedTime.Seconds())
		if br.directorySize > 0 {
			throughput = float64(opts.rangeSize) * float64(opts.numObjectsPerDirectory) / float64(br.elapsedTime.Seconds())
		}
	}

	return throughput
}

func (br *benchmarkResult) selectReadParams(opts benchmarkOptions, api benchmarkAPI) {
	br.params = randomizedParams{
		appBufferSize: opts.readBufferSize,
		crc32cEnabled: true,  // crc32c is always verified in the Go GCS library
		md5Enabled:    false, // we only need one integrity validation
		api:           api,
		rangeOffset:   randomInt64(opts.minReadOffset, opts.maxReadOffset),
	}

	if opts.readBufferSize == useDefault {
		switch api {
		case xmlAPI, jsonAPI:
			br.params.appBufferSize = 4 << 10 // default for HTTP
		case grpcAPI, directPath:
			br.params.appBufferSize = 32 << 10 // default for GRPC
		}
	}
}

func (br *benchmarkResult) selectWriteParams(opts benchmarkOptions, api benchmarkAPI) {
	// There is no XML implementation for writes
	if api == xmlAPI {
		api = jsonAPI
	}

	_, doMD5, doCRC32C := randomOf3()
	br.params = randomizedParams{
		appBufferSize: opts.writeBufferSize,
		chunkSize:     randomInt64(opts.minChunkSize, opts.maxChunkSize),
		crc32cEnabled: doCRC32C,
		md5Enabled:    doMD5,
		api:           api,
	}

	if opts.minChunkSize == useDefault || opts.maxChunkSize == useDefault {
		// get a writer on a non-existing object to check the default chunksize
		if c, err := storage.NewClient(context.Background()); err != nil {
			log.Printf("storage.NewClient: %v", err)
		} else {
			w := c.Bucket("").Object("").NewWriter(context.Background())
			br.params.chunkSize = int64(w.ChunkSize)
		}
	}
	if opts.writeBufferSize == useDefault {
		switch api {
		case xmlAPI, jsonAPI:
			br.params.appBufferSize = 4 << 10 // default for HTTP
		case grpcAPI, directPath:
			br.params.appBufferSize = 32 << 10 // default for GRPC
		}
	}
}

func (br *benchmarkResult) copyParams(from *benchmarkResult) {
	br.params = randomizedParams{
		appBufferSize: from.params.appBufferSize,
		chunkSize:     from.params.chunkSize,
		crc32cEnabled: from.params.crc32cEnabled,
		md5Enabled:    from.params.md5Enabled,
		api:           from.params.api,
		rangeOffset:   from.params.rangeOffset,
	}
}

// converts result to cloud monitoring format
func (br *benchmarkResult) cloudMonitoring() []byte {
	var sb strings.Builder
	op := "WRITE"
	if br.isRead {
		op = fmt.Sprintf("READ[%d]", br.readIteration)
	}
	status := "OK"
	if br.err != nil {
		status = "FAIL"
	}
	if br.timedOut {
		status = "TIMEOUT"
	}

	// Cloud monitoring only allows letters, numbers and underscores
	sanitizeKey := func(key string) string {
		key = strings.Replace(key, ".", "", -1)
		return strings.Replace(key, "/", "_", -1)
	}

	sanitizeValue := func(key string) string {
		return strings.Replace(key, "\"", "", -1)
	}

	// For values of type string
	makeStringQuoted := func(parameter string, value any) string {
		return fmt.Sprintf("%s=\"%v\"", parameter, value)
	}
	// For values of type int, bool
	makeStringUnquoted := func(parameter string, value any) string {
		return fmt.Sprintf("%s=%v", parameter, value)
	}

	sb.Grow(380)
	if br.metricName != "" {
		sb.WriteString(br.metricName)
		sb.WriteRune('{')
	} else {
		sb.WriteString("throughput{")
	}
	sb.WriteString(makeStringQuoted("library", "go"))
	sb.WriteString(",")
	sb.WriteString(makeStringQuoted("api", br.params.api))
	sb.WriteString(",")
	sb.WriteString(makeStringQuoted("op", op))
	sb.WriteString(",")

	if br.directorySize > 0 {
		sb.WriteString(makeStringUnquoted("directory_size", br.directorySize))
		sb.WriteString(",")
		sb.WriteString(makeStringUnquoted("num_objects", opts.numObjectsPerDirectory))
		sb.WriteString(",")
	} else {
		// object_size is not reliable for directories as they can have a mix of
		// object sizes; therefore, we do not output it
		sb.WriteString(makeStringUnquoted("object_size", br.objectSize))
		sb.WriteString(",")
	}

	if op != "WRITE" && opts.rangeSize > 0 {
		sb.WriteString(makeStringUnquoted("transfer_size", opts.rangeSize))
		sb.WriteString(",")
		sb.WriteString(makeStringUnquoted("transfer_offset", br.params.rangeOffset))
		sb.WriteString(",")
	}

	if op == "WRITE" {
		sb.WriteString(makeStringUnquoted("chunksize", br.params.chunkSize))
		sb.WriteString(",")
	}

	sb.WriteString(makeStringUnquoted("workers", opts.numWorkers))
	sb.WriteString(",")

	sb.WriteString(makeStringUnquoted("crc32c_enabled", br.params.crc32cEnabled))
	sb.WriteString(",")
	sb.WriteString(makeStringUnquoted("md5_enabled", br.params.md5Enabled))
	sb.WriteString(",")

	sb.WriteString(makeStringQuoted("bucket_name", opts.bucket))
	sb.WriteString(",")

	sb.WriteString(makeStringQuoted("status", status))
	sb.WriteString(",")
	if br.err != nil {
		sb.WriteString(makeStringQuoted("failure_msg", sanitizeValue(br.err.Error())))
		sb.WriteString(",")
	}

	sb.WriteString(makeStringUnquoted("app_buffer_size", br.params.appBufferSize))
	sb.WriteString(",")

	if br.params.api == directPath || br.params.api == grpcAPI {
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

	sb.WriteString("} ")
	sb.WriteString(strconv.FormatFloat(br.calculateThroughput(), 'f', 2, 64))

	return []byte(sb.String())
}

// converts result to csv writing format (ie. a slice of strings)
func (br *benchmarkResult) csv() []string {
	op := "WRITE"
	if br.isRead {
		op = fmt.Sprintf("READ[%d]", br.readIteration)
	}

	status := "OK"
	if br.err != nil {
		status = "FAIL"
	}
	if br.timedOut {
		status = "TIMEOUT"
	}

	record := make(map[string]string)

	record["Op"] = op
	if br.directorySize > 0 {
		record["DirectorySize"] = strconv.FormatInt(br.directorySize, 10)
		record["NumObjects"] = strconv.Itoa(opts.numObjectsPerDirectory)
	} else {
		record["ObjectSize"] = strconv.FormatInt(br.objectSize, 10)
	}
	if opts.rangeSize > 0 {
		record["TransferSize"] = strconv.Itoa(int(opts.rangeSize))
		record["TransferOffset"] = strconv.Itoa(int(br.params.rangeOffset))
	}
	record["AppBufferSize"] = strconv.Itoa(br.params.appBufferSize)
	record["LibBufferSize"] = strconv.Itoa(int(br.params.chunkSize))
	record["Crc32cEnabled"] = strconv.FormatBool(br.params.crc32cEnabled)
	record["MD5Enabled"] = strconv.FormatBool(br.params.md5Enabled)
	record["ApiName"] = string(br.params.api)
	record["ElapsedTimeUs"] = strconv.FormatInt(br.elapsedTime.Microseconds(), 10)
	record["CpuTimeUs"] = "-1" // TODO: record cpu time
	record["Status"] = status
	record["HeapSys"] = strconv.FormatUint(br.startMem.HeapSys, 10)
	record["HeapAlloc"] = strconv.FormatUint(br.startMem.HeapAlloc, 10)
	record["StackInUse"] = strconv.FormatUint(br.startMem.StackInuse, 10)
	// commented out to avoid large numbers messing up BigQuery imports
	record["HeapAllocDiff"] = "-1" //strconv.FormatUint(br.endMem.HeapAlloc-br.startMem.HeapAlloc, 10),
	record["MallocsDiff"] = strconv.FormatUint(br.endMem.Mallocs-br.startMem.Mallocs, 10)
	record["ConnectionPoolSize"] = strconv.Itoa(opts.connPoolSize)
	record["StartTime"] = strconv.FormatInt(br.start.Unix(), 10)
	record["EndTime"] = strconv.FormatInt(br.start.Add(br.elapsedTime).Unix(), 10)
	record["NumWorkers"] = strconv.Itoa(opts.numWorkers)
	record["CodeVersion"] = codeVersion
	record["BucketName"] = opts.bucket

	var result []string

	for _, h := range *selectHeader() {
		result = append(result, record[h])
	}

	return result
}

var (
	csvHeader = []string{
		"Op", "ObjectSize", "AppBufferSize", "LibBufferSize",
		"Crc32cEnabled", "MD5Enabled", "ApiName",
		"ElapsedTimeUs", "CpuTimeUs", "Status",
		"HeapSys", "HeapAlloc", "StackInUse", "HeapAllocDiff", "MallocsDiff",
		"ConnectionPoolSize", "StartTime", "EndTime", "NumWorkers",
		"CodeVersion", "BucketName",
	}
	csvHeaderRangeReads = []string{
		"Op", "ObjectSize", "TransferSize", "TransferOffset",
		"AppBufferSize", "LibBufferSize",
		"Crc32cEnabled", "MD5Enabled", "ApiName",
		"ElapsedTimeUs", "CpuTimeUs", "Status",
		"HeapSys", "HeapAlloc", "StackInUse", "HeapAllocDiff", "MallocsDiff",
		"ConnectionPoolSize", "StartTime", "EndTime", "NumWorkers",
		"CodeVersion", "BucketName",
	}
	csvHeaderWorkload6 = []string{
		"Op", "DirectorySize", "NumObjects", "AppBufferSize", "LibBufferSize",
		"Crc32cEnabled", "MD5Enabled", "ApiName",
		"ElapsedTimeUs", "CpuTimeUs", "Status",
		"HeapSys", "HeapAlloc", "StackInUse", "HeapAllocDiff", "MallocsDiff",
		"ConnectionPoolSize", "StartTime", "EndTime", "NumWorkers",
		"CodeVersion", "BucketName",
	}
)

func selectHeader() *[]string {
	header := &csvHeader
	if opts.workload == 6 {
		header = &csvHeaderWorkload6
	} else if opts.rangeSize > 0 {
		header = &csvHeaderRangeReads
	}
	return header
}
