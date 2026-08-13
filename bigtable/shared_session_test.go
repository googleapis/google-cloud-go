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
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"cloud.google.com/go/bigtable/internal/session"
)

// resetSharedSessionCache is the standard per-test cleanup for tests
// that touch acquireSharedSession. Every test in this file registers
// it via t.Cleanup so a failure in one test cannot leak entries into
// the next test's cache view.
func resetSharedSessionCache(t *testing.T) {
	t.Helper()
	t.Cleanup(ForceCloseSharedSessions)
	// Also snapshot at start so a previous test's leak doesn't taint us.
	ForceCloseSharedSessions()
}

// buildFake returns a build closure that always constructs a fresh
// fakeSessionClient and counts how many times it was invoked. Tests
// use the count to assert cache-hit vs. cache-miss.
func buildFake() (build func() (session.Client, error), calls *atomic.Int32, lastBuilt func() *fakeSessionClient) {
	var count atomic.Int32
	var mu sync.Mutex
	var last *fakeSessionClient
	build = func() (session.Client, error) {
		count.Add(1)
		fsc := newFakeSessionClient()
		mu.Lock()
		last = fsc
		mu.Unlock()
		return fsc, nil
	}
	return build, &count, func() *fakeSessionClient {
		mu.Lock()
		defer mu.Unlock()
		return last
	}
}

func fpBase() sessionFingerprint {
	return sessionFingerprint{
		metricsProviderKind:      "noop",
		clientSideMetricsEnabled: false,
		enableDirectAccess:       true,
	}
}

// TestSharedSession_SameKeyReturnsSameInstance pins the primary
// contract: two acquireSharedSession calls with the same key return
// the same underlying session.Client and the build closure is invoked
// exactly once.
func TestSharedSession_SameKeyReturnsSameInstance(t *testing.T) {
	resetSharedSessionCache(t)
	key := sharedKey{project: "p", instance: "i", appProfile: "ap", endpoint: "e"}
	build, calls, _ := buildFake()

	sc1, rel1, err := acquireSharedSession(key, fpBase(), build)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	t.Cleanup(func() { _ = rel1() })

	sc2, rel2, err := acquireSharedSession(key, fpBase(), build)
	if err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	t.Cleanup(func() { _ = rel2() })

	if sc1 != sc2 {
		t.Errorf("second acquire returned distinct session.Client: sc1=%p sc2=%p", sc1, sc2)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("build calls = %d, want 1", got)
	}
	if got := sharedSessionRefs(key); got != 2 {
		t.Errorf("refcount = %d, want 2", got)
	}
}

// TestSharedSession_DifferentKeysReturnDistinct pins that varying any
// axis of sharedKey (project, instance, appProfile, endpoint)
// produces a distinct entry and a distinct session.Client.
func TestSharedSession_DifferentKeysReturnDistinct(t *testing.T) {
	resetSharedSessionCache(t)

	base := sharedKey{project: "p", instance: "i", appProfile: "ap", endpoint: "e"}
	variants := []struct {
		name string
		mut  func(sharedKey) sharedKey
	}{
		{"project", func(k sharedKey) sharedKey { k.project = "p2"; return k }},
		{"instance", func(k sharedKey) sharedKey { k.instance = "i2"; return k }},
		{"appProfile", func(k sharedKey) sharedKey { k.appProfile = "ap2"; return k }},
		{"endpoint", func(k sharedKey) sharedKey { k.endpoint = "e2"; return k }},
	}
	build, calls, _ := buildFake()
	sc0, rel0, err := acquireSharedSession(base, fpBase(), build)
	if err != nil {
		t.Fatalf("acquire base: %v", err)
	}
	// Hold every release until after the whole-suite assertions so
	// sub-test cleanups don't drop the refcount to 0 mid-test.
	rels := []func() error{rel0}
	t.Cleanup(func() {
		for _, r := range rels {
			_ = r()
		}
	})

	for _, v := range variants {
		v := v
		t.Run(v.name, func(t *testing.T) {
			k := v.mut(base)
			sc, rel, err := acquireSharedSession(k, fpBase(), build)
			if err != nil {
				t.Fatalf("acquire %s-variant: %v", v.name, err)
			}
			rels = append(rels, rel)
			if sc == sc0 {
				t.Errorf("variant %s returned same session.Client as base (want distinct)", v.name)
			}
		})
	}
	// 5 total: 1 base + 4 variants.
	if got := calls.Load(); got != 5 {
		t.Errorf("build calls = %d, want 5", got)
	}
	if got := sharedSessionCount(); got != 5 {
		t.Errorf("cache size = %d, want 5", got)
	}
}

