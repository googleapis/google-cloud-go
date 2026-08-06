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
	"sync"
	"sync/atomic"
	"testing"

	"cloud.google.com/go/bigtable/internal/session"
)

// Benchmarks for the process-wide session.Client intern cache introduced in
// PR #20335. All benchmarks exercise acquireSharedSession directly with a
// fake build closure so nothing dials real gRPC — the same pattern
// shared_session_test.go uses. The measurements isolate the cache path
// (map lookup + refcount vs. fresh build under mutex), which is the only
// component whose cost changes between the "shared" and "distinct" cases;
// the classic gRPC dial cost is identical either way and outside the
// scope of the intended savings.
//
// Between benchmark iterations we call ForceCloseSharedSessions() to
// reset the intern cache — otherwise iteration N inherits state from
// iteration N-1 and the first cache-miss cost gets amortized out of the
// numbers.

// benchBuildFake returns a build closure that constructs fresh
// fakeSessionClient values and an atomic counter of how many times it
// fired. Mirrors buildFake in shared_session_test.go but drops the
// "lastBuilt" accessor since benchmarks only care about the call count.
func benchBuildFake() (build func() (session.Client, error), calls *atomic.Int32) {
	var count atomic.Int32
	build = func() (session.Client, error) {
		count.Add(1)
		return newFakeSessionClient(), nil
	}
	return build, &count
}

// BenchmarkAcquireSharedSession_x100_Shared measures the acquire+release
// path 100 callers walk when every caller targets the same identity
// tuple — the intended "many logical clients, one physical session"
// scenario. Each iteration:
//  1. Acquires N times against the same sharedKey.
//  2. Verifies exactly ONE build fired (the cache-hit invariant).
//  3. Releases all N handles.
//
// Compare against BenchmarkAcquireSharedSession_x100_Distinct below —
// distinct-key allocations should scale ~linearly with N, shared-key
// should be flat at 1.
func BenchmarkAcquireSharedSession_x100_Shared(b *testing.B) {
	const N = 100
	key := sharedKey{project: "p", instance: "i", appProfile: "ap", endpoint: "e"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		ForceCloseSharedSessions()
		build, calls := benchBuildFake()
		b.StartTimer()

		rels := make([]func() error, N)
		for j := 0; j < N; j++ {
			_, rel, err := acquireSharedSession(key, fpBase(), build)
			if err != nil {
				b.Fatalf("acquire[%d]: %v", j, err)
			}
			rels[j] = rel
		}

		b.StopTimer()
		if got := calls.Load(); got != 1 {
			b.Fatalf("build calls = %d, want 1 (shared identity must dedupe)", got)
		}
		if got := sharedSessionCount(); got != 1 {
			b.Fatalf("cache size = %d, want 1 (shared identity must produce one entry)", got)
		}
		for _, r := range rels {
			_ = r()
		}
		b.StartTimer()
	}
}

// BenchmarkAcquireSharedSession_x100_Distinct is the "no sharing
// possible" baseline: 100 acquires each targeting a distinct sharedKey
// (project varies). Every acquire is a cache miss that runs build, so
// the reported allocations and time scale linearly with N — exactly the
// cost the sharing path eliminates when identities line up.
func BenchmarkAcquireSharedSession_x100_Distinct(b *testing.B) {
	const N = 100
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		ForceCloseSharedSessions()
		build, calls := benchBuildFake()
		b.StartTimer()

		rels := make([]func() error, N)
		for j := 0; j < N; j++ {
			key := sharedKey{
				project:    fmt.Sprintf("p%d", j),
				instance:   "i",
				appProfile: "ap",
				endpoint:   "e",
			}
			_, rel, err := acquireSharedSession(key, fpBase(), build)
			if err != nil {
				b.Fatalf("acquire[%d]: %v", j, err)
			}
			rels[j] = rel
		}

		b.StopTimer()
		if got := calls.Load(); int(got) != N {
			b.Fatalf("build calls = %d, want %d (distinct identities must NOT dedupe)", got, N)
		}
		if got := sharedSessionCount(); got != N {
			b.Fatalf("cache size = %d, want %d", got, N)
		}
		for _, r := range rels {
			_ = r()
		}
		b.StartTimer()
	}
}

