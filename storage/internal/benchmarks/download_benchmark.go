// Copyright 2022 Google LLC
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
	"io"
	"os"
	"path"
	"time"

	"cloud.google.com/go/storage"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type downloadOpts struct {
	client              *storage.Client
	objectSize          int64
	bucket              string
	object              string
	rangeStart          int64
	rangeLength         int64
	downloadToDirectory string
	timeout             time.Duration
}

func downloadBenchmark(ctx context.Context, dopts downloadOpts) (elapsedTime time.Duration, rerr error) {
	var span trace.Span
	ctx, span = otel.GetTracerProvider().Tracer(tracerName).Start(ctx, "download")
	span.SetAttributes(
		attribute.KeyValue{Key: "object_size", Value: attribute.Int64Value(dopts.objectSize)},
		attribute.KeyValue{Key: "bucket", Value: attribute.StringValue(dopts.bucket)},
	)
	defer span.End()
	// Set timer
	start := time.Now()
	// Multiple defer statements execute in LIFO order, so this will be the last
	// thing executed. We use named return parameters so that we can set it directly
	// and defer the statement so that the time includes typical cleanup steps and
	// gets set regardless of errors.
	defer func() { elapsedTime = time.Since(start) }()

	// Set additional timeout
	ctx, cancel := context.WithTimeout(ctx, dopts.timeout)
	defer cancel()

	o := dopts.client.Bucket(dopts.bucket).Object(dopts.object)

	// Create file to download to
	f, err := os.Create(path.Join(dopts.downloadToDirectory, o.ObjectName()))
	if err != nil {
		rerr = fmt.Errorf("os.Create: %w", err)
		return
	}
	defer func() {
		closeErr := f.Close()
		// if we don't have another error to return, return error for closing file
		if rerr == nil {
			rerr = closeErr
		}
	}()

	// Get reader from object
	objectReader, err := o.NewRangeReader(ctx, dopts.rangeStart, dopts.rangeLength)
	if err != nil {
		rerr = fmt.Errorf("Object(%q).NewReader: %w", o.ObjectName(), err)
		return
	}
	defer func() {
		err := objectReader.Close()
		if rerr == nil {
			rerr = err
		}
	}()

	// Download
	read, err := io.Copy(f, objectReader)
	if err != nil {
		rerr = fmt.Errorf("io.Copy: %w", err)
		return
	}

	expectedReadBytes := dopts.objectSize
	if dopts.rangeLength > 0 {
		expectedReadBytes = dopts.rangeLength
	}

	if read != expectedReadBytes {
		rerr = fmt.Errorf("did not read all bytes; read: %d, expected to read: %d", read, dopts.objectSize)
		return
	}

	return
}
