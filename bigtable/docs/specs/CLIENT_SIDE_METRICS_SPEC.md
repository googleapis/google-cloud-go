# Client-Side Metrics — Behavioral Spec

**Scope.** This file governs the **runtime behavior** of client-side metrics emission across both data paths: (1) per-attempt stamping — `bigtable/internal/metrics/**` (tracer + attribute plumbing + OTel adapters), the classic-path stamp sites in the unary interceptor chain (`bigtable/gax_wrapper.go`, `bigtable/bigtable.go`), and the session-path stamp sites in `bigtable/internal/session/table.go` (`sessionTable.stampAttempt`) driven by `InvokeResult` from `bigtable/internal/transport/session_vrpc.go`; AND (2) session-tracer-hosted OTel histograms — `bigtable/internal/transport/session_tracer.go` (`sessionTracer`, `InitializeSessionMetrics`) plus the per-vRPC recording site at `bigtable/internal/transport/session_pool.go:646-650`. It covers what labels every emission MUST carry, where each label comes from on each path, and why those sources are architecturally distinct. Any change to those files MUST be checked against the 3 invariants below.

**Sibling behavioral specs.** `SESSION_SPEC.md` (per-Session lifecycle) · `SESSION_CLIENT_SPEC.md` (SessionClient topology + config + OpenSession envelope) · `SESSION_POOL_SPEC.md` (pool topology, picking, routing, scaling, debug non-blocking).

**Component/boundary spec.** `SESSION_COMPONENT_SPEC.md` — layering, ownership, import direction. See especially Part C ownership matrix for who writes each attribute.

**How to use.** Read top-to-bottom before editing files in scope. Cross-references to other specs use `<FILE>.md #N`-style anchors.

**Room to grow.** This file starts with 1 invariant (routing/identity attributes per attempt), but is deliberately scoped to hold future metrics rules — `client_uid` stability, method-label discipline, latency-histogram bucket contract, exemplar rules, cardinality budget, per-attempt vs per-operation aggregation. Add new invariants as #2, #3, … as they are decided. Do NOT invent invariants that aren't already understood — reserve the numbering.

**Java parity.** Where the two clients differ, both sides are cited. Deviations require an explicit note in the invariant.

---

### 1. `cluster_id` / `zone_id` / `transport peer` MUST be populated per-attempt on BOTH the classic (unary) and session (vRPC) paths, from path-specific sources

Client-side metrics (attribute labels on `attempt_latencies`, `attempt_latencies2`, `operation_latencies`) MUST carry the same set of routing/identity attributes regardless of which data path served the call. Cross-path dashboards would otherwise be unusable — half the traffic would be missing `cluster_id`. But the *source* of each field differs by path, and that difference is architectural, not accidental.

**Fields both paths MUST populate per attempt:**
- `cluster_id` — Bigtable cluster that served the request.
- `zone_id` — GCP zone of that cluster.
- `client_uid` — stable client identity (from the metrics factory).
- Transport peer labels — `transport_type`, `transport_region`, `transport_zone`, `transport_subzone` (from the AFE/backend PeerInfo).

**Where each field comes from — classic (unary) path:**
- **`cluster_id` + `zone_id`:** from the response's `ResponseParams` proto, packed into gRPC headers (`x-goog-cbt-cookie-*` routing cookies) and trailers by the server. Extracted per-attempt in `bigtable/internal/metrics/util.go:96-102` via `proto.Unmarshal` of the header/trailer bytes into a `btpb.ResponseParams`. Populated on the tracer's `attempt` state in `onAttemptCompletion`.
- **Transport peer labels:** from the gRPC connection's grpc.Peer info — a per-attempt observation, potentially different across attempts even in one operation.
- **Retry cookies:** the same `x-goog-cbt-cookie-routing-cookie` header is round-tripped as-is on the next attempt to preserve routing stickiness; that flow is separate from the metrics stamp but shares the same header.

