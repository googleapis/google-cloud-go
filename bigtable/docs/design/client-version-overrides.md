# Design: `GetClientConfiguration` overrides by grpc / bigtable version

**Status**: proposal
**Owner**: sushanb
**Last updated**: 2026-09-15

## TL;DR

Let the server return version-gated patches inside
`GetClientConfigurationResponse` so a `SessionClientConfiguration` can vary
by the running client's `grpc-go` and `cloud.google.com/go/bigtable` module
versions, without adding a client-version field to the request. The client
snapshots its own versions at startup, walks the response's
`versioned_session_overrides`, and applies the first matcher's
`FieldMask`-scoped patch on top of the base `SessionConfiguration`.

## Problem

Today `SessionClientConfiguration` is a single blob per instance/app-profile.
There is no way for the server to say "clients on grpc-go ≥ 1.83 can safely
use `session_load=1.0`; older clients stay at `0.5`" without either:

- Rolling out the same value to everyone (blocks progressive rollouts of
  session-mode); or
- Doing it on the client side with hardcoded version gates (couples the
  policy to the release cadence of every client library).

Concrete motivating cases:

1. **Session-mode rollout gates.** A fix in grpc-go 1.83+ makes bidi keepalive
   handle GOAWAY correctly under the vRPC pattern. Enabling session traffic
   for clients < 1.83 causes reconnect storms. Server needs to gate.
2. **Bigtable-client bug bands.** A regression in bigtable/go 1.51 corrupts
   `AttemptOutcome` under retries. Server can pin those clients to
   `session_load=0` until they upgrade.
3. **Feature rampup.** New pool parameters (headroom, larger min counts) may
   only be safe on client builds that ship the corresponding pool math.

## Non-goals

- Multi-axis matchers beyond `(grpc, bigtable)`. Java client, Go client
  build fingerprint, per-hostname pinning: out of scope. If they land, they
  add fields to `ClientVersionMatcher` — nothing else in this design changes.
- Server-driven feature flags for things unrelated to
  `SessionClientConfiguration` (e.g. `PollingConfiguration`,
  `TelemetryConfiguration`). Same mechanism can extend to those later by
  bumping the override target from `SessionClientConfiguration` to
  `ClientConfiguration`; deferred to keep the initial change small.
- Client-side version selection. The server ships every band it wants active;
  the client picks the first that matches. No client-side ordering or
  precedence tweaks.

## Proposal

### Wire shape

Add three additions to `google/bigtable/v2/session.proto`:

```proto
message ClientConfiguration {
  // existing fields ...
  SessionClientConfiguration session_configuration        = 2;
  PollingConfiguration       polling_configuration        = 3;
  TelemetryConfiguration     telemetry_configuration      = 4;

  // Version-gated patches applied on top of session_configuration.
  // Evaluated in order; first matching entry wins. No matching entry ⇒
  // session_configuration is used unchanged. Server orders most-specific
  // matcher first, catch-all last.
  repeated VersionedSessionOverride versioned_session_overrides = 5;
}

message VersionedSessionOverride {
  // Predicate over the running client's versions. A nil matcher matches
  // every client (catch-all).
  ClientVersionMatcher matcher = 1;

  // Partial SessionClientConfiguration whose fields overwrite the base
  // per update_mask. Only paths named in update_mask are applied.
  SessionClientConfiguration override = 2;

  // Which fields in `override` to apply. AIP-134 semantics: a masked
  // path always wins, even to clear (masked path + override doesn't set
  // it ⇒ clear on base). Empty mask ⇒ no-op (this entry is a
  // matcher-only annotation).
  google.protobuf.FieldMask update_mask = 3;
}

message ClientVersionMatcher {
  // Semver bounds per axis. Each string is optional; empty = unbounded.
  // Range is [min, max): min inclusive, max exclusive. grpc AND bigtable
  // conditions AND together. Cross-axis OR is expressed with a second
  // VersionedSessionOverride entry.
  //
  // Values are semver strings without a leading "v" ("1.83.1"). Clients
  // compare via semver ordering, not lexicographic.
  string grpc_min_version     = 1;
  string grpc_max_version     = 2;
  string bigtable_min_version = 3;
  string bigtable_max_version = 4;
}
```

