# SessionPool + Picking — Behavioral Spec

**Scope.** This file governs the **runtime behavior** of code under `bigtable/internal/transport/session_pool*.go`, `session_list.go`, `session_pool_lifecycle.go`, `pool_sizer.go`, `session_creation_budget.go`, all `AfePicker` implementations (`afe_picker.go`), `bigtable/table_shim.go`, `bigtable/internal/transport/diverter.go`, and `bigtable/debugview/**`. It covers pool topology per resource, the AFE picker + K-choice + PeakEwma discipline, Diverter+TableShim routing, debug-view non-blocking rules, server-driven pool scaling, and the sessionList per-handle state machine that underpins all of the above. Any change to those files MUST be checked against the 6 invariants below.

**Sibling behavioral specs.** `SESSION_SPEC.md` (per-Session lifecycle, 10 invariants) · `SESSION_CLIENT_SPEC.md` (SessionClient topology, channel pool, config, OpenSession envelope, 4 invariants) · `CLIENT_SIDE_METRICS_SPEC.md` (per-attempt metrics field provenance).

**Component/boundary spec.** `SESSION_COMPONENT_SPEC.md` — layering, ownership, import direction. Read it before any structural refactor.

**How to use.** Read top-to-bottom before editing files in scope. Cross-references to other specs use `<FILE>.md #N`-style anchors. When a change spans layers (e.g., a config knob that also reshapes pool sizing), verify against every spec in scope.

**Java parity.** Where the two clients differ, both sides are cited. Deviations require an explicit note in the invariant.

---

### 1. Every resource has two session pools — read and write — except MaterializedView, which has read only

- `SessionClient.OpenTable(name)` → `sessionTable{readPool, writePool}` with descriptors `READ_ROW` / `MUTATE_ROW` (`client.go:369`).
- `SessionClient.OpenAuthorizedView(view)` → `sessionTable{readPool, writePool}` with descriptors `READ_ROW_AUTH_VIEW` / `MUTATE_ROW_AUTH_VIEW` (`client.go:397`).
- `SessionClient.OpenMaterializedView(view)` → `sessionTable{readPool, writePool=nil}` with `READ_ROW_MAT_VIEW`; **MatView is read-only by contract** (`client.go:403-415`). `MutateRow` on a MatView MUST return `ErrWriteNotSupported` (`table.go:29-31, 154`).
- Pools are **`lazyPool`** — opened on first `ReadRow` / `Apply`, not at `OpenTable`. Failed opens are **not cached** (a transient `proto.Marshal` failure MUST NOT strand the table); the next call retries. See `SESSION_CLIENT_SPEC.md #1` for the two-layer lazy-creation topology.
- **Read and write pools do not share sessions.** Each pool holds its own set of Sessions typed for its direction (via `SessionType` on the underlying `Session`), each running its own OpenSession bidi stream. This is what keeps the multiplex=1 rule (`SESSION_SPEC.md #2`) from starving cross-direction traffic.
- Behavioral shift vs classic path: marshal failures on session payloads surface on first `ReadRow`/`Apply`, not at `OpenTable`. Callers that expected eager failure will see it later.

### 2. The AFE picker is two-tier, K-choice, and PeakEwma-OK-gated — Java-parity

The pool does NOT route to individual sessions directly. Every `CheckoutSession` runs a two-step dispatch (`session_pool.go:360-370`):

1. `ready := sl.ReadyAfes()` — snapshot of AFEs with ≥1 idle session (`session_list.go`).
2. `afe, decision := picker.PickAfe(ready)` — pick an AFE via one of three strategies.
3. `sh := sl.Checkout(afe)` — dequeue one session from that AFE's idle list.

The old flat `LeastInFlightPicker` over all sessions is gone. `SessionHandle` no longer holds per-session `ewma`/`freeSignal` — those live per-AFE on `afeHandle`, and wake-ups fire centrally from `s.hooks.onSlotDrained()` (the drain-driven `SessionHooks.OnSlotDrained` closure installed by `SessionPoolImpl.createSession` in the `SessionHooks{...}` literal handed to `NewSession`; see `SESSION_SPEC.md #2`). The pool's `Invoke` return path does NOT re-enqueue or wake — under Java-parity slot lifecycle, drain and "caller returned" can be far apart (ctx.Done leaves the slot held until the server response drains it), so folding both actions into the drain site is the only way to keep the queue honest without a busy-skip loop at `Checkout`.

**Three picker impls, chosen by server config `Session.SessionPool.LoadBalancing.Strategy`** (`SESSION_CLIENT_SPEC.md #3`):