**Where each field comes from — session (vRPC) path:**
- **`cluster_id` + `zone_id`:** from the vRPC response's typed `ClusterInformation` field, plumbed via `InvokeResult.ClusterInfo` (`session_vrpc.go:44-46, 216-219`). Stamped per-attempt on the tracer in `sessionTable.stampAttempt` → `att.SetClusterID(result.ClusterInfo.ClusterId)` / `att.SetZoneID(result.ClusterInfo.ZoneId)` (`internal/session/table.go:238-240`). **No `ResponseParams` unmarshal** — the server sends the cluster identity as a typed field on the vRPC response frame, not as an opaque header blob.
- **Transport peer labels:** from `InvokeResult.PeerInfo` (`session_vrpc.go`), which is a pointer to the owning `Session.peerInfo` — **the same PeerInfo every attempt on the same session sees**, because it was parsed once from the `bigtable-peer-info` header at session open (`SESSION_SPEC.md #3`). This is a real semantic difference from classic: on classic, transport peer can vary per attempt; on session, it's fixed for the session's lifetime.
- **Retry cookies:** vRPC does not use `x-goog-cbt-cookie-routing-cookie`. Retry routing on session is a property of picker + AFE selection (`SESSION_POOL_SPEC.md #2`), not a header round-trip.

**Consequences worth calling out:**

- **A classic-path retry may hop clusters.** The `cluster_id` on attempt N-1 and attempt N may differ (routing cookie updates between attempts). Dashboards MUST NOT assume `cluster_id` is invariant across an operation.
- **A session-path retry within the same session cannot hop backends.** All attempts on session S have identical `transport_*` labels. Retries that need a different backend must be checked out onto a different session by `RetryingVRpc` — an observable, dashboard-visible event.
- **`ClusterInfo` may be nil on a session response.** Server MAY omit it on some responses (typically errors); the metric stamp gracefully skips (`session/table.go:238` guards `if result.ClusterInfo != nil`). Classic path has the same nil-guard on `ResponseParams` unmarshal.
- **BOTH paths MUST NOT leak session-only sources into classic tracers, or vice versa.** The metrics tracer is a shared type (`internal/metrics/tracer.go`) but the *stamp sites* live in path-specific code — classic stamps from headers/trailers in the unary interceptor; session stamps from `InvokeResult` in `sessionTable.ReadRow`/`MutateRow`. Do not add a "grab ClusterInfo from wherever" utility that both call; it would collapse the source distinction and hide protocol bugs.

Ownership matrix additions live in `SESSION_COMPONENT_SPEC.md` Part C.

---

### 2. Per-attempt latency labels on the session path MUST be sourced from `InvokeResult` and stamped in exactly one place (`sessionTable.stampAttempt`)

The session path bypasses gRPC's per-call stats handler (one bidi stream carries many virtual RPCs), so the classic path's plumbing — `blockingLatencyTracker` fed by an `OutPayload` stats event, `ExtractServerLatency` reading `x-goog-cbt-*` metadata — is unavailable and MUST NOT be simulated on the session side. Instead, `Session.Invoke` captures every observable per-attempt latency locally (or receives it typed from the server) and returns them on `InvokeResult`; `sessionTable.stampAttempt` (`internal/session/table.go:233-254`) is the single writer onto the `AttemptTracer`. Every session RPC method (`ReadRow`, `MutateRow`, `SampleRowKeys`, …) MUST route its result through `stampAttempt` — scattering the stamp across method-specific code would drift from the shared source and break dashboards.

