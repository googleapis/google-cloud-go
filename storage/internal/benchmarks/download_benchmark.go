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

func downloadBenchmark(ctx context.Context, o *storage.ObjectHandle, objectSize int64) (elapsedTime time.Duration, err error) {
	var bytesRead int64
	var objectReader *storage.Reader
	start := time.Now()

	defer func() {
		if err == nil {
			err = objectReader.Close()
		}

		elapsedTime = time.Since(start)

		readAllBytes := bytesRead == objectSize
		if err == nil && !readAllBytes {
			err = fmt.Errorf("Did not read all bytes. Read: %d, Expected to read: %d", bytesRead, objectSize)
		}
	}()

	objectReader, err = o.NewReader(ctx)
	if err != nil {
		return elapsedTime, fmt.Errorf("NewReader on object %s/%s : %v", o.BucketName(), o.ObjectName(), err)
	}

	var read int64
	if read, err = io.Copy(io.Discard, objectReader); err != nil {
		return elapsedTime, fmt.Errorf("io.Copy: %v", err)
	}
	bytesRead += read

	return elapsedTime, err
}