| `Strategy` | Impl | Behavior | Reason string emitted |
|---|---|---|---|
| `Random` | `SimpleAfePicker` (`afe_picker.go:76-88`) | Uniform random over all ready AFEs | `"uniform-random"` |
| `LeastInFlight` (default) | `LeastInFlightAfePicker` (`afe_picker.go:111`) | K-choice: sample `RandomSubsetSize` AFEs, pick min in-flight | `"min-inflight"` |
| `PeakEwma` | `LeastLatencyAfePicker` (`afe_picker.go:136`) | K-choice: sample K AFEs, pick min `e2eEwma.Value()` | `"min-latency"` |

`pickerFromLoadBalancing` (`session_pool.go:553-577`) is the sole factory; unrecognized strategy falls back to `LeastInFlightAfePicker(defaultAfeRandomSubsetSize)`.

**K-choice / Power-of-Two-Choices.** For `LeastInFlight` and `PeakEwma`, when `len(ready) > RandomSubsetSize`, the picker samples that many candidates uniformly at random then applies the min-cost rule (`decisionFor` in `afe_picker.go:143-150`). Rationale: full-scan min creates herd effects (every caller picks the same "best" AFE); K-choice is randomized so no single AFE gets a rush. Default K = 2 (Java parity).

**PeakEwma updates are OK-gated.** `sessionList.RecordVRpcOutcome(sh, e2e, backend, ok)` (`session_list.go:238-253`) — non-OK responses do NOT feed the per-AFE trackers. Rationale: a fast-failing AFE would otherwise look free-cost and game the picker into steering more traffic at it. Java parity: `SessionList.java:181-187`.

**PeakEwma seeds are fixed:** `transportEwmaSeed = 500µs`, `e2eEwmaSeed = 1ms` (`session_list.go:31-37`). Matches Java. New AFEs don't win by looking zero-cost.

**Picker uses `e2eEwma`, NOT `transportEwma`.** `LeastLatencyAfePicker` reads `getE2eCost()` equivalent; `transportEwma` is telemetry-only (visible on `afez`/`loadz`, not consumed by the pick). Java parity: `LeastLatencyPicker.java:59-60` reads `getE2eCost()`; `getTransportCost()` has no callers. Rationale: e2e is what users experience; transport-only would isolate wire+AFE overhead but miss bad-backend AFEs.

**`PickDecision` is the picker's audit trail** (`afe_picker.go:37-55`). Every `PickAfe` returns `PickDecision{Candidates []PickCandidate, Winner *afeHandle, Reason string}` so `loadz` renders the K-choice trace verbatim. Recorded to a 500-slot ring buffer on `SessionPoolImpl.pickHistory` plus an `afePickCounts` map, surfaced via `SessionDebugProvider.LoadBalancingSnapshots()`. The 500-slot bound is a MUST — unbounded history would violate `SESSION_POOL_SPEC.md #4` (debug non-blocking, bounded work per request).

**Deadlock trap — `recordPickDecision` takes `pickerName` as a parameter.** `CheckoutSession` already holds `p.mu` when it calls the picker; re-reading `p.picker.Name()` from a callee would re-acquire `p.mu` and deadlock. The pattern MUST be:

```
p.mu.Lock()
picker := p.picker
pickerName := picker.Name()
p.mu.Unlock()
// ... call picker.PickAfe(...) ...
p.recordPickDecision(decision, pickerName)   // pickerName passed in
```

Any new pool method that reads `p.picker.Name()` from a method called by `CheckoutSession` is a re-entrant deadlock — pass the name in or read via an atomic snapshot.

**Zero AFE ID is a legal "unknown" bucket** (`SESSION_SPEC.md #3`) — still routable through the picker like any other AFE, but the PeakEwma trackers on `AfeID=0` are per-bucket, not per-real-backend. Fresh unknown always starts at seed values.

**When no candidates are ready.** `PickAfe` returns `(nil, PickDecision{Reason: "no-candidates"})` and `CheckoutSession` yields a "no ready AFEs" error → classified as `AttemptState=Rejected` per `SESSION_SPEC.md #9` (terminal, caller sees it directly, not retried by `RetryingVRpc`). Waiter queue is bounded by `Session.SessionPool.NewSessionQueueLength` from server config.

**GOAWAY re-shapes `ReadyAfes()` — see `SESSION_SPEC.md #6`.** A session in Closing state is removed from its AFE's idle list synchronously via `notifyClosing`, so within one CheckoutSession tick after GOAWAY the pool stops routing new work to that session. The picker doesn't need special-case GOAWAY handling — it just picks over `ReadyAfes()` which reflects the current routing set.

