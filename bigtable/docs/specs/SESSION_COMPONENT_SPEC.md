# Session — Component Topology & Boundary Spec

**Scope.** This file governs the **structural layout** of the Session subsystem: which package/type owns which concern, what MUST NOT import what, and where each responsibility lives. Companion to the four behavioral specs — `SESSION_SPEC.md` (per-Session lifecycle, 10 invariants), `SESSION_CLIENT_SPEC.md` (SessionClient topology + config + OpenSession envelope, 4 invariants), `SESSION_POOL_SPEC.md` (pool topology + picking + routing + scaling + debug non-blocking, 5 invariants), and `CLIENT_SIDE_METRICS_SPEC.md` (per-attempt metrics field provenance). Read all four before any structural refactor.

**Why this file exists.** The Session subsystem has three natural failure modes: (a) a lower layer starts knowing about a higher layer (session package importing `bigtable.Row`); (b) two peers reach into each other's guts (Diverter reading pool state, or `Session` holding pool-level counters); (c) a "convenient" shortcut collapses two layers into one and cements a design mistake. The rules in Part B forbid each. When they conflict with a change, either (1) the change is wrong, or (2) the rule is stale — update the rule in the same PR, cite the reason.

**File layout.**
- Part A — Layer map. What lives where. Descriptive, expected to drift with the code.
- Part B — Boundary invariants. Prescriptive MUST rules. `grep`-checkable where noted.
- Part C — Ownership matrix. Which type is the sole owner of which concern.

---

## Part A — Layer map (reference, expected to drift)

Snapshot as of 2026-07-10 against `feat/bigtable-sessionz-debug` (Go) and `~/google-cloud-java/java-bigtable/` (Java). File paths WILL drift; treat as a starting point for `grep`, not authoritative citations. When paths drift severely, update Part A in the same PR that moved the code — Part B and Part C are the durable specs.

### Layer order (bottom-up)
1. **Transport primitives** — one bidi stream, retry classification, msgtype, state enum. `bigtable/internal/transport/`.
2. **Session** — one bidi stream wrapped as a state machine with lifecycle, heartbeat, vRPC invoke. Still `bigtable/internal/transport/`.
3. **Pool** — many Sessions grouped by AFE, checked in/out, sized against server config. Still `bigtable/internal/transport/`.
4. **Session client + tables** — per-resource read/write pool pair, lazy open, proto-native table API. `bigtable/internal/session/`.
5. **Routing shim + diverter** — mixed-mode router between session and classic. `bigtable/table_shim.go`, `bigtable/internal/transport/diverter.go`.
6. **Public bigtable API** — user-facing `Client`, `Table`, `Mutation`, `Row`. `bigtable/`.
7. **Observability tier** — z-pages + snapshot DTOs. `bigtable/debugview/`, `bigtable/internal/transport/*_snapshot.go`, `bigtable/internal/transport/debug_api.go`.

### Core session
| Layer | Go (`bigtable/internal/transport/`) | Java (`.../internal/session/`) |
|---|---|---|
| Session type | `Session` struct — `session.go:226` | `Session` interface — `Session.java:35` + `SessionImpl` — `SessionImpl.java:61` |
| State enum | `State` (6 values) — `session.go:72`, `session_state.go` | `Session.SessionState` — `Session.java:37` |
| Lifecycle callbacks | `SessionHooks` — `session.go:145` | `Session.Listener` — `Session.java:116` |
| Stream abstraction | `Stream` — `session.go:114` | `SessionFactory` + `SessionStream` — `SessionFactory.java:27` |
| AFE ID | `afeID` + `Session.AfeID()` — `session.go:336-342` | `SessionList.AfeId` @AutoValue — `SessionList.java:373` |
| Session typing | `SessionType` (READ / WRITE) — `session_descriptors.go:39` | implicit in per-resource `SessionPoolImpl<OpenReqT>` |
| Open params carrier | `refreshConfig` atomic field on `Session` | `Session.OpenParams` @AutoValue — `Session.java:53` |
| Retry classification | `AttemptState` / `AttemptOutcome` / `tagErr` / `ClassifyErr` — `attempt_outcome.go` | `VRpc.VRpcResult.State` |
| Frame classifiers | `reqMsgType` / `respMsgType` — `session_msgtype.go:27,66` | `VRpcDescriptor` / `SessionDescriptor` — `VRpcDescriptor.java:59,297` |
| Timer infra | ad-hoc `time.AfterFunc` in `session_lifecycle.go` | `BigtableTimer` + `HashedWheelTimer` — `BigtableTimer.java:31`, `HashedWheelTimer.java:43` |

