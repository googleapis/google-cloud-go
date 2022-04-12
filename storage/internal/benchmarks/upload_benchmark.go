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
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"strings"
	"time"

	"cloud.google.com/go/storage"
)

type uploadParams struct {
	o          *storage.ObjectHandle
	contents   string
	objectSize int64
	chunkSize  int
	md5        bool
	crc32c     bool
}

func uploadBenchmark(ctx context.Context, u uploadParams) (time.Duration, error) {
	elapsedTime := time.Duration(0)
	start := time.Now()

	u.o.If(storage.Conditions{
		DoesNotExist: true,
	})

	objectWriter := u.o.NewWriter(ctx)
	objectWriter.ChunkSize = u.chunkSize

	if u.crc32c {
		objectWriter.SendCRC32C = true
	}

	writer := newUploadWriter(objectWriter, u.md5, u.crc32c)

	contentsReader := strings.NewReader(u.contents)

	var written int64
	for written < u.objectSize {
		// stop timer here
		elapsedTime += time.Since(start)
		rewindReader(contentsReader, len(u.contents), u.objectSize-written)

		// restart timer
		start = time.Now()

		w, err := io.Copy(writer, contentsReader)

		if err != nil {
			objectWriter.Close()
			elapsedTime += time.Since(start)
			return elapsedTime, fmt.Errorf("io.Copy after %d bytes: %v", written, err)
		}

		written += w
	}

	if u.crc32c {
		sum := writer.crcHash.Sum32()
		objectWriter.CRC32C = sum

		bytes := make([]byte, 4)
		binary.BigEndian.PutUint32(bytes, sum)
		s := base64.StdEncoding.EncodeToString(bytes)

		fmt.Printf("sum: %s\n", s)
		w, err := io.Copy(objectWriter, strings.NewReader(""))
		if err != nil {
			objectWriter.Close()
			elapsedTime += time.Since(start)
			return elapsedTime, fmt.Errorf("io.Copy after %d bytes: %v", written, err)
		}

		written += w
	}

	if err := objectWriter.Close(); err != nil {
		elapsedTime += time.Since(start)
		return elapsedTime, fmt.Errorf("objsize %d Writer.Close: %v", u.objectSize, err)
	}
	elapsedTime += time.Since(start)
	return elapsedTime, nil
}

// rewinds the ReadSeeker so we can copy more bytes
func rewindReader(r io.ReadSeeker, readerLen int, bytesLeftToWrite int64) error {
	if bytesLeftToWrite < int64(readerLen) {
		// set forward so we only copy remaining bytes
		_, err := r.Seek(-int64(bytesLeftToWrite), io.SeekEnd)
		return err
	}
	_, err := r.Seek(0, io.SeekStart)

	return err
}

// custom writer takes care of verifying crc32c and md5 hashes
type uploadWriter struct {
	md5Hash hash.Hash
	crcHash hash.Hash32
	w       *storage.Writer
	md5     bool
	crc     bool
}

// Uploads the wrong md5 hash on large objects and
// doesn't always send the last crc32 sum to the object
// not sure why
func (u *uploadWriter) Write(p []byte) (n int, err error) {
	if u.md5 {
		u.md5Hash.Write(p)
		u.w.MD5 = u.md5Hash.Sum([]byte{})
	}
	if u.crc {
		a, _ := u.crcHash.Write(p)
		fmt.Println("------")
		fmt.Println(a)
		sum := u.crcHash.Sum32()
		//objectWriter.CRC32C = sum

		bytes := make([]byte, 4)
		binary.BigEndian.PutUint32(bytes, sum)
		s := base64.StdEncoding.EncodeToString(bytes)
		fmt.Println(s)
		fmt.Println("------")
		u.w.CRC32C = sum
	}

	n, err = u.w.Write(p)
	return n, err
}

func newUploadWriter(w *storage.Writer, hashMD5, hashCRC bool) *uploadWriter {
	uw := &uploadWriter{w: w}

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
