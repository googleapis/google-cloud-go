# Session — Behavioral Spec

**Scope.** This file governs the **runtime behavior** of a single `Session`'s lifecycle: code under `bigtable/internal/transport/session*.go` (`session.go`, `session_lifecycle.go`, `session_vrpc.go`, `session_state.go`, etc.). It covers the state machine, one-in-flight-vRPC rule, PeerInfo timing, hook ordering, close/GOAWAY semantics, heartbeat, retry oracle, and concurrency discipline. Any change to those files MUST be checked against the 10 invariants below.

**Sibling behavioral specs.** `SESSION_CLIENT_SPEC.md` (SessionClient topology, channel pool, config, OpenSession envelope, 4 invariants) · `SESSION_POOL_SPEC.md` (pool topology, picking, routing, scaling, debug non-blocking, 5 invariants) · `CLIENT_SIDE_METRICS_SPEC.md` (per-attempt metrics field provenance).

**Component/boundary spec.** `SESSION_COMPONENT_SPEC.md` — layering, ownership, import direction. Read it before any structural refactor.

**How to use.** Read top-to-bottom before editing files in scope. Cross-references to other specs use `<FILE>.md #N`-style anchors. When a change spans layers (e.g., a Session-lifecycle change that also touches pool routing), verify against every spec in scope.

**How to verify.** Invariants here are enforced by the session reviewer agents (`session-reviewer` for behavioral, `session-component-review` for boundaries) — both auto-invoked by the PostToolUse hook on session-file edits. The unit-test smoke-gate command lives in `CLAUDE.md` → "Known gotchas"; use it instead of `go test ./...` in the top-level `bigtable` package (which hangs on real-backend integration tests).

**Java parity.** Where the two clients differ, both sides are cited so drift is visible. Deviations from Java parity require an explicit note in the invariant.

---

### 1. State machine is strictly forward, 6 states
`New → Starting → Ready → Closing → WaitServerClose → Closed` (Go, `session.go:74-116`) / `NEW → STARTING → READY → CLOSING → WAIT_SERVER_CLOSE → CLOSED` (Java, `Session.java:37-50`). Monotonic-by-phase; `Invoke` / `startRpc` MUST reject anything but Ready with a retryable-tagged error (Go: `ErrSessionNotActive` tagged `StateUncommitted`; Java: `INTERNAL` on a second concurrent `startRpc`, `SessionImpl.java:410-436`).

### 2. One in-flight vRPC per session — enforced, not advisory
Multiplex limit is **exactly 1** (`multiPlexingLimit=1`, `session.go:31`). Go enforces via `claimSlot` under `slotMu`; a losing claim is a legitimate runtime signal that a prior vRPC on this session has not drained on the wire yet (typically because its caller returned via ctx.Done — see below), returned as `StateUncommitted` so the retry oracle steers to another session. Java parity: `SessionImpl.startRpc` (`SessionImpl.java:420-444`) rejects concurrent claim with UNCOMMITTED (`createUncommitedError`, retry allowed by `RetryingVRpc.java:299-311`). Every vRPC carries a monotonic `rpcId` from `nextRPCID` / `nextRpcId`; the slot is released only by `handleVRPCResponse` / `handleVRPCErrorResponse` (successful drain via `drainSlot`), the Send-failure branch of `Invoke` (no server response is coming), or `cancelActiveRPCs` (session teardown). Caller ctx.Done runs `markCancelled` but leaves the slot occupied — the eventual server response drains it as a bookkeeping-only `tagSessionVRPCCancelledDrained`. **Every request-path `drainSlot` success fires `s.hooks.onSlotDrained()`** (excluding `cancelActiveRPCs`, which is a session-teardown path — see below), which invokes the `SessionHooks.OnSlotDrained` closure installed by `SessionPoolImpl.createSession` in the `SessionHooks{...}` literal handed to `NewSession`, capturing the session's `*SessionHandle` and the pool. The closure re-enqueues the session in its AFE idle queue AND wakes one parked `Checkout` waiter — this is the sole "session became free" signal to the pool under v3. Fire sites: normal `handleVRPCResponse` deliver branch, normal `handleVRPCErrorResponse` deliver branch, cancelled-drained branches (both), and the Send-failure branch of `Invoke`. The pool's `Invoke` return path no longer re-enqueues or wakes. `cancelActiveRPCs` intentionally does NOT fire the callback: the session is on its way out; `OnSessionClosing`/`OnSessionClosed` handle removal from routing structures, and the eventual replacement session's `OnActive` covers waker wake-up. See `SESSION_POOL_SPEC.md #6` for the state-machine consequence. A response whose `rpc_id` does not match the outstanding call is **dropped**, not delivered (canary counter for wire corruption; unreachable in normal production under slotMu).

