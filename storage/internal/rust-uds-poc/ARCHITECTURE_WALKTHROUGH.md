# Architecture & Code Walkthrough: Go Thin Client + Rust gRPC/UDS Sidecar

This document explains every component we built in the POC (`/tmp/gcg-udspoc/storage/internal/rust-uds-poc`), how calls flow from the Go application through the thin client into the Rust sidecar, and how lifecycle management differs between a **1:1 per-process child sidecar** and a **shared multi-client / multi-language daemon**.

---

## 1. The Local IPC Contract (`gcscore.proto`)

* **File:** [gcscore.proto](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/proto/gcscore.proto)
* **Generated Go stubs:** [gcscore.pb.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/gcscorepb/gcscore.pb.go), [gcscore_grpc.pb.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/gcscorepb/gcscore_grpc.pb.go)

### Why is `gcscore.proto` shared across languages?
You define [gcscore.proto](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/proto/gcscore.proto) **once** and build the Rust sidecar (`gcs-core-server`) **once** for all languages (Go, Python, C++, Java, Node.js). Each language only runs `protoc` to generate its client stubs and writes a ~1,000–2,000 LoC idiomatic wrapper that maps native types (`io.Reader`, `context.Context`, `oauth2.TokenSource` in Go; `BlobReader`/`BlobWriter` in Python; `ObjectReadStream` in C++) to those stubs.

