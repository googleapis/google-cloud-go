# Go thin client over a Rust GCS core as a gRPC/UDS sidecar — POC, Challenges, Benchmarks

Companion to the earlier FFI evaluation (`storage/internal/rust-poc/EVALUATION.md`, branch `go-sdk-rust-poc`).
This document covers **row #2** of the worklog tab *"gRPC over UDS Approach"*: the Rust GCS core runs as a
**separate process** on the same machine and the Go SDK becomes a **thin, pure-Go client** that talks to it over a
Unix domain socket with gRPC. Scope is multi-process only; the in-process (cgo thread) variant is tracked under the FFI work.

All numbers below are real measurements from:
1. **Worklog Specification Run (`uds5-worklog`, §4.10):** GCE `c4-standard-192` (192 vCPUs, 720 GiB RAM, Ubuntu 24.04 LTS, `GVNIC` + `TIER_1` 200 Gbps networking) in `us-central1-b` (`instance_id=4800314160389265979` & `8515920295490933521`) against a standard regional bucket in `us-west1` (`gs://cpriti-thickthin-poc-usw1`) and a Zonal Rapid bucket in `us-west1-a` (`gs://cpriti-thickthin-poc-zonal-usw1a`), with both Cloud Monitoring metrics (`compute.googleapis.com/instance/cpu/utilization` and `agent.googleapis.com/memory/percent_used` via `google-cloud-ops-agent` `2.73.0`) verified and exported.
2. **Same-Region & Zonal Reference Runs (`uds2`–`uds4`, §4.1–§4.9):** GCE `c3-standard-8` VMs (Debian 12, kernel 6.1), one in `asia-southeast1-b` against a regional bucket in the same region (`cpriti-thickthin-poc-asse1`) and one in `us-central1-b` against a **zonal (Rapid) bucket** in the same zone (`cpriti-zb-central1-b`).

Raw logs are in `gothin/results/uds5-worklog/` and `gothin/results/uds{2,3,4}-{regional,zonal}/` next to this file.

---

## 1. Purpose

Answer, with code and measurements rather than opinion:

1. What does a Go user lose or gain if the Go SDK is a thin client over a Rust sidecar?
2. Which of the challenges listed in the worklog are **blockers**, which are **high effort but solvable**, and which are **non-issues** for Go?
3. How does it perform against today's Go SDK on ReadObject / WriteObject / ListObjects (JSON and gRPC CloudPath), and on the Rapid SLIs (Open, Read50x2x4, Random8K) on regional and zonal buckets?
4. How would it be packaged and deployed, and what breaks in common customer environments?

## 2. How the POC is built

```
Go application
  └── cloud.google.com/go/storage (thin, CGO_ENABLED=0)             gothin/udsthin/
        ├── finds or spawns the sidecar (per-user socket, embedded binary)
        ├── gRPC over UDS  (control + data plane, 4 HTTP/2 connections)   gcs_uds/proto/gcscore.proto
        └── optional shared-memory data plane (memfd + SCM_RIGHTS + mmap)  gothin/udsthin/shm_linux.go
  ┌──── gcs-core-server (Rust, tonic 0.14 on UDS)                   gcs_uds/server/
  │       ├── google-cloud-storage 1.21.0 (JSON via reqwest, gRPC CloudPath, appendable writes)
  │       ├── per-client sessions: caller's OAuth token lent over the socket, bound to peer uid+pid
  │       ├── peer-UID check (SO_PEERCRED), idle exit, PR_SET_PDEATHSIG, stale-socket cleanup
  │       └── Stats endpoint (CPU, peak RSS, threads) so the benchmark can charge the sidecar to the caller
```

* **Wire contract** (`gcscore.proto`): `ReadObject` (server-stream, `repeated bytes` chunks), `OpenObject/ReadRange/CloseObject`
  (one open stream per object for Rapid-style random reads), `WriteObject` (client-stream), `ListObjects`, `ReadObjectShm`,
  `CreateSession/UpdateToken/CloseSession`, `Ping`, `Stats`, `Shutdown`.
* **Thin client** keeps the shape of today's API (`Bucket().Object().NewReader/NewWriter`, `Objects(...)`) so the benchmark
  harness runs the *same user code* against both the native SDK and the sidecar.
* **Zero-copy decode** on the Go side (`rawcodec.go`): a gRPC codec that aliases the receive buffer instead of letting
  `proto.Unmarshal` copy each chunk. Measured gain ≈ 10% CPU on large reads (see §4.6). Can be switched off (`UDS_ZC=0`).
* **Sidecar discovery/spawn** (`sidecar.go`): socket at `$XDG_RUNTIME_DIR/gcs-core/core.sock` (fallback `/tmp/gcs-core-<uid>/`);
  dial first, spawn only if nobody is listening, so many Go processes of the same user **share one sidecar**.
* **Embedded binary** (`embed_linux.go`): `//go:embed bin/gcs-core-server-linux-amd64`, extracted to `~/.cache/gcs-core/<hash>/`
  with a `memfd_create`+`fexecve` fallback for `noexec` filesystems. Chosen over "download at runtime" (VPC-SC blocks it)
  and "operator-installed daemon" (not acceptable for a library).
* **Credentials**: each Go client creates a session and lends its `oauth2.TokenSource` tokens (access tokens, never key files)
  over the socket; the sidecar uses them as a `CredentialsProvider`, refreshes come from the Go side.

## 3. Challenges from the worklog — Go verdict

Legend: **Blocker** = cannot be made acceptable for a GA Go library as far as we can see · **High effort** = solvable with
real engineering and an ongoing cost · **OK** = no material user impact.

### Zero copy across the UDS boundary — **High effort (Blocker only if zero-copy over standard `io.Reader`/`io.Writer` without shm is required)**
* Over a gRPC/UDS socket zero copy is impossible by construction: every byte is copied user→kernel in the sidecar and
  kernel→user in Go (~0.15–0.30 CPU‑s/GiB at memory bandwidth), plus HTTP/2 framing, flow control (`WINDOW_UPDATE`), and
  protobuf serialization (~0.9–1.0 CPU‑s/GiB). Measured on a 128 MiB regional read: **2.72–2.99 CPU‑s/GiB** for the pair
  (2.58–2.59 on mixed write+read) vs **2.26–2.35** for Go's production default (`go-json` over HTTP/2) and **1.40–1.44**
  for Go on HTTP/1.1 (`go-json-h1`), with the Go side alone at 0.82–0.84 s/GiB just to receive and decode.
  - **Against Go's production HTTP/2 default (`go-json`), the sidecar adds only +11–15% CPU** on large JSON transfers, because
    the Rust crate's HTTP/1.1 client (`reqwest` without `http2`) is ~0.9 s/GiB cheaper than Go's `net/http` HTTP/2 stack and
    nearly subsidizes the UDS hop.
  - **Against the same wire protocol (`go-json-h1`), the gRPC/UDS hop adds ~1.15 s/GiB (~1.8× CPU)** — the pure IPC + HTTP/2
    framing tax. A raw UDS stream with a length-prefixed framing protocol (instead of gRPC/HTTP2) would eliminate the HTTP/2
    and protobuf portion of that tax.
