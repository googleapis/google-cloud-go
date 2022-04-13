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
	"io"
	"os"
	"time"

	"cloud.google.com/go/storage"
)

type downloadOpts struct {
	o          *storage.ObjectHandle
	objectSize int64
	md5        bool
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

	w := newDowloadWriter(f, dopts.md5)

	written, err := io.Copy(w, objectReader)
	if err != nil {
		rerr = fmt.Errorf("io.Copy: %v", err)
		return
	}

	if written != dopts.objectSize {
		rerr = fmt.Errorf("did not read all bytes; read: %d, expected to read: %d", written, dopts.objectSize)
		return
	}

	if dopts.md5 {
		attrs, aerr := dopts.o.Attrs(ctx)
		if aerr != nil {
			return elapsedTime, fmt.Errorf("get attrs on object %s/%s : %v", dopts.o.BucketName(), dopts.o.ObjectName(), err)
		}
		rerr = w.verify(attrs.MD5)
	}
	return
}

// downloadWriter writes to given writer as well as md5 hash if applicable
// we can then get the checksum of the hash to verify the written bytes
type downloadWriter struct {
	md5Hash hash.Hash
	w       io.Writer
	md5     bool
}

func (c *downloadWriter) verify(expectedMD5Hash []byte) (err error) {
	if c.md5 {
		if got := c.md5Hash.Sum(nil); !bytes.Equal(got, expectedMD5Hash) {
			return fmt.Errorf("md5 checksum does not match; \n\tgot: \t\t%d, \n\texpected: \t%d", got, expectedMD5Hash)
		}
	}
	return nil
}

func (c *downloadWriter) Write(p []byte) (n int, err error) {
	n, err = c.w.Write(p)
	if c.md5 {
		c.md5Hash.Write(p)
	}
	return n, err
}

func newDowloadWriter(w io.Writer, hashMD5 bool) *downloadWriter {
	cw := &downloadWriter{w: w}

	if hashMD5 {
		cw.md5 = true
		cw.md5Hash = md5.New()
	}
	return cw
}