### 3. PeerInfo (AFE ID) MUST be resolved synchronously before Ready is announced
Go parses `bigtable-peer-info` (URL-safe base64 proto) inline in `handleOpenSession` and populates `peerInfo`/`AfeID` **before** `OnActive` fires (`session_lifecycle.go:266-289, 571-587`). Java's `SessionHandle.onSessionStarted` extracts `AfeId.extract(peerInfo)` before the session enters `afesWithReadySessions` (`SessionList.java:163, 376-378`). Zero AFE ID is a legal "unknown" bucket — still routable, doesn't count toward fanout.

### 4. Lifecycle hooks fire in a fixed order, exactly once each
`OnStart → OnActive → OnClosing → OnClose` (Go, `sync.Once`-guarded, `session.go:130-158`); `onReady → onGoAway? → onClose` (Java, dispatched on `sessionSyncContext`). `OnClosing`/`onGoAway` fires on **the first** exit from Ready via any path (`Close`, `ForceClose`, `handleGoAway`, `handleClose`) — subsequent triggers are no-ops.

### 5. Close is idempotent; graceful waits for in-flight vRPC; ForceClose does not
`Close`/`ForceClose`/`handleClose`/`handleGoAway` MUST be safe to call any number of times from any goroutine/thread. Graceful: Closing → drain via `quiescent`/in-flight-completion → Send `CloseSessionRequest` → `WaitServerClose`. `ForceClose`: jump past Closing, send **no** `CloseSessionRequest`, cancel active RPCs. **Close-reason is CAS-once** (`GoAway` / `MissedHeartbeat` / `Error` must beat a late `StreamEnd:*` classification).

### 6. GOAWAY does NOT cancel the in-flight vRPC

Server-initiated `GoAwayResponse` (`apiv2/bigtablepb/session.pb.go:2698-2761`) carries `{reason, description, last_rpc_id_admitted}`. On receipt, `handleGoAway` (`session_lifecycle.go:331-392`) does exactly the following, in order:

1. **Precondition assert.** `preState >= StateStarting`; a GOAWAY on a still-NEW session is a protocol oddity — recorded via `tagSessionGoawayBeforeStart` and the frame is dropped without advancing state.
2. **Ready → Closing** via `transitionTo(StateClosing, notState(Closing, WaitServerClose, Closed))`. A late GOAWAY on an already-terminal session (Closing / WaitServerClose / Closed) is a no-op — recorded via `tagSessionGoawayAfterClose` for observability but otherwise ignored (races a local teardown; harmless).
3. **`OnClosing` / `onGoAway` fires immediately** so the pool pulls the session out of `sessionList` routing structures. This is up to `waitServerCloseGrace` (30s) earlier than the actual stream close — the whole point of GOAWAY is *early* removal from routing, not synchronous teardown.
4. **`"GoAway"` wins the close-reason CAS** (`setCloseReason("GoAway")`) — beats any later `StreamEnd:*` classification that `handleClose` would otherwise stamp when the stream actually EOFs.
5. **`Reason` and `Description` from the payload are logged** via `debugf("received GOAWAY reason=%q description=%q", ...)` and recorded on the session event ring buffer so they surface on `sessionz`.
6. **In-flight vRPC is NOT canceled.** Java parity: `SessionImpl.java:689-716` — `handleGoAwayResponse` leaves `currentRpc` alone. If the server sends the vRPC response before dropping the stream, the RPC completes successfully. Only when the stream actually terminates does `handleClose → cancelActiveRPCs` fail it with `TransportFailure`. This grace period is what makes GOAWAY on server graceful drains safe for non-idempotent `Apply`: the previous behavior (fail-fast on GOAWAY) turned successful server-side commits into client-visible transport failures.
7. **Off-loop `Close` driver** — `handleGoAway` spawns a goroutine that calls `s.Close(ctx, CloseSessionRequest{Reason: CLOSE_SESSION_REASON_GOAWAY, Description: "client teardown after server GOAWAY"})` under a **30s bounded** ctx. `Close` drains via `quiescent` (or `ForceClose`s at deadline) before sending `CloseSession` — the in-flight vRPC gets its full chance without extra scheduling here.