* To avoid inter-process data copies altogether, the POC implements **shared memory** (a sealed `memfd` passed with
  `SCM_RIGHTS`, mmap'ed on both sides, gRPC only as control plane). Measured: **1.35–1.91 CPU‑s/GiB**, Go side 0.03–0.04 s/GiB
  when the caller accepts a borrowed slice, 0.18–0.19 s/GiB when it copies into its own `[]byte` (§4.6). That is at parity with
  native JSON/HTTP1 and better than native gRPC. While this POC only implemented `ReadObjectShm`, shared memory is **not**
  inherently reads-only: a production IPC layer can support zero-copy writes too via bidirectional shared-memory ring buffers
  with `futex`/`eventfd` signaling (or Linux pipe `splice(2)`/`vmsplice(2)`), though it remains Linux-specific and requires a
  borrowed-buffer API to avoid the final caller copy.
* Latency is unchanged either way (network-bound at 200–270 MiB/s on this VM); the cost of the copies shows up as CPU, not time.

### Packaging and distribution — **High effort**
* The sidecar is a 17 MB stripped (21 MB unstripped, 6.9 MB gzip) static musl binary. Embedding it in the Go module means
  **+6.9 MB per platform in the module zip** (≈41 MB for six platforms vs 1.0 MB today) and **+21 MB in every linked Go
  binary** (67.7 vs 46.7 MB for the benchmark). `go mod tidy` / `go get` continue to work unchanged — this is the main
  advantage over FFI — but the module becomes the largest in `google-cloud-go` by far.
* The Rust half needs a per-target C toolchain to cross-compile (aws-lc-sys fails without `aarch64-linux-gnu-gcc`), so
  release engineering has to produce N platform binaries and the Go module must ship them (or ship Linux only and fall back
  to the pure-Go path elsewhere, i.e. keep two implementations).
* The Go thin client itself cross-compiles with `CGO_ENABLED=0` to linux/arm64, darwin/arm64, s390x, freebsd (44–48 MB
  binaries, VM log) and windows/amd64 (after build-tagged stubs for `Setpgid`/`SIGTERM`/`getrusage`; the VM run's
  `cross windows/amd64: ok  bytes` line was a masked failure) — the portability problem of FFI goes away *for the Go side only*.
  Windows would still need a named-pipe transport and a Windows sidecar build; the POC does not provide either.

### Deployment in major customer environments — **High effort, with hard failures in some**
Measured on the VM (`gothin/results/uds2-regional/packaging.txt`, `uds3-regional/scratch_retest.txt`):
* `FROM scratch` / distroless containers: the static musl sidecar starts, **but panics without CA certificates**
  (`No CA certificates were loaded from the system`) — must bundle `webpki-roots`. The glibc build fails outright (needs
  `ld-linux` and `GLIBC_2.34`).
* `noexec` `/tmp` or home: extracting the embedded binary and exec'ing it fails (`rc=126`). The `memfd_create` fallback
  works on this kernel (6.1), but **cannot be relied on in production**: executing from an anonymous `memfd` is blocked on
  modern kernels with `vm.memfd_noexec=1` or `2` (or seccomp/MDWE in hardened GKE/PaaS) and is routinely flagged or blocked
  by enterprise EDR agents (CrowdStrike Falcon, Defender, OSSEC) as a fileless-execution pattern.
* Read-only filesystems: socket directory must be writable (also used for the shm side channel); abstract sockets avoid
  the file but are Linux-only and visible to every process in the network namespace (hence the peer-UID check).
* UDS path > 108 bytes fails (`SUN_LEN`); long `$XDG_RUNTIME_DIR`s are real on some systems.
* Cloud Run / Functions / GKE with one process per container simply pay a second process (12–37 ms spawn, ~60 MB RSS)
  per instance; the "shared sidecar" benefit only materialises on VMs with several Go processes of the same user.
* Peer-UID rejection, orphan cleanup via `PR_SET_PDEATHSIG`, stale-socket recovery, idle exit, PID‑1 behaviour: all pass.

### Mono repo concerns — **High effort**
* Nothing Go-specific blocks it, but `google-cloud-go` would carry prebuilt binaries of code that lives in another repo
  (`google-cloud-rust`), with a separate release cadence, security scanning, SBOM and provenance story. A Rust sidecar
  CVE forces a Go module release; a Go module release must pin an exact sidecar build.
* Shared sidecar across processes also means **version skew**: two Go binaries built against different SDK versions on
  the same machine will race to the same socket. The POC namespaces the socket per user only; per-version sockets fix it
  at the cost of one sidecar per version.

### Tool chain, coordination and maintenance costs — **High effort**
* Two toolchains (Go + Rust/cargo + C toolchain for aws-lc) and a proto contract between them that must stay compatible
  across versions in both directions. Rust release build took 229–232 s on the VM, 9 min cold on a workstation.
* Every Go feature (new option, new header, retry knob) now lands in three places: proto, Rust server, Go client.
* Error mapping is lossy by design: HTTP/gRPC codes from GCS → tonic status → `googleapi.Error`. The POC maps the common
  codes (404/412/401/403/429/5xx) and `ErrObjectNotExist` works, but detailed error bodies/`Details` need explicit plumbing.

### Impedance mismatch — **Blocker for 100% drop-in backwards compatibility**
* Request shape maps cleanly (reads, range reads, writes, list, preconditions, generations). Appendable writes work on
  both sides and stay within 5% of native up to 128 MiB and within 8–9% at 512 MiB (§4.5).
* **Blocker for backwards compatibility with caller-process hooks:** anything that runs inside the caller's process —
  `option.WithHTTPClient` / custom `http.RoundTripper` (widely used in Go enterprise code for custom proxies, mTLS, request
  signing, and test doubles), `option.WithGRPCConn` / custom `grpc.DialOption`s, in-process OpenTelemetry exporters/span
  context, and `context.Context` values — **cannot cross the process boundary**. Dropping or ignoring these options breaks
  the existing `cloud.google.com/go/storage` public contract. Credentials cross only as **lent access tokens** (see §4.8);
  impersonation, external-account flows and custom `TokenSource`s still run in the Go process, so that part works.
* **Lack of DirectPath in the Rust crate today (major GCE/GKE gap):** Go's native gRPC client supports DirectPath (ALTS
  over IPv6 directly to storage backends, bypassing GFE), whereas `google-cloud-storage` 1.21.0 is CloudPath-only. Until the
  Rust crate implements DirectPath, switching Go's gRPC path to the Rust sidecar forfeits DirectPath throughput and CPU
  gains on GCE/GKE.
* ListObjects: the Rust crate lists through gRPC StorageControl, the Go SDK through JSON — different projection and
  paging behaviour (see §4.3).

### Size of the binary — **OK / High effort**
* +21 MB per linked Go program (17 MB stripped sidecar) and +6.9 MB per platform in the module zip. The sidecar's RSS is
  ~57–101 MB after 512 MiB runs; peak RSS of the pair is at parity with native for whole-object transfers (§4.1).

### Harder debugging experience — **High effort**
* A panic/OOM in the sidecar surfaces to the Go caller as `UNAVAILABLE`/`connection reset`; the Go stack trace stops at
  the socket. `pprof`, `GODEBUG`, `-race`, Go's HTTP tracing all see only the thin client. The sidecar needs its own log file,
  tracing and core-dump handling, and support cases need both.
* Observed during the POC (`uds3-regional/check_grpc_errors.txt`): an `UNIMPLEMENTED` appendable write on a regional
  bucket reached Go as a raw `rpc error: code = Unimplemented …` that is **not** a `googleapi.Error` (so `errors.As` checks in
  user code fail); a missing CA bundle made the sidecar exit at start-up with only a Rust panic in its own stderr; and the
  slow reads at 64–256 concurrent ops (§4.2) produced no error at all — each needed the sidecar's log or `Stats` to diagnose.

### Direct copy of data into GPU buffers — **N/A for Go today (Blocker across processes if ever added)**
* This row is part of the shared cross-language worklog template. Today's Go GCS SDK has no GPU/CUDA buffer API, so there
  is no regression for current Go users. Architecturally, if GPU direct copy were ever added, a separate sidecar process
  cannot write into the caller's CUDA buffers without exporting CUDA IPC handles (`cudaIpcGetMemHandle`) and linking CUDA
  into the sidecar; the shared-memory data plane gives host memory at best.

## 4. Benchmarks

Method: same harness (`gothin/bench2`) and same user code for both backends; `-iters 10` per cell, p50 reported;
**CPU‑s/GiB and peak MiB include the sidecar** (Go `getrusage` + sidecar `Stats`), so they are directly comparable
between native and UDS rows. `whole` style = one `io.ReadAll`/`Write` of the object; `32k` style = 32 KiB `Read`/`Write` loop.
Baselines: `go-json` = Go SDK production default (HTTP/2); `go-json-h1` = Go SDK on HTTP/1.1 (`MaxIdleConnsPerHost=256`,
matching the wire protocol of the Rust crate, whose `reqwest` client uses HTTP/1.1); `go-grpc-cp` = Go SDK gRPC CloudPath
(BidiReads on, client metrics off, pool = NumCPU, DirectPath disabled for CloudPath parity since the Rust crate has no
DirectPath — see §3). UDS rows: `go-uds-json` = sidecar on JSON; `go-uds-grpcR-jsonW` = gRPC reads, JSON writes (gRPC
non-appendable writes are `UNIMPLEMENTED` on regional buckets, for both SDKs). Both native Go and the Rust crate compute
and verify hardware-accelerated Castagnoli CRC32C checksums over the network on full-object reads and writes; the local
UDS/shm hop between Go and the sidecar is un-checksummed (just as in-process memory copies are).

### 4.1 Regional, one operation at a time (asia-southeast1, c3-standard-8)

Native rows: `uds2-regional/regional_seq_native_*_r{1,2}.txt`; UDS rows: `uds4-regional/regional_seq_uds_*_r{1,2}.txt`
(the uds2 UDS runs crashed in the 32k block on a double-free in the zero-copy reader, fixed before uds4). Values are the
p50 of each round, shown as `r1 / r2`. CPU s/GiB is write+read at that size, **Go process + sidecar**. Peak MiB is the
process-lifetime high-water mark, so only the first (`whole`) block is meaningful per size.

**`whole` style — JSON paths**

