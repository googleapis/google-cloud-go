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
	"time"

	"cloud.google.com/go/storage"
)

type uploadParams struct {
	o              *storage.ObjectHandle
	contentsReader io.ReadSeeker
	readerLength   int
	objectSize     int64
	chunkSize      int
}

func uploadBenchmark(ctx context.Context, u uploadParams) (time.Duration, error) {
	elapsedTime := time.Duration(0)
	start := time.Now()

	u.o.If(storage.Conditions{
		DoesNotExist: true,
	})

	objectWriter := u.o.NewWriter(ctx)
	objectWriter.ChunkSize = u.chunkSize

	var written int64
	for written < u.objectSize {
		// stop timer here
		elapsedTime += time.Since(start)
		rewindReader(u, written)
		// restart timer
		start = time.Now()

		w, err := io.Copy(objectWriter, u.contentsReader)
		if err != nil {
			objectWriter.Close()
			elapsedTime += time.Since(start)
			return elapsedTime, fmt.Errorf("io.Copy after %d bytes: %v", written, err)
		}

		written += w
	}

	if err := objectWriter.Close(); err != nil {
		elapsedTime += time.Since(start)
		return elapsedTime, fmt.Errorf("Writer.Close: %v", err)
	}
	elapsedTime += time.Since(start)
	return elapsedTime, nil
}

// rewinds the ReadSeeker so we can copy more bytes
func rewindReader(u uploadParams, bytesWritten int64) error {
	bytesLeftToWrite := u.objectSize - bytesWritten

	if bytesLeftToWrite < int64(u.readerLength) {
		// set forward so we only copy remaining bytes
		_, err := u.contentsReader.Seek(-int64(bytesLeftToWrite), io.SeekEnd)
		return err
	}
	_, err := u.contentsReader.Seek(0, io.SeekStart)

	return err
}
