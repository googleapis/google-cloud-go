// Package rapid implements the Rapid workload SLIs from
// go/gcs-sdk:continuous-benchmarks-for-acv2 (Open, Read50x2x4, Random8K) plus
// Workloads 4, 5, and 6 from the GCS SDK Consolidation Worklog:
//   - Workload 4: Read 16 MiB chunks via BidiRead on 48 parallel threads (5m)
//   - Workload 5: Write 200 MiB chunks via BidiWrite on 48 parallel threads (5m)
//   - Workload 6: ListBucket and ListObject for 100 items
package rapid

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Session is an open object that can serve many range reads.
type Session interface {
	Size() int64
	// ReadAt fills p from offset off. Must be safe for concurrent use.
	ReadAt(ctx context.Context, p []byte, off int64) (int, error)
	Close(ctx context.Context) error
}

// Opener opens a Session; the open latency is the "Open" SLI.
type Opener interface {
	Open(ctx context.Context, bucket, object string) (Session, error)
}

// WorkloadExtras provides optional hooks for Workloads 5 (BidiWrite) and 6 (List 100).
type WorkloadExtras interface {
	WriteObject(ctx context.Context, bucket, object string, data []byte) error
	ListObjectsN(ctx context.Context, bucket, prefix string, limit int) (int, error)
	ListBucketsN(ctx context.Context, project string, limit int) (int, error)
}

type stats struct {
	mu   sync.Mutex
	durs []time.Duration
}

func (s *stats) add(d time.Duration) {
	s.mu.Lock()
	s.durs = append(s.durs, d)
	s.mu.Unlock()
}
func (s *stats) pct(p float64) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.durs) == 0 {
		return 0
	}
	c := append([]time.Duration(nil), s.durs...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	return c[int(float64(len(c)-1)*p)]
}
func (s *stats) mean() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	var t time.Duration
	for _, d := range s.durs {
		t += d
	}
	if len(s.durs) == 0 {
		return 0
	}
	return t / time.Duration(len(s.durs))
}

// Resource hooks (same contract as bench2).
var (
	CPUSeconds   func() float64
	ResetPeakRSS func()
	PeakRSSMiB   func() float64
)