### Worked example

Server wants: clients on grpc ≥ 1.83 get `session_load=1.0`; everyone else
stays at the base `0.5`.

```
ClientConfiguration {
  session_configuration: { session_load: 0.5, ... }
  versioned_session_overrides: [
    {
      matcher:     { grpc_min_version: "1.83.0" }
      override:    { session_load: 1.0 }
      update_mask: { paths: ["session_load"] }
    }
  ]
}
```

Wire size cost: ~30 bytes for one band. Server keeps the base config plus
one small entry per band, not one complete `SessionClientConfiguration` per
band.

### Client flow

1. At `ClientConfigurationManager` construction, snapshot `grpc.Version`
   and `bigtable/internal.Version` once. Versions don't change at runtime;
   snapshotting keeps the poll loop pure and avoids re-reading a constant
   on every poll.
2. On every successful poll, before the `proto.Equal` short-circuit and
   listener fanout:
   - Preserve `m.lastResponse` = raw server body (unchanged — configz keeps
     showing what the server actually sent, including the override list and
     the masks).
   - Walk `resp.VersionedSessionOverrides`. For each entry: skip if
     `Matcher` doesn't admit the snapshotted `(grpc, bigtable)` pair; else
     clone `resp.SessionConfiguration`, apply masked paths from
     `Override` → clone, replace `resp.SessionConfiguration`, and clear
     `resp.VersionedSessionOverrides` so downstream can't re-evaluate.
   - Continue the poll pipeline unchanged.
3. Listeners see the resolved config. `configz` shows both the raw response
   and the effective config side-by-side.

### Matching semantics

- **Nil matcher** matches every client — the catch-all idiom for a "reset
  to a specific override" band placed last.