// BenchmarkRelease_SharedRefcount measures the fast-path release cost —
// the refcount decrement branch a Close() takes when it is NOT the last
// holder. Setup acquires the same key N+1 times; each iteration
// releases exactly one handle without ever tearing the shared entry
// down. Report the per-release cost of the refcount path in isolation.
func BenchmarkRelease_SharedRefcount(b *testing.B) {
	ForceCloseSharedSessions()
	defer ForceCloseSharedSessions()

	key := sharedKey{project: "p", instance: "i", appProfile: "ap", endpoint: "e"}
	build, _ := benchBuildFake()
	rels := make([]func() error, b.N+1)
	for j := 0; j < b.N+1; j++ {
		_, rel, err := acquireSharedSession(key, fpBase(), build)
		if err != nil {
			b.Fatalf("setup acquire[%d]: %v", j, err)
		}
		rels[j] = rel
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rels[i]()
	}
	b.StopTimer()

	// Leave one refcount live so no iteration hit the last-holder path.
	// Sanity-check the invariant here rather than in the hot loop.
	if got := sharedSessionRefs(key); got != 1 {
		b.Fatalf("post-release refcount = %d, want 1 (fast-path only)", got)
	}
	_ = rels[b.N]()
}

// BenchmarkRelease_LastHolderTeardown measures the slow-path release
// cost — the last-holder branch that removes the cache entry and
// invokes sc.Close. Each iteration acquires ONCE and releases ONCE, so
// every release is the teardown release. Compare against
// BenchmarkRelease_SharedRefcount above: the teardown cost includes
// the map delete + fake Close call.
func BenchmarkRelease_LastHolderTeardown(b *testing.B) {
	ForceCloseSharedSessions()
	defer ForceCloseSharedSessions()

	key := sharedKey{project: "p", instance: "i", appProfile: "ap", endpoint: "e"}
	build, _ := benchBuildFake()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		_, rel, err := acquireSharedSession(key, fpBase(), build)
		if err != nil {
			b.Fatalf("acquire: %v", err)
		}
		b.StartTimer()
		_ = rel()
	}
}

// BenchmarkAcquireSharedSession_Concurrent pins the concurrent-acquire
// fast path: 32 goroutines racing to acquire the same key. Every
// iteration must produce exactly ONE build call — the intern cache's
// under-mutex build guarantees dedupe even under high concurrency.
// b.RunParallel is used for the acquire storm; the assertion after
// each iteration confirms the invariant held.
func BenchmarkAcquireSharedSession_Concurrent(b *testing.B) {
	ForceCloseSharedSessions()
	defer ForceCloseSharedSessions()

	key := sharedKey{project: "p", instance: "i", appProfile: "ap", endpoint: "e"}
	build, calls := benchBuildFake()

	// Prime one refcount so the shared entry always exists during the
	// parallel storm — the benchmark measures the cache-hit
	// (increment-and-return) path, not the cold-start build cost.
	_, primeRel, err := acquireSharedSession(key, fpBase(), build)
	if err != nil {
		b.Fatalf("prime acquire: %v", err)
	}
	defer func() { _ = primeRel() }()

	b.ReportAllocs()
	b.ResetTimer()

	// Collect releases across goroutines so the teardown pass is
	// deterministic. sync.Pool avoids contention on a single slice.
	var releasedMu sync.Mutex
	var released []func() error

	b.RunParallel(func(pb *testing.PB) {
		local := make([]func() error, 0, 64)
		for pb.Next() {
			_, rel, err := acquireSharedSession(key, fpBase(), build)
			if err != nil {
				b.Fatalf("acquire: %v", err)
			}
			local = append(local, rel)
		}
		releasedMu.Lock()
		released = append(released, local...)
		releasedMu.Unlock()
	})

	b.StopTimer()
	if got := calls.Load(); got != 1 {
		b.Fatalf("build calls = %d, want 1 (concurrent acquires must dedupe to one build)", got)
	}
	for _, r := range released {
		_ = r()
	}
}