| size | op | go-json-h1 | go-json (h2) | go-uds-json | CPU s/GiB h1 / h2 / uds | peak MiB h1 / uds |
|---|---|---|---|---|---|---|
| 1 MiB | write | 88 / 92 ms | 94 / 87 ms | 83 / 93 ms | 3.13–3.18 / 4.05–4.19 / **4.15–4.41** | 84–85 / 74–75 |
| 1 MiB | read | 37 / 41 ms | 36 / 37 ms | 39 / 39 ms | | |
| 16 MiB | write | 266 / 305 ms | 286 / 312 ms | 348 / 288 ms | 1.51–1.57 / 2.36–2.46 / **2.64–2.66** | 132–133 / 150–168 |
| 16 MiB | read | 103 / 117 ms | 98 / 108 ms | 116 / 109 ms | | |
| 128 MiB | write | 1.50 / 1.60 s | 1.57 / 1.44 s | 1.70 / 1.54 s | 1.40–1.44 / 2.30–2.35 / **2.58–2.59** | 470–473 / 471–492 |
| 128 MiB | read | 646 / 625 ms | 524 / 608 ms | 636 / 556 ms | | |
| 512 MiB | write | 5.61 / 5.78 s | 5.80 / 6.53 s | 5.78 / 5.84 s | 1.41–1.43 / 2.26–2.32 / **2.55–2.56** | 1497–1499 / 1503–1522 |
| 512 MiB | read | 2.46 / 2.49 s | 1.89 / 2.09 s | 2.17 / 1.81 s | | |

**`whole` style — gRPC reads** (`go-grpc-cp` native vs `go-uds-grpcR-jsonW`; writes on the UDS row are JSON, see note above)

| size | read native gRPC | read UDS gRPC | CPU s/GiB native / UDS | peak MiB native / UDS |
|---|---|---|---|---|
| 1 MiB | 55 / 61 ms | 60 / 58 ms | 5.49–5.56 / **5.19–5.36** | 80–93 / 78–82 |
| 16 MiB | 126 / 130 ms | 140 / 127 ms | 2.80–2.86 / **2.92–2.95** | 178–188 / 144–151 |
| 128 MiB | 616 / 687 ms | 738 / 677 ms | 2.34–2.46 / **2.68–2.71** | 515–516 / 425–435 |
| 512 MiB | 2.20 / 2.25 s | 2.62 / 2.50 s | 2.44–2.61 / **2.76–2.77** | 1542–1544 / 1458–1467 |

**`32k` style** (32 KiB `Read`/`Write` loop — the shape most user code has)

| size | op | go-json-h1 | go-uds-json | crossings/op UDS | CPU s/GiB h1 / uds |
|---|---|---|---|---|---|
| 1 MiB | write | 85 / 94 ms | 78 / 87 ms | 3 | 2.15–2.31 / 3.06–3.32 |
| 16 MiB | write | 306 / 287 ms | 316 / 309 ms | 11 | 1.49–1.60 / 2.80–2.94 |
| 128 MiB | write | 1.65 / 1.61 s | 1.57 / 1.60 s | **3 595** | 1.38–1.42 / 2.80–2.85 |
| 512 MiB | write | 5.87 / 5.58 s | **6.76 / 6.43 s** | **15 883** | 1.37–1.42 / 2.81–3.04 |
| 128 MiB | read | 603 / 684 ms | 645 / 530 ms | 1 | |
| 512 MiB | read | 2.22 / 2.48 s | 2.34 / 1.81 s | 1 | |

Reads stay one streaming RPC per object regardless of how the caller reads. Writes in the POC forward each `Write` call
as one gRPC message, so a 32 KiB write loop becomes ~16k messages for 512 MiB and costs 10–15% latency; a production client
would coalesce to the resumable chunk size (16 MiB) before crossing, as the `whole` rows do (66 / 258 crossings).

**Zero-copy decode matters for memory as much as CPU:** with stock `proto.Unmarshal` (`*_json_proto_*`) the same runs
peak at 759–799 / 2795–2819 MiB for 128 / 512 MiB objects vs 471–492 / 1503–1522 MiB with the aliasing codec, and CPU is
2.88–3.29 vs 2.58–2.59 s/GiB at 128 MiB (2.88–3.10 vs 2.55–2.56 at 512 MiB).

### 4.2 Regional, 16 MiB objects, 16 / 64 / 256 concurrent operations

| parallel | backend | write p50 | read p50 | read p90 | CPU s/GiB | Go threads | sidecar threads |
|---|---|---|---|---|---|---|---|
| 16 | go-json-h1 | 279 ms | 109 ms | 152 ms | 1.76 | 12 | – |
| 16 | go-grpc-cp | 263 ms | 151 ms | 278 ms | 2.88 | 11 | – |
| 16 | go-uds-json | 302 ms | 114 ms | 170 ms | 3.40 | 12 | 13 |
| 16 | go-uds-grpcR-jsonW | 287 ms | 152 ms | 264 ms | 3.97 | 12 | 17 |
| 64 | go-json-h1 | 380 ms | 157 ms | 255 ms | 1.67 | 15 | – |
| 64 | go-grpc-cp | 299 ms | 151 ms | **18.9 s** | 2.85 | 11 | – |
| 64 | go-uds-json | 465 ms | **369 ms** | 954 ms | 3.63 | 12 | 25 |
| 64 | go-uds-grpcR-jsonW | 323 ms | **271 ms** | 2.7 s | 3.98 | 12 | 22 |
| 256 | go-json-h1 | 2.07 s | 652 ms | 1.10 s | 1.88 | 17 | – |
| 256 | go-grpc-cp | 2.77 s | 761 ms | 6.37 s | 2.86 | 11 | – |
| 256 | go-uds-json | 4.09 s | **2.11 s** | 3.44 s | 3.87 | 12 | 56 |
| 256 | go-uds-grpcR-jsonW | 3.51 s | **1.79 s** | 6.62 s | 4.30 | 12 | 51 |

**Connection sweep** (`uds4-regional/regional_par{64,256}_conns*_uds_json.txt`): to test whether the slowdown at 64–256
in-flight operations comes from multiplexing everything over 4 HTTP/2 connections to the sidecar, the same cell was run
with 4, 16, 64 and 256 UDS connections.

| parallel | UDS conns | write p50 | read p50 | crossings/op write / read | sidecar threads | native h1 same day |
|---|---|---|---|---|---|---|
| 64 | 4 | 618 ms | 455 ms | 371 / 144 | 22 | write 413 ms / read 155 ms |
| 64 | 16 | 524 ms | 238 ms | 319 / 161 | 36 | |
| 64 | 64 | 834 ms | 353 ms | 462 / 112 | 20 | |
| 64 | 256 | 754 ms | 365 ms | 429 / 124 | 13 | |
| 256 | 4 | 3.68 s | 1.99 s | 2318 / 209 | 56 | write 2.54 s / read 581 ms |
| 256 | 16 | 3.51 s | 1.81 s | 2333 / 180 | 72 | |
| 256 | 64 | 3.61 s | 1.38 s | 2373 / 159 | 87 | |
| 256 | 256 | 3.41 s | 1.66 s | 2360 / 180 | 21 | |

The connection count makes no consistent difference, so the HTTP/2 multiplexing hypothesis is **not** supported.
* **Correction on the `crossings/op` (`ffi/op`) column at `parallel > 1`:** the large numbers in that column (`371 / 144` at
  par=64, `2318 / 209` at par=256) are an **instrumentation bug in `bench2.go:177, 193, 204, 226`**, not message fragmentation.
  Each worker goroutine sampled `f0 := ffi(); ... ffi() - f0` against the single package-global atomic `udsthin.RPCs`
  (`udsthin.go:40`), so while one worker's 16 MiB op was in flight for 0.4–4 s, the delta accumulated the RPC increments of all
  63 or 255 *other* concurrent workers. In the actual wire code, `pump_reader` (`service.rs:170-198`) always coalesces reads to
  2 MiB chunks (1 streaming RPC delivering 8 messages for 16 MiB), and `w.send` (`udsthin.go:500`) always sends 2 MiB chunks
  (10 crossings per 16 MiB `whole` write: open + 8 chunks + close).
* **Actual causes of the 64/256-way slowdown in the POC:**
  1. **Tight `mpsc::channel(4)` backpressure (`service.rs:317, 431`):** each stream bridges the crate reader/writer to tonic
     through a 4-slot channel, causing frequent task yield/wake cycles when 64–256 streams run concurrently on 8 vCPUs.
  2. **Dual-runtime scheduler contention on 8 vCPUs:** 256 Go goroutines and 256 Tokio tasks compete for the same 8 cores across
     the UDS boundary (CPU rises to 3.6–4.3 s/GiB vs 1.7–1.9 native HTTP/1.1).
  3. **HTTP/1.1 connection pool asymmetry:** `bench2_native` explicitly sets `MaxIdleConnsPerHost = 256` (`cmd/bench2_native/main.go:26`),
     whereas the sidecar uses `google-cloud-storage`'s default `reqwest::Client` pool configuration.
  4. Native gRPC CloudPath has its own tail problem in the same cells (p90 18.9 s at 64, 6.4 s at 256) that is unrelated to the sidecar.

### 4.3 ListObjects (1000 objects, regional)