// TestSharedSession_RefcountReleaseOnLastClose pins the reference-
// counting: two acquires + one release leaves the session live; the
// second release tears it down (session.Client.Close called exactly
// once). Subsequent release invocations on either closure are no-ops
// so double-Close from a bigtable.Client can't turn into a double-
// Close on the underlying session.
func TestSharedSession_RefcountReleaseOnLastClose(t *testing.T) {
	resetSharedSessionCache(t)
	key := sharedKey{project: "p", instance: "i", appProfile: "ap", endpoint: "e"}
	build, _, lastBuilt := buildFake()

	_, rel1, err := acquireSharedSession(key, fpBase(), build)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	_, rel2, err := acquireSharedSession(key, fpBase(), build)
	if err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	fsc := lastBuilt()
	if fsc == nil {
		t.Fatal("no fake session built")
	}

	if err := rel1(); err != nil {
		t.Errorf("first release: %v", err)
	}
	if got := fsc.closeCalls; got != 0 {
		t.Errorf("after first release, Close calls = %d, want 0", got)
	}
	if got := sharedSessionRefs(key); got != 1 {
		t.Errorf("after first release, refcount = %d, want 1", got)
	}

	if err := rel2(); err != nil {
		t.Errorf("second release: %v", err)
	}
	if got := fsc.closeCalls; got != 1 {
		t.Errorf("after second release, Close calls = %d, want 1", got)
	}
	if got := sharedSessionCount(); got != 0 {
		t.Errorf("cache size after last release = %d, want 0", got)
	}

	// Idempotence: calling either release again is a no-op.
	if err := rel1(); err != nil {
		t.Errorf("repeat rel1 after teardown: %v", err)
	}
	if err := rel2(); err != nil {
		t.Errorf("repeat rel2 after teardown: %v", err)
	}
	if got := fsc.closeCalls; got != 1 {
		t.Errorf("after repeat releases, Close calls = %d, want 1 (idempotent)", got)
	}
}

// TestSharedSession_ForceClose_TearsDownAll pins ForceCloseSharedSessions
// as an all-at-once teardown that closes every cached session.Client
// and empties the map. Refcount-tracking release closures on the
// still-live handles become no-ops.
func TestSharedSession_ForceClose_TearsDownAll(t *testing.T) {
	resetSharedSessionCache(t)
	build, _, _ := buildFake()

	// Track every fake we build so we can assert Close was called on
	// each after ForceClose (the buildFake helper only remembers the
	// last one).
	var built []*fakeSessionClient
	var builtMu sync.Mutex
	trackedBuild := func() (session.Client, error) {
		sc, err := build()
		if err != nil {
			return nil, err
		}
		builtMu.Lock()
		built = append(built, sc.(*fakeSessionClient))
		builtMu.Unlock()
		return sc, nil
	}

	const N = 5
	releases := make([]func() error, 0, N)
	for i := 0; i < N; i++ {
		key := sharedKey{project: fmt.Sprintf("p%d", i), instance: "i", appProfile: "ap", endpoint: "e"}
		_, rel, err := acquireSharedSession(key, fpBase(), trackedBuild)
		if err != nil {
			t.Fatalf("acquire %d: %v", i, err)
		}
		releases = append(releases, rel)
	}
	if got := sharedSessionCount(); got != N {
		t.Errorf("cache size before ForceClose = %d, want %d", got, N)
	}

	ForceCloseSharedSessions()

	if got := sharedSessionCount(); got != 0 {
		t.Errorf("cache size after ForceClose = %d, want 0", got)
	}
	builtMu.Lock()
	defer builtMu.Unlock()
	for i, fsc := range built {
		if fsc.closeCalls != 1 {
			t.Errorf("built[%d].closeCalls = %d, want 1", i, fsc.closeCalls)
		}
	}
	// Now-stale release closures must be no-ops (the entry is gone).
	for i, rel := range releases {
		if err := rel(); err != nil {
			t.Errorf("release[%d] after ForceClose: %v", i, err)
		}
	}
	for i, fsc := range built {
		if fsc.closeCalls != 1 {
			t.Errorf("built[%d].closeCalls after post-ForceClose releases = %d, want 1", i, fsc.closeCalls)
		}
	}
}