// Run executes the SLIs. Objects: <prefix>/200MiB and <prefix>/100MiB must exist.
func Run(name string, op Opener) {
	bucket := flag.String("bucket", "cpriti-thickthin-poc-use1", "bucket")
	project := flag.String("project", "", "GCP project ID for ListBuckets (optional)")
	prefix := flag.String("prefix", "rapid", "object prefix")
	listPrefix := flag.String("list-prefix", "", "prefix for ListObjects 100-item SLI (default: <prefix>/list/)")
	iters := flag.Int("iters", 10, "iterations per SLI")
	randomReads := flag.Int("random-reads", 200, "number of 8KiB random reads per iteration")
	sustainedThreads := flag.Int("sustained-threads", 48, "parallel threads for Workloads 4 & 5")
	sustainedDur := flag.Duration("sustained-dur", 0, "duration for Workloads 4 & 5 (e.g. 5m; 0 skips sustained runs)")
	onlySLI := flag.String("sli", "", "if set, run only matching SLI (open|read50x2x4|random8k|bidiread48|bidiwrite48|list100)")
	flag.Parse()
	if *listPrefix == "" {
		*listPrefix = *prefix + "/list/"
	}
	ctx := context.Background()
	const MiB = 1 << 20

	shouldRun := func(tag string) bool {
		if *onlySLI == "" {
			return true
		}
		for _, part := range strings.Split(*onlySLI, ",") {
			if strings.TrimSpace(part) == tag {
				return true
			}
		}
		return false
	}

	fmt.Printf("backend=%s bucket=%s vcpus=%d\n", name, *bucket, runtime.NumCPU())
	fmt.Printf("%-14s %-24s %9s %9s %9s %9s %9s %10s %10s %9s %7s %9s\n",
		"backend", "sli", "p50", "p90", "p99", "max", "mean", "MiB/s", "cpu s/GiB", "cpu ms/op", "cpu %", "peak MiB")

	report := func(sli string, s *stats, bytesMoved int64, wall time.Duration, cpu float64, peak float64) {
		n := max(1, len(s.durs))
		perOp := cpu * 1000 / float64(n)
		perGiB := 0.0
		mibs := 0.0
		if bytesMoved > 0 {
			perGiB = cpu / (float64(bytesMoved) / (1 << 30))
			if wall > 0 {
				mibs = (float64(bytesMoved) / float64(MiB)) / wall.Seconds()
			}
		}
		cpuPct := 0.0
		if wall > 0 {
			cpuPct = (cpu / (wall.Seconds() * float64(runtime.NumCPU()))) * 100.0
		}
		fmt.Printf("%-14s %-24s %9v %9v %9v %9v %9v %10.1f %10.2f %9.2f %6.1f%% %9.0f\n",
			name, sli,
			s.pct(.5).Round(time.Microsecond), s.pct(.9).Round(time.Microsecond),
			s.pct(.99).Round(time.Microsecond), s.pct(1).Round(time.Microsecond),
			s.mean().Round(time.Microsecond),
			mibs, perGiB, perOp, cpuPct, peak)
	}
	measure := func(f func()) (wall time.Duration, cpu float64, peak float64) {
		if ResetPeakRSS != nil {
			ResetPeakRSS()
		}
		c0 := 0.0
		if CPUSeconds != nil {
			c0 = CPUSeconds()
		}
		t0 := time.Now()
		f()
		wall = time.Since(t0)
		if CPUSeconds != nil {
			cpu = CPUSeconds() - c0
		}
		if PeakRSSMiB != nil {
			peak = PeakRSSMiB()
		}
		return
	}

	// ---- Workload 1: Open ----
	if shouldRun("open") {
		s := &stats{}
		wall, cpu, peak := measure(func() {
			for i := 0; i < *iters; i++ {
				t := time.Now()
				sess, err := op.Open(ctx, *bucket, *prefix+"/200MiB")
				if err != nil {
					fmt.Fprintln(os.Stderr, "open:", err)
					return
				}
				s.add(time.Since(t))
				sess.Close(ctx)
			}
		})
		report("open", s, 0, wall, cpu, peak)
	}

	// ---- Workload 2: Read50x2x4 (50 ranges of 2 MiB, 4 MiB stride, from 200 MiB object) ----
	if shouldRun("read50x2x4") {
		for _, mode := range []string{"serial", "concurrent"} {
			s := &stats{}
			var bytesMoved int64
			wall, cpu, peak := measure(func() {
				sess, err := op.Open(ctx, *bucket, *prefix+"/200MiB")
				if err != nil {
					fmt.Fprintln(os.Stderr, "open:", err)
					return
				}
				defer sess.Close(ctx)
				bufs := make([][]byte, 50)
				for i := range bufs {
					bufs[i] = make([]byte, 2*MiB)
				}
				for it := 0; it < *iters; it++ {
					t := time.Now()
					if mode == "serial" {
						for i := 0; i < 50; i++ {
							if _, err := sess.ReadAt(ctx, bufs[i], int64(i)*4*MiB); err != nil {
								fmt.Fprintln(os.Stderr, "read:", err)
								return
							}
						}
					} else {
						var wg sync.WaitGroup
						errs := make(chan error, 50)
						for i := 0; i < 50; i++ {
							wg.Add(1)
							go func(i int) {
								defer wg.Done()
								if _, err := sess.ReadAt(ctx, bufs[i], int64(i)*4*MiB); err != nil {
									errs <- err
								}
							}(i)
						}
						wg.Wait()
						select {
						case err := <-errs:
							fmt.Fprintln(os.Stderr, "read:", err)
							return
						default:
						}
					}
					s.add(time.Since(t))
					bytesMoved += 100 * MiB
				}
			})
			report("read50x2x4/"+mode, s, bytesMoved, wall, cpu, peak)
		}
	}

	// ---- Workload 3: Random8K (random 8 KiB reads from open 100 MiB object) ----
	if shouldRun("random8k") {
		s := &stats{}
		var bytesMoved int64
		rng := rand.New(rand.NewSource(42))
		wall, cpu, peak := measure(func() {
			sess, err := op.Open(ctx, *bucket, *prefix+"/100MiB")
			if err != nil {
				fmt.Fprintln(os.Stderr, "open:", err)
				return
			}
			defer sess.Close(ctx)
			buf := make([]byte, 8<<10)
			for i := 0; i < *iters**randomReads; i++ {
				off := rng.Int63n(sess.Size() - int64(len(buf)))
				t := time.Now()
				if _, err := sess.ReadAt(ctx, buf, off); err != nil {
					fmt.Fprintln(os.Stderr, "read:", err)
					return
				}
				s.add(time.Since(t))
				bytesMoved += int64(len(buf))
			}
		})
		report("random8k", s, bytesMoved, wall, cpu, peak)
	}

	// ---- Workload 4: Read 16 MiB chunks of 200 MiB via BidiRead on 48 parallel threads ----
	if *sustainedDur > 0 && shouldRun("bidiread48") {
		chunkStats := &stats{}
		objStats := &stats{}
		var bytesMoved atomic.Int64
		wall, cpu, peak := measure(func() {
			deadline := time.Now().Add(*sustainedDur)
			var wg sync.WaitGroup
			const chunk = int64(16 * MiB)
			for w := 0; w < *sustainedThreads; w++ {
				wg.Add(1)
				go func(w int) {
					defer wg.Done()
					// Each worker opens its own BidiReadObject session so 48 streams spread
					// across the gRPC connection pool to saturate the 200 Gbps NIC.
					sess, err := op.Open(ctx, *bucket, *prefix+"/200MiB")
					if err != nil {
						fmt.Fprintln(os.Stderr, "bidiread48 open:", err)
						return
					}
					defer sess.Close(ctx)
					objSize := sess.Size()
					buf := make([]byte, chunk)
					for time.Now().Before(deadline) {
						tObj := time.Now()
						for off := int64(0); off < objSize && time.Now().Before(deadline); off += chunk {
							want := min(chunk, objSize-off)
							tChunk := time.Now()
							n, err := sess.ReadAt(ctx, buf[:want], off)
							if err != nil {
								fmt.Fprintln(os.Stderr, "bidiread48 read:", err)
								return
							}
							if want == chunk {
								chunkStats.add(time.Since(tChunk))
							}
							bytesMoved.Add(int64(n))
						}
						if time.Now().Before(deadline) {
							objStats.add(time.Since(tObj))
						}
					}
				}(w)
			}
			wg.Wait()
		})
		report(fmt.Sprintf("bidiread-16m-%dt", *sustainedThreads), chunkStats, bytesMoved.Load(), wall, cpu, peak)
		if len(objStats.durs) > 0 {
			report(fmt.Sprintf("bidiread-200m-%dt", *sustainedThreads), objStats, bytesMoved.Load(), wall, cpu, peak)
		}
	}

	ext, hasExt := op.(WorkloadExtras)

	// ---- Workload 5: Write 200 MiB chunks via BidiWrite on 48 parallel threads ----
	if *sustainedDur > 0 && hasExt && shouldRun("bidiwrite48") {
		s := &stats{}
		var bytesMoved atomic.Int64
		const chunk = 200 * MiB
		payload := make([]byte, chunk)
		for i := range payload {
			payload[i] = byte(i * 13)
		}
		wall, cpu, peak := measure(func() {
			deadline := time.Now().Add(*sustainedDur)
			var wg sync.WaitGroup
			for w := 0; w < *sustainedThreads; w++ {
				wg.Add(1)
				go func(w int) {
					defer wg.Done()
					seq := 0
					for time.Now().Before(deadline) {
						obj := fmt.Sprintf("%s/bidiwrite48/w%d-%d", *prefix, w, seq%4)
						seq++
						t := time.Now()
						if err := ext.WriteObject(ctx, *bucket, obj, payload); err != nil {
							fmt.Fprintln(os.Stderr, "bidiwrite48:", err)
							return
						}
						s.add(time.Since(t))
						bytesMoved.Add(chunk)
					}
				}(w)
			}
			wg.Wait()
		})
		report(fmt.Sprintf("bidiwrite-200m-%dt", *sustainedThreads), s, bytesMoved.Load(), wall, cpu, peak)
	}

	// ---- Workload 6: ListObject (100 items) and ListBucket (up to 100 items) ----
	if hasExt && shouldRun("list100") {
		{
			s := &stats{}
			wall, cpu, peak := measure(func() {
				for i := 0; i < *iters; i++ {
					t := time.Now()
					if _, err := ext.ListObjectsN(ctx, *bucket, *listPrefix, 100); err != nil {
						fmt.Fprintln(os.Stderr, "list-objects-100:", err)
						return
					}
					s.add(time.Since(t))
				}
			})
			report("list-objects-100", s, 0, wall, cpu, peak)
		}
		if *project != "" {
			s := &stats{}
			wall, cpu, peak := measure(func() {
				for i := 0; i < *iters; i++ {
					t := time.Now()
					if _, err := ext.ListBucketsN(ctx, *project, 100); err != nil {
						fmt.Fprintln(os.Stderr, "list-buckets-100:", err)
						return
					}
					s.add(time.Since(t))
				}
			})
			report("list-buckets-100", s, 0, wall, cpu, peak)
		}
	}
}