### Pool
| Layer | Go | Java |
|---|---|---|
| Pool | `SessionPoolImpl` — `session_pool.go:80` | `SessionPool` iface + `SessionPoolImpl` — `SessionPool.java:25`, `SessionPoolImpl.java:77` |
| Watchdog | `sweepStuckSessions` in `session_pool_lifecycle.go:63-88` | `SessionPoolImpl.Watchdog` — `SessionPoolImpl.java:844` |
| Per-AFE grouping | `sessionList` — `session_list.go:81` | `SessionList` — `SessionList.java:52` |
| AFE bucket | `afeHandle` — `session_list.go:50` | `SessionList.AfeHandle` — `SessionList.java:381` |
| Per-session handle | `SessionHandle` — `session_list.go:110` | `SessionList.SessionHandle` — `SessionList.java:143` |
| PeakEwma tracker | `PeakEwma` — `peak_emwa.go:24` | `SessionList.PeakEwma` — `SessionList.java:415` |
| Pool sizing math | `PoolSizer` — `pool_sizer.go:56` (formula + `ScaleDecision`) | `PoolSizer` — `PoolSizer.java:25` |
| Pool sizing action | `session_pool_scaling.go` + `ScalingEvent` — `:52` (consumes `ScaleDecision`, drives `openSession` / passive shrink) | inline in `SessionPoolImpl.java` |
| Creation budget | `SessionThrottler` / `AdaptiveSessionThrottler` — `session_creation_budget.go:24,56` | `SessionCreationBudget` — `SessionCreationBudget.java:32` |

### Picker
| Layer | Go | Java |
|---|---|---|
| Interface | `AfePicker` — `afe_picker.go:60` | `Picker` (abstract) — `Picker.java:22` |
| Impls | `SimpleAfePicker`, `LeastInFlightAfePicker`, `LeastLatencyAfePicker` — `afe_picker.go:67,94,122` | `SimplePicker`, `LeastInFlightPicker`, `LeastLatencyPicker` — `.../session/*Picker.java` |
| Trace record | `PickCandidate`, `PickDecision` — `afe_picker.go:32,40`; `PickHistoryEvent` — `session_pool_debug.go:404` | (no persistent trace record) |
| Hot-swap | ad-hoc in `SessionPoolImpl` | `DynamicPicker` — `DynamicPicker.java:29` |

### RPC path
| Layer | Go | Java |
|---|---|---|
| vRPC call | `session_vrpc.go` (methods on `Session`) | `VRpc` iface + `VRpcImpl` — `VRpc.java:48`, `VRpcImpl.java:50` |
| Retry loop | `RetryingVRpc` interceptor — `retrying.go` | `RetryingVRpc` — `RetryingVRpc.java:36` |
| Pool binding around vRPC | `lazyPool.Invoke` — `internal/session/lazy_pool.go:49` | `ForwardingVRpc` inside `SessionPoolImpl.newCall` — `SessionPoolImpl.java:644-662` |

### Client-facing surface
| Layer | Go | Java |
|---|---|---|
| Top-level client | `bigtable.Client` (mixed-mode) + `SessionClient` iface — `internal/session/api.go:74` | `Client` — `Client.java:60` |
| Per-resource surface | `SessionTableApi` / `sessionTable` — `internal/session/api.go:41`, `internal/session/table.go:37` | `TableBase` + `TableAsync` / `AuthorizedViewAsync` / `MaterializedViewAsync` — `TableBase.java:44` |
| Lazy per-op pool | `lazyPool` (`internal/session/lazy_pool.go:49`) — Go-only | **no analog** — Java eagerly builds one `SessionPoolImpl<OpenTableRequest>` per resource |
| Routing shim | `TableShim` — `bigtable/table_shim.go` — Go-only mixed-mode router | no analog (Java client is session-only or classic-only, not mixed) |
| Diverter | `Diverter` — `internal/transport/diverter.go` — Go-only mixed-mode traffic gate | no analog |
| Config manager | `ClientConfigurationManager` — `internal/transport/client_configuration_manager.go:98` | `ClientConfigurationManager` — `ClientConfigurationManager.java:72` |