- **Empty bound** on either side is unbounded (`""` grpc_min = "from any
  version").
- **Range** is `[min, max)`: min inclusive, max exclusive. Matches the
  convention used by pip, semver-tilde, and Kubernetes admission policies.
- **AND across axes**: a matcher with both grpc and bigtable bounds set
  matches only when the client falls in both ranges. Cross-axis OR is
  expressed as two separate entries.
- **Invalid semver** on either side (client-shipped or server-shipped) is
  treated as "does not match". Defensive: if we can't compare, we don't
  apply an override we can't reason about.
- **Semver, not lexicographic**: `"1.10.0"` sorts after `"1.9.0"`. Client
  uses `golang.org/x/mod/semver`.

### FieldMask semantics

Follows AIP-134:

- Each mask path names a field on `SessionClientConfiguration` to overwrite
  from `override` → base.
- If a masked path exists in the mask but `override` doesn't set that
  field, base is **cleared** at that path. Mask always wins.
- Nested paths (`"channel_configuration.min_server_count"`) walk into
  sub-messages, materializing empty parents on base if needed.
- **Unknown paths** (server named a field this client version doesn't
  ship) are silently skipped. Forward-compat concession — an older client
  applies the paths it recognizes and ignores the rest, rather than
  refusing the whole override.
- Empty or missing mask ⇒ no-op. Servers that want "apply everything in
  the payload" must enumerate paths explicitly.

## Alternatives considered

### A — Client sends version in the request

Add `ClientVersion { grpc_version, bigtable_version }` to
`GetClientConfigurationRequest`. Server picks the right config and returns
it directly.

- **Pro**: server has all the smarts; client is dumb.
- **Con**: every request carries the version even when nothing depends on
  it. Server must key its cache by `(instance, profile, grpc_version,
  bigtable_version)` — cardinality blows up with every client release. No
  way for a debug operator to see "what would this server return for grpc
  1.90?" without spoofing the request.
- **Verdict**: rejected. Puts routing logic behind an opaque server-side
  branch. The response-side variant lets the operator see the full policy
  in one place (configz).

### B — Full-replacement overrides (no mask)

`VersionedSessionOverride { matcher, complete_override }` where `override`
replaces `session_configuration` entirely.

- **Pro**: zero merge logic. No "was this zero explicit or unset"
  ambiguity.
- **Con**: wire payload duplicates every field the server didn't intend to
  change. Config is ~15 scalars deep today so the absolute cost is small,
  but it will grow. "What actually changed between bands?" requires
  diffing full configs.
- **Verdict**: rejected in favor of `FieldMask` patch. The mask makes
  authoritative "what changed" auditable at the wire level, and AIP-134
  is the standard Google API convention for this shape.

### C — Wrapper-typed patch (`google.protobuf.FloatValue` on every scalar)

`SessionClientConfigurationPatch` with wrappers on every scalar so
"unset" is distinguishable from "explicitly zero".

- **Pro**: no `FieldMask` walker; merge is a straight recursive
  nil-check.
- **Con**: parallel `Patch` type per nested layer
  (`ChannelPoolConfigurationPatch`, `SessionPoolConfigurationPatch`, ...).
  Every future scalar added to `SessionClientConfiguration` must be
  mirrored into the patch type or it's silently un-patchable. Drift-prone.
- **Verdict**: rejected. Rejecting it also because the wire size wins over
  full-replacement, but the maintenance cost outweighs the tiny wire
  saving vs. FieldMask.

## Rollout

1. **Server** ships the proto change first. Existing clients receive
   `versioned_session_overrides: []` — no-op, behavior unchanged.
2. **Java + Go clients** land the response-side plumbing behind the new
   proto. The apply-logic PR is a single-file addition
   (`versioned_override.go` + tests) plus a ~10-line hunk in
   `ClientConfigurationManager.poll`. Zero listener-contract changes.
3. **Server** starts publishing bands for the first use case (e.g. session
   traffic gate on grpc ≥ 1.83). Because clients without the response-side
   plumbing simply ignore the new field, the rollout is safe on any client
   version.

No feature flag needed on the client side — the presence or absence of
`versioned_session_overrides` in the response drives everything. A client
that has the plumbing but never sees an override behaves identically to
today.

## Debuggability

`configz` renders both sides:

- **Raw response** (unchanged): includes the full override list and each
  entry's matcher + mask so an operator can see exactly what the server
  is asserting.
- **Effective config**: what listeners actually saw after the override
  applied. Diff between the two is trivially "which mask path won".

A single-line debug log at apply time cites `(matcher, mask.paths)` so a
`grep`-able trail exists in production logs when a client discovers it's
inside a gated band.

## Metrics

Add one counter tag to the existing debug-tag catalog:

- `client_config_versioned_override_applied` — fires once per successful
  poll where a matcher admitted this client and the mask actually changed
  anything. Distinct from `client_config_session_configuration_null` and
  `client_config_session_load_zero` (which key on the resolved config, not
  on whether an override participated).

## Open questions

1. **Scope of override**: today the design overrides only
   `session_configuration`. Should it also allow overriding
   `polling_configuration` (e.g. "old clients poll less often")? Trivial to
   extend by promoting `override` to `ClientConfiguration`; deferred.
2. **First-match vs. best-match**: current design is first-match, ordering
   is authoritatively the server's. Best-match (narrowest matcher wins)
   pushes ordering onto the client; simpler client but more complex
   authoring. Recommend keeping first-match — matches every other
   Google-API rule engine (LB configs, admission policies).
3. **Absent-field clear semantics in the mask**: AIP-134 default is
   "mask always wins, even to clear". Some codebases interpret FieldMask
   as "merge only present fields". This design commits to AIP-134;
   flag it explicitly in the proto field comment so server authors don't
   get surprised.
