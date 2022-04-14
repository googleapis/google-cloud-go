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
	"time"

	"cloud.google.com/go/storage"
)

type downloadOpts struct {
	o          *storage.ObjectHandle
	objectSize int64
}

func downloadBenchmark(ctx context.Context, dopts downloadOpts) (elapsedTime time.Duration, rerr error) {
	start := time.Now()
	defer func() {
		elapsedTime = time.Since(start)
	}()

	f, err := os.Create(dopts.o.ObjectName())
	if err != nil {
		rerr = fmt.Errorf("os.Create: %v", err)
		return
	}
	defer func() {
		closeErr := f.Close()
		removeErr := os.Remove(dopts.o.ObjectName())
		// if we don't have another error to return, return error for closing file
		// if that error is also nil, return removeErr
		if rerr == nil {
			rerr = removeErr
			if closeErr != nil {
				rerr = closeErr
			}
		}
	}()

	objectReader, err := dopts.o.NewReader(ctx)
	if err != nil {
		rerr = fmt.Errorf("Object(%q).NewReader: %v", dopts.o.ObjectName(), err)
		return
	}
	defer func() {
		err := objectReader.Close()
		if rerr == nil {
			rerr = err
		}
	}()

	written, err := io.Copy(f, objectReader)
	if err != nil {
		rerr = fmt.Errorf("io.Copy: %v", err)
		return
	}

	if written != dopts.objectSize {
		rerr = fmt.Errorf("did not read all bytes; read: %d, expected to read: %d", written, dopts.objectSize)
		return
	}

	return
}