### 3. Traffic split between session and classic (unary) pools is a two-piece routing tier: `Diverter` (policy) + `TableShim` (mechanism)

**`Diverter` (`internal/transport/diverter.go`) — policy layer.**
- Stores one `sessionLoad` (float64, 0.0–1.0) as `atomic.Uint64` of `math.Float64bits(load)`. Updated by `SetSessionLoad(load)` — the ONLY writer is `ClientConfigurationManager` via `SessionLoadListener` (`SESSION_CLIENT_SPEC.md #3`). Manual overrides for tests/staging use the same setter.
- `UseSession()` decides per call: `load<=0` → false; `load>=1` → true; otherwise `rand.Float64() <= load`. Every call increments either `sessionPicks` or `classicPicks` (atomic counters) so `configz`/`sessionz` can show **actual** ratio vs configured — the two diverge during rollouts and are the ground-truth signal.
- Diverter is stateless per-call and **has no memory of a specific row/key** — the split is stochastic across calls, not sticky. A single logical operation on the same row may land on classic once and session next time. Any invariant that assumes the same connection for two consecutive calls MUST NOT rely on this layer.
- Snapshot: `DiverterSnapshot{SessionLoad, SessionPicks, ClassicPicks}` — surfaced under the `SessionDebugProvider.Diverter()` method (see `SESSION_POOL_SPEC.md #4`).

**`TableShim` (`bigtable/table_shim.go`) — mechanism layer.**
- Implements the public `TableAPI` — this is what user code holds when running mixed-mode. Owns `(classic TableAPI, session SessionTableApi, diverter *Diverter)`. Any of `session` / `diverter` may be nil → **shim degrades to classic-only** silently. This is the fallback contract when session support is not enabled or the pool failed to open.
- Per-call routing rule: `if !t.useSession() { classic } else { session }`. `useSession()` = `session != nil && diverter != nil && diverter.UseSession()`.
- **`session` is a `*sessionTableHandle` that self-heals across cache eviction.** The pointer TableShim caches at Open time stays valid for the shim's lifetime; the wrapper routes evicted RPCs through `cache.getOrOpen`. Go-specific hazard — Java's session-table cache has no TTL sweeper, so this handle-past-eviction gap doesn't exist there and Design C has no direct Java analog. Mechanism lives in `session_table_cache.go` (see `sessionTableHandle.dispatch` + `resolveSuccessor`); guarded by `TestSessionTableHandle_EvictedSelfHeals` / `TestSessionTableHandle_SweeperEvictionSelfHeals`.
- Owns **all proto ↔ `bigtable.Row` conversion** at the boundary — the `internal/session` package stays proto-native (never sees `bigtable.Row`, `Mutation`, `Filter`, etc.). This is how the two data planes stay decoupled.
- Method routability is **fixed by shape**, not by config — the shim MUST NOT attempt to route an operation whose vRPC equivalent doesn't exist. Enforced today as:

| `TableAPI` method | Routable? | Why |
|---|---|---|
| `ReadRow` | via Diverter | `SessionReadRow` vRPC exists |
| `Apply` (non-conditional) | via Diverter | `SessionMutateRow` vRPC exists |
| `Apply` (conditional, `m.isConditional`) | **always classic** | `CheckAndMutateRow` has no vRPC equivalent |
| `ReadRows` | always classic | streaming reads not in vRPC surface |
| `SampleRowKeys` | always classic | no vRPC equivalent |
| `ApplyBulk` | always classic | no vRPC equivalent |
| `ApplyReadModifyWrite` | always classic | no vRPC equivalent |

- **Read/write direction determines which of the two session pools is engaged** (`SESSION_POOL_SPEC.md #1`): `ReadRow` → session table's read pool; `Apply` → write pool. The Diverter's decision precedes the pool split — it says "session vs classic", then the direction picks read-pool vs write-pool inside the session side.
- Response-side plumbing lives in the shim, not the session package: `WithFullReadStats` callbacks fire from `TableShim.ReadRow` after `protoRowToRow` converts the response.
- Errors from either side are surfaced to the caller as-is — the shim does NOT retry a session-side failure on classic. That would violate the retry-oracle contract (`SESSION_SPEC.md #9`): a session `TransportFailure` on a non-idempotent `Apply` is not automatically safe to re-run on classic.

### 4. Debug views (all `/-z` pages) MUST NOT block hot-path latencies

