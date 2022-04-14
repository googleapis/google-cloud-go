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
	"crypto/md5"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
)

// hashWriter writes to the given writer and to md5 and crc32c hashes, as applicable.
// We can then get the checksum of the hash to verify the written bytes
type hashWriter struct {
	md5Hash hash.Hash
	crcHash hash.Hash32
	w       io.Writer
	md5     bool
	crc32c  bool
}

// verify checks the bytes written to hashWriter against the given md5 and crc32c
// checksums, as applicable.
func (hw *hashWriter) verify(expectedMD5Hash []byte, expectedCRCChecksum uint32) (err error) {
	if hw.md5 {
		if got := hw.md5Hash.Sum(nil); !bytes.Equal(got, expectedMD5Hash) {
			return fmt.Errorf("md5 checksum does not match; \n\tgot: \t\t%d, \n\texpected: \t%d", got, expectedMD5Hash)
		}
	}
	if hw.crc32c {
		if got := hw.crcHash.Sum32(); got != expectedCRCChecksum {
			return fmt.Errorf("crc checksum does not match; got: %d, expected: %d", got, expectedCRCChecksum)
		}
	}
	return nil
}

func (hw *hashWriter) Write(p []byte) (n int, err error) {
	n, err = hw.w.Write(p)
	if err != nil {
		return
	}
	if hw.md5 {
		nm, mErr := hw.md5Hash.Write(p)
		if mErr != nil {
			return nm, mErr
		}
	}
	if hw.crc32c {
		nc, cErr := hw.crcHash.Write(p)
		if cErr != nil {
			return nc, cErr
		}
	}

	return
}

func newHashWriter(w io.Writer, hashMD5, hashCRC bool) *hashWriter {
	uw := &hashWriter{w: w}

	if hashMD5 {
		uw.md5 = true
		uw.md5Hash = md5.New()
	}
	if hashCRC {
		uw.crc32c = true
		uw.crcHash = crc32.New(crc32.MakeTable(crc32.Castagnoli))
	}

	return uw
}
