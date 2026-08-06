// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bigtable

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"cloud.google.com/go/bigtable/internal/session"
)

// This file implements process-wide implicit sharing of the underlying
// session.Client across NewClient calls that target the same tuple.
// Two bigtable.NewClient calls with matching (project, instance,
// appProfile, endpoint) end up backed by ONE session.Client — one gRPC
// channel pool, one ClientConfigurationManager poll goroutine, one
// per-resource session-pool set.
//
// Java-parity note: matches the intent of google-cloud-java PR #13829
// (BigtableDataClientFactory session support). The Go side does it
// implicitly via an intern cache rather than introducing a new public
// factory type — callers keep using NewClient / NewClientWithConfig and
// automatically get sharing when their identity tuples line up.
//
// What is shared: the session.Client (channel pool, config manager,
// per-resource session pools + streams).
//
// What stays per-Client: the classic gRPC ConnPool + client stub, the
// metrics tracer factory, the Diverter, and the session.TableCache
// wrapping per-Client TableAPI handles. The per-Client Diverter is
// wired into the shared session via its own AddSessionLoadListener
// entry so each Client's Diverter still receives server-driven session
// load updates.

// sharedKey identifies one shared session.Client entry. Callers with
// matching sharedKey values share one underlying session.Client. Fields
// are chosen so equality corresponds to "these callers can safely share
// one on-wire connection to Bigtable":
//
//   - project / instance / appProfile: the resource-identity triple the
//     session client bakes into every request.
//   - endpoint: the resolved gRPC endpoint (via
//     internaloption.UnsafeResolver.ResolvedGRPCEndpoint). Captures both
//     option.WithEndpoint and option.WithUniverseDomain, since the
//     latter feeds into endpoint resolution.
//
// Credentials, custom dialers, gRPC dial options, and callers that
// pre-dial via option.WithGRPCConn are NOT keyed. Pre-dialed callers
// skip the shared cache entirely (see NewClientWithConfig's preDialed
// gate). For non-pre-dialed callers with a custom dialer, sharing is
// still applied — the assumption is that the shared session's dial
// path is legitimate for any caller with the same identity tuple. This
// matters mostly in tests that dial different fake servers through
// bufconn using the same "test-project"/"test-instance" pair; those
// tests run sequentially, close their Client at teardown (which
// releases the shared session's refcount), so the next test cleanly
// re-builds its own session.
type sharedKey struct {
	project    string
	instance   string
	appProfile string
	endpoint   string
}

func (k sharedKey) String() string {
	return fmt.Sprintf("project=%q instance=%q appProfile=%q endpoint=%q",
		k.project, k.instance, k.appProfile, k.endpoint)
}

// sessionFingerprint captures the option-derived properties of a
// session.Client that must AGREE across all callers sharing one
// underlying instance. The fingerprint is compared via == on cache
// hit; any mismatch is a hard error at NewClient time so a caller
// cannot silently attach to a session that was constructed with
// different metrics / feature-flag settings than it asked for.
//
// Fields are chosen so equality implies "the session-side wire
// behavior would be identical if we built a fresh one":
//
//   - metricsProviderKind is the normalized MetricsProvider type
//     (nil and DefaultMetricsProvider{} both map to "default", since
//     metrics.NewFactory treats them the same).
//   - clientSideMetricsEnabled + enableDirectAccess drive the
//     FeatureFlags proto stamped onto every OpenSessionRequest.Flags
//     and mirrored in the bigtable-features header. Server rejects
//     OpenSession with INVALID_ARGUMENT when these disagree, so
//     two callers with different values genuinely cannot share.
type sessionFingerprint struct {
	metricsProviderKind      string
	clientSideMetricsEnabled bool
	enableDirectAccess       bool
}

// diff returns a comma-separated description of every field that
// differs between want and got. Used to render the incompatible-options
// error message in a form that tells the caller which knob they need
// to normalize.
func (want sessionFingerprint) diff(got sessionFingerprint) string {
	var diffs []string
	if want.metricsProviderKind != got.metricsProviderKind {
		diffs = append(diffs, fmt.Sprintf("MetricsProvider=%s existing=%s", got.metricsProviderKind, want.metricsProviderKind))
	}
	if want.clientSideMetricsEnabled != got.clientSideMetricsEnabled {
		diffs = append(diffs, fmt.Sprintf("clientSideMetricsEnabled=%t existing=%t", got.clientSideMetricsEnabled, want.clientSideMetricsEnabled))
	}
	if want.enableDirectAccess != got.enableDirectAccess {
		diffs = append(diffs, fmt.Sprintf("enableDirectAccess=%t existing=%t", got.enableDirectAccess, want.enableDirectAccess))
	}
	sort.Strings(diffs)
	return strings.Join(diffs, ", ")
}

// metricsProviderKind normalizes a MetricsProvider value into the
// discriminator used by the fingerprint. Nil and DefaultMetricsProvider{}
// collapse to the same "default" bucket because metrics.NewFactory
// treats them identically (see internal/metrics/factory.go). Unknown
// user-supplied implementations fall through to their Go type name so
// two different custom providers never share a session by accident.
func metricsProviderKind(mp MetricsProvider) string {
	switch mp.(type) {
	case nil, DefaultMetricsProvider:
		return "default"
	case NoopMetricsProvider:
		return "noop"
	default:
		return fmt.Sprintf("%T", mp)
	}
}