Every debug view — `sessionz`, `afez`, `flightz`, `loadz`, `channelz`, `configz`, `tcpz`, `debugtagsz` — is a **passive observer** of session/pool state. It MUST NOT be able to slow down a real vRPC, session Send/Recv, pool checkout/return, or heartbeat.

**Concrete rules:**

- **Snapshot returns a value, not a live view.** Every `SessionDebugProvider.Snapshot()`, `LoadBalancingSnapshots()`, `Diverter()`, `Snapshot()` on any provider MUST copy the state it needs under its lock and release the lock before returning. The z-page handler holds no lock while writing the HTTP response body (`debugview/sessionz.go:100-104`, `debugview/afez.go:104`, `debugview/loadz.go:125`, `debugview/configz.go:61`).
- **Snapshots take at most an RLock, never a write lock, on any mutex held by the hot path.** `SessionPoolImpl.mu` and `sessionList.mu` are hot-path mutexes (acquired by `CheckoutSession`, `ReleaseSession`, `RecordVRpcOutcome`, `AddSession`). A snapshot method that takes the write side would serialize the hot path against a random HTTP request.
- **No lock on `Session` is held across HTTP write.** `Session`'s ring buffer (`sessionDebug.events`) and slow-vRPC log MUST be snapshotted with a short RLock or via `atomic.*` reads, then rendered lock-free.
- **Bounded work per request.** Ring buffers are fixed-size (session event ring, `pickHistory` at 500, `pollHistory` capped). No z-page may iterate an unbounded structure — if a snapshot would exceed a bound, it MUST truncate and mark the response with a `truncated=true` flag.
- **Metric emission is separate from snapshot rendering.** OTel histograms/counters are recorded synchronously on the hot path by the tracer (`internal/metrics/tracer.go`) — that's the source of truth. Z-pages read those metrics via the OTel SDK's own async collection path; they MUST NOT compute derived statistics inline that would require iterating live pool state a second time.
- **Auto-refresh cadence has a floor.** Each z-page's client-side refresh MUST be ≥ 2s (currently 2–10s per view). No page may poll faster; a 100ms refresh over 20 pools would burn measurable CPU on snapshot construction.
- **Provider dependency is via interface, not concrete type.** Per `SESSION_COMPONENT_SPEC.md` B3, debug code takes `SessionDebugProvider` (and siblings), never `*SessionPoolImpl`. This is what lets snapshot semantics evolve without breaking the debug UI, and it's what makes the "no hot-path lock" invariant enforceable — the interface says "return a snapshot," not "give me a pointer to internal state."
- **A hung z-page MUST NOT hang the process.** All snapshot methods MUST complete in bounded time even if the target pool is deadlocked (i.e. tryLock semantics or best-effort read of atomics). A z-page that blocks on `p.mu` forever, waiting on a stuck checkout, turns a debug URL into a liveness weapon.

**How the mixed-mode Client stays honest.** `mixedModeSessionDebug` (`bigtable/session_debug.go:66`) MUST NOT synthesize snapshots by reaching into `*Client` internals. It composes the underlying `SessionClient`'s provider with the classic-path diverter snapshot, both accessed through interfaces. Any new mixed-mode observability field goes on the provider interface, not on `*Client`.

### 5. Session pool scaling is server-driven and MUST NOT overwhelm the control plane

Pool size respects `MinSessionCount` / `MaxSessionCount` and `Headroom` from `GetClientConfiguration` (`SESSION_CLIENT_SPEC.md #3`), and rate-limits `OpenSession` creation via `NewSessionCreationBudget` + `NewSessionCreationPenalty` back-off + `ConsecutiveSessionFailureThreshold` circuit breaker — so a bad server response cannot trigger a session-creation storm. Scale-down is passive: closed sessions are simply not replaced when the pool has slack above headroom (Java-parity replace-on-close; supersedes any active-reaper approach).

**When more sessions are added.** On every `PoolSizer` tick (`pool_sizer.go:161-199`), `DesiredCapacity = clamp(SessionsInUse + IdleHeadroom, MinSessions, MaxSessions)` where `IdleHeadroom = max(minIdleSessions, ceil(SessionsInUse × HeadroomPct))` (default 10%). A scale-up (`Delta > 0`) fires **only** when `DesiredCapacity > EventualCapacity` — i.e., in-use load has grown past what already-open plus already-pending sessions can absorb with headroom. Startup fills to `MinSessions` unconditionally so a fresh pool always has that floor before the first request.

### 6. sessionList tracks a strict per-handle state machine — six named invariants gate every mutator

