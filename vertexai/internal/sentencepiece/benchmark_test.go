// Copyright 2024 Google LLC
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

package sentencepiece

import (
	"io/ioutil"
	"path/filepath"
	"runtime"
	"testing"
)

func BenchmarkEncoder(b *testing.B) {
	buf, err := ioutil.ReadFile(filepath.Join("test", "pg7193_english.txt"))
	if err != nil {
		b.Fatal(err)
	}
	sbuf := string(buf[:2000])

	enc := createEncoder(b)
	b.ResetTimer()
	total := 0

	for i := 0; i < b.N; i++ {
		toks := enc.Encode(sbuf)
		total += len(toks)
	}
	runtime.KeepAlive(total)

	b.ReportMetric(float64(total)/float64(b.Elapsed().Seconds()), "tokens/sec")
}