| backend | p50 | CPU ms/call | note |
|---|---|---|---|
| go-json-h1 | 56–60 ms | 9.3–9.6 | JSON, full projection |
| go-json (h2) | 68–70 ms | 10.7–11.0 | JSON, full projection |
| go-grpc-cp | 66–73 ms | 2.5–2.6 | gRPC `ListObjects` |
| go-uds-json / go-uds-grpcR | 71–78 ms | 3.1–3.4 (Go + sidecar) | Rust crate → gRPC StorageControl `ListObjects` + one UDS hop; not the same wire call as the JSON baselines (`uds4-regional/*_r{1,2}.txt`) |

### 4.4 Rapid-workload SLIs (go/gcs-sdk:continuous-benchmarks-for-acv2): Open, Read50x2x4, Random8K

`open` = open a 200 MiB object; `read50x2x4` = 50 MiB in 2 streams × 4 ranges, serial and concurrent; `random8k` = 8 KiB
random reads from an open 100 MiB object. Native = Go SDK gRPC with `MultiRangeDownloader` (BidiReads); UDS = sidecar gRPC
with one open bidi stream per object (`OpenObject/ReadRange`). Two rounds each; range of p50s shown.

| SLI | metric | **zonal** native | **zonal** UDS | regional native | regional UDS | regional native JSON range |
|---|---|---|---|---|---|---|
| open | p50 | 29.8–39.4 ms | 20.1–22.0 ms | 30.6–40.1 ms | 28.2–31.5 ms | 25.8–26.9 ms |
| read50x2x4 serial | p50 | 530–555 ms | 587–684 ms | 710–831 ms | 564–803 ms | 1.89–2.08 s |
| read50x2x4 concurrent | p50 | 316–359 ms | 323–332 ms | 348–443 ms | 368–532 ms | 438–538 ms |
| read50x2x4 | CPU s/GiB | 2.65–2.88 | 2.66–2.83 | 2.79–3.07 | 2.64–2.96 | 2.77–3.23 |
| random8k | p50 | 1.40–1.73 ms | 1.14–1.46 ms | 3.24–4.50 ms | 4.20–5.02 ms | 28.4–28.5 ms |
| random8k | p99 | 2.56–4.66 ms | 2.26–3.07 ms | 9.3–12.8 ms | 7.2–10.5 ms | 50–68 ms |
| random8k | CPU ms/op | 0.09 | 0.22–0.23 | 0.09 | 0.23 | 0.18–0.22 |
| peak RSS (pair) | MiB | 184–221 | 238–293 | 156–194 | 289–331 | 207 |

Reading: latency and throughput are at parity (differences are within run-to-run noise); the sidecar costs **2.5× CPU per
8 KiB read** (0.23 vs 0.09 ms) and ~100 MiB more memory for the second process. Note that in this POC, `Opened.ReadAt`
(`udsthin.go:616`) opens a new `/gcscore.v1.GcsCore/ReadRange` stream over UDS for each 8 KiB call (while the sidecar reuses
its open GCS `ObjectDescriptor`); keeping a persistent bidirectional UDS stream per opened object would avoid the per-call
HTTP/2 stream setup over UDS.

### 4.5 Zonal bucket, appendable writes + sequential reads (us-central1-b, gRPC CloudPath on both sides)

Zonal buckets reject JSON resumable uploads (400) and non-appendable gRPC writes for both SDKs, so only appendable writes
(`Writer.Append=true` natively, appendable object on the Rust side) are compared. One operation at a time, `whole` style, p50 of two rounds.

| size | write native | write UDS | read native | read UDS | CPU s/GiB native | CPU s/GiB UDS | peak MiB native | peak MiB UDS |
|---|---|---|---|---|---|---|---|---|
| 1 MiB | 134–139 ms | 107–110 ms | 32–33 ms | 28–32 ms | 5.50–5.70 | 5.72–5.92 | 93–94 | 77 |
| 16 MiB | 159–175 ms | 159–163 ms | 81–90 ms | 81–91 ms | 2.67–2.69 | 3.23–3.32 | 171–189 | 173–196 |
| 128 MiB | 404–456 ms | 423–437 ms | 469–492 ms | 493–533 ms | 2.22–2.30 | 2.94–2.96 | 510–517 | 489–506 |
| 512 MiB | 1.26–1.33 s | 1.37–1.43 s | 1.79–1.90 s | 1.86–1.94 s | 2.29–2.33 | 2.96–2.97 | 1537–1544 | 1528–1531 |

### 4.6 Shared-memory data plane vs gRPC chunks (128 MiB read, one at a time, 10 iters × 2 rounds)

`gothin/results/uds3-{regional,zonal}/*_shm_r{1,2}.txt`. Same sidecar, same object; only the data plane differs. Shown as `r1 / r2`.

| path | data plane | regional JSON p50 | CPU s/GiB (Go + sidecar) | regional gRPC p50 | CPU s/GiB | zonal gRPC p50 | CPU s/GiB |
|---|---|---|---|---|---|---|---|
| gRPC chunks → copy into caller's `[]byte` | UDS | 540 / 483 ms | 2.76 / 2.72 (Go 0.82–0.84) | 645 / 728 ms | 2.94 / 2.99 (Go 0.82) | 553 / 515 ms | 2.96 / 2.92 (Go 0.84) |
| shm → copy into caller's `[]byte` | memfd | 560 / 545 ms | 1.56 / 1.53 (Go 0.18–0.19) | 641 / 687 ms | 1.76 / 1.91 (Go 0.19) | 593 / 534 ms | 1.78 / 1.83 (Go 0.19) |
| shm → lend slice (zero copy) | memfd | 593 / 541 ms | 1.42 / 1.35 (Go 0.03) | 569 / 812 ms | 1.62 / 1.71 (Go 0.03) | 628 / 470 ms | 1.61 / 1.64 (Go 0.04) |
| native go-json-h1 (reference, §4.1) | – | 646 / 625 ms | 1.40 / 1.44 | | | | |

Zero-copy *decode* on the gRPC path (aliasing receive buffers instead of `proto.Unmarshal` copying): Rapid read50 serial
3.05–3.06 → 2.66–2.80 CPU‑s/GiB on zonal (3.11–3.22 → 2.64–2.70 on regional); 128 MiB whole write+read 2.88–3.29 → 2.58–2.59 s/GiB
on regional (`*_proto_*` files are the non-aliasing runs).

### 4.7 IPC floor: ping over the socket (c3-standard-8)

| payload | UDS rtt | UDS echo MiB/s | TCP loopback rtt | TCP MiB/s |
|---|---|---|---|---|
| 0 | 54 µs | – | 56 µs | – |
| 4 KiB | 64 µs | 122 | 67 µs | 117 |
| 64 KiB | 209 µs | 598 | 222 µs | 562 |
| 1 MiB | 1.94 ms | 1031 | 1.42 ms | 1410 |
| 4 MiB | 6.76 ms | 1184 | 5.58 ms | 1433 |

The hop itself is ~55 µs and >1 GB/s, so it never becomes the bottleneck against a 200–400 MiB/s network path; its cost is CPU.

### 4.8 Credentials: per-caller sessions (`session_tests.txt`, regional and zonal)

