// bench2_uds: the same 1:1 workload against udsthin (pure Go -> gRPC/UDS -> Rust core).
//
//	UDS_TRANSPORT=json|grpc   path the Rust core uses towards GCS (default json)
//	UDS_CHUNK=<bytes>         ReadChunk size requested from the sidecar
//	UDS_CONNS=<n>             number of UDS connections multiplexed
//	GCS_CORE_SERVER=<path>    sidecar binary
//	bench2_uds check          behaviour checks
//	bench2_uds ping           UDS RTT / throughput microbenchmark (and TCP loopback if UDS_TCP=addr)
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"poc/gothin/bench2"
	"poc/gothin/udsthin"
	pb "poc/gothin/udsthin/gcscorepb"
)

func main() {
	ctx := context.Background()
	opts := udsthin.Options{Transport: udsthin.JSON, IdleSecs: 120}
	name := "go-uds-json"
	if os.Getenv("UDS_TRANSPORT") == "grpc" {
		opts.Transport = udsthin.GRPC
		name = "go-uds-grpc"
	}
	switch os.Getenv("UDS_WRITE_TRANSPORT") {
	case "json":
		opts.WriteTransport = udsthin.JSON
		if opts.Transport == udsthin.GRPC {
			name = "go-uds-grpcR-jsonW"
		}
	case "grpc":
		opts.WriteTransport = udsthin.GRPC
	}
	if v := os.Getenv("UDS_CHUNK"); v != "" {
		n, _ := strconv.Atoi(v)
		opts.ChunkSize = uint32(n)
	}
	if v := os.Getenv("UDS_CONNS"); v != "" {
		opts.Conns, _ = strconv.Atoi(v)
	}
	if v := os.Getenv("UDS_SOCKET"); v != "" {
		opts.Socket = v
	}
	opts.TCP = os.Getenv("UDS_TCP")
	opts.ZeroCopyDecode = os.Getenv("UDS_ZC") != "0" // default on; UDS_ZC=0 = stock proto decode
	if os.Getenv("UDS_ZC") == "0" {
		name += "-protodecode"
	}
	opts.UseEmbedded = os.Getenv("UDS_EMBEDDED") == "1"
	if os.Getenv("UDS_SESSION") == "1" {
		// Lend this process's own credentials to the sidecar (per-caller session).
		ts, err := google.DefaultTokenSource(ctx, storage.ScopeFullControl)
		if err != nil {
			panic(err)
		}
		opts.TokenSource = ts
		name += "-session"
	}

	t := time.Now()
	c, err := udsthin.NewClient(ctx, opts)
	if err != nil {
		panic(err)
	}
	fmt.Printf("client init: %v (sidecar spawn %v, pid %d, owned=%v, how=%s, session=%v)\n", time.Since(t), udsthin.SpawnLatency, c.Sidecar().PID, c.Sidecar().Owned(), udsthin.HowStarted, c.SessionID() != "")

	// Account the sidecar's CPU and peak RSS together with this process.
	bench2.ExtraCPU = func() float64 {
		cpu, _, _, _, err := c.SidecarStats(ctx, false)
		if err != nil {
			return 0
		}
		return cpu
	}
	bench2.ExtraPeakRSS = func(reset bool) float64 {
		_, _, peak, _, err := c.SidecarStats(ctx, reset)
		if err != nil {
			return 0
		}
		return peak
	}

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "check":
			check(c)
			return
		case "ping":
			ping(c, opts)
			return
		case "session-tests":
			sessionTests(ctx, c, opts)
			return
		case "hijack": // child process: try to use a session id that belongs to another pid
			c.ForceSession(os.Args[2])
			_, err := c.Bucket(bucket()).Object("check/uds-exists").NewReader(ctx)
			fmt.Printf("hijack from pid %d -> %v\n", os.Getpid(), err)
			return
		case "shm":
			shmBench(ctx, c)
			return
		}
	}
	bench2.Run[*udsthin.Client, *udsthin.BucketHandle, *udsthin.ObjectHandle, *udsthin.Reader, *udsthin.Writer, *udsthin.ObjectIterator](name, c, &udsthin.RPCs)
	scpu, rss, peak, threads, _ := c.SidecarStats(ctx, false)
	gcpu := bench2.CPUSeconds()
	fmt.Printf("sidecar: rss=%.0f MiB peak=%.0f MiB threads=%d | cpu split whole run: go=%.1fs sidecar=%.1fs\n", rss, peak, threads, gcpu, scpu)
}