### Observability
| Layer | Go | Java |
|---|---|---|
| Debug provider iface | `SessionDebugProvider` / `ChannelDebugProvider` / `ConfigDebugProvider` — `internal/transport/debug_api.go:31,47,76` | (none — metrics-only) |
| Provider impls | `sessionDebugProviderImpl`, `channelDebugProviderImpl`, `configDebugProviderImpl` — `internal/session/debug.go:50,71,113`; `mixedModeSessionDebug` — `bigtable/session_debug.go:66` | — |
| Snapshot DTOs | `SessionSnapshot` / `PoolSnapshot` / `LoadBalancingSnapshot` / `AfeSnapshotRow` — `internal/transport/session_snapshot.go:81,149,266,236` | — |
| z-pages | `debugview/{sessionz,afez,flightz,loadz,configz,channelz,tcpz}.go` + `debugview/handler.go:74` | — |
| Tracer | `sessionTracer` — `session_tracer.go:121` | `SessionTracer` / `SessionTracerImpl` |
| Debug tags | `sessionDebug` embedded in `Session` — `session_debug.go:36` | `DebugTagTracer` |

### PoolSizer decision anatomy

`PoolSizer.Decide()` (`pool_sizer.go:154`) is the only place the desired-capacity formula lives. Every intermediate is stamped on `ScaleDecision` so `session_pool_scaling.go`, z-pages, and tests all consume the same numbers instead of recomputing.

Inputs from `PoolStats` (via injected `StatsFetcher` — the sizer never reads pool internals directly): `ReadyCount`, `StartingCount`, `InUseCount`, `PendingCount`. Server-driven config from `ClientConfigurationManager.UpdateConfig` (see `SESSION_CLIENT_SPEC.md #3` for the authoritative field list): `MinSessions`, `MaxSessions`, `HeadroomPct`, `NewSessionQueueLength`. Client-side constant (not server-driven): `MinIdleSessions` — hardcoded to `defaultMinIdleSessions = 1` in `NewPoolSizer` (`pool_sizer.go:68,82`) and never mutated.

Formula (`pool_sizer.go:176-192`):

```
EffectivePending = ceil(PendingCount / NewSessionQueueLength)
SessionsInUse    = InUseCount + EffectivePending
IdleHeadroom     = max(MinIdleSessions, ceil(SessionsInUse × HeadroomPct))
DesiredRaw       = SessionsInUse + IdleHeadroom
DesiredCapacity  = clamp(DesiredRaw, MinSessions, MaxSessions)
```

What each intermediate means:

- **`EffectivePending`** — waiters at the pool boundary converted into *sessions we'd need to drain them*. `NewSessionQueueLength` is the server-advertised per-session vRPC pipeline depth (default 10); 47 waiters at qlen=10 becomes `EffectivePending = 5`, not 47. `ceil` guarantees 1 waiter still triggers 1 new session, not fractional.
- **`SessionsInUse`** — true current load: sessions actively serving vRPCs plus the sessions we'd have to open to drop `PendingCount` to zero.
- **`IdleHeadroom`** — cushion of idle sessions on top of load, as a fraction of `SessionsInUse`. Floor via `MinIdleSessions` (default 1) prevents an idle pool from wanting zero sessions (`ceil(0 × pct) = 0` would starve the warmup path).
- **`DesiredCapacity`** — clamped to server-configured bounds. Startup fills to `MinSessions` unconditionally; SESSION_POOL_SPEC #5 governs when a scale-up actually fires (`DesiredCapacity > EventualCapacity`).