// TestSharedSession_IncompatibleOptions_Error pins the safety
// guardrail: a second acquire with the same key but a different
// fingerprint returns an error, does NOT increment the refcount, and
// does NOT invoke the build closure.
func TestSharedSession_IncompatibleOptions_Error(t *testing.T) {
	resetSharedSessionCache(t)
	key := sharedKey{project: "p", instance: "i", appProfile: "ap", endpoint: "e"}
	build, calls, _ := buildFake()

	_, rel1, err := acquireSharedSession(key, fpBase(), build)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	t.Cleanup(func() { _ = rel1() })

	incompat := fpBase()
	incompat.enableDirectAccess = !incompat.enableDirectAccess
	sc2, rel2, err := acquireSharedSession(key, incompat, build)
	if err == nil {
		t.Fatalf("incompatible acquire succeeded (sc=%v), want error", sc2)
	}
	if rel2 != nil {
		t.Errorf("incompatible acquire returned non-nil release closure")
	}
	if sc2 != nil {
		t.Errorf("incompatible acquire returned non-nil session.Client %v", sc2)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("build calls after incompatible acquire = %d, want 1 (must not re-build)", got)
	}
	if got := sharedSessionRefs(key); got != 1 {
		t.Errorf("refcount after incompatible acquire = %d, want 1 (must not increment)", got)
	}
	// Error message must name the diverging field so callers can act on it.
	if !strings.Contains(err.Error(), "enableDirectAccess") {
		t.Errorf("error %q does not mention the diverging field", err.Error())
	}
	if !strings.Contains(err.Error(), "incompatible options") {
		t.Errorf("error %q missing 'incompatible options' phrase", err.Error())
	}
}

// TestSharedSession_CloseAndReopen_FreshInstance pins that once the
// refcount drops to zero and the cache entry is evicted, a subsequent
// acquire builds a NEW session.Client rather than resurrecting the
// closed one.
func TestSharedSession_CloseAndReopen_FreshInstance(t *testing.T) {
	resetSharedSessionCache(t)
	key := sharedKey{project: "p", instance: "i", appProfile: "ap", endpoint: "e"}
	build, calls, _ := buildFake()

	sc1, rel1, err := acquireSharedSession(key, fpBase(), build)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := rel1(); err != nil {
		t.Fatalf("release to zero: %v", err)
	}
	if got := sharedSessionCount(); got != 0 {
		t.Fatalf("cache size after full release = %d, want 0", got)
	}

	sc2, rel2, err := acquireSharedSession(key, fpBase(), build)
	if err != nil {
		t.Fatalf("second acquire after teardown: %v", err)
	}
	t.Cleanup(func() { _ = rel2() })
	if sc1 == sc2 {
		t.Errorf("second acquire returned resurrected session (%p); want distinct fresh instance", sc1)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("build calls after reopen = %d, want 2", got)
	}
}

