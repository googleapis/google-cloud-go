// rapid_bench: Open / Read50x2x4 / Random8K SLIs.
//
//	rapid_bench native   -> cloud.google.com/go/storage gRPC client, MultiRangeDownloader (CloudPath; DirectPath disabled)
//	rapid_bench uds      -> udsthin OpenObject/ReadRange (Rust core BidiRead over gRPC CloudPath)
//	rapid_bench native-json -> baseline with plain NewRangeReader per range over JSON (no session)
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"poc/gothin/bench2"
	"runtime"
	"strconv"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"poc/gothin/rapid"
	"poc/gothin/udsthin"
)

// ---- native: MultiRangeDownloader ----

type nativeOpener struct {
	c      *storage.Client
	ctrl   *storage.Client
	append bool
}

type mrdSession struct {
	mrd  *storage.MultiRangeDownloader
	size int64
	mu   sync.Mutex // MRD.Add is goroutine-safe; keep for symmetry
}

func (n nativeOpener) Open(ctx context.Context, bucket, object string) (rapid.Session, error) {
	mrd, err := n.c.Bucket(bucket).Object(object).NewMultiRangeDownloader(ctx)
	if err != nil {
		return nil, err
	}
	return &mrdSession{mrd: mrd, size: mrd.Attrs.Size}, nil
}
func (n nativeOpener) WriteObject(ctx context.Context, bucket, object string, data []byte) error {
	w := n.c.Bucket(bucket).Object(object).NewWriter(ctx)
	if n.append {
		w.Append, w.FinalizeOnClose = true, true
	}
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}
func (n nativeOpener) ListObjectsN(ctx context.Context, bucket, prefix string, limit int) (int, error) {
	cli := n.ctrl
	if cli == nil {
		cli = n.c
	}
	it := cli.Bucket(bucket).Objects(ctx, &storage.Query{Prefix: prefix})
	cnt := 0
	for cnt < limit {
		_, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return cnt, err
		}
		cnt++
	}
	return cnt, nil
}
func (n nativeOpener) ListBucketsN(ctx context.Context, project string, limit int) (int, error) {
	cli := n.ctrl
	if cli == nil {
		cli = n.c
	}
	it := cli.Buckets(ctx, project)
	cnt := 0
	for cnt < limit {
		_, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return cnt, err
		}
		cnt++
	}
	return cnt, nil
}
func (s *mrdSession) Size() int64 { return s.size }
func (s *mrdSession) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	w := &sliceWriter{p: p}
	done := make(chan error, 1)
	s.mrd.Add(w, off, int64(len(p)), func(_ int64, _ int64, err error) { done <- err })
	select {
	case err := <-done:
		return w.n, err
	case <-ctx.Done():
		return w.n, ctx.Err()
	}
}
func (s *mrdSession) Close(ctx context.Context) error { return s.mrd.Close() }

type sliceWriter struct {
	p []byte
	n int
}

func (w *sliceWriter) Write(b []byte) (int, error) {
	k := copy(w.p[w.n:], b)
	w.n += k
	if k < len(b) {
		return k, io.ErrShortWrite
	}
	return k, nil
}

// ---- native JSON: one NewRangeReader per range (no session) ----

type jsonOpener struct{ c *storage.Client }
type jsonSession struct {
	o    *storage.ObjectHandle
	size int64
}

func (j jsonOpener) Open(ctx context.Context, bucket, object string) (rapid.Session, error) {
	o := j.c.Bucket(bucket).Object(object)
	a, err := o.Attrs(ctx)
	if err != nil {
		return nil, err
	}
	return &jsonSession{o: o, size: a.Size}, nil
}
func (s *jsonSession) Size() int64 { return s.size }
func (s *jsonSession) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	r, err := s.o.NewRangeReader(ctx, off, int64(len(p)))
	if err != nil {
		return 0, err
	}
	defer r.Close()
	return io.ReadFull(r, p)
}
func (s *jsonSession) Close(ctx context.Context) error { return nil }