// refcountedSession is one intern-cache entry. sc is the shared
// session.Client; refs counts live NewClient acquisitions that have
// not yet released via their Close(). The entry is removed from
// sharedSessions and sc.Close is invoked when refs drops to zero.
type refcountedSession struct {
	sc          session.Client
	fingerprint sessionFingerprint
	refs        int
}

var (
	// sharedSessionsMu serializes the entire acquire/release/close
	// dance. Also held across the build closure on cache miss so
	// concurrent NewClient calls with the same sharedKey dedup to a
	// single session build — the second caller blocks on the mutex,
	// finds the entry populated on wake, and takes a reference.
	//
	// Holding the lock across a real gRPC dial is a startup-only cost
	// (the dial only runs once per (key) per process lifetime) and
	// keeps the state machine trivially correct. Concurrent NewClient
	// calls with DIFFERENT keys briefly queue behind each other; this
	// is acceptable given how rarely NewClient is called in practice.
	sharedSessionsMu sync.Mutex
	sharedSessions   = map[sharedKey]*refcountedSession{}
)

// acquireSharedSession returns the session.Client for key, either by
// incrementing the refcount on an existing cached entry or by calling
// build to construct a fresh one on cache miss.
//
// On success it returns a release closure the caller MUST invoke
// exactly once when it is done with the returned session.Client. The
// closure decrements the refcount and, when it reaches zero, deletes
// the cache entry and calls sc.Close. Multiple invocations of the
// same release closure past the first are no-ops (idempotent).
//
// On cache hit, the caller's fp is compared against the cached
// entry's fingerprint. A mismatch returns an error without
// incrementing any refcount so the caller cannot silently attach to a
// session that would behave differently from what it configured. The
// build closure is NOT invoked in that case.
func acquireSharedSession(key sharedKey, fp sessionFingerprint, build func() (session.Client, error)) (session.Client, func() error, error) {
	sharedSessionsMu.Lock()
	defer sharedSessionsMu.Unlock()
	if entry, ok := sharedSessions[key]; ok {
		if entry.fingerprint != fp {
			return nil, nil, fmt.Errorf(
				"bigtable: NewClient called with same (%s) but incompatible options (%s); "+
					"to use different options, use different resource identifiers",
				key, entry.fingerprint.diff(fp))
		}
		entry.refs++
		return entry.sc, releaseFor(key), nil
	}
	sc, err := build()
	if err != nil {
		return nil, nil, err
	}
	sharedSessions[key] = &refcountedSession{sc: sc, fingerprint: fp, refs: 1}
	return sc, releaseFor(key), nil
}

// releaseFor returns a release closure bound to key. The closure is
// idempotent — repeat invocations after the first return nil without
// touching the cache. On the invocation that drops the refcount to
// zero, the entry is deleted and sc.Close is called; that Close's
// error is returned to the caller.
//
// Idempotence comes from the "not found" branch below: once the entry
// has been removed (either by our own last-close or by
// ForceCloseSharedSessions), subsequent calls find nothing and no-op.
func releaseFor(key sharedKey) func() error {
	var once sync.Once
	var closeErr error
	return func() error {
		once.Do(func() {
			sharedSessionsMu.Lock()
			entry, ok := sharedSessions[key]
			if !ok {
				sharedSessionsMu.Unlock()
				return
			}
			entry.refs--
			if entry.refs > 0 {
				sharedSessionsMu.Unlock()
				return
			}
			delete(sharedSessions, key)
			sc := entry.sc
			sharedSessionsMu.Unlock()
			closeErr = sc.Close()
		})
		return closeErr
	}
}

// ForceCloseSharedSessions closes every cached session.Client and
// empties the shared-session cache. Intended for test teardown between
// suites and for drastic process-wide session shutdown; production
// code should rely on Client.Close to reference-count entries down to
// zero naturally.
//
// Callers that still hold a *Client after ForceCloseSharedSessions
// runs will see subsequent RPCs on that Client fail — the session
// backing it has been torn down out from under it. This is the
// intended tradeoff for a "burn it all down" teardown hook; wire it
// only from tests or from a shutdown path where you know no Client is
// still expected to serve traffic.
func ForceCloseSharedSessions() {
	sharedSessionsMu.Lock()
	old := sharedSessions
	sharedSessions = map[sharedKey]*refcountedSession{}
	sharedSessionsMu.Unlock()
	for _, entry := range old {
		_ = entry.sc.Close()
	}
}

// sharedSessionCount returns the number of live cache entries.
// Exposed for tests only.
func sharedSessionCount() int {
	sharedSessionsMu.Lock()
	defer sharedSessionsMu.Unlock()
	return len(sharedSessions)
}

// sharedSessionRefs returns the refcount for key, or 0 if absent.
// Exposed for tests only.
func sharedSessionRefs(key sharedKey) int {
	sharedSessionsMu.Lock()
	defer sharedSessionsMu.Unlock()
	if e, ok := sharedSessions[key]; ok {
		return e.refs
	}
	return 0
}