Branch semantics (`scale-up` / `scale-down` / `dead-band` / `no-stats`) compare `DesiredCapacity` against `ImmediateCapacity = ReadyCount` and `EventualCapacity = ReadyCount + StartingCount`. `scale-down` deltas are **advisory** — the pool never proactively kills sessions; `OnClose` reads the delta and declines replacement so the pool shrinks passively as the server ages sessions out (SESSION_POOL_SPEC #5).

### Two structural divergences worth remembering
- **Lazy per-op pools (Go-only).** `SessionTable` holds two `*lazyPool` (read + write) opened on first `ReadRow` / `Apply`. Marshal errors surface at first call, not `OpenTable`.
- **z-page observability tier (Go-only).** Java relies purely on OTel metrics + tracer spans; Go additionally exposes 7 HTML/JSON debug views.

---

## Part B — Boundary invariants (prescriptive, MUST rules)

Every rule is a MUST. Violations are bugs, not preferences.

### B1. `bigtable/internal/session/**` MUST stay proto-native
- MUST NOT import `cloud.google.com/go/bigtable` (import cycle would prevent this at build time, but also prohibits reaching in through internal packages).
- MUST NOT reference public bigtable types: `bigtable.Row`, `Mutation`, `Filter`, `ReadOption`, `ApplyOption`, `RowSet`, `Family`, `ReadItem`, `Timestamp` (public alias), etc.
- MAY reference: proto types (`btpb.*`), transport types (`btransport.*`), metrics (`metrics.*`), stdlib, gRPC.
- Rationale: `TableShim` (Part C) is the sole translation boundary between proto and the public API. If the session package learns about `bigtable.Row`, the boundary collapses and mixed-mode becomes untestable.
- **`grep` check:** `git grep -l 'bigtable\.\(Row\|Mutation\|Filter\|ReadOption\|ApplyOption\|RowSet\)' bigtable/internal/session/` MUST return no results.

### B2. `bigtable/internal/transport/**` MUST NOT import `bigtable/internal/session`
- Direction is **transport → nothing above it**; session depends on transport, never the reverse.
- Applies to test files too — `_test.go` under transport MUST NOT import session.
- Rationale: session composes transport primitives. If transport reaches into session, the layer inversion means either transport is doing something it shouldn't, or session is exposing something it shouldn't.
- **`grep` check:** `git grep -l 'cloud.google.com/go/bigtable/internal/session' bigtable/internal/transport/` MUST return no results.

### B3. `bigtable/debugview/**` MUST access pool/session state ONLY via `SessionDebugProvider` (and its sibling interfaces)
- MUST NOT import `bigtable/internal/transport` for concrete types (`*SessionPoolImpl`, `*Session`, `*sessionList`, `*afeHandle`).
- MAY import `bigtable/internal/transport` for **snapshot DTO types** (`PoolSnapshot`, `SessionSnapshot`, `AfeSnapshotRow`, `DiverterSnapshot`, etc.) since those are the interface's return values.
- MAY import the three provider interfaces: `SessionDebugProvider`, `ChannelDebugProvider`, `ConfigDebugProvider`.
- Rationale: z-pages are consumers. If a z-page reaches into `*SessionPoolImpl` to read a field directly, refactoring the pool becomes impossible without breaking the debug UI. The provider interface is the versioned surface.
- **Adding a 4th method to `SessionDebugProvider`?** Update **every** fake in `afez/afez_test.go`, `flightz/flightz_test.go`, `sessionz/sessionz_test.go`, and any new debug-page tests — the interface is defensive by design.

### B4. `Diverter` MUST NOT know about specific RPCs
- `Diverter` has one input (`SessionLoad`), one output (`UseSession() bool`), and two counters (`SessionPicks`, `ClassicPicks`). That is the complete surface.
- MUST NOT gain per-method routing (e.g., "route ReadRow to session but Apply to classic"). Method-shape routing lives in `TableShim` (Part C).
- MUST NOT gain per-row/per-key stickiness. It is a stochastic gate, not a router. Any invariant that assumes "the same call went to the same place last time" MUST NOT rely on `Diverter`.
- Rationale: keeping `Diverter` shape-agnostic means adding a new vRPC method requires zero `Diverter` change — it's a `TableShim` change plus a session-side descriptor.

### B5. `TableShim` MUST NOT reach into `Session` or pool internals
- `TableShim` holds `SessionTableApi` (the interface), never `*SessionPoolImpl` or `*Session`.
- Method routability (which methods CAN take the session path) MUST be a switch statement inside `TableShim`, not a runtime capability query on the session API. Adding a new method's session support = extending `SessionTableApi` + adding a `TableShim` case, in that order.
- Errors from either data path MUST surface to the caller as-is. `TableShim` MUST NOT retry a session-side failure on classic — that violates the retry oracle (`SESSION_SPEC.md` #9).

### B6. `ClientConfigurationManager` is the SOLE writer of pool-shaping config
- `SessionPoolImpl.UpdateConfig`, `Diverter.SetSessionLoad`, and every pool-shaping setter MUST be called from `ClientConfigurationManager` in production code paths.
- Tests MAY call these setters directly for fixture setup. This is the only exception.
- Rationale: if pools self-tune, or if application code toggles `Diverter.SetSessionLoad` outside the config path, the polled config is no longer authoritative (`SESSION_SPEC.md` #13) and the client stops honoring server-driven rollouts.
- **`grep` sanity check for new call sites:** `git grep -n 'SetSessionLoad\|UpdateConfig' -- ':!*_test.go' ':!*_configuration_manager.go' ':!*/diverter_test.go'` — new hits should be reviewed.

### B7. `Session` MUST NOT hold pool-level counters
- Per-Session state: `state`, `activeRPC`, `peerInfo`, `refreshConfig`, `heartbeat*`, `nextRPCID`, `hooks` (`SessionHooks` struct — opaque per-session lifecycle + slot-drained callbacks, wired once at construction by `SessionPoolImpl.createSession`), tracer, debug ring buffer.
- Per-AFE state that MUST live on `afeHandle`, NOT on `Session`: `refCount`, `idleQueue`, `transportEwma`, `e2eEwma`, `lastConnected`.
- Per-pool state that MUST live on `SessionPoolImpl`, NOT on `Session`: `waiter queue`, `picker`, `sizer`, `budget`, `pickHistory`, `scalingHistory`.
- Rationale: a `Session` is one bidi stream. Anything that requires aggregating across streams is not per-Session state. Storing it there means pool operations start needing to iterate every Session, which turns O(1) checkout into O(N).

### B8. Pool → SessionList lock ordering is fixed
- `pool.mu` first, `sl.mu` second. **Never take `pool.mu` while holding `sl.mu`.**
- Methods that need `p.picker.Name()` from inside a `CheckoutSession`-style path (which already holds `p.mu`) MUST take the name as a parameter or read from an atomic snapshot. Re-acquiring `p.mu` transitively is a re-entrant deadlock.
- Rationale: `CheckoutSession` is on the hot path; a re-entrant deadlock here stalls every client using session traffic.

### B9. Retry classification lives at the vRPC boundary, not in Session or pool
- `Session.Invoke` tags outcomes with `AttemptState` (`attempt_outcome.go`).
- `RetryingVRpc` reads tags via `ClassifyErr` and decides retryability.
- **Neither layer should second-guess the other.** Session MUST NOT filter which errors reach the caller; RetryingVRpc MUST NOT invent classifications from raw gRPC codes.
- Adding a new terminal-vs-retryable error path? Tag it at the source in `session_vrpc.go` (see the tag-site reference table in `SESSION_SPEC.md` #9). Don't add a special case in `RetryingVRpc`.

### B10. z-page code MUST NOT synthesize state that could be computed on the producer
- If a debug view needs a derived value (e.g., "session age", "in-flight duration"), the snapshot DTO MUST carry it. z-page code renders; it does not compute business logic.
- Rationale: a second consumer of the DTO (test, exporter, JSON API) shouldn't have to re-derive the same value. Computation lives where the raw state does.

### B11. Runtime-behavior invariants (`SESSION_SPEC.md` #1-#14) MUST NOT be re-implemented in another layer
- Example: `SESSION_SPEC.md` #4 says lifecycle hooks fire in a fixed order exactly once. `SessionPoolImpl` MUST NOT add its own "seen this session before?" dedup — the hooks are already exactly-once.
- Example: `SESSION_SPEC.md` #7 says heartbeat is armed only while a vRPC is in flight. `SessionPoolImpl.Watchdog` MUST NOT independently poke idle sessions "just in case."
- If you find yourself adding defensive logic that duplicates a behavioral invariant, either the invariant is not actually holding (fix the source, not the caller) or the invariant is wrong (update `SESSION_SPEC.md`, don't paper over).

### B12. Session-level (#1-#10) and pool-level (#11-#14) invariants MUST NOT be mixed in one component
- The removed AFE-grouping items (former `SESSION_SPEC.md` #5 and #6) are a cautionary example — they conflated per-Session behavior with per-pool topology. When speccing or coding a component, ask: "is this about *this Session's* behavior, or about *the pool's* composition?" If both, split into two components.

---

## Part C — Ownership matrix (who is the sole owner of what)

"Sole owner" means: this type is the only writer / only source of truth. Other components read via accessor or interface; they do NOT hold parallel copies.

| Concern | Sole owner (Go) | Notes |
|---|---|---|
| Session state (`New`/…/`Closed`) | `Session` | atomic; read via `Session.State()` |
| AFE ID | `Session.peerInfo` (set once by `handleOpenSession`) | pool reads via `Session.AfeID()`; pool MUST NOT cache |
| Per-AFE refCount, idle queue, PeakEwmas | `afeHandle` | pool interacts only via `sessionList` methods |
| Ready AFE set | `sessionList` | `SessionPoolImpl` reads via `sl.ReadyAfes()` |
| In-flight vRPC slot | `Session.(activeRPC, currentCancel)` under `slotMu` — Java `SessionImpl.currentRpc/currentCancel` parity | pool MUST NOT track "which session has an outstanding call" separately |
| Session→pool "slot drained" signal | `Session.hooks.onSlotDrained()` → `SessionHooks.OnSlotDrained` closure (installed by `SessionPoolImpl.createSession` in the `SessionHooks{...}` literal handed to `NewSession`; captures `*SessionHandle` for `sl.ReleaseToPool` + `p` for `p.signalFree`) | closure fires `sl.ReleaseToPool(sh)` + `p.signalFree()`; the pool's `Invoke` return path MUST NOT re-enqueue the session in the AFE idle queue OR wake a `Checkout` waiter (both live at the drain site only, so `Idle` queue membership stays coupled 1:1 with slot vacancy — see `SESSION_POOL_SPEC.md #6` transition table) |
| Pool.Close Phase1↔Phase2 dedup + defensive onActive re-entry gate | `SessionHandle.{activated, closingRecorded, closeRecorded}` (each `atomic.Bool`, CAS-once) | Sole writers are `SessionPoolImpl.{onActive, onClosing, onClose}` and `Pool.Close` Phase-1. Phase-1 order per snapshot handle: `recordLifetime` → `recordSessionClose` → flip both `closingRecorded` / `closeRecorded` → `sl.OnSessionClosed`, so Phase-2's `s.Close` driving the hook chain finds the flags tripped and short-circuits `onClosing` / `onClose`. Scope is pool-teardown ordering only — this is NOT a substitute for `SESSION_SPEC #4`, which guarantees `hooks.On*` fire exactly once at the source via `closingOnce` / `closeOnce` on `Session`. |
| Heartbeat deadline | `Session.nextHeartbeatDeadlineNano` (atomic) | pool watchdog MUST NOT independently reset |
| Session pool composition (min/max/waiters) | `SessionPoolImpl` | driven by `ClientConfigurationManager.UpdateConfig` |
| Pool sizing formula + `ScaleDecision` | `PoolSizer` (`pool_sizer.go:154`) | `session_pool_scaling.go`, z-pages, tests MUST consume `Decide()`/`ScaleDecision`; MUST NOT recompute `EffectivePending`/`IdleHeadroom`/`DesiredCapacity` from raw `PoolStats` |
| Sizer inputs (`PoolStats`) | `SessionPoolImpl` via injected `StatsFetcher` | `PoolSizer` reads via fetcher closure; MUST NOT touch pool internals directly |
| Traffic split ratio (session vs classic) | `Diverter.sessionLoadBits` (atomic float64) | driven by `ClientConfigurationManager.SessionLoadListener` |
| Server-driven config | `ClientConfigurationManager.currentConfig` (RW-mutex) | pools subscribe via listener |
| Retry classification (`AttemptState`) | `session_vrpc.go` tag sites | `RetryingVRpc` reads via `ClassifyErr`; MUST NOT re-classify |
| Retry policy (max attempts, backoff) | `RetryingVRpc` | Session and pool MUST NOT enforce their own retry caps |
| Proto ↔ public-type translation | `TableShim` (public side) + `sessionTable` (proto side) | session package NEVER sees public types; public callers NEVER see proto |
| Read pool vs write pool selection | `sessionTable.ReadRow` / `sessionTable.MutateRow` | `TableShim` MUST NOT pick the pool — it picks session-vs-classic only |
| Debug/z-page state access | `SessionDebugProvider` (interface) | providers implemented on `sessionClient` and `mixedModeSessionDebug`; z-pages depend only on the interface |
| Close reason | `Session.closeReason` (CAS-once) | first cause wins; late `StreamEnd:*` classifications MUST NOT overwrite |
| Per-attempt `ClusterInfo` (classic path) | metrics tracer's `attempt` state, stamped from response `ResponseParams` in unary interceptor | `internal/metrics/util.go:96-102`; MUST NOT be stamped from session-side sources |
| Per-attempt `ClusterInfo` (session path) | `InvokeResult.ClusterInfo` (returned by `Session.Invoke`) | `session_vrpc.go:44-46, 216-219`; stamped by `sessionTable.stampAttempt`; MUST NOT be re-derived from headers |
| Per-attempt transport peer labels (classic) | grpc.Peer at attempt time | can vary across attempts within an operation |
| Per-attempt transport peer labels (session) | `InvokeResult.PeerInfo` (== `Session.peerInfo`, set once) | fixed for the session's lifetime; all attempts on session S share it (spec #16) |
| Per-attempt `client_blocking_latencies` (session path) | `InvokeResult.SentAt` (captured by `Session.Invoke` immediately before `s.Send`, `session_vrpc.go:124-127`) | stamped once by `sessionTable.stampAttempt` (`internal/session/table.go:242-243`); MUST NOT be recomputed from any other timestamp; classic path uses `blockingLatencyTracker.messageSentNanos` instead (`CLIENT_SIDE_METRICS_SPEC.md #2`) |
| Per-attempt `server_latencies` (session path) | `InvokeResult.Stats.BackendLatency` (typed proto duration on the vRPC response frame) | stamped once by `sessionTable.stampAttempt` (`internal/session/table.go:245-247`); MUST NOT be read from `x-goog-cbt-*` headers on this path; classic path uses `ExtractServerLatency` + t4t7 fallback (`CLIENT_SIDE_METRICS_SPEC.md #2`) |
| Session-tracer OTel histograms (`session.durations`, `session.open_latencies`, `session.uptime`, `transport_latencies`) | `sessionTracer` (`internal/transport/session_tracer.go:121`); registered once by `InitializeSessionMetrics` (`:67-114`) | Sole writer; `transport_latencies` recorded only from `session_pool.go:650` under the positive-delta gate at `:646-647`; `session_name` label is pool-scoped (bounded), NOT `Session.LogName` (unbounded); do NOT record from any other site or add a second registration path (`CLIENT_SIDE_METRICS_SPEC.md #3`) |
| Debug snapshot lock discipline | `SessionPoolImpl`/`Session`/etc. — snapshot methods only | MUST take at most RLock, release before returning value; z-pages hold no lock across HTTP write (spec #15) |

---

## Where to add new things (decision guide)

- **New vRPC method** (e.g., `SessionSampleRowKeys` if it ever ships): (1) proto/descriptor in `session_descriptors.go`; (2) tag/dispatch site in `session_vrpc.go`; (3) `SessionTableApi` method + impl in `internal/session/table.go`; (4) if surfaced on public API, add a `TableShim` case. No `Diverter` change.
- **New load-balancing strategy**: add an `AfePicker` impl in `afe_picker.go`; extend the `LoadBalancing.Strategy` enum in `client_configuration_manager.go`; `SessionPoolImpl.UpdateConfig` wires it. No `Session` change.
- **New pool-shaping knob**: add the field to `sessionPoolConfig` in `client_configuration_manager.go`; extend the proto parsing; `SessionPoolImpl.UpdateConfig` consumes it. No `Diverter` or `Session` change.
- **New z-page**: add `debugview/<name>.go`; extend `SessionDebugProvider` (or a sibling) with the new snapshot method; update every fake provider (see B3); mount in `debugview/handler.go`. No pool internals import.
- **New resource type** (like a hypothetical "GlobalTable"): add `SessionClient.OpenGlobalTable` in `internal/session/client.go` following the read/write closure pattern (`SESSION_SPEC.md` #11); add descriptors; if the resource is read-only like MatView, pass `openWrite=nil`.
- **New session-level state field**: check Part C first. If the concern already has a sole owner elsewhere, that's a bug — do not duplicate.
