package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
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
	writeSize := opts.writeQuantum * randomInt(opts.minWriteSize/opts.writeQuantum, opts.maxWriteSize/opts.writeQuantum)
	readSize := opts.readQuantum * randomInt(opts.minReadSize/opts.readQuantum, opts.maxReadSize/opts.readQuantum)

	results.writeResult.appBufferSize = writeSize
	results.applyToReadResults(func(r *benchmarkResult) {
		r.appBufferSize = readSize
	})
}

func selectChunkSize(opts benchmarkOptions, results *w1r3) {
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
func initClient(opts benchmarkOptions, results *w1r3) error {
	ctx := context.Background()

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

	// TODO: remove use of separate client once grpc is fully implemented

	return func(ctx context.Context) error {
		if results.writeResult.API == grpcAPI {
			clientMu.Lock()
			httpClient, err := storage.NewClient(ctx, option.WithCredentialsFile(credentialsFile))
			clientMu.Unlock()
			if err != nil {
				return fmt.Errorf("NewClient: %v", err)
			}
			defer httpClient.Close()
			o = httpClient.Bucket(o.BucketName()).Object(o.ObjectName())
		}

		return o.Delete(ctx)
	}
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

func upload(opts benchmarkOptions, results *w1r3) (err error) {
	ctx := context.Background()
	start := time.Now()
	defer func() {
		results.writeResult.elapsedTime = time.Since(start)
		results.writeResult.completed = err == nil
	}()

	o := results.client.Bucket(results.bucketName).Object(results.objectName)
	o = o.If(storage.Conditions{DoesNotExist: true})

	objectWriter := o.NewWriter(ctx)
	objectWriter.ChunkSize = results.writeResult.chunkSize

	f, err := os.Open(results.objectName)
	if err != nil {
		return fmt.Errorf("os.Open: %v", err)
	}
	defer f.Close()

	mw, md5Hash, crc32cHash := generateUploadWriter(objectWriter, results.writeResult.md5Enabled, results.writeResult.crc32cEnabled)

	if _, err = io.Copy(mw, f); err != nil {
		return fmt.Errorf("io.Copy: %v", err)
	}

	err = objectWriter.Close()
	if err != nil {
		return fmt.Errorf("writer.Close: %v", err)
	}

	if results.writeResult.md5Enabled || results.writeResult.crc32cEnabled {
		// TODO: remove use of separate client once grpc is fully implemented
		clientMu.Lock()
		httpClient, err := storage.NewClient(ctx, option.WithCredentialsFile(credentialsFile))
		clientMu.Unlock()
		if err != nil {
			return fmt.Errorf("NewClient: %v", err)
		}
		o := httpClient.Bucket(results.bucketName).Object(results.objectName)

		attrs, aerr := o.Attrs(ctx)
		if aerr != nil {
			return fmt.Errorf("get attrs on object: %v", aerr)
		}

		return verifyHash(md5Hash, crc32cHash, attrs.MD5, attrs.CRC32C)
	}

	return
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

func download(opts benchmarkOptions, results *w1r3, idx int) (err error) {
	result := results.readResults[idx]
	ctx := context.Background()
	start := time.Now()
	defer func() {
		result.isRead = true
		result.readIteration = idx
		result.elapsedTime = time.Since(start)
		result.completed = err == nil
	}()

	o := results.client.Bucket(results.bucketName).Object(results.objectName)

	f, err := os.Create(o.ObjectName())
	if err != nil {
		return fmt.Errorf("os.Create: %v", err)
	}
	defer func() {
		closeErr := f.Close()
		removeErr := os.Remove(o.ObjectName())
		// if we don't have another error to return, return error for closing file
		// if that error is also nil, return removeErr
		if err == nil {
			err = removeErr
			if closeErr != nil {
				err = closeErr
			}
		}
	}()

	objectReader, err := o.NewReader(ctx)
	if err != nil {
		return fmt.Errorf("Object(%q).NewReader: %v", o.ObjectName(), err)
	}
	defer func() {
		rerr := objectReader.Close()
		if rerr == nil {
			err = rerr
		}
	}()

	written, err := io.Copy(f, objectReader)
	if err != nil {
		return fmt.Errorf("io.Copy: %v", err)
	}

	if written != result.objectSize {
		return fmt.Errorf("did not read all bytes; read: %d, expected to read: %d", written, result.objectSize)
	}
	return nil
}