**`client_blocking_latencies`** (`MetricNameClientBlockingLatencies` = `"throttling_latencies"` on the OTel wire; label the code with the internal constant so a future rename is a one-site change).
- **Definition (both paths, identical):** `ConvertToMs(send_timestamp − attempt_start)`. Captures everything the client added before the frame hit the wire — gRPC LB pick / session-pool checkout / `activeRPC` CAS / proto marshal.
- **Classic source:** gRPC's stats handler fires `OutPayload`, which stamps `messageSentNanos` on `AttemptTracer.blockingLatencyTracker`; `Tracer.RecordAttemptCompletionWithMetadata` subtracts `attempt.startTime` (`internal/metrics/tracer.go:815-820`).
- **Session source:** `Session.Invoke` sets `result.SentAt = time.Now()` immediately before `s.Send(sessionReq)` (`session_vrpc.go:124-127`) — after rpcID assignment, marshal, and the `activeRPC` CAS, but before the frame reaches the bidi stream. `stampAttempt` computes `att.SetClientBlockingLatency(ConvertToMs(result.SentAt.Sub(att.StartTime())))` (`internal/session/table.go:242-243`).
- **Zero-guard:** stamp is skipped when `SentAt.IsZero()` OR `att.StartTime().IsZero()` — `Invoke` returned before the pre-send capture (state-check refusal, immediate context cancel, pool checkout failure). Classic has the analogous `messageSentNanos > 0` guard. A skipped stamp records zero, which OTel treats as "no measurement" — that gap semantics is intentional, do not synthesize a fallback value.

**`server_latencies`** (`MetricNameServerLatencies`).
- **Classic source:** `ExtractServerLatency(headers, trailers)` reads `x-goog-cbt-*` server-timing metadata; a t4t7 fallback tracker fills in when the header is absent (`internal/metrics/tracer.go:822-828`).
- **Session source:** `result.Stats.BackendLatency` — a typed `google.protobuf.Duration` on `SessionRequestStats` (part of the vRPC response frame). `stampAttempt` converts and stamps: `att.SetServerLatency(ConvertToMs(result.Stats.GetBackendLatency().AsDuration()))` (`internal/session/table.go:245-247`).
- **Nil-guard:** stamp is skipped when `Stats == nil` OR `Stats.BackendLatency == nil` — server MAY omit either (typically on error frames). The session path has NO t4t7-equivalent fallback; if a fallback becomes necessary, add it here rather than on the classic side.

**`transport_type` / `transport_region` / `transport_zone` / `transport_subzone`** (per-attempt labels on `attempt_latencies2`).
- Fully covered by **invariant #1**. Recap: `stampAttempt` reads `result.PeerInfo` (a pointer to `Session.peerInfo`, set once by `handleOpenSession`) and calls `SetTransportType`/`SetTransportRegion`/`SetTransportZone`/`SetTransportSubZone` (`internal/session/table.go:248-253`). Same PeerInfo pointer on every attempt on session S — semantic difference from classic where `grpc.Peer` can vary per attempt.

**`cluster_id` / `zone_id`.** Fully covered by **invariant #1**. Session source is `result.ClusterInfo` (typed field on the vRPC response), not `ResponseParams` unmarshal. `stampAttempt` guards `if result.ClusterInfo != nil`.

**`InvokeResult.TransportLatency` is NOT stamped onto `AttemptTracer`, but it IS a client-side OTel metric via a *different* tracer.** It is computed as `time.Since(result.SentAt)` in the Recv path (`session_vrpc.go:215`), then fanned out from `session_pool.go:646-650` — gated on `TransportLatency > 0 && backendDur > 0` — to three sinks as `transportOverhead = TransportLatency − Stats.BackendLatency` (positive delta only):
1. **`transport_latencies` OTel histogram** — recorded via `sh.session.RecordTransportOverhead(ctx, desc.Method(), d)` → `sessionTracer.recordTransportOverhead` (`session_tracer.go:299-311`). This is a real client-side metric, but hosted on `sessionTracer` (a per-`Session` tracer), NOT the request-path `AttemptTracer`. See **invariant #3** for the full label set and emission contract.
2. **Internal pool histogram** `p.m.transportLatencyHist` — feeds `sessionz` z-page `TransportLatency p50/p95/p99` (`debugview/sessionz.go:584,724`); not exported via OTel.
3. **Per-AFE `transportEwma` on `afeHandle`** — updated by the SEPARATE call at `session_pool.go:249` (`p.sl.RecordVRpcOutcome(sh, e2e, backend, ok)` → `session_list.go:252 afe.transportEwma.Update(transport)`). Java-parity telemetry only; the AFE picker reads `e2eEwma`, NOT `transportEwma` (see `SESSION_POOL_SPEC.md #2`).

