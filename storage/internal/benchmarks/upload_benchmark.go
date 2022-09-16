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

type uploadOpts struct {
	client              *storage.Client
	params              randomizedParams
	bucket              string
	object              string
	useDefaultChunkSize bool
}

func uploadBenchmark(ctx context.Context, uopts uploadOpts) (elapsedTime time.Duration, rerr error) {
	start := time.Now()
	defer func() { elapsedTime = time.Since(start) }()

	o := uopts.client.Bucket(uopts.bucket).Object(uopts.object)

	objectWriter := o.If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)

	if !uopts.useDefaultChunkSize {
		objectWriter.ChunkSize = int(uopts.params.chunkSize)
	}

	f, err := os.Open(uopts.object)
	if err != nil {
		return elapsedTime, fmt.Errorf("os.Open: %w", err)
	}
	defer f.Close()

	mw, md5Hash, crc32cHash := generateUploadWriter(objectWriter, uopts.params.md5Enabled, uopts.params.crc32cEnabled)

	if _, err = io.Copy(mw, f); err != nil {
		return elapsedTime, fmt.Errorf("io.Copy: %w", err)
	}

	err = objectWriter.Close()
	if err != nil {
		return elapsedTime, fmt.Errorf("writer.Close: %w", err)
	}

	if uopts.params.crc32cEnabled || uopts.params.md5Enabled {
		attrs, aerr := o.Attrs(ctx)
		if aerr != nil {
			return elapsedTime, fmt.Errorf("get attrs on object %s/%s : %w", o.BucketName(), o.ObjectName(), aerr)
		}

		rerr = verifyHash(md5Hash, crc32cHash, attrs.MD5, attrs.CRC32C)
	}

	return
}

// generateUploadWriter selects the appropriate writer for an upload benchmark.
// If one of hashMD5 or hashCRC is true, it returns a MultiWriter that writes to
// the provided writer, as well as to the respective hash (also returned).
// If neither is true, generateUploadWriter returns the provided writer and nil hashes.
func generateUploadWriter(w io.Writer, hashMD5, hashCRC bool) (mw io.Writer, md5Hash hash.Hash, crc32cHash hash.Hash32) {
	if hashMD5 {
		md5Hash = md5.New()
		mw = io.MultiWriter(w, md5Hash)
		return
	}
	if hashCRC {
		crc32cHash = crc32.New(crc32.MakeTable(crc32.Castagnoli))
		mw = io.MultiWriter(w, crc32cHash)
		return
	}

	return w, nil, nil
}

// verify checks the hashs against the given md5 and crc32c checksums. If a hash
// is nil, the check is skipped.
func verifyHash(md5Hash hash.Hash, crc32cHash hash.Hash32, expectedMD5Hash []byte, expectedCRCChecksum uint32) (err error) {
	if md5Hash != nil {
		if got := md5Hash.Sum(nil); !bytes.Equal(got, expectedMD5Hash) {
			return fmt.Errorf("md5 checksum does not match; \n\tgot: \t\t%d, \n\texpected: \t%d", got, expectedMD5Hash)
		}
	}
	if crc32cHash != nil {
		if got := crc32cHash.Sum32(); got != expectedCRCChecksum {
			return fmt.Errorf("crc checksum does not match; got: %d, expected: %d", got, expectedCRCChecksum)
		}
	}
	return nil
}
