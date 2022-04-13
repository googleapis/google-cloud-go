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
	"bytes"
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

type downloadParams struct {
	o          *storage.ObjectHandle
	objectSize int64
	md5        bool
	crc32c     bool
}

func downloadBenchmark(ctx context.Context, d downloadParams) (elapsedTime time.Duration, rerr error) {
	start := time.Now()
	defer func() {
		elapsedTime = time.Since(start)
	}()

	f, err := os.Create(d.o.ObjectName())
	if err != nil {
		rerr = fmt.Errorf("os.Create: %v", err)
		return
	}
	defer func() {
		closeErr := f.Close()
		removeErr := os.Remove(d.o.ObjectName())
		// if we don't have another error to return, return error for closing file
		// if that error is also nil, return removeErr
		if rerr == nil {
			rerr = removeErr
			if closeErr != nil {
				rerr = closeErr
			}
		}
	}()

	objectReader, err := d.o.NewReader(ctx)
	if err != nil {
		rerr = fmt.Errorf("Object(%q).NewReader: %v", d.o.ObjectName(), err)
		return
	}
	defer func() {
		err := objectReader.Close()
		if rerr == nil {
			rerr = err
		}
	}()

	w := newDowloadWriter(f, d.md5, d.crc32c)

	written, err := io.Copy(w, objectReader)
	if err != nil {
		rerr = fmt.Errorf("io.Copy: %v", err)
		return
	}

	if written != d.objectSize {
		rerr = fmt.Errorf("did not read all bytes; read: %d, expected to read: %d", written, d.objectSize)
		return
	}

	if d.md5 || d.crc32c {
		attrs, aerr := d.o.Attrs(ctx)
		if aerr != nil {
			return elapsedTime, fmt.Errorf("get attrs on object %s/%s : %v", d.o.BucketName(), d.o.ObjectName(), err)
		}

		expectedCRCChecksum := attrs.CRC32C
		expectedMD5Hash := attrs.MD5
		rerr = w.verify(expectedMD5Hash, expectedCRCChecksum)
	}
	return
}

// downloadWriter writes to given writer as well as md5 and crc32c hashes as applicable
// we can then get the checksum of these hashes to verify the written bytes
type downloadWriter struct {
	md5Hash hash.Hash
	crcHash hash.Hash32
	w       io.Writer
	md5     bool
	crc     bool
}

func (c *downloadWriter) verify(expectedMD5Hash []byte, expectedCRCChecksum uint32) (err error) {
	if c.md5 {
		if got := c.md5Hash.Sum(nil); !bytes.Equal(got, expectedMD5Hash) {
			return fmt.Errorf("md5 checksum does not match; \n\tgot: \t\t%d, \n\texpected: \t%d", got, expectedMD5Hash)
		}
	}
	if c.crc {
		if got := c.crcHash.Sum32(); got != expectedCRCChecksum {
			return fmt.Errorf("crc checksum does not match; got: %d, expected: %d", got, expectedCRCChecksum)
		}
	}
	return nil
}

func (c *downloadWriter) Write(p []byte) (n int, err error) {
	n, err = c.w.Write(p)
	if c.md5 {
		c.md5Hash.Write(p)
	}
	if c.crc {
		c.crcHash.Write(p)
	}
	return n, err
}

func newDowloadWriter(w io.Writer, hashMD5, hashCRC bool) *downloadWriter {
	cw := &downloadWriter{w: w}

	if hashMD5 {
		cw.md5 = true
		cw.md5Hash = md5.New()
	}
	if hashCRC {
		cw.crc = true
		cw.crcHash = crc32.New(crc32.MakeTable(crc32.Castagnoli))
	}

	return cw
}
