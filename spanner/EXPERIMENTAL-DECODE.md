# Experimental low-allocation Spanner decoding

This branch is a customer trial for Go applications whose Spanner reads are limited by allocation count or protobuf decode CPU. It is not a released or supported API.

## What changes

All RPCs made by the Spanner data client use vtprotobuf at the gRPC connection level for both request marshaling and response unmarshaling. This includes session RPCs, unary calls such as `ExecuteSql` and `Commit`, and streaming calls such as `StreamingRead` and `ExecuteStreamingSql`.

The connection codec keeps the normal protobuf wire content subtype. Messages generated with vtprotobuf use `MarshalVT` and `UnmarshalVT`; other protobuf messages use the standard reflection implementation. Admin clients, long-running-operation polling, and connections owned by other clients are not changed.

## Use this branch from another module

Pin the trial. Do not depend on a moving branch name in a reproducible build.

```mod
require cloud.google.com/go/spanner v1.91.0

replace cloud.google.com/go/spanner => cloud.google.com/go/spanner v1.91.1-0.20260803085303-2e16398ef74c
```

Equivalent commands:

```sh
go mod edit -require=cloud.google.com/go/spanner@v1.91.0
go mod edit -replace=cloud.google.com/go/spanner=cloud.google.com/go/spanner@v1.91.1-0.20260803085303-2e16398ef74c
go mod tidy
go list -m cloud.google.com/go/spanner
```

`go list` must print `v1.91.1-0.20260803085303-2e16398ef74c`. Commit both `go.mod` and `go.sum`. Remove the `replace` directive to return to a released client.

## Two decode modes

### Safe default

No application change is needed. Use normal `Read`, `Query`, DML, and transaction APIs.

The default codec calls `UnmarshalVT`, which copies strings and bytes out of gRPC receive buffers. Rows, `ColumnValue` results, and decoded Go strings keep normal client semantics: callers may retain them after another `Next`, after `Stop`, or after the iterator is gone.

This mode primarily reduces protobuf decode CPU. It intentionally keeps copy allocations needed for ordinary Go ownership. In the 800-string decode benchmark on this branch, safe vtprotobuf was about 58% faster than reflection but had essentially the same allocation count.

### Opt-in fast query path

Enable the existing query option:

```go
iter := client.Single().QueryWithOptions(ctx, stmt, spanner.QueryOptions{
    ExperimentalRawDecode: true,
})
defer iter.Stop()
```

This mode uses `UnmarshalVTUnsafe`, pooled `PartialResultSet` and `structpb.Value` objects, and a row reused by `RowIterator.Next`. It removes string copies and recycles the receive representation. The isolated 800-string benchmark used exactly one scalar oneof allocation per column instead of about three allocations per column on the stock path.

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

The safe default preserves public data-lifetime semantics, but the codec implementation itself is still a branch experiment. Re-test workload correctness and performance before moving to any later commit or released replacement.
