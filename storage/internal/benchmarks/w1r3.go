package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"

	"cloud.google.com/go/storage"
)

type w1r3 struct {
	client                 *storage.Client
	bucketName, objectName string
	writeResult            *benchmarkResult
	readResults            []*benchmarkResult
}

func (r *w1r3) applyToAllResults(funcToApply func(r *benchmarkResult)) {
	for _, res := range r.readResults {
		funcToApply(res)
	}
	funcToApply(r.writeResult)
}

func (r *w1r3) applyToReadResults(funcToApply func(r *benchmarkResult)) {
	for _, res := range r.readResults {
		funcToApply(res)
	}
}

func selectChecksumsToPerform(opts benchmarkOptions, results *w1r3) {
	_, doMD5, doCRC32C := randomOf3()
	results.writeResult.md5Enabled = doMD5
	results.writeResult.crc32cEnabled = doCRC32C

	results.applyToReadResults(func(r *benchmarkResult) {
		// crc32c is always verified in the Go GCS library
		r.crc32cEnabled = true
		// we only need one integrity validation
		r.md5Enabled = false
	})
}

func selectObjectSize(opts benchmarkOptions, results *w1r3) {
	size := randomInt64(opts.minObjectSize, opts.maxObjectSize)

	results.applyToAllResults(func(r *benchmarkResult) {
		r.objectSize = size
	})
}

func selectAppBufferSize(opts benchmarkOptions, results *w1r3) {
	writeSize := -1
	readSize := -1
	if !opts.useDefaults {
		writeSize = opts.writeQuantum * randomInt(opts.minWriteSize/opts.writeQuantum, opts.maxWriteSize/opts.writeQuantum)
		readSize = opts.readQuantum * randomInt(opts.minReadSize/opts.readQuantum, opts.maxReadSize/opts.readQuantum)
	}

	results.writeResult.appBufferSize = writeSize
	results.applyToReadResults(func(r *benchmarkResult) {
		r.appBufferSize = readSize
	})
}

func selectChunkSize(opts benchmarkOptions, results *w1r3) {
	if opts.useDefaults {
		results.writeResult.chunkSize = -1
		return
	}
	results.writeResult.chunkSize = int(randomInt64(opts.minChunkSize, opts.maxChunkSize))
	// reads don't use chunk size
}

func selectClient(opts benchmarkOptions, results *w1r3) {
	apiToUse := opts.api
	if apiToUse == mixedAPIs {
		if randomBool() {
			apiToUse = xmlAPI
		} else {
			apiToUse = grpcAPI
		}
	}

	switch apiToUse {
	case xmlAPI, jsonAPI:
		results.writeResult.API = jsonAPI
		results.applyToReadResults(func(r *benchmarkResult) {
			r.API = xmlAPI
		})
	case grpcAPI:
		results.applyToAllResults(func(r *benchmarkResult) {
			r.API = grpcAPI
		})
	default:
		// Stop the whole program; subsequent calls with same api will fail too
		log.Fatalf("%s API not supported.\n", opts.api)
	}
}

// selectClient must be called before initClient
func initClient(ctx context.Context, opts benchmarkOptions, results *w1r3) error {
	if opts.useDefaults {
		if results.writeResult.API == grpcAPI {
			clientMu.Lock()
			os.Setenv("STORAGE_USE_GRPC", "true")
			c, err := storage.NewClient(ctx)
			os.Unsetenv("STORAGE_USE_GRPC")
			clientMu.Unlock()

			results.client = c
			return err
		}

		if results.writeResult.API == jsonAPI {
			clientMu.Lock()
			c, err := storage.NewClient(ctx)
			clientMu.Unlock()

			results.client = c
			return err
		}
		return fmt.Errorf("something went wrong; was selectClient called before initClient?")
	}

	writeSize := results.writeResult.appBufferSize
	readSize := results.readResults[0].appBufferSize

	if results.writeResult.API == grpcAPI {
		c, err := initGRPCClient(ctx, writeSize, readSize)

		results.client = c
		return err
	}

	if results.writeResult.API == jsonAPI {
		c, err := initHTTPClient(ctx, writeSize, readSize)

		results.client = c
		return err
	}

	return fmt.Errorf("something went wrong; was selectClient called before initClient?")
}

func createContents(opts benchmarkOptions, results *w1r3) error {
	objectName, err := generateRandomFile(results.writeResult.objectSize)

	results.objectName = objectName
	return err
}

func deleteObjectFunc(opts benchmarkOptions, results *w1r3) func(context.Context) error {
	objectName := results.objectName
	o := results.client.Bucket(results.bucketName).Object(objectName)
	return o.Delete
}

func captureInitialMemoryWrites(opts benchmarkOptions, results *w1r3) {
	forceGarbageCollection(opts.forceGC)
	var memStats *runtime.MemStats = &runtime.MemStats{}

	runtime.ReadMemStats(memStats)
	results.writeResult.heapAllocDiff = memStats.HeapAlloc
	results.writeResult.mallocsDiff = memStats.Mallocs
}

func captureFinalMemoryWrites(results *w1r3) {
	var memStats *runtime.MemStats = &runtime.MemStats{}

	runtime.ReadMemStats(memStats)
	results.writeResult.heapAllocDiff = memStats.HeapAlloc - results.writeResult.heapAllocDiff
	results.writeResult.mallocsDiff = memStats.Mallocs - results.writeResult.mallocsDiff
	results.writeResult.heapAlloc = memStats.HeapAlloc
	results.writeResult.heapSys = memStats.HeapSys
	results.writeResult.stackInUse = memStats.StackInuse
}

func captureInitialMemoryReads(opts benchmarkOptions, results *w1r3) {
	forceGarbageCollection(opts.forceGC)
	var memStats *runtime.MemStats = &runtime.MemStats{}

	runtime.ReadMemStats(memStats)
	results.applyToReadResults(func(r *benchmarkResult) {
		r.heapAllocDiff = memStats.HeapAlloc
		r.mallocsDiff = memStats.Mallocs
	})
}

func captureFinalMemoryReads(results *w1r3) {
	var memStats *runtime.MemStats = &runtime.MemStats{}

	runtime.ReadMemStats(memStats)
	runtime.ReadMemStats(memStats)
	results.applyToReadResults(func(r *benchmarkResult) {
		r.heapAllocDiff = memStats.HeapAlloc - results.writeResult.heapAllocDiff
		r.mallocsDiff = memStats.Mallocs - results.writeResult.mallocsDiff
		r.heapAlloc = memStats.HeapAlloc
		r.heapSys = memStats.HeapSys
		r.stackInUse = memStats.StackInuse
	})
}