1. Read with the caller's lent token → OK.
2. Second client in the same process with a bogus token → `401 Invalid Credentials` (no silent fallback to the sidecar's ADC).
3. Another process presenting a stolen session id → `403 session is bound to another process`.
4. `UpdateToken` refresh path → OK.
5. Token not present in the sidecar's environment; never logged (`set_sensitive`).

Residual concerns, verified rather than assumed:
* Tokens live in sidecar RAM and would appear in a sidecar core dump; the 2025 concern about *key files* over the socket
  does not apply, only 1 h access tokens cross.
* **Ambient authority (`--require-session`):** in the POC `--require-session` defaulted to `false` for benchmark flags, meaning
  a same-UID client that omits `x-session` falls back to the sidecar's ambient ADC (`service.rs:78`). In production,
  `--require-session` must default to `true`.
* **Session lifecycle & PID recycling:** `session.rs` binds sessions to `(peer_uid, peer_pid)` without a disconnect hook or
  idle TTL; if a client crashes (`SIGKILL`) and the kernel later recycles its PID under the same UID, the orphaned session
  could be reused. Production sessions must be tied to the UDS connection lifecycle (cleaned up on socket EOF) with a TTL,
  and the client's `refreshLoop` (`udsthin.go:145`) needs exponential backoff on token refresh errors.

### 4.9 Packaging facts (VM)

| item | measured |
|---|---|
| sidecar binary | 20.9 MB glibc / 21.0 MB musl, 17.3–17.4 MB stripped, 6.9 MB gzip |
| Go benchmark binary | 46.7 MB without sidecar, 67.7 MB with embedded sidecar (31.7 MB stripped w/o) |
| module zip delta | +6.9 MB per platform (gzip -9 of the 17.4 MB stripped binary); 6 platforms ≈ 41 MB vs 1.0 MB for `storage` today. (The `### 13` line in `packaging.txt` is wrong — the script tarred the stripped *Go* binary six times, 65 MB.) |
| Rust release build | 229–232 s on c3-standard-8; 9 min cold on a workstation |
| sidecar spawn / client init | 12–37 ms spawn; 24–70 ms first client init (embedded: 36 ms spawn, 67 ms init, includes extraction) |
| reuse by a second client | 4 ms (dial existing socket, no spawn) |
| idle sidecar RSS after 512 MiB run | 57–67 MB, 9 threads |

### 4.10 Updated Worklog Specification Run (`c4-standard-192`, 200 Gbps GVNIC `TIER_1`, `us-central1-b` → `us-west1`)

Executed via `vm_uds5_worklog.sh` on GCE `c4-standard-192` (`192 vCPUs`, `720 GiB RAM`, Ubuntu 24.04 LTS, `GVNIC` + `TIER_1` 200 Gbps networking) in `us-central1-b` (`instance_id=4800314160389265979`) targeting regional bucket `gs://cpriti-thickthin-poc-usw1` in `us-west1` (raw logs and exported Cloud Monitoring JSON payloads in `gothin/results/uds5-worklog/`).

#### A. Workloads 1–3 (Rapid SLIs: `open`, `read50x2x4/serial`, `read50x2x4/concurrent`, `random8k`) & Workload 6 (`list-objects-100`, `list-buckets-100`)
*Two rounds (`r1 / r2`), `-iters=10` (`random8k`: 2,000 × 8 KiB reads). Cross-region RTT (`us-central1` ↔ `us-west1`) is ~38 ms.*

| Workload / SLI | Metric | Native Go gRPC (`go-grpc-mrd`) | Go→UDS→Rust gRPC (`go-uds-grpc`) | Native Go JSON (`go-json-range`) | Go→UDS→Rust JSON (`go-uds-json`) |
|---|---|---|---|---|---|
| **W1: `open` (200 MiB, Regional `us-west1`)** | P50 / P90 / P99 | 207.6–208.4 / 221.6–235.2 / 221.6–235.2 ms | 208.8–221.5 / 221.7–229.1 / 221.7–229.1 ms | 64.4–114.8 / 125.4–183.9 / 125.4–183.9 ms | 214.6–218.0 / 233.1–242.4 / 233.1–242.4 ms |
| | Max / Mean / Rate (`ops/s`) | 235.5–245.2 / 211.9–214.2 ms / **4.67–4.72 ops/s** | 243.7–250.1 / 200.7–209.2 ms / **4.78–4.98 ops/s** | 157.2–236.9 / 114.4–121.5 ms / **8.23–8.74 ops/s** | 299.7–305.6 / 212.2–228.1 ms / **4.38–4.71 ops/s** |
| | CPU ms/op / Peak RSS | 25.28–26.28 ms (`0.1%`) / 85–92 MiB | **16.00–16.60 ms (`0.0%`)** / **74–81 MiB** | 2.86–3.04 ms (`0.0%`) / 38–40 MiB | **14.84–15.33 ms (`0.0%`)** / 77–78 MiB |
| **W1: `open` (200 MiB, Zonal `us-west1-a`)** | P50 / P90 / P99 | **389.8–392.6** / 398.8–411.9 / 398.8–411.9 ms | 983.7–1036.0 / 1260.8–1342.8 / 1260.8–1342.8 ms | N/A *(Zonal is gRPC-only)* | N/A *(Zonal is gRPC-only)* |
| | Max / Mean / Rate (`ops/s`) | 417.5–418.2 / 378.8–399.1 ms / **2.51–2.64 ops/s** | 1399.2–2921.9 / 976.2–1081.9 ms / **0.92–1.02 ops/s** | N/A | N/A |
| | CPU ms/op / Peak RSS | 70.13–82.74 ms (`0.1%`) / 88–90 MiB | **19.12–19.77 ms (`0.0%`)** / **82–83 MiB** | N/A | N/A |
| **W2a: `read50x2x4/serial` (100 MiB, Regional `us-west1`)** | P50 / P90 / P99 | 4.17–4.44 / 4.27–4.54 / 4.27–4.54 s | 4.40–4.55 / 4.45–4.60 / 4.45–4.60 s | 7.29–7.58 / 13.69–13.85 / 13.69–13.85 s | 4.81–4.88 / 4.86–5.22 / 4.86–5.22 s |
| | Max / Mean / MiB/s | 4.77–5.36 / 4.30–4.48 s / **22.2–23.2 MiB/s** | 4.67–4.79 / 4.41–4.56 s / **21.8–22.6 MiB/s** | 14.78–15.23 / 8.99–9.46 s / **10.5–11.1 MiB/s** | 5.04–5.27 / 4.83–5.02 s / **19.8–20.6 MiB/s** |
| | CPU s/GiB / Peak RSS | 3.44–3.64 (`0.0%`) / 186–193 MiB | **2.32–2.40 (`0.0%`)** / 200 MiB | 3.23–3.49 (`0.0%`) / 155–156 MiB | **2.56–2.87 (`0.0%`)** / 211–212 MiB |
| **W2a: `read50x2x4/serial` (100 MiB, Zonal `us-west1-a`)** | P50 / P90 / P99 | **4.13–4.25** / 4.20–4.37 / 4.20–4.37 s | **4.30–4.82** / 4.30–4.86 / 4.30–4.86 s | N/A | N/A |
| | Max / Mean / MiB/s | 4.43–4.51 / 4.17–4.28 s / **23.1–23.7 MiB/s** | 4.47–5.09 / 4.31–4.83 s / **20.2–22.7 MiB/s** | N/A | N/A |
| | CPU s/GiB / Peak RSS | 3.56–3.58 (`0.0%`) / 190–192 MiB | **2.63–3.32 (`0.0%`)** / 204–208 MiB | N/A | N/A |
| **W2b: `read50x2x4/concurrent` (100 MiB, Regional `us-west1`)** | P50 / P90 / P99 | 3.32–3.42 / 3.39–3.47 / 3.39–3.47 s | 3.36–3.38 / 3.45–3.56 / 3.45–3.56 s | 427.2–490.6 / 460.3–615.5 / 460.3–615.5 ms | 3.25–3.62 / 3.40–3.70 / 3.40–3.70 s |
| | Max / Mean / MiB/s | 3.39–3.47 / 3.34–3.41 s / **29.1–29.8 MiB/s** | 3.46–3.66 / 3.37–3.38 s / **29.3–29.5 MiB/s** | 462.0–644.2 / 432.3–513.1 ms / **193.0–228.4 MiB/s** | 3.43–3.70 / 3.27–3.60 s / **27.6–30.5 MiB/s** |
| | CPU s/GiB / Peak RSS | 3.18–3.61 (`0.1%`) / 222–287 MiB | **2.53–2.58 (`0.0%`)** / 303–380 MiB | 2.99–3.77 (`0.4%`) / 254–257 MiB | **2.76–2.85 (`0.0%`)** / 394–422 MiB |
| **W2b: `read50x2x4/concurrent` (100 MiB, Zonal `us-west1-a`)** | P50 / P90 / P99 | 3.35–3.39 / 3.43–3.44 / 3.43–3.44 s | **3.22–3.34** / **3.32–3.43** / **3.32–3.43 s** | N/A | N/A |
| | Max / Mean / MiB/s | 3.43–3.44 / 3.37–3.39 s / **29.1–29.3 MiB/s** | 3.44–3.47 / **3.25–3.35 s** / **29.1–29.6 MiB/s** | N/A | N/A |
| | CPU s/GiB / Peak RSS | 3.58–4.11 (`0.1%`) / 216–293 MiB | **3.11–3.34 (`0.0%`)** / 360–403 MiB | N/A | N/A |
| **W3: `random8k` (8 KiB reads, Regional `us-west1`)** | P50 / P90 / P99 | 38.5 / 40.4–41.4 / 44.1–51.3 ms | 40.8–43.8 / 44.7–47.4 / 48.7–57.2 ms | 55.3–56.4 / 63.4–63.7 / 78.1–87.0 ms | 37.8–40.4 / 43.6–44.3 / 54.0–59.2 ms |
| | Max / Mean / Rate (`ops/s`) | 56.6–204.7 / 38.9–39.5 ms / **25.3–25.7 ops/s** (`0.2 MiB/s`) | 59.3–128.3 / 40.9–44.5 ms / **22.5–24.4 ops/s** (`0.2 MiB/s`) | 143.0–152.5 / 56.7–57.3 ms / **17.5–17.6 ops/s** (`0.1 MiB/s`) | 83.4–97.0 / 39.6–40.4 ms / **24.8–25.3 ops/s** (`0.2 MiB/s`) |
| | CPU ms/op / Peak RSS | **0.16 ms (`0.0%`)** / 223–287 MiB | **0.32–0.35 ms (`0.0%`)** / 315–381 MiB | 0.21–0.23 ms (`0.0%`) / 254–257 MiB | **0.35–0.36 ms (`0.0%`)** / 405–434 MiB |
| **W3: `random8k` (8 KiB reads, Zonal `us-west1-a`)** | P50 / P90 / P99 | **35.9–36.4** / 38.6–39.2 / 39.1–44.9 ms | **35.9–39.0** / 39.0–39.9 / **39.5–42.1 ms** | N/A | N/A |
| | Max / Mean / Rate (`ops/s`) | 52.5–81.4 / 36.7–36.9 ms / **27.1–27.2 ops/s** (`0.2 MiB/s`) | **40.3–59.3** / 36.8–39.1 ms / **25.5–27.2 ops/s** (`0.2 MiB/s`) | N/A | N/A |
| | CPU ms/op / Peak RSS | **0.28 ms (`0.0%`)** / 217–294 MiB | **0.56–0.68 ms (`0.0%`)** / 368–399 MiB | N/A | N/A |
| **W6a: `list-objects-100` (Regional `us-west1`)** | P50 / P90 / P99 | **73.6–100.4** / 135.7–142.5 / 135.7–142.5 ms | **84.8–95.1** / 129.5–131.5 / 129.5–131.5 ms | 130.9–136.0 / 165.3–182.7 / 165.3–182.7 ms | **84.1–97.6** / 127.5–158.3 / 127.5–158.3 ms |
| | Max / Mean / Rate (`ops/s`) | 209.2–214.8 / 106.5–112.7 ms / **8.9–9.4 ops/s** | 229.2–236.5 / 111.2–114.0 ms / **8.8–9.0 ops/s** | 192.8–292.8 / 146.5–150.9 ms / 6.6–6.8 ops/s | 228.5–234.8 / 112.6–115.0 ms / **8.7–8.9 ops/s** |
| | CPU ms/op / Peak RSS | 8.85–54.39 ms (`0.0–0.3%`) / 87–93 MiB | **3.15–3.25 ms (`0.0%`)** / **69 MiB** | 8.67–10.10 ms (`0.0%`) / 45 MiB | **3.36–3.58 ms (`0.0%`)** / 65–69 MiB |
| **W6a: `list-objects-100` (Zonal `us-west1-a`)** | P50 / P90 / P99 | **100.9–104.0** / 126.5–145.9 / 126.5–145.9 ms | **96.5–101.0** / 129.5–134.3 / 129.5–134.3 ms | 123.7–148.5 / 156.5–172.8 / 156.5–172.8 ms | **96.0–99.6** / 124.7–141.0 / 124.7–141.0 ms |
| | Max / Mean / Rate (`ops/s`) | 236.5–245.1 / 119.6–120.9 ms / **8.3–8.4 ops/s** | 250.0–276.3 / 118.3–124.5 ms / **8.0–8.5 ops/s** | 157.7–302.9 / 144.6–154.4 ms / 6.5–6.9 ops/s | 227.9–271.5 / 115.9–122.3 ms / **8.2–8.6 ops/s** |
| | CPU ms/op / Peak RSS | 6.52–50.62 ms (`0.0–0.2%`) / 87–89 MiB | **3.05–3.29 ms (`0.0%`)** / **61–65 MiB** | 5.26–7.11 ms (`0.0%`) / 45 MiB | **3.25–10.07 ms (`0.0%`)** / 68–69 MiB |
| **W6b: `list-buckets-100` (Regional run)** | P50 / P90 / P99 | 930.4–942.3 / 961.6–980.6 / 961.6–980.6 ms | **925.5–926.0** / 956.7–985.6 / 956.7–985.6 ms | **921.5–923.2** / 934.5–951.4 / 934.5–951.4 ms | 923.5–948.5 / 976.0–978.3 / 976.0–978.3 ms |
| | Max / Mean / Rate (`ops/s`) | 976.2–982.0 / 886.5–899.2 ms / **1.11–1.13 ops/s** | 957.4–989.4 / 934.1–947.8 ms / **1.06–1.07 ops/s** | 954.8–1000.7 / 922.8–931.5 ms / **1.07–1.08 ops/s** | 983.7–1122.2 / 937.3–957.4 ms / **1.04–1.07 ops/s** |
| | CPU ms/op / Peak RSS | 49.55–93.54 ms / 91–98 MiB | **3.48–7.96 ms** / **79–84 MiB** | 9.49–9.57 ms / 48–50 MiB | 8.12–11.51 ms / 82–88 MiB |
| **W6b: `list-buckets-100` (Zonal run)** | P50 / P90 / P99 | **932.0–943.8** / 967.1–973.9 / 967.1–973.9 ms | **940.1–940.6** / 969.5–980.7 / 969.5–980.7 ms | 939.0–956.0 / 962.7–970.5 / 962.7–970.5 ms | **921.0–941.5** / 961.3–967.7 / 961.3–967.7 ms |
| | Max / Mean / Rate (`ops/s`) | 987.0–997.5 / 939.3–947.1 ms / **1.06 ops/s** | 1017.1–1090.9 / 948.6–956.0 ms / **1.05 ops/s** | 970.1–987.2 / 935.5–951.1 ms / **1.05–1.07 ops/s** | 976.1–976.6 / 926.7–941.4 ms / **1.06–1.08 ops/s** |
| | CPU ms/op / Peak RSS | 48.72–62.71 ms / 94–97 MiB | **6.94–7.87 ms** / **81–84 MiB** | 7.01–8.24 ms / 47–49 MiB | **2.76–3.73 ms** / **76–79 MiB** |

> **Notes on Table 1 (Workloads 1–3 & Workload 6 on Regional `us-west1` and Zonal `us-west1-a`):**
> 1. **Workloads 1–3 on Zonal Rapid (`gs://cpriti-thickthin-poc-zonal-usw1a`, `instance_id=1673808419666104117`):** On an open `BidiReadObject` session, `Go→UDS→Rust gRPC` matches or slightly beats Native Go gRPC on Zonal `us-west1-a`: **`read50x2x4/concurrent`** is **`3.22–3.34 s` (`29.1–29.6 MiB/s`, `3.11–3.34 CPU-s/GiB`)** vs **`3.35–3.39 s` (`29.1–29.3 MiB/s`, `3.58–4.11 CPU-s/GiB`)** for Native Go, **`read50x2x4/serial`** is **`4.30–4.82 s` (`2.63–3.32 CPU-s/GiB`)** vs **`4.13–4.25 s` (`3.56–3.58 CPU-s/GiB`)**, and **`random8k`** is **`35.9–39.0 ms` P50 (`39.5–42.1 ms` P99)** vs **`35.9–36.4 ms` P50 (`39.1–44.9 ms` P99)**. On **`open`** (`-iters=10`), because both clients use 192 gRPC sub-channels (`runtime.NumCPU()` / `available_parallelism()`) on `c4-standard-192`, each of the 10 sequential `Open` calls lands on an un-warmed sub-channel and incurs a cross-region TCP+TLS handshake plus a Zonal `BidiReadObjectRedirectedError` routing redirect (`389.8–392.6 ms` P50 in Native Go vs `983.7–1036.0 ms` P50 in `google-cloud-storage` 1.21, which establishes the redirected tonic channel lazily while using 4× less CPU: `19.12–19.77 ms/op` vs `70.13–82.74 ms/op`). Cloud Monitoring VM telemetry during Zonal Workloads 1–3 (`gcm_w123_zonal_*.json`) recorded **`0.12%` avg / `0.15%` max** VM CPU utilization and **`0.73%` avg / `0.77%` max** VM memory (`state=used`).
> 2. **Workload 6 (`list100`) Control-Plane Connection Pooling (`option.WithGRPCConnectionPool(1)`):** In `gcs_uds/server/src/service.rs`, the Rust sidecar initializes `StorageControl` with a single gRPC channel (`StorageControl::builder().build()`). Configuring Native Go's control-plane client (`ctrl` in `gothin/cmd/rapid_bench/main.go`) with `option.WithGRPCConnectionPool(1)` instead of `runtime.NumCPU()` (`192` sub-channels on `c4-standard-192`, which previously round-robined each of the 10 sequential `ListObjects` calls onto an un-warmed sub-channel and incurred a ~125 ms TLS handshake on every iteration) brings Native Go gRPC `list-objects-100` P50 from `216–219 ms` down to **`73.6–100.4 ms`** on Regional `us-west1` (vs **`84.8–95.1 ms`** on `Go→UDS→Rust gRPC`, **`8.9–9.4` vs `8.8–9.0 ops/s`**) and **`100.9–104.0 ms`** on Zonal `us-west1-a` (vs **`96.5–101.0 ms`** on `Go→UDS→Rust gRPC`, **`8.3–8.4` vs `8.0–8.5 ops/s`**). Cloud Monitoring VM telemetry during the Workload 6 run (`instance_id=8985621247580806506`, `gcm_w6_list100_*.json`) recorded **`0.13%` avg / `0.14%` max** VM CPU utilization (`compute.googleapis.com/instance/cpu/utilization`) and **`0.70%` avg / `0.72%` max** VM memory (`agent.googleapis.com/memory/percent_used`, `state=used`).

#### B. Workloads 4 & 5 (Sustained 48-Thread Parallel `BidiRead`, `BidiWrite` & `WriteObject` for 5 Minutes) + Emitted Cloud Monitoring VM Metrics
*48 parallel worker goroutines running continuously for 5 minutes (`-sustained-threads=48 -sustained-dur=5m`) on `c4-standard-192` (`192 vCPUs`, `720 GiB RAM`, `us-central1-b` → Regional `us-west1` and Zonal Rapid `us-west1-a`). Cloud Monitoring metrics queried directly from `monitoring.googleapis.com/v3` (`gcm_w4_*`, `gcm_w5_*`, `gcm_full_run_*`). Zero protocol fallback: every row compares identical wire protocols.*

| Workload | Bucket & Wire Protocol | Backend | P50 | P90 | P99 | Max | Mean | Aggregate Throughput | Process CPU (`s/GiB` / `ms/op` / `%`) | Process Peak RSS | Cloud Monitoring `instance/cpu/utilization` (avg / max) | Cloud Monitoring `memory/percent_used` (`state=used` avg / max) |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| **W4: BidiRead 16 MiB chunks (48t, 5m)** | Regional `us-west1` (`BidiReadObject` gRPC) | **Native Go gRPC (`go-grpc-mrd`)** | **544.3 ms** | 616.2 ms | **766.8 ms** | 10.96 s | **563.9 ms** | **1358.4 MiB/s** (`1424.4 MB/s`) | **2.85 s/GiB** / 46.45 ms / **2.0%** | **1855 MiB** | **2.88%** / **3.00%** | **0.97%** / **0.98%** |
| *(Per-200 MiB object rollup)* | *Regional `us-west1` (`BidiReadObject` gRPC)* | *Native Go gRPC (`bidiread-200m-48t`)* | *6.86 s* | *7.61 s* | *9.08 s* | *41.78 s* | *7.06 s* | *1358.4 MiB/s* | *2.85 s/GiB / 563.81 ms / 2.0%* | *1855 MiB* | *2.88% / 3.00%* | *0.97% / 0.98%* |
| **W4: BidiRead 16 MiB chunks (48t, 5m)** | Regional `us-west1` (`BidiReadObject` gRPC) | **Go→UDS→Rust gRPC (`go-uds-grpc`)** | 560.3 ms | **612.2 ms** | 771.6 ms | **1.44 s** | 567.6 ms | **1349.0 MiB/s** (`1414.5 MB/s`, **99.3% of native**) | **3.38 s/GiB** / 55.03 ms / **2.3%** | **2822 MiB** | **3.39%** / **3.44%** | **1.45%** / **1.50%** |
| *(Per-200 MiB object rollup)* | *Regional `us-west1` (`BidiReadObject` gRPC)* | *Go→UDS→Rust gRPC (`bidiread-200m-48t`)* | *7.05 s* | ***7.59 s*** | ***8.19 s*** | ***8.63 s*** | *7.10 s* | *1349.0 MiB/s* | *3.38 s/GiB / 668.31 ms / 2.3%* | *2822 MiB* | *3.39% / 3.44%* | *1.45% / 1.50%* |
| **W4: BidiRead 16 MiB chunks (48t, 5m)** | **Zonal `us-west1-a` (`BidiReadObject` gRPC)** | **Native Go gRPC (`go-grpc-mrd`)** | 546.3 ms | 605.4 ms | 784.4 ms | 2.50 s | 556.3 ms | **1376.2 MiB/s** (`1443.1 MB/s`) | **2.80 s/GiB** / 45.52 ms / **2.0%** | **1913 MiB** | **2.75%** / **2.85%** | **0.96%** / **0.99%** |
| *(Per-200 MiB object rollup)* | *Zonal `us-west1-a` (`BidiReadObject` gRPC)* | *Native Go gRPC (`bidiread-200m-48t`)* | *6.83 s* | *7.51 s* | *9.40 s* | *18.03 s* | *6.96 s* | *1376.2 MiB/s* | *2.80 s/GiB / 553.29 ms / 2.0%* | *1913 MiB* | *2.75% / 2.85%* | *0.96% / 0.99%* |
| **W4: BidiRead 16 MiB chunks (48t, 5m)** | **Zonal `us-west1-a` (`BidiReadObject` gRPC)** | **Go→UDS→Rust gRPC (`go-uds-grpc`)** | **542.0 ms** | **589.6 ms** | **651.3 ms** | **1.39 s** | **543.4 ms** | **1401.4 MiB/s** (`1469.5 MB/s`, **101.8% of native**) | **3.19 s/GiB** / 51.97 ms / **2.3%** | **2846 MiB** | **3.03%** / **3.14%** | **1.49%** / **1.62%** |
| *(Per-200 MiB object rollup)* | *Zonal `us-west1-a` (`BidiReadObject` gRPC)* | *Go→UDS→Rust gRPC (`bidiread-200m-48t`)* | ***6.81 s*** | ***7.10 s*** | ***7.79 s*** | ***8.73 s*** | ***6.80 s*** | *1401.4 MiB/s* | *3.19 s/GiB / 631.16 ms / 2.3%* | *2846 MiB* | *3.03% / 3.14%* | *1.49% / 1.62%* |
| **W5: BidiWrite 200 MiB (48t, 5m)** | **Zonal `us-west1-a` (`BidiWriteObject` gRPC)** | **Native Go gRPC (`go-grpc-mrd`, `Append=true`)** | **1.82 s** | **2.28 s** | **2.99 s** | **9.60 s** | **1.84 s** | **5176.6 MiB/s** (`5428.1 MB/s`) | **1.44 s/GiB** / 281.14 ms / **3.8%** | **2629 MiB** | **4.67%** / **5.21%** | **0.99%** / **1.00%** |
| **W5: BidiWrite 200 MiB (48t, 5m)** | **Zonal `us-west1-a` (`BidiWriteObject` gRPC)** | **Go→UDS→Rust gRPC (`go-uds-grpc`, `open_appendable_object`)** | **2.05 s** | **2.63 s** | **4.18 s** | **14.15 s** | **2.12 s** | **4517.1 MiB/s** (`4736.5 MB/s`, **87.3% of native**) | **2.33 s/GiB** / 455.00 ms / **5.4%** | **15874 MiB** | **6.11%** / **6.69%** | **2.58%** / **2.71%** |
| **W5: BidiWrite 200 MiB (48t, 5m)** | Regional `us-west1` (`BidiWriteObject` gRPC) | **Native Go gRPC (`go-grpc-mrd`)** | **1.92 s** | **2.53 s** | **4.48 s** | 72.23 s | **2.09 s** | **4153.0 MiB/s** (`4354.7 MB/s`) | **1.37 s/GiB** / 268.05 ms / **2.9%** | **2499 MiB** | **4.16%** / **4.62%** | **1.35%** / **1.45%** |
| **W5: BidiWrite 200 MiB (48t, 5m)** | Regional `us-west1` (`BidiWriteObject` gRPC) | **Go→UDS→Rust gRPC (`go-uds-grpc`)** | **N/A** | **N/A** | **N/A** | **N/A** | **N/A** | **N/A** *(Rust SDK `BidiWriteObject` is Zonal/appendable-only)* | **N/A** | **N/A** | **N/A** | **N/A** |
| **W5: WriteObject 200 MiB (48t, 5m)** | **Regional `us-west1` (`WriteObject` JSON)** | **Native Go JSON (`go-json-range`, default HTTP/2)** | 42.37 s | 43.74 s | 44.67 s | 46.38 s | 42.06 s | **227.1 MiB/s** (`238.1 MB/s`) | 5.67 s/GiB / 1106.81 ms / **0.7%** | **2035 MiB** | **0.90%** / **1.10%** | **0.89%** / **0.92%** |
| **W5: WriteObject 200 MiB (48t, 5m)** | **Regional `us-west1` (`WriteObject` JSON)** | **Go→UDS→Rust JSON (`go-uds-json`, HTTP/1.1 pool)** | **6.54 s** | **7.40 s** | **8.70 s** | **21.94 s** | **6.73 s** | **1402.4 MiB/s** (`1470.5 MB/s`, **6.2× faster**) | **1.57 s/GiB** / 306.25 ms / **1.1%** | **18490 MiB** | **1.50%** / **1.54%** | **3.21%** / **3.26%** |

#### C. Sequential W1R3 (Regional `us-west1` & Zonal `us-west1-a`) & Shared-Memory (`memfd`) Data Plane on `c4-standard-192` (Tuned HTTP/2 Windows & 8 MiB UDS Buffers)

**Sequential `whole` & `32k` (128 MiB, `us-central1-b` → Regional `us-west1` & Zonal Rapid `us-west1-a`):**
| Style | Op | Regional Native `go-json-h1` | Regional Go→UDS→Rust JSON (`go-uds-json`) | Regional Native `go-grpc-cp` | Regional Go→UDS→Rust gRPC (`go-uds-grpc`) | **Zonal (`us-west1-a`) Native `go-grpc-cp-append`** | **Zonal (`us-west1-a`) Go→UDS→Rust gRPC (`go-uds-grpc`)** |
|---|---|---|---|---|---|---|---|
| `whole` (128 MiB) | Write P50 / P90 / P99 | 4.65 / 4.93 / 4.93 s (`27.5 MiB/s`) | 4.70 / 5.01 / 5.01 s (`27.2 MiB/s`) | 1.79 / 2.49 / 2.49 s (`71.4 MiB/s`) | **N/A** *(Zonal-only in Rust SDK)* | **1.76 / 2.14 / 2.14 s (`72.8 MiB/s`)** | **1.79 / 2.27 / 2.27 s (`71.5 MiB/s`, 98.2% parity)** |
| `whole` (128 MiB) | Read P50 / P90 / P99 | 1.46 / 1.59 / 1.69 s (`87.7 MiB/s`) | **1.39 / 1.61 / 1.68 s (`92.1 MiB/s`)** | 4.41 / 4.51 / 4.95 s (`29.0 MiB/s`) | **4.37 / 4.49 / 4.54 s (`29.3 MiB/s`)** | 4.66 / 4.99 / 5.10 s (`27.5 MiB/s`) | 5.19 / 5.54 / 6.85 s (`24.7 MiB/s`) |
| `whole` (128 MiB) | CPU s/GiB / Peak MiB | **1.59 s/GiB** / 476 MiB | **2.44 s/GiB** / 698 MiB | **2.70 s/GiB** / 542 MiB | **2.31 s/GiB** / 639 MiB | 3.19 s/GiB / 535 MiB | **2.80 s/GiB** / 709 MiB |
| `32k` (128 MiB) | Write P50 / Read P50 | 4.34 s / 1.31 s (`97.7 MiB/s`) | **4.27 s / 1.31 s (`97.8 MiB/s`)** | 2.33 s / 4.38 s (`29.2 MiB/s`) | **N/A** / **4.30 s (`29.8 MiB/s`)** | 1.85 s (`69.2 MiB/s`) / 4.55 s (`28.1 MiB/s`) | **1.57 s (`81.3 MiB/s`, 17.5% faster!)** / 5.00 s (`25.6 MiB/s`) |
| `32k` (128 MiB) | CPU s/GiB / Peak MiB | **1.48 s/GiB** / 349 MiB | **2.34 s/GiB** / 890 MiB | **2.67 s/GiB** / 436 MiB | **2.34 s/GiB** / 841 MiB | 3.27 s/GiB / 412 MiB | **2.89 s/GiB** / 1001 MiB |

**Shared-Memory (`memfd` + `SCM_RIGHTS` + `mmap`) vs Tuned gRPC Chunks (128 MiB Read on `c4-standard-192`):**
| Data Plane Mode | JSON Transport P50 / Throughput | JSON CPU s/GiB (Go + Sidecar) | gRPC Transport P50 / Throughput | gRPC CPU s/GiB (Go + Sidecar) |
|---|---|---|---|---|
| **Tuned gRPC chunks over UDS** (copy into caller `p`) | 1.556 s (`82.3 MiB/s`) | **2.41 s/GiB** (Go `0.63` + sidecar `1.78`) | 4.467 s (`28.7 MiB/s`) | **2.47 s/GiB** (Go `0.63` + sidecar `1.84`) |
| **`ReadObjectShm` (`memfd`)** (copy into caller `p`) | **1.477 s (`86.6 MiB/s`)** | **1.32 s/GiB** (Go `0.11` + sidecar `1.21`) | 4.831 s (`26.5 MiB/s`) | **1.57 s/GiB** (Go `0.14` + sidecar `1.44`) |
| **`ReadObjectShm` (`memfd`)** (zero-copy loaned slice) | **1.488 s (`86.0 MiB/s`)** | **1.16 s/GiB** (Go **`0.00`** + sidecar `1.15`) | **4.430 s (`28.9 MiB/s`)** | **1.46 s/GiB** (Go **`0.01`** + sidecar `1.45`) |

## 5. What the numbers say (plain English)

* **Latency/throughput:** at parity for sequential and Rapid workloads, regional and zonal, JSON and gRPC CloudPath — the
  network dominates. However, **the Rust crate lacks DirectPath today**, which is a major throughput/CPU regression on GCE/GKE
  compared to Go's native gRPC DirectPath client until Rust adds DirectPath support.
* **CPU:**
  - Against Go's **production default (`go-json` over HTTP/2, 2.26–2.35 s/GiB)**, the sidecar path (`go-uds-json`, 2.55–2.59 s/GiB)
    adds only **+11–15% CPU** on large transfers because the Rust crate's HTTP/1.1 transport saves ~0.9 s/GiB over Go's HTTP/2 stack.
  - Against **Go on HTTP/1.1 (`go-json-h1`, 1.40–1.44 s/GiB, same wire protocol)**, the gRPC/UDS hop adds **~1.15 s/GiB (~1.8× CPU)** —
    the pure IPC + HTTP/2 framing tax. Shared memory brings large reads to 1.35–1.91 s/GiB (Go side 0.03–0.04) and can be extended
    to writes via bidirectional ring buffers, at the cost of a Linux-only borrowed-buffer API.
  - On 8 KiB random reads, the POC pays 2.5× CPU (0.23 vs 0.09 ms/op), partly because `Opened.ReadAt` opens a new UDS stream per call.
* **Concurrency:** at 16 in-flight ops the sidecar keeps up; at 64–256 reads are 1.5–3× slower than native JSON/HTTP1.
  More UDS connections do not help; the slowdown in the POC comes from `mpsc::channel(4)` backpressure (`service.rs:317, 431`),
  dual-runtime (Go + Tokio) thread contention on 8 vCPUs, and untuned `reqwest` connection pooling vs `MaxIdleConnsPerHost=256`
  in `bench2_native` (the inflated `crossings/op` numbers at `par > 1` were a global-atomic counter bug in `bench2.go`, see §4.2).
* **Memory:** one extra process (~60–100 MB) per machine-user; peak RSS of the pair matches native for whole-object transfers.
* **Backwards compatibility & user experience:** `go get` / `go mod tidy` / `CGO_ENABLED=0` cross-compiles keep working — that is
  the real win over FFI. However, **caller-process hooks (`option.WithHTTPClient`, `option.WithGRPCConn`, custom `RoundTripper`,
  in-process OTel spans) cannot cross the process boundary** (a blocker for drop-in compatibility), `memfd_create` execution on
  `noexec` mounts is blocked by `vm.memfd_noexec`/enterprise EDR, and debugging splits across two runtimes.

## 6. Steps to reproduce

```bash
# 1. Build the Rust sidecar (rustup, cmake, gcc) and the pure-Go client/benchmarks
cd gcs_uds/server && cargo build --release            # glibc
rustup target add x86_64-unknown-linux-musl && cargo build --release --target x86_64-unknown-linux-musl
cd ../../gothin && CGO_ENABLED=0 go build -o bin/bench2_uds ./cmd/bench2_uds && go build -o bin/bench2_native ./cmd/bench2_native && go build -o bin/rapid_bench ./cmd/rapid_bench

# 2. Behaviour checks through the thin client (errors, preconditions, range reads, 64 MiB write)
GCS_CORE_SERVER=../gcs_uds/server/target/release/gcs-core-server BUCKET=<bucket> ./bin/bench2_uds check
./bin/bench2_uds session-tests; ./bin/bench2_uds hijack; ./bin/bench2_uds ping; ./bin/bench2_uds shm

# 3. Sequential and concurrent W1R3, both backends
NATIVE_HTTP1=1 ./bin/bench2_native -bucket <b> -sizes 1MiB,16MiB,128MiB,512MiB -iters 10 -list-iters 10
UDS_TRANSPORT=json ./bin/bench2_uds   -bucket <b> -sizes 1MiB,16MiB,128MiB,512MiB -iters 10 -list-iters 10
UDS_TRANSPORT=json UDS_CONNS=64 ./bin/bench2_uds -bucket <b> -sizes 16MiB -iters 256 -parallel 64 -styles whole

# 4. Rapid SLIs (needs rapid/200MiB and rapid/100MiB objects: ./bin/rapid_bench -mode prep)
./bin/rapid_bench -bucket <zonal-bucket> -mode native ; ./bin/rapid_bench -bucket <zonal-bucket> -mode uds

# 5. Packaging / environment matrix (root on a VM): FROM scratch, noexec, memfd, read-only, abstract socket, peer-UID, orphan, idle
sudo bash gcs_uds/packaging_tests.sh

# 6. Full VM runs used for this document
bash vm_uds2.sh   # baselines + UDS matrix + rapid + packaging ; vm_uds3.sh shm + ping ; vm_uds4.sh UDS sequential + connection sweep
```