The AFE grouping data structure (`session_list.go`) owns six interdependent pieces of state — `sh.inExpectedCount`, `handleToAfe[sh]`, `afe.sessions`, `afe.refCount`, `sl.readyCount`, `sl.afesWithReady` — and every public method (`OnSessionStarted`, `Checkout`, `ReleaseToPool`, `OnSessionClosing`, `OnSessionClosed`, `ReadyAfes`, `Prune`) MUST preserve the invariants below. The state model is copied verbatim from the file's top-of-file doc block so specs and code cannot drift.

A registered SessionHandle transitions through:

```
  NotRegistered → Idle → InFlight → Closing → Closed
                       ↺ ReleaseToPool

  NotRegistered  no entry in handleToAfe.
  Idle           handleToAfe[sh] = afe, sh in afe.sessions, inExpectedCount=true.
  InFlight       handleToAfe[sh] = afe, sh NOT in afe.sessions, inExpectedCount=true.
  Closing        handleToAfe[sh] = afe, sh NOT in afe.sessions, inExpectedCount=false.
  Closed         handleToAfe has no entry for sh.
```

Transitions:

| From | To | Method | Triggered by |
|---|---|---|---|
| NotRegistered | Idle | `OnSessionStarted` | `SessionPoolImpl.onActive` (fired via the per-session `hooks.OnActive` closure wired in `createSession`) |
| Idle | InFlight | `Checkout` | `CheckoutSession` after `picker.PickAfe` |
| InFlight | Idle | `ReleaseToPool` (only if `inExpectedCount`) | `s.hooks.onSlotDrained()` → `SessionHooks.OnSlotDrained` — the drain-driven closure installed by `SessionPoolImpl.createSession` in the `SessionHooks{...}` literal handed to `NewSession`, **not** the pool's Invoke return path (Java-parity slot lifecycle: drain and "caller returned" can be far apart; see `SESSION_SPEC.md #2`) |
| {Idle, InFlight} | Closing | `OnSessionClosing` | Session's `notifyClosing` hook |
| {Idle, InFlight, Closing} | Closed | `OnSessionClosed` | Session's `notifyClosed` hook |

Invariants (all guarded by `sl.mu`):

- **I1** `sh.inExpectedCount == true` ⇒ `handleToAfe[sh] != nil`
- **I2** `readyCount` == count of `sh` in `handleToAfe` with `inExpectedCount==true`
- **I3** `afesWithReady == { afe : len(afe.sessions) > 0 }`, as a set (order irrelevant)
- **I4** `afe.refCount` == count of `sh` in `handleToAfe` pointing at `afe` (idle + inFlight + closing)
- **I5** `sh in afe.sessions` ⇒ `handleToAfe[sh] == afe` AND `inExpectedCount==true`
- **I6** `refCount` is decremented ONLY on `OnSessionClosed` (Closing keeps the slot warm)

**Lock order.** `sl.mu` is always innermost. `pool.mu` MUST NOT be taken while holding `sl.mu` (also stated in `SESSION_COMPONENT_SPEC.md §B8`). Every `sessionList` method takes `sl.mu` itself; production callers never hold both mutexes.

**Why I5 is load-bearing — WaitServerClose retry storm.** `ReleaseToPool` MUST early-return when `!sh.inExpectedCount`. Omitting this guard causes the following: a GOAWAY landing on an in-flight session fires `OnSessionClosing` (drops the handle from the expected set via I2, flips `inExpectedCount=false`) while the RPC is still running. When the RPC's defer then calls `ReleaseToPool`, without the guard the drained handle gets re-appended to `afe.sessions` — violating I5. The next `CheckoutSession` picks it, `Session.Invoke` returns `Unavailable("session is not active (state: WaitServerClose)")`, and callers see a retry storm where the same session ID reappears at attempts 2 and 3. Regression coverage lives in `session_list_test.go` (state-machine unit tests) and `session_pool_lifecycle_test.go:*OnClosing*` (end-to-end accounting).

**Why I6 matters — sizer stability across the close handshake.** `refCount` decrementing only on `OnSessionClosed` (never on `OnSessionClosing`) keeps the AFE bucket's capacity accounting stable during the ~ waitServerCloseGrace window between GOAWAY and the actual `CloseSession` reply. The sizer (`SESSION_POOL_SPEC #5`) reads `refCount`-derived counts; premature decrement would cause a scale-up burst on every server-driven session recycle.

**Invariant IDs appear as inline comments** on every mutator in `session_list.go` (e.g. `// I2`, `// I5`) so a reader stepping through a method can jump to this spec to see the rule being maintained.

---

**See `SESSION_COMPONENT_SPEC.md`** for the component reference map and boundary/layering rules that prevent one component's logic from muddling into another's.
