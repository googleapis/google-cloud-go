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
