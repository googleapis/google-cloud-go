# Experimental low-allocation Spanner decoding

This branch is a customer trial for Go applications whose Spanner reads are limited by allocation count or protobuf decode CPU. It is not a released or supported API.

> **Required to test the fast path:** use `QueryWithOptions` with `ExperimentalRawDecode: true` at every benchmark call site. Changing only the dependency does not enable it. Plain `Query`, `QueryWithStats`, and every `Read*` API stay on the safe default, so a benchmark that continues to call `client.Single().Query(ctx, stmt)` measures the +38% safe default rather than the +154% opt-in path.

## What changes

All RPCs made by the Spanner data client use vtprotobuf at the gRPC connection level for both request marshaling and response unmarshaling. This includes session RPCs, unary calls such as `ExecuteSql` and `Commit`, and streaming calls such as `StreamingRead` and `ExecuteStreamingSql`.

The connection codec keeps the normal protobuf wire content subtype. Messages generated with vtprotobuf use `MarshalVT` and `UnmarshalVT`; other protobuf messages use the standard reflection implementation. Admin clients, long-running-operation polling, and connections owned by other clients are not changed.

## Use this branch from another module

From your module, run:

```sh
go get cloud.google.com/go/spanner@spanner-decode-perf
go mod tidy
go list -m cloud.google.com/go/spanner
```

The final command must print:

```text
cloud.google.com/go/spanner v1.94.1-0.20260803090513-da1e076ac0bc
```

This output confirms which client version is in the build. If it differs, stop before benchmarking. A branch name is a moving target; for a reproducible build, use the explicit pin instead:

```sh
go get cloud.google.com/go/spanner@v1.94.1-0.20260803090513-da1e076ac0bc
```

Commit both `go.mod` and `go.sum` used for the trial.

## Two decode modes

### Safe default

No application change is needed. Use normal `Read`, `Query`, DML, and transaction APIs.

The default codec calls `UnmarshalVT`, which copies strings and bytes out of gRPC receive buffers. Rows, `ColumnValue` results, and decoded Go strings keep normal client semantics: callers may retain them after another `Next`, after `Stop`, or after the iterator is gone.

This mode primarily reduces protobuf decode CPU. It intentionally keeps copy allocations needed for ordinary Go ownership. In the verified 800-string decode benchmark on this branch, safe vtprotobuf was 57.97% faster than reflection while allocations per operation changed from 2,412 to 2,411.

### Opt-in fast query path

**Changing the dependency is not enough. `QueryWithOptions` is required to reach the fast path.** Enable the existing query option at every benchmark call site:

```go
iter := client.Single().QueryWithOptions(ctx, stmt, spanner.QueryOptions{
    ExperimentalRawDecode: true,
})
defer iter.Stop()
```

This mode uses `UnmarshalVTUnsafe`, pooled `PartialResultSet` and `structpb.Value` objects, and a row reused by `RowIterator.Next`. It removes string copies and recycles the receive representation. The verified 800-string benchmark used 1.000 scalar oneof allocation per column instead of 3.015 allocations per column on the stock path.

### API coverage

`ExperimentalRawDecode` exists only on `QueryOptions` and is wired only through the SQL query path. The opt-in fast path applies to result rows returned by `QueryWithOptions` when `ExperimentalRawDecode` is `true`.

There is no corresponding field on `ReadOptions`. `Read`, `ReadWithOptions`, `ReadUsingIndex`, `ReadRow`, `ReadRowWithOptions`, and `ReadRowUsingIndex` remain on the safe default. Plain `Query` and `QueryWithStats` also remain on the safe default because they do not enable this option.

### Verified end-to-end results

On the same 30-vCPU host against a 10-node instance, the three measured arms produced:

| Metric | Released client | Safe default | Opt-in fast path |
| --- | ---: | ---: | ---: |
| Throughput | 3.77M rows/s | 5.18M rows/s (+38%) | 9.57M rows/s (+154%) |
| Allocations per row | 36.44 | 33.40 | 15.49 |
| Bytes per row | 1657 | 1317 | 290 |
| `mcache.refill` CPU share | 7.52% | 11.78% | 2.57% |

### Read consistency coverage

The verification runs for this branch used strong reads. The stale-read arm failed on the benchmark rig with `Bad BeginTransaction request` under multiplexed sessions, so the stale-read path is unverified on this branch. If your workload uses stale reads, please report the behavior and measurements you observe.

### Memory coverage

The opt-in path raised end-of-run resident memory roughly 2x because receive buffers stayed pinned. Peak resident memory stayed within ~3% of the released client on the measured host. Monitor both peak and steady-state resident memory in your environment.

## Opt-in lifetime contract

**A row returned by the opt-in path, its `ColumnValue` objects, and strings decoded from it are valid only until the next call to `Next` or `Stop`, whichever comes first.** The next call may overwrite pooled objects and gRPC receive memory. Retaining any of them without copying can silently change data.

| Correct: consume or copy before `Next` | Incorrect: retain aliases past `Next` |
| --- | --- |
| <pre><code class="language-go">var names []string<br>for {<br>    row, err := iter.Next()<br>    if err == iterator.Done {<br>        break<br>    }<br>    if err != nil {<br>        return err<br>    }<br><br>    var name string<br>    if err := row.Column(0, &amp;name); err != nil {<br>        return err<br>    }<br>    // Clone while data is valid.<br>    names = append(names, strings.Clone(name))<br>}</code></pre> | <pre><code class="language-go">var rows []*spanner.Row<br>var names []string<br>for {<br>    row, err := iter.Next()<br>    if err == iterator.Done {<br>        break<br>    }<br>    if err != nil {<br>        return err<br>    }<br><br>    var name string<br>    _ = row.Column(0, &amp;name)<br>    rows = append(rows, row) // Row is reused<br>    names = append(names, name) // aliases memory<br>}<br>// Data may now be overwritten.</code></pre> |

Copy every retained string with `strings.Clone` before calling `Next`. Materialize and deep-copy retained protobuf values. Do not pass the row to another goroutine unless that goroutine finishes before the next `Next` call.

If changing ownership patterns is risky, stay on the safe default.

## Experimental limits

Do not rely long term on:

- the `ExperimentalRawDecode` field name or its continued existence;
- row reuse, pooling details, allocation counts, or benchmark ratios;
- generated vtprotobuf methods being part of the public Spanner API;
- this branch receiving release support, compatibility guarantees, or security updates.

The safe default preserves public data-lifetime semantics, but the codec implementation itself is still a branch experiment. Re-test workload correctness and performance before moving to any later commit or released implementation.