func check(c *udsthin.Client) {
	ctx := context.Background()
	b := c.Bucket(bucket())

	_, err := b.Object("does/not/exist").NewReader(ctx)
	fmt.Printf("missing object -> errors.Is(ErrObjectNotExist)=%v (%v)\n", errors.Is(err, storage.ErrObjectNotExist), err)

	w := b.Object("check/uds-exists").NewWriter(ctx)
	w.Write([]byte("x"))
	if err := w.Close(); err != nil {
		fmt.Printf("create -> err=%v\n", err)
	} else {
		fmt.Printf("create -> ok size=%d gen=%d\n", w.Attrs().Size, w.Attrs().Generation)
	}

	w = b.Object("check/uds-exists").If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)
	w.Write([]byte("y"))
	err = w.Close()
	var ge *googleapi.Error
	code := 0
	if errors.As(err, &ge) {
		code = ge.Code
	}
	fmt.Printf("DoesNotExist on existing -> googleapi.Error=%v code=%d (%v)\n", ge != nil, code, err)

	r, err := b.Object("check/uds-exists").NewRangeReader(ctx, 0, 1)
	if err == nil {
		data, _ := io.ReadAll(r)
		fmt.Printf("range read -> attrs=%+v remain=%d data=%q\n", r.Attrs, r.Remain(), data)
		r.Close()
	} else {
		fmt.Println("range read err:", err)
	}

	w = b.Object("check/uds-big").NewWriter(ctx)
	w.Write(make([]byte, 64<<20))
	fmt.Println("64MiB write ->", w.Close())
	cctx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	t := time.Now()
	r, err = b.Object("check/uds-big").NewReader(cctx)
	var n int64
	if err == nil {
		n, err = io.Copy(io.Discard, r)
		r.Close()
	}
	fmt.Printf("ctx timeout 30ms mid-stream -> after %v copied=%d err=%v ctxErr=%v\n", time.Since(t).Round(time.Millisecond), n, err, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled))

	// Handle-based reads (gRPC CloudPath on the core, regardless of transport flag).
	op, err := b.Object("check/uds-big").Open(ctx)
	if err != nil {
		fmt.Println("open (BidiRead) err:", err)
		return
	}
	buf := make([]byte, 8<<10)
	t = time.Now()
	nn, err := op.ReadAt(ctx, buf, 1<<20)
	fmt.Printf("open+readAt 8KiB@1MiB -> n=%d err=%v in %v (attrs size=%d)\n", nn, err, time.Since(t).Round(time.Microsecond), op.Attrs.Size)
	op.Close(ctx)
}

// ping measures the local hop: RTT for tiny messages and throughput for 1 MiB
// payloads over UDS, and over TCP loopback when UDS_TCP is set, so the IPC
// overhead can be separated from GCS latency.
func ping(c *udsthin.Client, opts udsthin.Options) {
	ctx := context.Background()
	run := func(label string, stub pb.GcsCoreClient) {
		for _, sz := range []int{0, 4 << 10, 64 << 10, 1 << 20, 4 << 20} {
			payload := make([]byte, sz)
			iters := 2000
			if sz >= 1<<20 {
				iters = 300
			}
			stub.Ping(ctx, &pb.PingRequest{Payload: payload})
			t := time.Now()
			for i := 0; i < iters; i++ {
				if _, err := stub.Ping(ctx, &pb.PingRequest{Payload: payload}); err != nil {
					panic(err)
				}
			}
			d := time.Since(t)
			per := d / time.Duration(iters)
			mbps := float64(sz) * 2 * float64(iters) / (1 << 20) / d.Seconds()
			fmt.Printf("%-12s payload=%-8d rtt=%-10v %8.0f MiB/s (echo, both directions)\n", label, sz, per.Round(time.Microsecond), mbps)
		}
	}
	cc, err := grpc.NewClient("unix:"+c.Sidecar().Socket, grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64<<20), grpc.MaxCallSendMsgSize(64<<20)))
	if err != nil {
		panic(err)
	}
	run("uds", pb.NewGcsCoreClient(cc))
	if opts.TCP != "" {
		tc, err := grpc.NewClient(opts.TCP, grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64<<20), grpc.MaxCallSendMsgSize(64<<20)))
		if err != nil {
			panic(err)
		}
		run("tcp-loopback", pb.NewGcsCoreClient(tc))
	}
}

func bucket() string {
	if b := os.Getenv("BUCKET"); b != "" {
		return b
	}
	return "cpriti-thickthin-poc-asse1"
}

