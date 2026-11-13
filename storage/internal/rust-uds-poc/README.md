# Go GCS SDK over a Shared Rust Core as a gRPC/UDS Sidecar — Proof of Concept

**Not for production. Not part of the `cloud.google.com/go/storage` module.**

- **Full Deep-Dive Evaluation:** [EVALUATION.md](./EVALUATION.md)
- **Architecture & End-to-End Walkthrough:** [ARCHITECTURE_WALKTHROUGH.md](./ARCHITECTURE_WALKTHROUGH.md)
- **Worklog Summary ("gRPC over UDS Approach" — Go column):** [UDS_DOC_TEXT.md](./UDS_DOC_TEXT.md)
- **Rust Sidecar Crate PR:** [`googleapis/google-cloud-rust#7350`](https://github.com/googleapis/google-cloud-rust/pull/7350) (`cpriti-os/google-cloud-rust` branches `grpc-uds` / `feat/storage-grpc-uds-sidecar`)
- **Raw Logs & Cloud Monitoring JSON:** [`gothin/results/uds5-worklog/`](./gothin/results/uds5-worklog)

---

## 1. Architecture & POC Summary (POC Row #2 — Go)

**Go thin client (pure Go, `CGO_ENABLED=0`) → gRPC over UDS → Rust sidecar (`gcs-core-server`, `tonic` + `google-cloud-storage` 1.21, [PR #7350](https://github.com/googleapis/google-cloud-rust/pull/7350)).**

- **Lifecycle & Packaging:** Sidecar embedded via `//go:embed`, extracted to `~/.cache/gcs-core/<hash>/` (`memfd_create(2)` fallback on `noexec`/read-only disks), one sidecar per OS UID shared across Go processes (connection-refcounted `active_conns` + idle timeout). Also exports `gcs_core_start_inproc` / `gcs_core_stop_inproc` (`staticlib`/`cdylib`, `include/gcs_core_uds.h`) for single-process embedded UDS deployment.
- **Auth & IPC:** Per-caller credential sessions (OAuth2 access tokens lent over UDS, bound to peer `uid+pid` via `SO_PEERCRED` with dead-PID reaping), tuned HTTP/2 flow control (`16 MiB` stream / `64 MiB` conn windows, `4 MiB` frames, `8 MiB` kernel `SO_SNDBUF`/`SO_RCVBUF`, zero-copy `RawCodec`), plus optional shared-memory data plane (`memfd` + `SCM_RIGHTS` + `mmap`).
- **Connection Pool Alignment:** Both Native Go and the Rust sidecar use `192` gRPC sub-channels (`1` per vCPU via `option.WithGRPCConnectionPool(runtime.NumCPU())` in Go and `with_grpc_subchannel_count` in Rust) on data-plane `Storage` streams, and `1` gRPC channel (`option.WithGRPCConnectionPool(1)` / `StorageControl::builder().build()`) on control-plane `ListObjects` / `ListBuckets` operations.

---

## 2. Key Findings & Challenges Table (Go Verdict & Measured Impact)

| Challenge | Go Verdict & Measured Impact |
|---|---|
| **Zero copy across UDS boundary** | **High effort (Blocker only if zero-copy over standard `io.Reader`/`io.Writer` without shm is required).** Over a gRPC/UDS socket every byte incurs dual kernel copies (~0.2 CPU‑s/GiB) plus HTTP/2 framing. With tuned 16 MiB/64 MiB HTTP/2 windows, 4 MiB frames, 8 MiB UDS buffers, and zero-copy `RawCodec`, 128 MiB `us-central1` → `us-west1` / `us-west1-a` transfers measure **2.31–2.89 CPU‑s/GiB** on UDS (`go-uds-grpc` **2.31–2.34** regional / **2.80–2.89** zonal, `go-uds-json` **2.34–2.44**) vs **2.67–3.27 CPU‑s/GiB** for native `go-grpc-cp` and **1.48–1.59 CPU‑s/GiB** for `go-json-h1`. On 48-thread 5-minute sustained workloads, UDS reaches **1,414.5 MB/s vs 1,424.4 MB/s** (**-0.7% diff**) on Regional gRPC `BidiRead` (`1,469.5 MB/s vs 1,443.1 MB/s`, **+1.8%** on Zonal) and **4,736.5 MB/s vs 5,428.1 MB/s** (**-12.7% diff**) on Zonal gRPC `BidiWrite`. Shared memory (`memfd` + `SCM_RIGHTS` + `mmap`) drops 128 MiB read CPU from **2.41–2.47 s/GiB** to **1.32–1.57 s/GiB** with `copy()` (`Go 0.11–0.14`) and **1.16–1.46 s/GiB** with a loaned zero-copy slice (**Go `0.00–0.01 s/GiB`**). |
| **Packaging and Distribution** | **High effort.** `go get` / `go mod tidy` and `CGO_ENABLED=0` cross-compilation keep working (main win over FFI). Cost: **+6.9 MB per platform** in the Go module zip (~41 MB for 6 platforms vs 1 MB today) and **+21 MB** per linked Go binary (17.4 MB stripped static musl sidecar). Rust side still requires a C toolchain per target (`aws-lc-sys`). |
| **Deployment in major customer environments** | **High effort, with hard failures unless mitigated.** Distroless/`FROM scratch` requires bundled `webpki-roots` CA certs (otherwise sidecar panics at startup); `noexec` `/tmp` blocks extracted binary execution, and the `memfd_create` fallback is blocked on hardened kernels (`vm.memfd_noexec=1/2` / MDWE) and flagged by enterprise EDR (Falcon/Defender); UDS path >108 bytes fails; Cloud Run/Functions/GKE pay a second process (~60–100 MB idle RSS, 8–37 ms spawn) per container instance. Peer-UID verification, dead-PID session/handle reaping, stale socket recovery, and connection-refcounted idle exit all pass. |
| **Mono Repo Concerns** | **High effort.** `google-cloud-go` would ship prebuilt binaries compiled from `google-cloud-rust` across separate release cadences, SBOMs, and CVE pipelines (a sidecar CVE forces a Go module release and vice-versa). Shared sidecar sockets require per-version namespacing to prevent version skew across binaries on the same host. |
| **Tool chain, coordination and maintenance costs** | **High effort.** Requires Go + Rust + C toolchains and a bidirectional proto contract (`gcscore.proto`); every new SDK option/header lands in proto + Rust + Go. Error mapping preserves HTTP/gRPC status codes and auth failures (`401`/`403`/`404`/`412`), but structured `google.rpc.Status.details` require explicit plumbing. |
| **Impedance mismatch** | **Blocker for 100% drop-in backwards compatibility.** Standard reads/writes/appendable writes/lists/preconditions map cleanly, and credentials cross as lent 1 h OAuth2 access tokens bound to `(uid, pid)` via `SO_PEERCRED`. **Unfixable across processes:** caller-process hooks (`option.WithHTTPClient` / custom `http.RoundTripper`, `option.WithGRPCConn`, in-process OTel span/meter providers, `context.Context` values) cannot cross the socket. **Current Rust crate gaps:** (1) lacks **DirectPath** on GCE/GKE; (2) `open_appendable_object` (gRPC `BidiWriteObject`) in `google-cloud-storage` 1.21 is **Zonal/appendable-only** (Regional gRPC `BidiWriteObject` is `N/A`; on Zonal `us-west1-a` where both use gRPC `BidiWriteObject`, UDS reaches `4,736.5 MB/s` vs `5,428.1 MB/s` native on 48-thread 200 MB writes). |
| **Size of the Binary** | **OK / High effort.** +21 MB per Go binary (17.4 MB stripped sidecar), +6.9 MB per platform in module zip. Peak Memory on the 720 GiB VM is `0.98%` (Native) vs `1.50%` (`udsthin`) on 48-thread Regional BidiRead (`0.99%` vs `1.62%` on Zonal), and `1.00%` (Native) vs `2.71%` (`udsthin`) on 48-thread Zonal BidiWrite due to `AppendableObjectWriter` per-stream `ReplayBuffer` retention in `google-cloud-storage` 1.21. |
| **Harder debugging experience** | **High effort.** Go stack traces, `pprof`, `-race`, and HTTP/gRPC tracing stop at the UDS boundary; sidecar panics/stalls surface to Go as generic `UNAVAILABLE` errors and require correlating separate sidecar logs, metrics (`Stats`), and core dumps. |
| **Direct copy of data into GPU buffers** | **N/A for Go today (Blocker across processes if ever added).** Today's Go SDK has no GPU/CUDA buffer API. If ever added, a separate sidecar process cannot write into caller CUDA buffers without CUDA IPC handles (`cudaIpcGetMemHandle`) and linking CUDA into the sidecar; `memfd` shm provides host RAM only. |

---

## 3. Benchmarking Results (`c4-standard-192`, `us-central1` VM → `us-west1` Regional & `us-west1-a` Zonal Buckets, 200 Gbps `TIER_1`)

- **Configuration:** GCE `c4-standard-192` (`192 vCPUs`, `720 GiB RAM`, Ubuntu 24.04 LTS, `GVNIC` + `TIER_1` 200 Gbps NIC) in `us-central1-b` targeting Standard Regional bucket `gs://cpriti-thickthin-poc-usw1` (`us-west1`) and Zonal Rapid bucket `gs://cpriti-thickthin-poc-zonal-usw1a` (`us-west1-a`) over CloudPath.
- **Metrics:**
  - **Latency (`P50 / P90 / P99`) & Throughput:** Measured at the outermost Go caller boundary.
  - **CPU Utilization:** Cloud Monitoring `compute.googleapis.com/instance/cpu/utilization`.
  - **Peak Memory:** Cloud Monitoring `agent.googleapis.com/memory/percent_used` (`state=used`).

### Open (Start a BidiRead streaming request on a 200 MB object)

#### Go

| **Metric** | **Standard Regional (`us-west1`): Native Go** | **Standard Regional (`us-west1`): `udsthin`** | **Standard Regional `% Diff`** | **Zonal (`us-west1-a`): Native Go** | **Zonal (`us-west1-a`): `udsthin`** | **Zonal `% Diff`** |
| --- | --- | --- | --- | --- | --- | --- |
| **Latency (P50 / P90 / P99)** | **208.4 ms** / **221.6 ms** / **221.6 ms** | 208.8 ms / 221.7 ms / 221.7 ms | **+0.2% / 0.0% / 0.0%** | **389.8 ms** / **398.8 ms** / **398.8 ms** | 983.7 ms / 1,260.8 ms / 1,260.8 ms | **+152.4% / +216.1% / +216.1%** |
| **Throughput** | 4.7 ops/s | **5.0 ops/s** | **+6.4%** | **2.6 ops/s** | 1.0 ops/s | **-61.5%** |
| **CPU Utilization** | **0.41%** | **0.41%** | **0.0%** | **0.12%** | **0.12%** | **0.0%** |
| **Peak Memory** | **0.76%** | 0.78% | **+2.6%** | **0.77%** | **0.77%** | **0.0%** |

### Read50x2x4 Serial (Read 50 ranges of 2 MB with 4 MB stride from a 200 MB object)

#### Go

| **Metric** | **Standard Regional (`us-west1`): Native Go** | **Standard Regional (`us-west1`): `udsthin`** | **Standard Regional `% Diff`** | **Zonal (`us-west1-a`): Native Go** | **Zonal (`us-west1-a`): `udsthin`** | **Zonal `% Diff`** |
| --- | --- | --- | --- | --- | --- | --- |
| **Latency (P50 / P90 / P99)** | **4.17 s** / **4.27 s** / **4.27 s** | 4.40 s / 4.45 s / 4.45 s | **+5.5% / +4.2% / +4.2%** | **4.25 s** / 4.37 s / 4.37 s | 4.30 s / **4.30 s** / **4.30 s** | **+1.2% / -1.6% / -1.6%** |
| **Throughput** | **24.3 MB/s** | 23.7 MB/s | **-2.6%** | **24.2 MB/s** | 23.8 MB/s | **-1.7%** |
| **CPU Utilization** | **0.41%** | **0.41%** | **0.0%** | **0.12%** | **0.12%** | **0.0%** |
| **Peak Memory** | **0.76%** | 0.78% | **+2.6%** | **0.77%** | **0.77%** | **0.0%** |

### Read50x2x4 Concurrent (Read 50 ranges of 2 MB with 4 MB stride from a 200 MB object)

#### Go

| **Metric** | **Standard Regional (`us-west1`): Native Go** | **Standard Regional (`us-west1`): `udsthin`** | **Standard Regional `% Diff`** | **Zonal (`us-west1-a`): Native Go** | **Zonal (`us-west1-a`): `udsthin`** | **Zonal `% Diff`** |
| --- | --- | --- | --- | --- | --- | --- |
| **Latency (P50 / P90 / P99)** | 3.42 s / 3.47 s / 3.47 s | **3.38 s** / **3.45 s** / **3.45 s** | **-1.2% / -0.6% / -0.6%** | 3.35 s / 3.44 s / 3.44 s | **3.22 s** / **3.32 s** / **3.32 s** | **-3.9% / -3.5% / -3.5%** |
| **Throughput** | 30.5 MB/s | **30.7 MB/s** | **+0.7%** | 30.5 MB/s | **31.0 MB/s** | **+1.7%** |
| **CPU Utilization** | **0.41%** | **0.41%** | **0.0%** | **0.12%** | **0.12%** | **0.0%** |
| **Peak Memory** | **0.76%** | 0.78% | **+2.6%** | **0.77%** | **0.77%** | **0.0%** |

### Random8K (Read 2,000 random 8 KiB ranges from an open 100 MB object)

#### Go

| **Metric** | **Standard Regional (`us-west1`): Native Go** | **Standard Regional (`us-west1`): `udsthin`** | **Standard Regional `% Diff`** | **Zonal (`us-west1-a`): Native Go** | **Zonal (`us-west1-a`): `udsthin`** | **Zonal `% Diff`** |
| --- | --- | --- | --- | --- | --- | --- |
| **Latency (P50 / P90 / P99)** | **38.5 ms** / **40.4 ms** / **44.1 ms** | 40.8 ms / 44.7 ms / 48.7 ms | **+6.0% / +10.6% / +10.4%** | **35.9 ms** / 39.2 ms / 44.9 ms | **35.9 ms** / **39.0 ms** / **39.5 ms** | **0.0% / -0.5% / -12.0%** |
| **Throughput** | **0.21 MB/s** (`25.7 ops/s`) | **0.21 MB/s** (`24.4 ops/s`) | **0.0%** (`-5.1% ops/s`) | **0.21 MB/s** (`27.2 ops/s`) | **0.21 MB/s** (`27.2 ops/s`) | **0.0%** |
| **CPU Utilization** | **0.41%** | **0.41%** | **0.0%** | **0.12%** | **0.12%** | **0.0%** |
| **Peak Memory** | **0.76%** | 0.78% | **+2.6%** | **0.77%** | **0.77%** | **0.0%** |

### Read 16 MB chunks of 200 MB via BidiRead on 48 Parallel threads and try to saturate the NIC, run workload for 5 mins.

#### Go

| **Metric** | **Standard Regional (`us-west1`): Native Go** | **Standard Regional (`us-west1`): `udsthin`** | **Standard Regional `% Diff`** | **Zonal (`us-west1-a`): Native Go** | **Zonal (`us-west1-a`): `udsthin`** | **Zonal `% Diff`** |
| --- | --- | --- | --- | --- | --- | --- |
| **Latency (P50 / P90 / P99)** | **544.3 ms** / 616.2 ms / **766.8 ms** | 560.3 ms / **612.2 ms** / 771.6 ms | **+2.9% / -0.6% / +0.6%** | 546.3 ms / 605.4 ms / 784.4 ms | **542.0 ms** / **589.6 ms** / **651.3 ms** | **-0.8% / -2.6% / -17.0%** |
| **Throughput** | **1,424.4 MB/s** | 1,414.5 MB/s | **-0.7%** | 1,443.1 MB/s | **1,469.5 MB/s** | **+1.8%** |
| **CPU Utilization** | **2.88%** | 3.39% | **+17.7%** | **2.75%** | 3.03% | **+10.2%** |
| **Peak Memory** | **0.98%** | 1.50% | **+53.1%** | **0.99%** | 1.62% | **+63.6%** |

### Write 200 MB objects via BidiWrite on 48 Parallel threads and try to saturate the NIC, run workload for 5 mins.

*Note: Standard Regional is **N/A** for `udsthin` on `BidiWriteObject` because `google-cloud-storage` (Rust) only supports `BidiWriteObject` via `open_appendable_object` on Zonal buckets.*

#### Go

| **Metric** | **Native Go** | **`udsthin`** | **`% Diff` (`udsthin` vs. Native)** |
| --- | --- | --- | --- |
| **Latency (P50 / P90 / P99)** | **1.82 s** / **2.28 s** / **2.99 s** | 2.05 s / 2.63 s / 4.18 s | **+12.6% / +15.4% / +39.8%** |
| **Throughput** | **5,428.1 MB/s** (`5,176.6 MiB/s`) | **4,736.5 MB/s** (`4,517.1 MiB/s`) | **-12.7%** (`87.3%` of Native) |
| **CPU Utilization** | **4.67%** | 6.11% | **+30.8%** |
| **Peak Memory** | **1.00%** | 2.71% | **+171.0%** |

### Perform List Bucket and List Object for 100 items

#### Go

| **Operation & Metric** | **Standard Regional gRPC: Native Go** | **Standard Regional gRPC: `udsthin`** | **Standard Regional `% Diff`** | **Zonal gRPC: Native Go** | **Zonal gRPC: `udsthin`** | **Zonal `% Diff`** |
| --- | --- | --- | --- | --- | --- | --- |
| **`ListObject` Latency (P50 / P90 / P99)** | **87.0 ms** / 139.1 ms / 139.1 ms | 90.0 ms / **130.5 ms** / **130.5 ms** | **+3.4% / -6.2% / -6.2%** | 102.4 ms / 136.2 ms / 136.2 ms | **98.8 ms** / **131.9 ms** / **131.9 ms** | **-3.5% / -3.2% / -3.2%** |
| **`ListObject` Throughput** | **9.1 ops/s** | 8.9 ops/s | **-2.2%** | **8.3 ops/s** | 8.2 ops/s | **-1.2%** |
| **`ListBucket` Latency (P50 / P90 / P99)** | 936.4 ms / **971.1 ms** / **971.1 ms** | **925.8 ms** / 971.2 ms / 971.2 ms | **-1.1% / 0.0% / 0.0%** | **937.9 ms** / **970.5 ms** / **970.5 ms** | 940.4 ms / 975.1 ms / 975.1 ms | **+0.3% / +0.5% / +0.5%** |
| **`ListBucket` Throughput** | **1.12 ops/s** | 1.06 ops/s | **-5.4%** | **1.06 ops/s** | 1.05 ops/s | **-0.9%** |
| **CPU Utilization** | **0.13%** | **0.13%** | **0.0%** | **0.13%** | **0.13%** | **0.0%** |
| **Peak Memory** | **0.72%** | **0.72%** | **0.0%** | **0.72%** | **0.72%** | **0.0%** |

---

### Visual Comparison Graphs (Native Go vs. `udsthin`)

![Graph 1: Throughput & Latency Across Workloads and Buckets](./docs/graph1_throughput_latency_comparison.png)

![Graph 2: CPU Efficiency & Zero-Copy Shared Memory (memfd)](./docs/graph2_cpu_and_shm_zerocopy.png)

![Graph 3: Cloud Monitoring CPU Utilization (%) & Peak Memory (%)](./docs/graph3_memory_and_vm_telemetry.png)

---

## 4. Repository Layout

- `gcs_uds/proto/gcscore.proto` — the local IPC contract (`Ping`, `ReadObject`,
  `OpenObject`/`ReadRange`/`CloseObject`, `WriteObject`, `ListObjects`, `ListBuckets`,
  `ReadObjectShm`, `CreateSession`/`UpdateToken`/`CloseSession`, `Stats`, `Shutdown`).
- `gcs_uds/server/` — Rust sidecar (`gcs-core-server`) over the unmodified
  `google-cloud-storage` crate: `tonic` on a Unix socket, peer-UID check, idle
  exit, parent-death signal, stale-socket cleanup, per-caller credential
  sessions (`session.rs`), shared-memory data plane (`shm.rs`), `Stats` RPC, and
  optional C-FFI in-process UDS entry points (`gcs_core_start_inproc` / `gcs_core_stop_inproc`
  in `include/gcs_core_uds.h`).
- `gcs_uds/packaging_tests.sh` — `FROM scratch` / `noexec` / `memfd` / read-only /
  UDS path length / peer-UID / orphan / stale socket / PID 1 / idle-exit /
  cross-compile experiments.
- `gothin/udsthin/` — pure-Go thin client (`CGO_ENABLED=0`), structured into
  production SDK files vs. POC measurement hooks:
  - **Core SDK API** (mirrors `cloud.google.com/go/storage`):
    - `client.go` — `Client`, `Options`, `NewClient`, `Close`, `BucketHandle`,
      `ObjectHandle`, gRPC/UDS dialer & error mapping.
    - `reader.go` — `Reader` (`NewReader`, `NewRangeReader`, `Read`, `Remain`,
      `Close`), `Opened` (`Open`, `ReadAt`, `Close`), and `MultiRangeDownloader`
      (`NewMultiRangeDownloader`, `Add`, `Wait`, `Close`).
    - `writer.go` — `Writer` (`NewWriter`, `Write`, `Close`, `Attrs`) with
      2 MiB client-stream chunking.
    - `list.go` — `ObjectIterator` (`Objects`, `Next`, `PageInfo`) and `BucketIterator` (`Buckets`).
    - `session.go` — per-`Client` OAuth2 `TokenSource` lending over UDS
      (`CreateSession`, background `UpdateToken` refresh with backoff,
      `CloseSession`, and gRPC `x-gcs-session` metadata interceptors).
  - **Sidecar lifecycle & zero-copy transport**:
    - `sidecar.go` — process spawn, UDS readiness probe, parent-death signal,
      graceful shutdown (`sysproc_unix.go`, `sysproc_windows.go`).
    - `embed_linux.go` (`embed_other.go`, `embed_bin_*.go`) — `//go:embed`
      extraction with `memfd_create(2)` fallback on `noexec`/read-only disks.
    - `rawcodec.go` — custom gRPC codec that aliases the transport buffer on
      `ReadObjectChunk` to avoid protobuf copy overhead.
    - `shm_linux.go` (`shm_other.go`) — `SCM_RIGHTS` + `mmap` shared-memory
      data plane (`ReadShm` copy and `ReadShmZeroCopy` `LoanedView`).
  - **POC-only observability & test hooks** (isolated from SDK methods):
    - `observability.go` — `RPCs` counter (wired via gRPC client interceptors
      so `reader.go`/`writer.go`/`list.go` have zero counter calls),
      `SidecarStats()`, `Sidecar()`, `SessionID()`, and `ForceSession()`.
- `gothin/bench2/` — one W1R3 benchmark body compiled against both
  `*storage.Client` and `*udsthin.Client`; records latency, crossings/op,
  whole-process CPU per GiB (Go + sidecar) and peak RSS (both processes).
- `gothin/rapid/` + `gothin/cmd/rapid_bench` — Workloads 1–6 (`open`, `read50x2x4/serial`,
  `read50x2x4/concurrent`, `random8k`, 48-thread 5-minute sustained `BidiRead` & `Write`,
  `list-objects-100`, `list-buckets-100`).
- `gothin/cmd/bench2_native`, `gothin/cmd/bench2_uds` — binaries (`check`,
  `ping`, `session-tests`, `shm`, `hijack` subcommands on the UDS one).
- `vm_uds5_worklog.sh`, `vm_uds2.sh`, `vm_uds3.sh`, `vm_uds4.sh` — GCE startup scripts
  used for the published numbers; `gothin/results/` — raw logs & Cloud Monitoring JSON.

---

## 5. Design FAQ: Why a custom `gcscore.proto` instead of `google/storage/v2/storage.proto`?

1. **Shared across all target languages (Go, Python, C++, Java, Node.js):**
   `gcs_uds/proto/gcscore.proto` and the Rust sidecar (`gcs-core-server`) are
   language-agnostic and written **once**. Each language only generates its own
   `protoc` client stubs and writes a thin idiomatic wrapper (~1,000–2,000 LoC)
   that maps native language types (`io.Reader`/`io.Writer`, `context.Context`,
   `oauth2.TokenSource` in Go; file-like `BlobReader`/`BlobWriter` in Python;
   `ObjectReadStream` in C++) to the generated stubs. Multiple processes in
   different languages running under the same OS UID can even share a single
   running `gcs-core-server` daemon concurrently via per-client sessions.
2. **SDK-level operations vs. raw backend wire protocol:**
   `google/storage/v2/storage.proto` is the low-level GCS wire protocol
   (resumable upload session IDs, `BidiReadObject` redirect tokens and stream
   reopen state machines, manual CRC32C/MD5 verification, per-chunk offsets,
   and retry loops). Exposing `storage.proto` over UDS would force every thin
   language SDK to re-implement resumable uploads, retries, and checksums —
   defeating the purpose of a shared Rust core. `gcscore.proto` exposes
   high-level SDK operations so all retry, resumable session, stall/timeout,
   and checksum logic lives once inside the Rust sidecar.
3. **Local IPC primitives not present in the Cloud API:**
   `gcscore.proto` defines local-only RPCs for per-client credential lending
   (`CreateSession` / `UpdateToken` / `CloseSession`) and shared-memory transport
   (`ReadObjectShm`), as well as a flat `ReadObjectChunk` layout (`bytes data = 1`
   as field #1) that lets thin clients (`rawcodec.go`) slice the wire payload
   without protobuf copy overhead.