func (j jsonOpener) WriteObject(ctx context.Context, bucket, object string, data []byte) error {
	w := j.c.Bucket(bucket).Object(object).NewWriter(ctx)
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}
func (j jsonOpener) ListObjectsN(ctx context.Context, bucket, prefix string, limit int) (int, error) {
	return (nativeOpener{c: j.c}).ListObjectsN(ctx, bucket, prefix, limit)
}
func (j jsonOpener) ListBucketsN(ctx context.Context, project string, limit int) (int, error) {
	return (nativeOpener{c: j.c}).ListBucketsN(ctx, project, limit)
}

// ---- uds ----

type udsOpener struct{ c *udsthin.Client }
type udsSession struct{ op *udsthin.Opened }

func (u udsOpener) Open(ctx context.Context, bucket, object string) (rapid.Session, error) {
	op, err := u.c.Bucket(bucket).Object(object).Open(ctx)
	if err != nil {
		return nil, err
	}
	return &udsSession{op: op}, nil
}
func (s *udsSession) Size() int64 { return s.op.Attrs.Size }
func (s *udsSession) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	return s.op.ReadAt(ctx, p, off)
}
func (s *udsSession) Close(ctx context.Context) error { return s.op.Close(ctx) }

func (u udsOpener) WriteObject(ctx context.Context, bucket, object string, data []byte) error {
	w := u.c.Bucket(bucket).Object(object).NewWriter(ctx)
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}
func (u udsOpener) ListObjectsN(ctx context.Context, bucket, prefix string, limit int) (int, error) {
	it := u.c.Bucket(bucket).Objects(ctx, &storage.Query{Prefix: prefix})
	cnt := 0
	for cnt < limit {
		_, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return cnt, err
		}
		cnt++
	}
	return cnt, nil
}
func (u udsOpener) ListBucketsN(ctx context.Context, project string, limit int) (int, error) {
	it := u.c.Buckets(ctx, project)
	cnt := 0
	for cnt < limit {
		_, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return cnt, err
		}
		cnt++
	}
	return cnt, nil
}

// ---- resources ----