// TestSharedSession_ConcurrentAcquire_Deduped pins that a burst of
// concurrent acquire calls on the same key dedups to a single build
// invocation, each caller gets the same session.Client, and each
// release closure decrements the refcount independently — releasing
// all N drops the count to zero and closes the underlying session
// exactly once.
func TestSharedSession_ConcurrentAcquire_Deduped(t *testing.T) {
	resetSharedSessionCache(t)
	key := sharedKey{project: "p", instance: "i", appProfile: "ap", endpoint: "e"}
	build, calls, lastBuilt := buildFake()

	const N = 10
	var wg sync.WaitGroup
	results := make([]session.Client, N)
	releases := make([]func() error, N)
	errs := make([]error, N)
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			sc, rel, err := acquireSharedSession(key, fpBase(), build)
			results[i] = sc
			releases[i] = rel
			errs[i] = err
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d acquire: %v", i, err)
		}
	}
	// Exactly one build under the global mutex.
	if got := calls.Load(); got != 1 {
		t.Errorf("build calls = %d, want 1", got)
	}
	fsc := lastBuilt()
	if fsc == nil {
		t.Fatal("no fake built")
	}
	// Every returned session.Client is the same instance.
	for i := 1; i < N; i++ {
		if results[i] != results[0] {
			t.Errorf("results[%d] != results[0] (%p vs %p)", i, results[i], results[0])
		}
	}
	if got := sharedSessionRefs(key); got != N {
		t.Errorf("refcount = %d, want %d", got, N)
	}

	// Release from N goroutines concurrently — must decrement to
	// zero and Close exactly once.
	var wg2 sync.WaitGroup
	relStart := make(chan struct{})
	for i := 0; i < N; i++ {
		i := i
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			<-relStart
			_ = releases[i]()
		}()
	}
	close(relStart)
	wg2.Wait()

	if got := fsc.closeCalls; got != 1 {
		t.Errorf("closeCalls = %d, want 1 (exactly one Close on refcount→0)", got)
	}
	if got := sharedSessionCount(); got != 0 {
		t.Errorf("cache size after all releases = %d, want 0", got)
	}
}

// TestSharedSession_BuildErrorNotCached pins that a build failure
// returns the error to the caller AND does NOT populate the cache —
// a subsequent acquire retries build. Otherwise a transient dial
// failure would strand the key permanently.
func TestSharedSession_BuildErrorNotCached(t *testing.T) {
	resetSharedSessionCache(t)
	key := sharedKey{project: "p", instance: "i", appProfile: "ap", endpoint: "e"}

	wantErr := errors.New("dial failed")
	var calls atomic.Int32
	buildOnceThenSucceed := func() (session.Client, error) {
		n := calls.Add(1)
		if n == 1 {
			return nil, wantErr
		}
		return newFakeSessionClient(), nil
	}

	_, _, err := acquireSharedSession(key, fpBase(), buildOnceThenSucceed)
	if !errors.Is(err, wantErr) {
		t.Fatalf("first acquire err = %v, want %v", err, wantErr)
	}
	if got := sharedSessionCount(); got != 0 {
		t.Errorf("cache size after failed build = %d, want 0", got)
	}

	sc, rel, err := acquireSharedSession(key, fpBase(), buildOnceThenSucceed)
	if err != nil {
		t.Fatalf("retry acquire: %v", err)
	}
	t.Cleanup(func() { _ = rel() })
	if sc == nil {
		t.Error("retry acquire returned nil session.Client")
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("build calls after retry = %d, want 2", got)
	}
}

// TestMetricsProviderKind pins the normalization that nil and
// DefaultMetricsProvider{} collapse into the same fingerprint bucket
// (matching metrics.NewFactory behavior — both trigger the built-in
// exporter path), NoopMetricsProvider{} takes its own bucket, and
// unknown user types fall back to their Go type name.
func TestMetricsProviderKind(t *testing.T) {
	if got := metricsProviderKind(nil); got != "default" {
		t.Errorf("nil → %q, want %q", got, "default")
	}
	if got := metricsProviderKind(DefaultMetricsProvider{}); got != "default" {
		t.Errorf("DefaultMetricsProvider → %q, want %q", got, "default")
	}
	if got := metricsProviderKind(NoopMetricsProvider{}); got != "noop" {
		t.Errorf("NoopMetricsProvider → %q, want %q", got, "noop")
	}
}
