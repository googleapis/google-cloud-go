# SessionClient — Behavioral Spec

**Scope.** This file governs the **runtime behavior** of the `SessionClient` layer: `bigtable/internal/session/client.go`, `bigtable/internal/session/client_configuration_manager.go`, `bigtable/internal/transport/session_pool.go`'s construction path, and the mixed-mode `bigtable.Client` field wiring in `bigtable/client.go` that owns a `SessionClient`. It covers Client↔SessionClient topology, the shared channel pool, the authoritative config source, and the `OpenSessionRequest` handshake envelope. Any change to those files MUST be checked against the 4 invariants below.

**Sibling behavioral specs.** `SESSION_SPEC.md` (per-Session lifecycle, 10 invariants) · `SESSION_POOL_SPEC.md` (pool topology, picking, routing, scaling, debug non-blocking, 5 invariants) · `CLIENT_SIDE_METRICS_SPEC.md` (per-attempt metrics field provenance).

**Component/boundary spec.** `SESSION_COMPONENT_SPEC.md` — layering, ownership, import direction. Read it before any structural refactor.

**How to use.** Read top-to-bottom before editing files in scope. Cross-references to other specs use `<FILE>.md #N`-style anchors. When a change spans layers (e.g., a config knob that also reshapes pool sizing), verify against every spec in scope.

**How to verify.** Invariants here are enforced by the session reviewer agents (`session-reviewer` for behavioral, `session-component-review` for boundaries) — both auto-invoked by the PostToolUse hook on session-file edits. The unit-test smoke-gate command lives in `CLAUDE.md` → "Known gotchas"; use it instead of `go test ./...` in the top-level `bigtable` package (which hangs on real-backend integration tests).

**Java parity.** Where the two clients differ, both sides are cited. Deviations require an explicit note in the invariant.

---

### 1. One `Client` owns exactly one `SessionClient`; one `SessionClient` owns many session pools, all lazily created

**`Client` → `SessionClient` is strictly 1:1.** `bigtable.Client` holds `sessionImpl session.SessionClient` as a single field (`client.go:53`). When `EnableSessionPool` is set on `ClientConfig`, `NewClientWithConfig` calls `session.NewSessionClient(ctx, project, instance, appProfile, metricsProvider, o...)` **exactly once** during construction and stores the returned handle (`client.go:224`). There is no per-request `SessionClient`; every session-eligible operation on that `Client` fans through the same `SessionClient` instance. Standalone `NewSessionClient` consumers likewise get one `SessionClient` per logical client — the type is not intended to be pooled or shared across logical clients.

**`SessionClient` → many `*SessionPoolImpl` is 1:N**, keyed by `{resource, direction}`. The `SessionClient` maintains an internal `pools map[string]*ManagedPool` guarded by `poolsMu`. Each `OpenSessionTable` / `OpenAuthorizedView` / `OpenMaterializedView` call registers up to two entries (read + write; MaterializedView is read-only, no write entry). Keys use fixed prefixes so different resource kinds cannot collide:

| Call | Key(s) written |
|---|---|
| `OpenSessionTable("t")` | `"table:<full-name>:read"`, `"table:<full-name>:write"` |
| `OpenAuthorizedView("t","v")` | `"av:<table>:<view>:read"`, `"av:<table>:<view>:write"` |
| `OpenMaterializedView("v")` | `"mv:<view>:read"` |