### Why a custom `gcscore.proto` instead of `google/storage/v2/storage.proto`?
1. **SDK-level semantics vs. raw wire protocol:** `google/storage/v2/storage.proto` is the low-level backend protocol (resumable upload session IDs, `BidiReadObject` redirect tokens and stream reopen state machines, manual CRC32C/MD5 verification, per-chunk offsets, and retry loops). If the UDS boundary exposed `storage.proto`, every thin SDK (Go, Python, C++) would still have to implement resumable upload state machines, retries, and checksums. `gcscore.proto` exposes **SDK-level operations** so all retry, resumable session, and checksum logic stays in the Rust sidecar.
2. **Local IPC primitives:** `gcscore.proto` defines local-only RPCs that do not exist in the Cloud API:
   * Per-caller credential lending (`CreateSession`, `UpdateToken`, `CloseSession`)
   * Shared-memory coordination (`ReadObjectShm`)
   * Flattened [`ReadObjectChunk`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/proto/gcscore.proto#L79-L89) layout (`bytes data = 1` as field #1) so thin clients ([rawcodec.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/rawcodec.go)) can zero-copy slice the wire payload without parsing nested protobuf messages.

---

## 2. The Go Thin Client (`gothin/udsthin`)

Did we modify the existing `cloud.google.com/go/storage` code? **No.** We wrote a standalone pure-Go (`CGO_ENABLED=0`) package `udsthin` with the **same public method shapes** as `cloud.google.com/go/storage` (`c.Bucket(b).Object(o).NewReader(ctx)`, `NewRangeReader`, `NewWriter`, `Objects`), cleanly separated into **production SDK files** vs. **isolated POC observability hooks**:

### A. Production Thin-Client Files

| File | What it does |
| --- | --- |
| [client.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/client.go) | Defines [`Client`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/client.go#L79-L86), [`Options`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/client.go#L35-L76), [`NewClient`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/client.go#L90-L129), [`Close`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/client.go#L134-L143), [`BucketHandle`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/client.go#L160-L163), [`ObjectHandle`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/client.go#L170-L174), and [`mapErr`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/client.go#L214-L231). Dials `opts.Conns` (default 4) `grpc.ClientConn` connections over `unix:<socket>` with 64 MiB max message size (`MaxCallRecvMsgSize`/`MaxCallSendMsgSize`) and 1 MiB socket read/write buffers (`WithReadBufferSize(1<<20)` / `WithWriteBufferSize(1<<20)`). |
| [reader.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/reader.go) | <ul><li>[`NewRangeReader`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/reader.go#L41-L97): Opens a server-streaming `ReadObject` RPC and eagerly receives the first `ReadObjectChunk` so `r.Attrs.Size` is populated before `NewRangeReader` returns (matching `cloud.google.com/go/storage` behavior).</li><li>[`Reader.Read(p)`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/reader.go#L101-L143): Copies out of the current 2 MiB chunk into `p` and only calls `stream.Recv()` when the current chunk is drained.</li><li>[`Opened`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/reader.go#L168-L246) (`Open`, `ReadAt`, `Close`): Calls `OpenObject` on the sidecar to get a persistent `uint64 handle` (backed by a GCS `BidiReadObject` stream in Rust) and issues concurrent `ReadRange` calls on that handle.</li></ul> |
| [writer.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/writer.go) | [`Writer.Write(p)`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/writer.go#L53-L99) buffers caller bytes up to 2 MiB (`writeChunkSize`), sends object metadata on the first `WriteObjectRequest`, and streams 2 MiB chunks via `stream.Send()`. [`Writer.Close()`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/writer.go#L103-L124) flushes trailing bytes, calls `CloseAndRecv()`, and populates `w.attrs` (`Size`, `CRC32C`, `Generation`). |
| [list.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/list.go) | [`ObjectIterator`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/list.go#L26-L36) (`Objects`, `Next`, `PageInfo`): Opens the server-streaming `ListObjects` RPC on first `Next()` and yields `ObjectAttrs` items until `io.EOF`, returning `iterator.Done`. |
| [session.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/session.go) | Per-`Client` OAuth2 `TokenSource` lending over UDS: [`startSession`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/session.go#L13-L32) mints the initial bearer token in Go and calls `CreateSession` over UDS to receive a 128-bit `session_id`. [`unarySession`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/session.go#L66-L70) and [`streamSession`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/session.go#L72-L75) attach `x-gcs-session: <session_id>` on every gRPC call, while [`refreshLoop`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/session.go#L36-L56) pushes refreshed tokens via `UpdateToken` (with exponential backoff on error). |
| [sidecar.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/sidecar.go) & [embed_linux.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/embed_linux.go) | Spawns `gcs-core-server` or extracts the `//go:embed`-ded static musl binary to `/tmp` (with [`memfd_create(2)`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/embed_linux.go#L74-L104) anonymous RAM execution fallback when `/tmp` is `noexec` or read-only). |
| [rawcodec.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/rawcodec.go) | Custom `grpc/encoding.CodecV2` (`RawCodec`) that parses field #1 (`bytes data`) directly out of gRPC-Go's `mem.BufferSlice` on `ReadObjectChunk`, eliminating the protobuf deserialization copy. |
| [shm_linux.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/shm_linux.go) | Receives the sidecar's `memfd` file descriptor via `SCM_RIGHTS` over `<socket>.shm`, `mmap`s it once per client, and exposes [`ReadShm`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/shm_linux.go#L61-L82) (1 copy into caller `dst []byte`) and [`ReadShmZeroCopy`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/shm_linux.go#L88-L115) (0-copy [`LoanedView`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/shm_linux.go#L25-L39) directly over the `mmap` slice). |

### B. Isolated POC Observability & Test Hooks
* **[observability.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/observability.go):**
  * Contains [`RPCs`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/observability.go#L20) (counted transparently via the [`countUnary`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/observability.go#L23-L28) and [`countStream`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/observability.go#L31-L40) gRPC client interceptors so `reader.go`, `writer.go`, and `list.go` have zero counter calls), [`SidecarStats`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/observability.go#L60-L62), [`Sidecar`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/observability.go#L66-L68), [`SessionID`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/observability.go#L72-L74), and [`ForceSession`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/observability.go#L78-L80).

---

## 3. The Rust Sidecar Server (`gcs_uds/server`)

Did we modify the official `google-cloud-storage` Rust crate? **No.** The sidecar imports unmodified `google-cloud-storage = "1.21.0"` from `crates.io` ([Cargo.toml](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/Cargo.toml#L15-L21)) and wraps it in a `tonic` gRPC server listening on a Unix domain socket.

| File | What it does |
| --- | --- |
| [main.rs](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/main.rs) | <ul><li>[`prepare_socket_path`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/main.rs#L94-L143): Creates parent directory with `0700`, probes any existing socket file with `UnixStream::connect` (refuses to overwrite if a live sidecar answers; unlinks stale socket if `ECONNREFUSED`), and binds `UnixListener`.</li><li>[`peer_uid_ok`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/main.rs#L148-L163): Inspects `UnixStream::peer_cred()` (`SO_PEERCRED`) on every accepted connection and drops any client whose UID != `geteuid()`.</li><li>Supports `--parent-death-sig` (`prctl(PR_SET_PDEATHSIG, SIGTERM)`) and `--idle-timeout-ms` (auto-exit when `in_flight == 0` for N ms).</li></ul> |
| [session.rs](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/session.rs) | Implements [`SwappableTokenProvider`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/session.rs#L24-L47) (`TokenProvider` + `CacheableResource` traits backed by `RwLock<Arc<String>>`). [`SessionStore::create`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/session.rs#L74-L110) constructs a dedicated `google_cloud_storage::client::Storage` instance wired to that `Credentials` object under a 128-bit `OsRng` hex ID, so `UpdateToken` swaps the token in place without rebuilding the HTTP/gRPC connection pool. |
| [service.rs](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/service.rs) | <ul><li>[`storage_for`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/service.rs#L140-L157): Inspects `x-gcs-session` metadata on every RPC and routes to the caller's session `Storage` client (or default ADC client if unset).</li><li>[`read_object`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/service.rs#L279-L337): Calls `storage.read_object(bucket, object).with_range(...).send().await`, coalesces network frames into 2 MiB `BytesMut` buffers, and streams `ReadObjectChunk` over `mpsc::channel(4)`.</li><li>[`open_object` / `read_range` / `close_object`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/service.rs#L339-L416): Opens a persistent `ObjectDescriptor` (`BidiReadObject` stream), stores it in `RwLock<HashMap<u64, ObjectDescriptor>>`, and multiplexes concurrent `ReadRange` RPCs over `&ObjectDescriptor`.</li><li>[`write_object`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/service.rs#L418-L484): Bridges the incoming gRPC stream into [`ChannelPayload`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/service.rs#L533-L561) (`StreamingSource` trait) and calls `storage.write_object(...).send_unbuffered().await` (or `append_object` when `appendable = true`).</li></ul> |
| [shm.rs](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/shm.rs) | Allocates a `256 MiB` (`--shm-size`) `memfd_create("gcscore-shm", MFD_CLOEXEC)` arena on startup, sends the fd over `<socket>.shm` via `SCM_RIGHTS`, and lets [`read_object_shm`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/service.rs#L211-L265) write incoming network chunks directly into the `mmap` slot (`slot_offset .. slot_offset + len`). |

---

## 4. Shared Multi-Client / Multi-Language Lifecycle (Why the Client Doesn't Need to Guess)

In [sidecar.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/sidecar.go#L35-L43) and [client.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/client.go#L131-L143), our POC **already** defaults to a shared per-UID socket:
1. [`defaultSocket()`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/sidecar.go#L35-L43) chooses `$XDG_RUNTIME_DIR/gcs-core/core.sock` or `/tmp/gcs-core-<uid>/core.sock` (mode `0700`).
2. [`Start()`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/sidecar.go#L90-L92) checks `if dialable(sock) { return s, nil }` — if a sidecar is already running for that UID (started by another Go process, Python, or C++), `NewClient` simply connects to it in `< 0.5 ms`.
3. [`Client.Close()`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/client.go#L131-L143) only calls `CloseSession` (wiping that client's OAuth2 token) and closes its UDS `grpc.ClientConn`s — it **leaves the sidecar running** for other processes until `--idle-secs` expires.

### Why `PR_SET_PDEATHSIG` Is Wrong for a Shared Sidecar — and What Replaces It
* **The problem with `PR_SET_PDEATHSIG` ([main.rs](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/main.rs#L92-L103)):** `PR_SET_PDEATHSIG` binds the sidecar's life to the **single parent PID** that happened to spawn it first. If Process A spawns the sidecar, Process B (Python) connects to it, and Process A exits, the kernel sends `SIGTERM` to the sidecar and kills Process B's in-flight transfers!
* **Why the client shouldn't have to know whether it's in a 1:1 sandbox or a multi-process machine:** An SDK caller calling `storage.NewClient(ctx)` has no idea whether it is the only process in a Cloud Run container or one of 20 processes on a shared VM.
* **The unified production solution (Connection Reference Counting + Idle Grace):**
  * Disable `PR_SET_PDEATHSIG` (`--pdeathsig=false`) and spawn the sidecar in its own process group (`Setpgid: true` in [sysproc_unix.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/sysproc_unix.go)).
  * Instead, the Rust sidecar counts **active UDS connections (`active_conns`)**.
  * Whenever any client process (Go, Python, C++) exits normally **or is `SIGKILL`ed by the OS**, the Linux kernel **immediately** closes all of that process's file descriptors (`EOF` on its UDS socket in `< 0.1 ms`), decrementing `active_conns`.
  * As long as `active_conns > 0` (Process B is still alive), the sidecar stays up even if Process A (the original spawner) exits or crashes.
  * When the **last** client disconnects or crashes (`active_conns` drops to `0`), the sidecar waits `--idle-secs` (or exits immediately if `--idle-secs=0`) and unlinks the socket.

---

## 5. Pipeline Parallelism in `Reader.Read(p)` & UDS / HTTP/2 Tuning (Staged & Ready to Benchmark)

You asked two connected questions on `Reader.Read(p)` and `client.go`:
1. *"Should there be any parallelism here [in `Reader.Read(p)` when draining the current 2 MiB chunk]?"*
2. *"Can these [HTTP/2 window & socket buffer] values affect performance, what if our benchmark numbers can sway because of this?"*

### Why `Reader.Read(p)` Was Previously Stop-and-Wait Instead of Pipelined
By Go's `io.Reader` contract, a single `*Reader` is called sequentially by one caller goroutine (`io.Copy` / `io.ReadFull`), **but receiving Chunk $N+1$ over the UDS socket should happen in parallel in the background while the caller is copying Chunk $N$ into `p`**.
* `grpc-go` **already** runs a dedicated background reader goroutine (`http2Client.reader()`) per connection, and the Rust sidecar **already** runs `pump_reader` on a background Tokio task (`tokio::spawn`).
* **However**, both `tonic` and `grpc-go` default to a **`64 KiB` HTTP/2 initial stream window (`initialWindowSize = 65,535`)** and a **`16 KiB` HTTP/2 max frame size**, while our `ReadChunk` size is **`2 MiB`**.
* Because a single `2 MiB` chunk is `32×` larger than the `64 KiB` stream window, `grpc-go` did not grant HTTP/2 window credit for Chunk $N+1$ until the caller finished copying Chunk $N$ and called `stream.RecvMsg()`! That turned the UDS pipeline into **stop-and-wait per 2 MiB chunk** and chopped every 2 MiB chunk into `128 × 16 KiB` HTTP/2 `DATA` frames.

### Measured Impact of the Five UDS & HTTP/2 Tunings (`c4-standard-192`, `us-central1-b` → `us-west1`)
We implemented and benchmarked five tunings across [client.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/client.go), [rawcodec.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/rawcodec.go), [lib.rs](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/lib.rs), and [service.rs](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/service.rs):
- **128 MiB Sequential Reads (`w1r3_uds_*_tuned.txt`):** `go-uds-grpc` CPU dropped to **`2.31–2.34 CPU-s/GiB`** (lower than native `go-grpc-cp` at `2.67–2.70 CPU-s/GiB`!), and `go-uds-json` dropped to **`2.34–2.44 CPU-s/GiB`**.
- **128 MiB Shared-Memory vs Tuned gRPC Chunks (`shm_*_tuned.txt`):** Tuned gRPC chunks ran at `2.41–2.47 CPU-s/GiB` (`Go 0.63 + sidecar 1.78–1.84`), while `ReadObjectShm` (`memfd`) dropped total CPU to **`1.32–1.57 CPU-s/GiB`** with `copy()` (`Go 0.11–0.14`) and **`1.16–1.46 CPU-s/GiB`** with a zero-copy loaned slice (`Go 0.00–0.01 s/GiB`).
- **48-Thread Sustained BidiRead (`w4_bidiread48_uds_grpc.txt`):** Reached **`1349.0 MiB/s`** on UDS vs **`1358.4 MiB/s`** on native Go (**`99.3%` throughput parity**), with P90 `612.2 ms` vs `616.2 ms`, P99 `771.6 ms` vs `766.8 ms`, and Max `1.44 s` vs `10.96 s`.

1. **Unthrottled Background Prefetching (`16 MiB` Stream Window / `64 MiB` Conn Window):**
   * Added `grpc.WithInitialWindowSize(16<<20)` and `grpc.WithInitialConnWindowSize(64<<20)` in [client.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/client.go#L127-L128), and `.initial_stream_window_size(16<<20)` / `.initial_connection_window_size(64<<20)` on `tonic::transport::Server::builder()` in [main.rs](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/main.rs#L261-L263).
   * Now `grpc-go`'s background `http2Client.reader()` goroutine prefetches up to **8 × 2 MiB chunks in parallel** off the UDS socket while `Reader.Read(p)` copies the current chunk.
2. **4 MiB HTTP/2 Frame Size (`max_frame_size = 4 MiB`):**
   * Set `.max_frame_size(4 * 1024 * 1024)` in [main.rs](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/main.rs#L263) and `WithReadBufferSize(4<<20)` / `WithWriteBufferSize(4<<20)` in [client.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/client.go#L129-L130). Instead of slicing each 2 MiB chunk into 128 × 16 KiB HTTP/2 frames, it travels in **1 single HTTP/2 `DATA` frame**.
3. **8 MiB Kernel UDS Socket Buffers (`SO_SNDBUF` / `SO_RCVBUF`):**
   * Linux's default `SOCK_STREAM` UDS kernel buffer (`wmem_default`) is only `208 KiB`, causing a 2 MiB write to hit `EAGAIN` + `epoll` wakeups ~10 times per chunk. Both [client.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/client.go#L101-L104) (`dialUDS`) and [main.rs](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/main.rs#L236-L256) now request `8 MiB` `SO_SNDBUF`/`SO_RCVBUF` on the UDS socket.
4. **Zero-Copy `WriteRequest` Marshal & Fast Single-Buffer Unmarshal ([rawcodec.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/rawcodec.go#L31-L115)):**
   * Previously, `rawCodec.Marshal` fell back to `proto.Marshal` on writes, allocating and copying 2 MiB on every `WriteRequest`. Now `rawCodec.Marshal` encodes only the 5-byte protobuf tag+varint header and references `d.Data` zero-copy in `mem.BufferSlice`.
   * Added `unmarshalSingleBuffer` fast path when the 2 MiB message arrives in a single 4 MiB read buffer.
5. **16-Slot Tokio Pipeline Channels ([service.rs](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/service.rs#L317)):**
   * Raised `mpsc::channel(4)` to `mpsc::channel(16)` on `read_object`, `read_range`, and `write_object` so the Rust network task never stalls waiting for the UDS sender.

---

## 6. How Did We Choose `2 MiB` vs. `16 MiB` vs. `64 MiB`?

### A. Why `2 MiB` for the Local UDS Chunk Size (`ReadChunk` & `WriteRequest`)? Is It a GCS Server Limitation?
1. **On the GCS gRPC backend (`google/storage/v2/storage.proto`): Yes, `2 MiB` is the hard protocol limit.**
   * `ServiceConstants.Values.MAX_WRITE_CHUNK_BYTES = 2,097,152` (2 MiB), and GCS gRPC `BidiReadObject` / `ReadObject` server responses are also capped at `2 MiB` per protobuf message.
2. **On the GCS JSON backend: No — and we already use `16 MiB` towards GCS!**
   * Look at [writer.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/writer.go#L29-L48): `w.ChunkSize` (passed to the Rust sidecar in `WriteHeader.ChunkSize`) is **`16 MiB` (`googleapi.DefaultUploadChunkSize`)**, matching native Go `cloud.google.com/go/storage`.
   * The `2 MiB` (`const msg = 2 << 20` in [writer.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/writer.go#L60) and `chunk_size = 2 MiB` in [main.rs](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/main.rs#L47)) is **only the local UDS streaming slice size** between Go and the Rust sidecar.
3. **Should the local UDS chunk size be increased above `2 MiB` (e.g., to `8–16 MiB`)?**
   * **No — increasing it above `2–4 MiB` actually hurts latency and CPU cache locality** for two reasons:
     * **Store-and-forward latency (Time-to-First-Byte):** In [`pump_reader`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/service.rs#L184), the Rust sidecar accumulates network bytes until `acc_len >= chunk` before sending a `ReadChunk` to Go. At `2 MiB`, Go starts receiving data after the first `2 MiB` arrives from GCS (~1 ms). If `chunk` were `16 MiB`, Rust would buffer `16 MiB` before sending the first byte over UDS to Go — adding ~10–15 ms of store-and-forward stall!
     * **L3 CPU cache residency:** On `c3-standard-8` (Intel Sapphire Rapids), L3 cache is ~1.875 MiB per core. A `2 MiB` chunk written by Rust into the UDS kernel buffer and immediately read by Go stays **hot in L2/L3 cache**, whereas a `16 MiB` chunk overflows L3 cache and forces DRAM round-trips.
     * At `2 MiB`, a 128 MiB object is only `64` messages (~30 µs total gRPC call overhead), so you already get >99% of the syscall amortization without spilling L3 cache or delaying first-byte delivery.

### B. How Did We Decide `64 MiB`?
`64 MiB` (`64 << 20`) appears in two places in [client.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/client.go#L123-L128) and [main.rs](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/main.rs#L157-L158):
1. **`WithInitialConnWindowSize(64 << 20)` (HTTP/2 Connection Flow-Control Window):**
   * This is the **exact same constant used in production by `cloud.google.com/go/storage`** (`grpcClient` in Go's native storage SDK sets `grpc.WithInitialConnWindowSize(64 << 20)`).
   * In HTTP/2, the connection window is shared across concurrent streams on that connection. With a `16 MiB` per-stream window (`WithInitialWindowSize(16 << 20)`), setting the connection window to `4 × 16 MiB = 64 MiB` ensures up to 4 concurrent streams per connection can saturate their stream windows without starving one another.
2. **`MaxCallRecvMsgSize(64 << 20)` / `MaxCallSendMsgSize(64 << 20)` (gRPC Max Message Guard):**
   * Standard gRPC defaults to rejecting any single message `> 4 MiB` with `ResourceExhausted`.
   * Raising this guard check to `64 MiB` allocates **0 bytes of memory** up front (it is just an `if length > 64 MiB` check on the 5-byte gRPC frame header) while allowing experiments with larger `--chunk-size` values (e.g., `8 MiB`, `16 MiB`, `32 MiB`) without hitting `ResourceExhausted`.

---

## 7. Alignment with Newly Added Worklog Specifications (`t.rn625s1tqaw2`)

We inspected the latest additions to the **gRPC over UDS Approach** tab (`t.rn625s1tqaw2`) of the Worklog and wired every new requirement into the POC so it is ready to run on command:

### A. All 6 Required APIs Implemented
| Worklog API Table Entry | Go Thin Client (`udsthin`) Method | Rust Sidecar (`gcs_uds/server`) Handler |
| --- | --- | --- |
| `BidiReadObject` in gRPC | [`ObjectHandle.Open(ctx)`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/reader.go#L173-L193) / [`Opened.ReadAt`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/reader.go#L196-L247) & `NewRangeReader` (`Transport: GRPC`) | [`open_object`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/service.rs#L355-L368) / [`read_range`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/service.rs#L370-L388) (`storage.open_object().send_and_read()`) |
| `BidiWriteObject` in gRPC | [`ObjectHandle.NewWriter(ctx)`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/writer.go#L28-L30) (`WriteTransport: GRPC`) | [`write_object`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/service.rs#L407-L428) (`storage.open_appendable_object()`) |
| `ReadObject` in JSON | [`ObjectHandle.NewReader(ctx)`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/reader.go#L36-L38) / [`NewRangeReader`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/reader.go#L41-L89) (`Transport: JSON`) | [`read_object`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/service.rs#L338-L351) (`storage.read_object()`) |
| `WriteObject` in JSON | [`ObjectHandle.NewWriter(ctx)`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/writer.go#L28-L30) (`Transport: JSON`) | [`write_object`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/service.rs#L430-L465) (`storage.write_object().send_buffered()`) |
| `ListObject` in gRPC | [`BucketHandle.Objects(ctx, q)`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/list.go#L23-L32) → `ObjectIterator.Next()` | [`list_objects`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/service.rs#L468-L507) (`control.list_objects()`) |
| `ListBucket` in gRPC | [`Client.Buckets(ctx, projectID)`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/udsthin/list.go#L90-L97) → `BucketIterator.Next()` | [`list_buckets`](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gcs_uds/server/src/service.rs#L509-L541) (`control.list_buckets()`) |

### B. All 6 Workloads & All 4 Performance Parameters Wired ([rapid.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/rapid/rapid.go), [bench2.go](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/gothin/bench2/bench2.go), [vm_uds5_worklog.sh](file:///tmp/gcg-udspoc/storage/internal/rust-uds-poc/vm_uds5_worklog.sh))
1. **Workloads 1–3 (Rapid SLIs):** `open`, `read50x2x4/serial`, `read50x2x4/concurrent`, and `random8k`.
2. **Workload 4 (`bidiread-16m-48t`):** Reads 16 MiB chunks via `BidiRead` across 48 parallel threads (`-sustained-threads=48 -sustained-dur=5m`) to saturate the NIC.
3. **Workload 5 (`bidiwrite-200m-48t`):** Writes 200 MiB chunks via `BidiWrite` / `WriteObject` across 48 parallel threads (`-sustained-threads=48 -sustained-dur=5m`).
4. **Workload 6 (`list-objects-100` & `list-buckets-100`):** Lists 100 objects and up to 100 buckets via gRPC `StorageControl`.
5. **Performance Parameters Collected on Every Run:**
   * **Latency:** Outermost Go boundary timers reporting **`p50`, `p90`, `p99`, `max`, `mean`**.
   * **Throughput:** Aggregate wall-clock **`MiB/s`** (`total_bytes / wall_elapsed`).
   * **CPU Utilization:** **`cpu %`** (`(Go CPU + Sidecar CPU) / (wall_time * NumCPU) * 100%`), **`cpu s/GiB`**, and **`cpu ms/op`**.
   * **Memory Consumption (Peak Memory):** **`peak MiB`** (`Go VmHWM + Sidecar VmHWM`).
