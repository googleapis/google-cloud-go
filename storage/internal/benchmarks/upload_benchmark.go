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

type uploadOpts struct {
	o         *storage.ObjectHandle
	fileName  string
	chunkSize int
	md5       bool
	crc32c    bool
}

func uploadBenchmark(ctx context.Context, uopts uploadOpts) (elapsedTime time.Duration, rerr error) {
	start := time.Now()
	defer func() {
		elapsedTime = time.Since(start)
	}()

	o := uopts.o.If(storage.Conditions{DoesNotExist: true})
	objectWriter := o.NewWriter(ctx)
	objectWriter.ChunkSize = uopts.chunkSize

	f, err := os.Open(uopts.fileName)
	if err != nil {
		return elapsedTime, fmt.Errorf("os.Open: %v", err)
	}
	defer f.Close()

	w := newHashWriter(objectWriter, uopts.md5, uopts.crc32c)

	if _, err = io.Copy(w, f); err != nil {
		return elapsedTime, fmt.Errorf("io.Copy: %v", err)
	}

	err = objectWriter.Close()
	if err != nil {
		return elapsedTime, fmt.Errorf("writer.Close: %v", err)
	}

	if uopts.crc32c || uopts.md5 {
		attrs, aerr := uopts.o.Attrs(ctx)
		if aerr != nil {
			return elapsedTime, fmt.Errorf("get attrs on object %s/%s : %v", uopts.o.BucketName(), uopts.o.ObjectName(), aerr)
		}
		rerr = w.verify(attrs.MD5, attrs.CRC32C)
	}

	return
}
