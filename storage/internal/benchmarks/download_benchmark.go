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
	"time"

	"cloud.google.com/go/storage"
)

type downloadParams struct {
	o             *storage.ObjectHandle
	objectSize    int64
	appBufferSize int
	md5           bool
	crc32c        bool
}

func downloadBenchmark(ctx context.Context, d downloadParams) (elapsedTime time.Duration, err error) {
	bytesLeftToRead := d.objectSize
	var objectReader *storage.Reader
	start := time.Now()

	defer func() {
		if err == nil {
			err = objectReader.Close()
		}

		elapsedTime = time.Since(start)

		readAllBytes := bytesLeftToRead == 0
		if err == nil && !readAllBytes {
			err = fmt.Errorf("did not read all bytes; read: %d, expected to read: %d", d.objectSize-bytesLeftToRead, d.objectSize)
		}
	}()

	objectReader, err = d.o.NewReader(ctx)
	if err != nil {
		return elapsedTime, fmt.Errorf("NewReader on object %s/%s : %v", d.o.BucketName(), d.o.ObjectName(), err)
	}

	appBuffer := bytes.NewBuffer(make([]byte, 0, d.appBufferSize))
	w := newDowloadWriter(appBuffer, d.md5, d.crc32c)

	for bytesLeftToRead > 0 {
		shouldRead := int64(d.appBufferSize)
		if int64(d.appBufferSize) > bytesLeftToRead {
			shouldRead = bytesLeftToRead
		}

		var read int64
		if read, err = io.CopyN(w, objectReader, shouldRead); err != nil {
			return elapsedTime, fmt.Errorf("io.Copy: %v", err)
		}

		bytesLeftToRead -= read

		// Empty the buffer for re-use
		appBuffer.Reset()
	}

	if d.md5 || d.crc32c {
		attrs, aerr := d.o.Attrs(ctx)
		if aerr != nil {
			return elapsedTime, fmt.Errorf("get attrs on object %s/%s : %v", d.o.BucketName(), d.o.ObjectName(), err)
		}

		expectedCRCChecksum := attrs.CRC32C
		expectedMD5Hash := attrs.MD5
		err = w.verify(expectedMD5Hash, expectedCRCChecksum)
	}

	return elapsedTime, err
}

// downloadWriter takes care of verifying crc32c and md5 hashes
type downloadWriter struct {
	md5Hash hash.Hash
	crcHash hash.Hash32
	w       io.Writer
	md5     bool
	crc     bool
}

func (c *downloadWriter) verify(expectedMD5Hash []byte, expectedCRCChecksum uint32) (err error) {
	if c.md5 {
		if got := c.md5Hash.Sum([]byte{}); !bytes.Equal(got, expectedMD5Hash) {
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