// sessionTests verifies the per-caller credential model end to end.
func sessionTests(ctx context.Context, c *udsthin.Client, opts udsthin.Options) {
	b := c.Bucket(bucket())
	// 1. session with the app's own token works
	r, err := b.Object("check/uds-exists").NewReader(ctx)
	if err == nil {
		io.Copy(io.Discard, r)
		r.Close()
	}
	fmt.Printf("1 read with lent token (session=%v) -> err=%v\n", c.SessionID() != "", err)

	// 2. a second client in the same process with a *bogus* token must fail with 401, not fall back to ADC
	bogus, err := udsthin.NewClient(ctx, udsthin.Options{Socket: opts.Socket, NoSpawn: true, TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "invalid-token", Expiry: time.Now().Add(time.Hour)})})
	if err != nil {
		fmt.Println("2 bogus session create err:", err)
	} else {
		_, err = bogus.Bucket(bucket()).Object("check/uds-exists").NewReader(ctx)
		var ge *googleapi.Error
		code := 0
		if errors.As(err, &ge) {
			code = ge.Code
		}
		fmt.Printf("2 read with bogus token -> code=%d err=%v\n", code, err)
		bogus.Close()
	}

	// 3. another process presenting our session id must be rejected (pid binding)
	exe, _ := os.Executable()
	cmd := exec.Command(exe, "hijack", c.SessionID())
	cmd.Env = append(os.Environ(), "UDS_SOCKET="+c.Sidecar().Socket, "GCS_CORE_SERVER=/nonexistent", "UDS_SESSION=")
	out, _ := cmd.CombinedOutput()
	fmt.Printf("3 %s", out)

	// 4. token refresh path
	_, err = c.Bucket(bucket()).Object("check/uds-exists").NewReader(ctx)
	fmt.Printf("4 read after UpdateToken plumbing present -> err=%v\n", err)

	// 5. does the token ever appear in the sidecar's stderr/log? (we never log it) and in /proc/<pid>/environ? (no: it is sent over the socket, not env)
	env, _ := os.ReadFile(fmt.Sprintf("/proc/%d/environ", c.Sidecar().PID))
	fmt.Printf("5 token in sidecar environ: %v\n", strings.Contains(string(env), "ya29"))
}

// shmBench compares three ways of getting 128 MiB from the sidecar into Go.
func shmBench(ctx context.Context, c *udsthin.Client) {
	b := c.Bucket(bucket())
	const obj = "shm/128MiB"
	// Create the object over JSON regardless of the read transport (gRPC
	// appendable writes are rejected on regional buckets).
	jc, err := udsthin.NewClient(ctx, udsthin.Options{Socket: c.Sidecar().Socket, NoSpawn: true, Transport: udsthin.JSON})
	if err != nil {
		panic(err)
	}
	w := jc.Bucket(bucket()).Object(obj).NewWriter(ctx)
	w.Write(make([]byte, 128<<20))
	if err := w.Close(); err != nil {
		// Zonal buckets reject JSON resumable uploads: use gRPC appendable instead.
		gc, err2 := udsthin.NewClient(ctx, udsthin.Options{Socket: c.Sidecar().Socket, NoSpawn: true, Transport: udsthin.GRPC})
		if err2 != nil {
			panic(err)
		}
		w = gc.Bucket(bucket()).Object(obj).NewWriter(ctx)
		w.Write(make([]byte, 128<<20))
		if err := w.Close(); err != nil {
			panic(err)
		}
		gc.Close()
	}
	jc.Close()
	iters := 10
	if v := os.Getenv("ITERS"); v != "" {
		iters, _ = strconv.Atoi(v)
	}
	buf := make([]byte, 128<<20)
	cpu := func() (goCPU, sideCPU float64) {
		side, _, _, _, _ := c.SidecarStats(ctx, false)
		return bench2.CPUSeconds(), side
	}
	run := func(label string, f func() (int, error)) {
		f() // warm
		var ds []time.Duration
		g0, s0 := cpu()
		for i := 0; i < iters; i++ {
			t := time.Now()
			n, err := f()
			if err != nil || n != len(buf) {
				fmt.Println(label, "error:", n, err)
				return
			}
			ds = append(ds, time.Since(t))
		}
		g1, s1 := cpu()
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		gib := float64(len(buf)) * float64(iters) / (1 << 30)
		fmt.Printf("%-28s p50=%-9v min=%-9v %7.1f MiB/s  cpu %.2f s/GiB (Go %.2f + sidecar %.2f)\n", label, ds[len(ds)/2].Round(time.Millisecond), ds[0].Round(time.Millisecond), 128/ds[len(ds)/2].Seconds(), (g1-g0+s1-s0)/gib, (g1-g0)/gib, (s1-s0)/gib)
	}
	run("grpc-chunks (copy into p)", func() (int, error) {
		r, err := b.Object(obj).NewReader(ctx)
		if err != nil {
			return 0, err
		}
		defer r.Close()
		return io.ReadFull(r, buf)
	})
	region, err := c.OpenShm()
	if err != nil {
		fmt.Println("shm unavailable:", err)
		return
	}
	defer region.Close()
	run("shm (copy into p)", func() (int, error) {
		r, err := b.Object(obj).NewShmReader(ctx, region, 0, uint64(len(region.Mem)))
		if err != nil {
			return 0, err
		}
		defer r.Close()
		return io.ReadFull(r, buf)
	})
	run("shm (zero-copy, lend slice)", func() (int, error) {
		r, err := b.Object(obj).NewShmReader(ctx, region, 0, uint64(len(region.Mem)))
		if err != nil {
			return 0, err
		}
		defer r.Close()
		n := 0
		var sink byte
		for {
			s, err := r.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return n, err
			}
			sink ^= s[len(s)-1] // touch the data so the pages are actually faulted in
			n += len(s)
		}
		_ = sink
		return n, nil
	})
}