**`LastRpcIdAdmitted` retry oracle — DEPRECATED (not implemented in the Go client).** The proto field `GoAwayResponse.last_rpc_id_admitted` (`apiv2/bigtablepb/session.pb.go:2707`) continues to be sent by the server, but the Go transport does NOT currently read it — `git grep 'LastRpcIdAdmitted' bigtable/internal/transport/` returns zero hits (only the generated proto surfaces the field). A `TransportFailure` on the drained vRPC is classified purely by the standard #9 retry oracle — idempotency plus the underlying gRPC code — with no per-`rpc_id` gating.

The originally-specced behavior — treat any vRPC with `rpc_id > LastRpcIdAdmitted` as retryable regardless of gRPC code, funneled back through `RetryingVRpc` — is deferred. If a future change reintroduces it, it MUST land as a paired code + spec + test change; verify Java parity at that point (check `SessionImpl.java` `handleGoAwayResponse` for whether Java gates retries on this field before adopting a divergent contract here).

**Session `Close` reason on GOAWAY-driven teardown.** The `CloseSessionRequest` we send server-side stamps `CLOSE_SESSION_REASON_GOAWAY` (Go) / `CLOSE_SESSION_REASON_GOAWAY` with description `"Server sent GO_AWAY_" + reason` (Java, `SessionImpl.java:706-711`). Description prefix differs; enum value matches.

### 7. Heartbeat is armed only while a vRPC is in flight
- Enforced **only while `activeRPC != nil`** — server emits heartbeats *during* long-running vRPCs; idle sessions legitimately receive none and must not be torn down.
- Deadline = **`1 × heartbeatInterval`** (default 100 ms). A single missed heartbeat trips the watchdog; healthy sessions never reach the ForceClose branch because each inbound/outbound frame's `resetHeartbeatDeadline` fires before the timer wakes.
- **Any frame in either direction resets the deadline** — request Send, response Recv, heartbeat frame, `SessionRefreshConfig`, error responses. **Unknown frame types explicitly do NOT reset** (`session_lifecycle.go:233-263`) — otherwise a rogue future frame type would mask a broken stream.
- Reset is 1 atomic load + 1 atomic store on the hot path; no mutex.

### 8. Missed-heartbeat sequence is fixed and observable
On miss, in this exact order (`session_lifecycle.go:558-572`):
1. `recordDebugTag(tagSessionHeartbeatMissed)` — fires the debug-tag counter.
2. `debugf("heartbeat MISSED — forcing close in_flight=%d last_frame_age=%v ...")` — deterministic log marker **before** ForceClose so it isn't lost to a downstream cancel race.
3. `recordEvent("hb-missed", ...)` — appended to the session's event ring buffer (surfaces on `sessionz`).
4. `ForceClose(&CloseSessionRequest{Reason: CLOSE_SESSION_REASON_MISSED_HEARTBEAT, Description: "client terminated session due to missed server heartbeats"})`.
5. `heartBeatLoop` returns; does not respawn.

ForceClose semantics on this path: transition **directly to `WaitServerClose`, skipping `Closing`**; send **no** `CloseSessionRequest` (stream is presumed dead); `cancelActiveRPCs` delivers `ErrUnavailableHeartBeatMissed` (wrapped `codes.Unavailable`), **tagged `StateTransportFailure`** (`session_vrpc.go:378`) — server may or may not have processed the request, so retry is safe only for idempotent ops. `OnClosing`/`OnClose` still fire exactly once each. `"MissedHeartbeat"` wins the close-reason CAS over any late `"StreamEnd:*"`. Java parity: `SessionImpl.java:440-443, 460-490, 605, 674, 855`.

### 9. Server is the oracle for retry — client never invents retryability from raw gRPC codes
Retry decisions consume a **three-value classification** (Go `AttemptState` in `attempt_outcome.go:28-46`; Java `VRpc.VRpcResult.State`), NOT the underlying gRPC code:

| State | Meaning | Retry rule |
|---|---|---|
| `Uncommitted` | Never left the client (encode fail, session Closing, pool rejected, ctx dead before Send) | Retry unconditionally |
| `TransportFailure` | Handed to transport, no server response observed (Send err, Recv err, ctx cancel mid-flight, heartbeat miss) | Retry **only** if idempotent |
| `ServerResult` | Server returned a definitive `ErrorResponse` (or decode err on delivered response) | Retry **only** if server attached `RetryInfo`, or code is in the narrowed always-retryable set (`RetryingVRpc.java:290-311`) |

