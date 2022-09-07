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
)

func download(ctx context.Context, opts benchmarkOptions, results *w1r3, idx int) (err error) {
	result := results.readResults[idx]
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
		return fmt.Errorf("os.Create: %w", err)
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
		return fmt.Errorf("Object(%q).NewReader: %w", o.ObjectName(), err)
	}
	defer func() {
		rerr := objectReader.Close()
		if rerr == nil {
			err = rerr
		}
	}()

	written, err := io.Copy(f, objectReader)
	if err != nil {
		return fmt.Errorf("io.Copy: %w", err)
	}

	if written != result.objectSize {
		return fmt.Errorf("did not read all bytes; read: %d, expected to read: %d", written, result.objectSize)
	}
	return nil
}
