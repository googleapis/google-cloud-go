# Spanner allocation trial: three things to try

Thank you for testing the earlier patch and sharing that its improvement was modest. Based on the allocator profile and the follow-up measurements, I suggest trying these in order. The first change works with your current released client; the next two use the experimental branch.

> **Required to test the fast path:** change each benchmark call from `client.Single().Query(ctx, stmt)` to `QueryWithOptions` with `ExperimentalRawDecode: true`, as shown in Step 3. Changing only the dependency does not enable the fast path: plain `Query`, `QueryWithStats`, and every `Read*` API stay on the safe default. Without this call-site change, the trial measures the +38% safe default rather than the +154% opt-in path.

## Step 1: hoist row destinations out of the callback

This costs nothing and needs no branch. Declaring destination variables inside the per-row callback, then passing their addresses through the variadic `Row.Columns` interface, forces all eight destinations onto the heap: eight allocations per row before the Spanner library does any work. Hoisting them outside the callback reduces those caller-side allocations to zero.

At your reported ~7 M rows/s, this removes approximately 56 million allocations/s from the allocator that is already saturated. This can be applied to your current released client today.

| Before: destinations escape on every row | After: destinations are reused |
| --- | --- |
| <pre><code class="language-go">err := iter.Do(func(row *spanner.Row) error {<br>    var c1, c2, c3, c4 string<br>    var c5, c6, c7, c8 string<br><br>    return row.Columns(<br>        &amp;c1, &amp;c2, &amp;c3, &amp;c4,<br>        &amp;c5, &amp;c6, &amp;c7, &amp;c8,<br>    )<br>})</code></pre> | <pre><code class="language-go">var c1, c2, c3, c4 string<br>var c5, c6, c7, c8 string<br><br>err := iter.Do(func(row *spanner.Row) error {<br>    return row.Columns(<br>        &amp;c1, &amp;c2, &amp;c3, &amp;c4,<br>        &amp;c5, &amp;c6, &amp;c7, &amp;c8,<br>    )<br>})</code></pre> |

Keep each set of reused destinations local to one iterator or worker; do not share it concurrently.

## Step 2: try the branch's safe default

This needs no application code change and preserves normal row and string lifetimes. From your module, run:

```sh
go get cloud.google.com/go/spanner@spanner-decode-perf
go mod tidy
go list -m cloud.google.com/go/spanner
```

The final command must print:

```text
cloud.google.com/go/spanner v1.94.1-0.20260803090513-da1e076ac0bc
```

Confirm this before benchmarking so you know which client version is in the build. A branch name is a moving target; for a reproducible build, use the explicit pin instead:

```sh
go get cloud.google.com/go/spanner@v1.94.1-0.20260803090513-da1e076ac0bc
```

On our 30-vCPU host against a 10-node instance, the safe default produced:

- throughput: **+38%**;
- allocations per row: **36.44 → 33.40**;
- allocated bytes per row: **1657 → 1317**.

## Step 3: try the opt-in query fast path

**Changing the dependency is not enough. `QueryWithOptions` is required to reach the fast path.** Replace every benchmark call to plain `Query` with:

```go
iter := client.Single().QueryWithOptions(ctx, stmt, spanner.QueryOptions{
    ExperimentalRawDecode: true,
})
defer iter.Stop()
```

On the same host and instance, the three measured arms produced:

| Metric | Released client | Safe default | Opt-in fast path |
| --- | ---: | ---: | ---: |
| Throughput | 3.77M rows/s | 5.18M rows/s (+38%) | 9.57M rows/s (+154%) |
| Allocations per row | 36.44 | 33.40 | 15.49 |
| Bytes per row | 1657 | 1317 | 290 |
| `mcache.refill` CPU share | 7.52% | 11.78% | 2.57% |

### Lifetime contract

**A row, its column values, and strings decoded from it are valid only until the next `Next` or `Stop`. Anything retained must be copied first. Getting this wrong can silently corrupt data rather than return an error.**

| Correct: copy before `Next` | Incorrect: retain aliases past `Next` |
| --- | --- |
| <pre><code class="language-go">var names []string<br>for {<br>    row, err := iter.Next()<br>    if err == iterator.Done {<br>        break<br>    }<br>    if err != nil {<br>        return err<br>    }<br><br>    var name string<br>    if err := row.Column(0, &amp;name); err != nil {<br>        return err<br>    }<br>    names = append(names, strings.Clone(name))<br>}</code></pre> | <pre><code class="language-go">var rows []*spanner.Row<br>var names []string<br>for {<br>    row, err := iter.Next()<br>    if err == iterator.Done {<br>        break<br>    }<br>    if err != nil {<br>        return err<br>    }<br><br>    var name string<br>    _ = row.Column(0, &amp;name)<br>    rows = append(rows, row)<br>    names = append(names, name)<br>}<br>// Retained data may now be overwritten.</code></pre> |

Use `strings.Clone` for every retained string before advancing or stopping the iterator. Any retained row or composite value needs an appropriate deep copy. If auditing ownership is risky, Step 2 alone is still worth trying.

This opt-in applies only to SQL result rows produced by `QueryWithOptions`. `ExperimentalRawDecode` is not available on `ReadOptions`, so `Read`, `ReadWithOptions`, `ReadUsingIndex`, `ReadRow`, `ReadRowWithOptions`, and `ReadRowUsingIndex` stay on the safe default. Plain `Query` and `QueryWithStats` also stay on the safe default; they never enable the fast path.

## What to measure

Please compare each arm with your own released-client baseline on the same host and workload shape. Our absolute rates are not a useful target for a different environment. Capture:

- rows/sec;
- allocations per row;
- `runtime.MemStats` mallocs and total allocation;
- `runtime.(*mcache).refill` share from a CPU profile;
- resident memory, including peak and end-of-run values.

## Caveats

- Our measurements used a 10-node test instance, 128 threads, strong reads, and rows with eight columns on a 30-vCPU host. Your absolute figures will differ; relative gains are the meaningful comparison.
- The stale-read arm failed on our benchmark rig with `Bad BeginTransaction request` under multiplexed sessions. We did not get stale-read measurements, so the stale-read path is unverified on this branch. Because your workload uses stale reads, please report the behavior and measurements you see.
- The opt-in path raised end-of-run resident memory roughly 2x because receive buffers stayed pinned. Peak resident memory stayed within ~3% of the released client on our host. Please watch both peak and steady-state resident memory in your environment.
- This is an experimental branch with no support or compatibility guarantee.