func cpuSelf() float64 { return bench2.CPUSeconds() }
func statusKB(key string) float64 {
	b, _ := os.ReadFile("/proc/self/status")
	for _, l := range bytes.Split(b, []byte("\n")) {
		if bytes.HasPrefix(l, []byte(key+":")) {
			var v float64
			fmt.Sscanf(string(bytes.TrimSpace(l[len(key)+1:])), "%f", &v)
			return v
		}
	}
	return 0
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: rapid_bench native|native-json|uds [flags]")
		os.Exit(2)
	}
	mode := os.Args[1]
	os.Args = append(os.Args[:1], os.Args[2:]...)
	ctx := context.Background()

	rapid.CPUSeconds = cpuSelf
	rapid.ResetPeakRSS = func() { os.WriteFile("/proc/self/clear_refs", []byte("5"), 0) }
	rapid.PeakRSSMiB = func() float64 { return statusKB("VmHWM") / 1024 }

	switch mode {
	case "prep":
		// Create <prefix>/200MiB, <prefix>/100MiB, and ensure >=100 objects exist for list-objects-100.
		// Zonal buckets reject JSON resumable uploads, so use gRPC appendable writes there and fall back to JSON.
		bucket := flag.String("bucket", "cpriti-thickthin-poc-use1", "bucket")
		prefix := flag.String("prefix", "rapid", "prefix")
		flag.Parse()
		os.Setenv("GOOGLE_CLOUD_DISABLE_DIRECT_PATH", "true")
		gc, err := storage.NewGRPCClient(ctx, storage.WithDisabledClientMetrics())
		if err != nil {
			panic(err)
		}
		jc, _ := storage.NewClient(ctx)
		putObj := func(objName string, data []byte) {
			w := gc.Bucket(*bucket).Object(objName).NewWriter(ctx)
			w.Append, w.FinalizeOnClose = true, true
			_, _ = w.Write(data)
			if err := w.Close(); err != nil {
				w2 := jc.Bucket(*bucket).Object(objName).NewWriter(ctx)
				_, _ = w2.Write(data)
				if err := w2.Close(); err != nil {
					panic(err)
				}
			}
		}
		for name, mib := range map[string]int{"200MiB": 200, "100MiB": 100} {
			data := make([]byte, mib<<20)
			for i := range data {
				data[i] = byte(i * 7)
			}
			putObj(*prefix+"/"+name, data)
			fmt.Println("created", *prefix+"/"+name)
		}
		// Ensure at least 100 objects exist under <prefix>/list/ for Workload 6 (list-objects-100).
		existing, _ := (nativeOpener{c: gc}).ListObjectsN(ctx, *bucket, *prefix+"/list/", 100)
		if existing < 100 {
			var wg sync.WaitGroup
			sem := make(chan struct{}, 16)
			small := []byte("worklog-list-seed")
			for i := existing; i < 100; i++ {
				wg.Add(1)
				sem <- struct{}{}
				go func(idx int) {
					defer wg.Done()
					defer func() { <-sem }()
					putObj(fmt.Sprintf("%s/list/obj-%03d", *prefix, idx), small)
				}(i)
			}
			wg.Wait()
			fmt.Printf("seeded %d small objects under %s/list/ for list-objects-100\n", 100-existing, *prefix)
		}
		return
	case "native":
		os.Setenv("GOOGLE_CLOUD_DISABLE_DIRECT_PATH", "true")
		c, err := storage.NewGRPCClient(ctx, storage.WithGRPCBidiReads(), storage.WithDisabledClientMetrics(), option.WithGRPCConnectionPool(runtime.NumCPU()))
		if err != nil {
			panic(err)
		}
		ctrl, err := storage.NewGRPCClient(ctx, storage.WithDisabledClientMetrics(), option.WithGRPCConnectionPool(1))
		if err != nil {
			panic(err)
		}
		rapid.Run("go-grpc-mrd", nativeOpener{c: c, ctrl: ctrl, append: os.Getenv("ZONAL") == "1"})
	case "native-json":
		c, err := storage.NewClient(ctx)
		if err != nil {
			panic(err)
		}
		rapid.Run("go-json-range", jsonOpener{c})
	case "uds":
		t := time.Now()
		rt := udsthin.GRPC
		if os.Getenv("UDS_TRANSPORT") == "json" {
			rt = udsthin.JSON
		}
		wt := udsthin.GRPC
		if os.Getenv("UDS_WRITE_TRANSPORT") == "json" {
			wt = udsthin.JSON
		}
		conns := min(runtime.NumCPU(), 32)
		if v, err := strconv.Atoi(os.Getenv("UDS_CONNS")); err == nil && v > 0 {
			conns = v
		}
		c, err := udsthin.NewClient(ctx, udsthin.Options{
			Transport:      rt,
			WriteTransport: wt,
			Conns:          conns,
			IdleSecs:       600,
			ServerBinary:   os.Getenv("GCS_CORE_SERVER"),
			Socket:         os.Getenv("UDS_SOCKET"),
			ZeroCopyDecode: os.Getenv("UDS_ZC") != "0",
		})
		if err != nil {
			panic(err)
		}
		fmt.Printf("client init: %v (sidecar pid %d, conns %d)\n", time.Since(t), c.Sidecar().PID, conns)
		rapid.CPUSeconds = func() float64 {
			cpu, _, _, _, _ := c.SidecarStats(ctx, false)
			return cpuSelf() + cpu
		}
		rapid.ResetPeakRSS = func() {
			os.WriteFile("/proc/self/clear_refs", []byte("5"), 0)
			c.SidecarStats(ctx, true)
		}
		rapid.PeakRSSMiB = func() float64 {
			_, _, peak, _, _ := c.SidecarStats(ctx, false)
			return statusKB("VmHWM")/1024 + peak
		}
		rapid.Run("go-uds-grpc", udsOpener{c})
	default:
		panic("unknown mode " + mode)
	}
}
