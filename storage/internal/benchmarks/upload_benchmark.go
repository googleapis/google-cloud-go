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
	"crypto/md5"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"os"
	"time"

	"cloud.google.com/go/storage"
)

type uploadParams struct {
	o         *storage.ObjectHandle
	fileName  string
	chunkSize int
	md5       bool
	crc32c    bool
}

func uploadBenchmark(ctx context.Context, u uploadParams) (elapsedTime time.Duration, rerr error) {
	start := time.Now()
	defer func() {
		elapsedTime = time.Since(start)
	}()

	o := u.o.If(storage.Conditions{DoesNotExist: true})
	objectWriter := o.NewWriter(ctx)
	objectWriter.ChunkSize = u.chunkSize

	defer func() {
		err := objectWriter.Close()
		if rerr == nil {
			rerr = err
		}
	}()

	f, err := os.Open(u.fileName)
	if err != nil {
		return elapsedTime, fmt.Errorf("os.Open: %v", err)
	}
	defer f.Close()

	if u.crc32c || u.md5 {
		w := newHashWriter(u.md5, u.crc32c)
		if _, err = io.Copy(w, f); err != nil {
			return elapsedTime, fmt.Errorf("io.Copy hash: %v", err)
		}
		w.applyToWriter(objectWriter)
		f.Seek(0, 0)
	}

	if _, err = io.Copy(objectWriter, f); err != nil {
		return elapsedTime, fmt.Errorf("io.Copy: %v", err)
	}

	return
}

// hashWriter writes to md5 and crc32c hashes as applicable
// we can then get the checksum of these hashes to apply to a storage.Writer and
// make sure that GCS receives the same bytes as written to hashWriter
type hashWriter struct {
	md5Hash hash.Hash
	crcHash hash.Hash32
	md5     bool
	crc     bool
}

func (u *hashWriter) applyToWriter(w *storage.Writer) {
	if u.md5 {
		w.MD5 = u.md5Hash.Sum(nil)
	}
	if u.crc {
		w.SendCRC32C = true
		w.CRC32C = u.crcHash.Sum32()
	}
}

func (u *hashWriter) Write(p []byte) (n int, err error) {
	if u.md5 {
		n, err = u.md5Hash.Write(p)
	}
	if u.crc {
		n, err = u.crcHash.Write(p)
	}
	fmt.Println(n)
	return n, err
}

func newHashWriter(hashMD5, hashCRC bool) *hashWriter {
	uw := &hashWriter{}

	if hashMD5 {
		uw.md5 = true
		uw.md5Hash = md5.New()
	}
	if hashCRC {
		uw.crc = true
		uw.crcHash = crc32.New(crc32.MakeTable(crc32.Castagnoli))
	}

	return uw
}