**Retry-signal provenance.** Every input to the retry decision is either purely server-driven or purely client-driven — no signal carries mixed authority. Adding a new "server signal to opt out of retry" (beyond `RetryInfo` absence) would require a spec bump because the current oracle has no channel for it.

| Server-only inputs | Client-only inputs |
|---|---|
| **`RetryInfo` on `ErrorResponse`** — grants retry permission for a `ServerResult`; `RetryInfo.retryDelay` sets the backoff. Plumbed via `status.WithDetails` (`session_vrpc.go:320-335`); no side-channel. | **`AttemptState` classification** (`Uncommitted`/`TransportFailure`/`ServerResult`) — set at the client-side tag site (see tag reference below); the server has no channel to override the classification. |
| **Narrow always-retry code set** — a `ServerResult` without `RetryInfo` is retryable iff its gRPC code sits in the hard-coded allowlist (`RetryingVRpc.java:290-311`). | **Idempotency of the request** — blocks retry of `TransportFailure` when a mutation is non-idempotent (mutations with `TimestampMicros == -1` server-time are non-retryable). |
| **`x-goog-cbt-cookie-routing-cookie`** (classic path only) — round-tripped verbatim to preserve cluster stickiness across retries; not itself a retry gate. Session path has no analog — routing is picker-driven (`SESSION_POOL_SPEC.md #2`). | **3-attempt cap** — hard cap (`RetryingVRpc.java:305-311`), overrides any amount of server `RetryInfo`. |
| ~~`GoAwayResponse.last_rpc_id_admitted`~~ — **DEPRECATED in the Go client**; see #6. | **Deadline-fit check** — `RetryInfo.retryDelay` is IGNORED (not clipped) if it would exhaust the caller's remaining deadline (`RetryingVRpc.java:290-298`). |
|  | **Zero-value / untagged error → `ServerResult` = do-not-retry** (fail-closed by design). Any retryable path MUST call `tagErr(...)` explicitly (`attempt_outcome.go:28-42`). |

Additional invariants:
- **`Rejected` is terminal** (pool closing, wrong state, no ready AFEs) — the caller sees it, not RetryingVRpc.
- **GOAWAY does NOT re-classify an in-flight vRPC** — retry oracle fires only on the vRPC's terminal outcome (see #6).

Reference Go tag sites (`session_vrpc.go`): encode err → `Uncommitted:88-94`; session not Ready → `Uncommitted:108`; Send err → `TransportFailure:129`; ctx.Done during Recv → `TransportFailure:213`; server `ErrorResponse` → `ServerResult:225`; response decode err → `ServerResult:240`; `cancelActiveRPCs` (close/goaway/heartbeat) → `TransportFailure:378`.

### 10. Concurrency discipline is model-specific but non-negotiable
- **Go:** most Session mutable state is `atomic.*` (`state`, `peerInfo`, `refreshConfig`, `heartbeat*Nano`, `nextRPCID`). One plain field, the `hooks SessionHooks` struct, is single-writer-at-construction: `NewSession` stores the caller-supplied value before returning, and `Session.Start` is what spawns any goroutine — so all `hooks.on*()` reads on the readLoop / heartbeat / Invoke paths happen after the store and honor the Go memory model without an atomic. Two mutexes: `sendMu` (`grpc.ClientStream.Send` is not concurrent-safe) and `slotMu` — the Go analog of Java's `sessionSyncContext`, guarding the `(activeRPC, currentCancel)` pair so `claimSlot`/`markCancelled`/`drainSlot` mutate them as one atomic unit (Java `SessionImpl.java:420-457, 599-602` parity). `slotMu` is **innermost**: held only across pointer assignments, never nested with `sendMu` / `pool.mu` / `sl.mu`, no I/O, no chan ops. `deliver` is a plain send onto the per-rpc cap-1 buffered chan — `drainSlot` guarantees single-writer, so the pre-slotMu two-writers-race first-wins semantics is unreachable; the cap-1 buffer is retained as ctx.Done-vs-response tick-race defense in `awaitInvokeResult`. Lock ordering: **never take `pool.mu` while holding `sl.mu`**.
- **Java:** every mutation runs inside `sessionSyncContext` (io.grpc `SynchronizationContext`); public entrypoints trampoline via `execute(...)`; handlers assert `throwIfNotInThisSynchronizationContext()`. Uncaught exceptions invoke `abortFromUncaughtException` — re-entrancy-guarded, force-closes the stream, calls `notifyTerminalClose` exactly once.

---

**See `SESSION_COMPONENT_SPEC.md`** for the component reference map and boundary/layering rules that prevent one component's logic from muddling into another's.