Two `OpenSessionTable("t")` calls dedup on this key and return handles backed by the same underlying pool pair. The dedup is per-`SessionClient`; two different `SessionClient` instances would open independent pools even for the same resource — which is why the 1:1 `Client → SessionClient` rule matters (it's what guarantees one Bigtable `Client` never fans out to duplicate session infra for the same table).

**`SessionClient` does NOT cache the returned `SessionTableApi`.** The consumer does — mixed-mode `bigtable.Client` holds `sessionTables map[string]session.SessionTableApi` guarded by `sessionTablesMu` (`client.go:59-64`). Rationale: `NewSessionClient` returns fresh `SessionTableApi` handles that share the underlying pool via the `SessionClient`-level pool cache; the per-`Client` cache is only for user-facing identity (same `Open*` call returns the same handle so callers can compare by pointer). Standalone `NewSessionClient` consumers MUST implement equivalent caching if they need handle identity — the pool dedup is only inside `SessionClient`.

**Pools are lazily created — at both layers of the topology.**

- **Pool-level (per resource+direction).** `SessionTable` holds two `*lazyPool` (`internal/session/lazy_pool.go`, `internal/session/table.go`). The pool's channel-pool subscription and `OpenSession` streams do NOT fire at `OpenTable` time. First `ReadRow` on the table triggers `readPool.get()` which runs the `openRead` closure (produced by `buildLazyOpener` at `client.go:455-475`); first `Apply` triggers `writePool.get()` symmetrically. **Failed opens are NOT cached** — the next call retries. This is deliberate: a transient `proto.Marshal` failure on the OpenSessionRequest inner proto (`SESSION_CLIENT_SPEC.md #4`) MUST NOT strand the table permanently.
- **Session-level (inside a pool).** Even after a pool is opened, individual `Session`s inside it are minted on demand. A brand-new pool starts empty; `PoolSizer` (`SESSION_POOL_SPEC.md #5`) fills it to `MinSessions` on its first tick, then grows/shrinks per load and `Headroom`. `OpenSession` bandwidth is proportional to actual traffic, not to `sessionTables` cardinality.

**Consequences worth calling out.**
- **Deferred failure surface:** marshal failures on session payloads surface on first `ReadRow`/`Apply`, not at `OpenTable`. Callers that expected eager failure (matching classic Bigtable's behavior) will see it later.
- **Zero-traffic Clients open zero streams.** A `Client` that never issues a session-eligible call opens exactly zero `OpenSession` streams. Only `GetClientConfiguration` polls run from client construction time (they travel through the shared session channel pool — `SESSION_CLIENT_SPEC.md #2`).
- **Config listener registration is deferred to pool open.** Per-pool `UpdateConfig` listeners (`SESSION_CLIENT_SPEC.md #3`) only register after the pool is actually opened by `getOrCreatePool` (`internal/session/client.go:536+`). Config polls run from `SessionClient` construction — the first poll's result is stored on the `ClientConfigurationManager` and used to seed the pool at open time.

### 2. All Session Pools on a `SessionClient` share one channel pool

- `sessionClient` owns exactly one `ChannelPool` (`client.go:126-160`), one gRPC stub bound to that pool, and one `ClientConfigurationManager` polling through the same stub.
- Every `*SessionPoolImpl` constructed by that `SessionClient` — regardless of resource (table/AV/MatView) or direction (read/write) — dials via that single channel pool. Session traffic is fanned out **inside** the pool (across sub-channels sized by `ChannelPool.MinServerCount` / `MaxServerCount` / `PerServerSessionCount`), not per-pool.
- The session channel pool is **distinct from the classic (data-plane) channel pool**. Mixed-mode `bigtable.Client` holds both: the classic pool warmed with pingAndWarm, and the session pool warmed with `getClientConfigDirectAccessChecker` (no priming). The `Diverter` (`SESSION_POOL_SPEC.md #3`) is what routes each user call to one or the other.
- Standalone `NewSessionClient` skips the classic pool entirely — one channel pool per client, one stub, one config manager. All session traffic — vRPC, `GetClientConfiguration` polls, `OpenSession` streams — goes through it.
- Teardown: `SessionClient.Close()` closes every `SessionPoolImpl`, then the shared channel pool, then the metrics factory. `backgroundCancel` unwinds every per-pool goroutine parented on the client's background ctx.

### 3. `GetClientConfiguration` is the authoritative source for pool load and shape

Every knob that governs a session pool's **size, admission, load-balancing, and traffic share** MUST come from the server-driven `ClientConfiguration`, not from hardcoded client defaults (defaults exist only as bootstrap fallback until the first successful poll). The polled `clientConfig` (`client_configuration_manager.go:37-89`) carries:

| Field | Governs |
|---|---|
| `Session.SessionLoad` (float64, 0.0–1.0) | Fraction of traffic the `Diverter` sends via session pools vs classic. Emitted to callers via `SessionLoadListener` → `Diverter.SetSessionLoad`. |
| `Session.ChannelPool.MinServerCount` / `MaxServerCount` | Channel-pool sizing bounds. |
| `Session.ChannelPool.PerServerSessionCount` | Target sessions per sub-channel; drives the pool's steady-state session count. |
| `Session.ChannelPool.DirectAccessCheckInterval` / `DirectAccessErrorThreshold` | DirectAccess health probe cadence + trip threshold. |
| `Session.SessionPool.Headroom` (0.0–1.0) | Slack above in-use count that `PoolSizer` maintains; drives scale-up decisions. |
| `Session.SessionPool.MinSessionCount` / `MaxSessionCount` | Hard bounds on the pool's active session count. |
| `Session.SessionPool.NewSessionCreationBudget` / `NewSessionCreationPenalty` | Concurrency gate + failure back-off on new `OpenSession` calls (feeds `AdaptiveSessionThrottler`). |
| `Session.SessionPool.ConsecutiveSessionFailureThreshold` | Trip threshold for the creation-budget circuit breaker. |
| `Session.SessionPool.NewSessionQueueLength` | Waiter queue depth in `CheckoutSession`. |
| `Session.SessionPool.LoadBalancing.Strategy` ∈ {`LeastInFlight`, `Random`, `PeakEwma`} + `RandomSubsetSize` | Picker choice (`AfePicker` impl) and P2C sample width. See `SESSION_POOL_SPEC.md #2`. |
| `Polling.PollingInterval` / `ValidityDuration` / `MaxRpcRetryCount` | Cadence + retry policy of the config poll itself. Interval is **clamped to `MinPollingInterval = 1 min`** — server cannot ask the client to poll faster (`client_configuration_manager.go:33`). |

Additional invariants:
- **Config changes MUST reshape live pools, not just future ones.** `ClientConfigurationManager` fires registered `configListener` callbacks with a monotonic `configSeq`; each pool's `SessionPoolImpl.UpdateConfig` reshapes the picker, sizer, budget, and load-balancing strategy in place. New sessions come and go per the new bounds; existing in-flight vRPCs are undisturbed.
- **`configSeq` is monotonic** — listeners MUST ignore any callback with a `seq` older than the last one they processed (out-of-order delivery from the polling loop is possible under Close-race).
- **Close happens-before listener silence.** `ClientConfigurationManager.Close()` sets `closed.Store(true)` **before** closing `done`, waits `pollsWG`, and thereby guarantees no listener callback fires after `Close()` returns — pools tearing down cannot race an inbound `UpdateConfig`.
- **On poll failure**, the last successfully-fetched config remains authoritative until `ValidityDuration` expires; after expiry, the client MUST fall back to bootstrap defaults, not to an arbitrarily stale config. `lastResponse` / `lastErr` / `pollHistory` are kept verbatim for the `configz` debug page.
- **`SessionLoad` is the only knob that couples session and classic paths.** All other fields shape session-side infra only. This is what makes mixed-mode safe: turning `SessionLoad` to 0 quiesces session traffic without touching classic pool sizing.

### 4. `OpenSessionRequest` is a resource-agnostic transport envelope; the resource-typed inner proto is marshaled to `Payload` bytes

The wire-level `bigtable.v2.OpenSessionRequest` is deliberately resource-agnostic: `{protocol_version, payload []byte, flags}` (see `apiv2/bigtablepb/session.pb.go`, `OpenSessionRequest`). Every session pool ships the **same** envelope shape — what changes per resource is (a) the *inner* proto marshaled into `Payload`, (b) the bidi RPC method invoked on the stub, and (c) the routing headers accompanying the RPC.

**Three inner proto types, one per resource kind:**

| Resource | Inner proto | Bidi RPC method | Distinguishing fields | Route header key |
|---|---|---|---|---|
| Table | `btpb.OpenTableRequest` | `stub.OpenTable(ctx)` | `TableName`, `AppProfileId`, `Permission ∈ {READ, WRITE}` | `open_session.payload.table_name` |
| Authorized view | `btpb.OpenAuthorizedViewRequest` | `stub.OpenAuthorizedView(ctx)` | `AuthorizedViewName`, `AppProfileId`, `Permission ∈ {READ, WRITE}` | `open_session.payload.authorized_view_name` |
| Materialized view | `btpb.OpenMaterializedViewRequest` | `stub.OpenMaterializedView(ctx)` | `MaterializedViewName`, `AppProfileId` (no `Permission` — MatView is read-only) | `open_session.payload.materialized_view_name` |

**The three RPC methods are distinct stub methods**, not one polymorphic method dispatched on payload type. `OpenTable` / `OpenAuthorizedView` / `OpenMaterializedView` each open a separate bidi stream — the RPC method itself is what tells the server which resource kind is being opened; the `OpenSessionRequest.Payload` bytes carry the resource-scoped fields for *that* RPC. The server MUST NOT need to peek at `Payload` to know the resource kind; it MUST NOT need to peek at the RPC method to decode `Payload`. Both are self-consistent.

**`Permission` is baked into the pool key** (`SESSION_CLIENT_SPEC.md #1`). For Table and AuthorizedView, `Permission ∈ {PERMISSION_READ, PERMISSION_WRITE}` is encoded into the inner proto at pool construction time. This is *the* reason read and write pools are separate `*SessionPoolImpl` instances (`SESSION_POOL_SPEC.md #1`) — the `OpenSessionRequest.Payload` bytes for the read pool encode `Permission=READ` and the write pool encodes `Permission=WRITE`, so the server issues a session scoped to the right permission. MaterializedView has no `Permission` field on its inner proto and no write pool — attempting `MutateRow` on a MatView returns `ErrWriteNotSupported` client-side without any wire traffic (`SESSION_POOL_SPEC.md #1`).

**Payload construction happens once per pool.** `createPoolForPayload` (`internal/session/client.go:481-531`) marshals the inner proto into `Payload` bytes at pool creation time and stashes the resulting `OpenSessionRequest` on the pool as its `handshake`. The same `handshake` is re-used **verbatim** for every session that pool opens over its lifetime — the payload bytes are immutable per pool. This is what keeps `OpenSession` cheap: no per-session proto encoding.

**Routing metadata is derived from the same inner proto** via `SessionDescriptor.MetadataFn` (`internal/transport/session_descriptors.go:79-172`). The per-resource `MetadataFn` walks the inner proto to build `open_session.payload.*` header values, URL-escapes each, and joins them into the `x-goog-request-params` header alongside the `x-goog-request-params` resource prefix. This is what lets AFEs route the `OpenSession` stream without deserializing `Payload` — the resource identity is duplicated as headers so the transport tier can route on them directly. Headers and payload MUST agree; they are constructed from the same proto instance, so drift is not possible unless one code path bypasses `createPoolForPayload`.

**`SessionType` (`session_descriptors.go:38-77`) is the compile-time discriminator** carried on `*SessionPoolImpl` and each `Session` so the internal transport tier can tag its debug output and log names — e.g., `"OpenTablePool-3 (sushanb) [READ]"` — without re-parsing the payload bytes. The enum is `{SessionTypeTable, SessionTypeAuthorizedView, SessionTypeMaterializedView}`, set when the pool is constructed from the descriptor. `SessionDescriptor.ProtoName()` returns the bare inner-proto name (`"OpenTable"` / `"OpenAuthorizedView"` / `"OpenMaterializedView"`) used to bake the human-readable pool identifier.

**A marshal failure on the inner proto is fatal to that specific pool open**, not to the SessionClient. `createPoolForPayload` returns `fmt.Errorf("proto.Marshal session payload: %w", err)`; `lazyPool.get()` surfaces it to the caller and does NOT cache the failure (`SESSION_CLIENT_SPEC.md #1`). The `SessionClient` itself, its channel pool, and its config manager are untouched — other pools remain openable.

**`SessionRefreshConfig.OptimizedOpenRequest`** (`session.pb.go:2764+`) is the server's mechanism for supplying a *replacement* `OpenSessionRequest` the client should use on session refresh — an AFE may pre-encode a cheaper handshake (e.g., a pre-planned query for BTQL) and hand it to the client via the refresh path. When set, it overrides the pool's cached `handshake` for the next `OpenSession` call on that specific replacement session. This does not violate the "payload bytes are immutable per pool" rule — the refresh config replaces the handshake atomically on receipt, and subsequent sessions minted by the same pool use the new bytes.

---

**See `SESSION_COMPONENT_SPEC.md`** for the component reference map and boundary/layering rules that prevent one component's logic from muddling into another's.