There is no `SetTransportLatency` on `AttemptTracer` and no `MetricNameTransportLatencies` in `internal/metrics/tracer.go` — because `transport_latencies` is emitted on the SESSION-scoped meter with SESSION-scoped labels (`session_name`, `afe_location`, `session_type`, `method`), not the attempt-scoped labels (`cluster_id`, `zone_id`, etc.). Do NOT copy this value onto `AttemptTracer` — the axes wouldn't match and dashboards MUST NOT try to join transport_latencies against attempt_latencies via routing labels.

**Consequences worth calling out:**
- **`stampAttempt` is the ONE-AND-ONLY session-side writer onto `AttemptTracer`.** Adding a per-method stamp (e.g., in `ReadRow` after `stampAttempt` returns) is a spec violation — dashboards depend on all session RPCs having identical stamp coverage.
- **DO NOT introduce a shared `stampLatenciesFromResult(any)` helper that both paths call.** The classic path reads headers/trailers/gRPC-stats-handler outputs; the session path reads a typed struct. Collapsing them behind one facade hides the source distinction (same reasoning as invariant #1's final bullet) and would let a protocol bug on one path silently corrupt metrics on the other.
- **A session retry across sessions gets a fresh `att.StartTime()`.** `AttemptTracer` is created per attempt; a retry that lands on a different session records the *newest* attempt's checkout+encode overhead, not cumulative. The `client_blocking_latencies` metric therefore measures per-attempt gate contention, not per-operation.

Ownership matrix additions live in `SESSION_COMPONENT_SPEC.md` Part C.

---

### 3. Session-tracer-hosted OTel histograms MUST be emitted only by `sessionTracer`, registered exactly once via `InitializeSessionMetrics`

Four OTel histograms live on a **separate tracer type** — `sessionTracer` (`bigtable/internal/transport/session_tracer.go:121`), scoped per-`Session` — distinct from the request-path `Tracer`/`AttemptTracer` in `bigtable/internal/metrics/tracer.go`. They share a `sessionMetricsOnce` registration guard (`session_tracer.go:67-114`), so a process registers them with the first non-nil `MeterProvider` it sees and never again. Passing a nil provider leaves the histograms unset; every record site is nil-guarded (`if sessionUptime == nil { return }` and friends). This ordering — one-shot registration, nil-safe recording — is what makes the label schema stable, so DO NOT add lazy-init-on-first-record fallbacks.

**Metric roster.**

| OTel name | When recorded | Recorder | Value | Labels |
|---|---|---|---|---|
| `session.open_latencies` | after `handleOpenSession` (success OR failure) | `sessionTracer.recordOpen` (`session_tracer.go:196-217`) | `msSince(startTime)` | `transport_type`, `status`, `session_type`, `afe_location`, `session_name` |
| `session.durations` | at terminal Close (any reason, including pre-open close) | `sessionTracer.recordClose` (`session_tracer.go:227-253`) | `msSince(startTime)` (single anchor, matches Java `SessionTracerImpl.uptime.elapsed()`; the `ready` label distinguishes sessions that reached `Ready` from pre-open deaths) | above + `closing_reason`, `vrpcs` (`none`/`all_ok`/`all_error`/`some_ok`), `ready` (bool) |
| `session.uptime` | periodic sample from pool scaling loop (`session_pool_scaling.go:114` calls `p.sampleActiveUptimes(ctx)`) | `sessionTracer.sampleUptime` (`session_tracer.go:277-291`) | `msSince(startTime)` per still-active session; `ready` label sourced from the tracer's `opened` flag | `transport_type`, `session_type`, `ready`, `afe_location`, `session_name` |
| `transport_latencies` | per-vRPC on OK response with valid backend latency (positive delta) | `sessionTracer.recordTransportOverhead` (`session_tracer.go:304-316`) | `TransportLatency − Stats.BackendLatency` in ms | above + `method` |

**Label discipline.**
- **`session_name` is the pool-scoped name (bounded cardinality — one per pool per process)**, matching Java's `SessionPoolInfo.name`. It is NOT `Session.LogName` (which is per-session and unbounded — a fatal cardinality bug if ever stamped as a label).
- `afe_location` = `PeerInfo.ApplicationFrontendSubzone`; empty when `PeerInfo` never arrived (pre-open close).
- `session_type` = `SessionType.String()` (`READ`/`WRITE`).
- `status` on `session.open_latencies` / `session.durations` = `"OK"` or `status.Code(err).String()`.
- **Session-tracer metrics do NOT carry `cluster_id` / `zone_id`.** The axes are session-scoped, not attempt-scoped. Dashboards MUST NOT try to join session-tracer histograms against `attempt_latencies`/`operation_latencies` on cluster/zone keys — the invariants that populate those keys (`invariant #1`) don't apply here.

**Histogram bucket contract.**
- **`transport_latencies`** uses `FineGrainLatencyBounds` (`session_tracer.go:47-60`) — linear 0→3ms at 0.1ms steps plus coarse tail out to 5000s. Matches Java's `AGGREGATION_WITH_MILLIS_HISTOGRAM`. The same bounds are shared with `attempt_latencies2` (`internal/metrics/tracer.go:442-452`); keep them in sync if either is retuned.
- **`session.durations` / `session.open_latencies` / `session.uptime`** use OTel-default bounds — session lifetimes span seconds to hours, not sub-ms, so the fine-grain distribution is inappropriate.

**Sole-writer discipline.**
- `sessionTracer` is the ONE-AND-ONLY writer of these four histograms. A "quick uptime sample" from `session_snapshot.go` or a test helper that calls `sessionUptime.Record(...)` directly MUST NOT exist — route through `sampleUptime` so labels stay identical.
- **`transport_latencies` is recorded ONLY from `session_pool.go:650`** (via `sh.session.RecordTransportOverhead`). The positive-delta gate at `session_pool.go:646-647` is the metric's correctness contract — a call site that bypasses that gate (e.g., recording raw `TransportLatency` without subtracting `BackendLatency`, or recording a negative delta from clock skew) would corrupt the histogram.
- **`InitializeSessionMetrics(meterProvider)` is the ONLY registration path.** Called once per process from the session-client bootstrap. Do NOT expose a `ReRegisterSessionMetrics` or per-Client override — the OTel meter provider is process-wide by design, and duplicate registration returns the same instrument silently (masking bugs).

**Consequences worth calling out:**
- **`transport_latencies` is a client-side OTel metric.** Prior wording that called it "session-internal diagnostic" was wrong. It IS exported to the OTel pipeline, just from a different tracer than the per-attempt metrics — see the `TransportLatency` bullet in invariant #2 for the fan-out from `session_pool.go:646-650`.
- **Session-tracer metrics have different observability semantics from attempt-scoped metrics.** They answer "how healthy are my sessions?" (uptime, close reasons, open latency), not "how healthy was this operation?" Dashboards mixing them need explicit cross-join documentation — invariant #1's axes don't transitively cover invariant #3.
- **The four histograms are declared and registered together** in `InitializeSessionMetrics` even though `transport_latencies` is per-vRPC and the other three are per-session-lifetime. Java parity: `MetricRegistry` groups them the same way. Splitting them across two init paths would just add a race window without buying anything.

Ownership matrix additions live in `SESSION_COMPONENT_SPEC.md` Part C.

---

**See `SESSION_COMPONENT_SPEC.md`** for the component reference map and boundary/layering rules that prevent one component's logic from muddling into another's.
