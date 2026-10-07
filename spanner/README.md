## Cloud Spanner [![Go Reference](https://pkg.go.dev/badge/cloud.google.com/go/spanner.svg)](https://pkg.go.dev/cloud.google.com/go/spanner)

- [About Cloud Spanner](https://cloud.google.com/spanner/)
- [API documentation](https://cloud.google.com/spanner/docs)
- [Go client documentation](https://pkg.go.dev/cloud.google.com/go/spanner)

### Example Usage

First create a `spanner.Client` to use throughout your application:

[snip]:# (spanner-1)
```go
client, err := spanner.NewClient(ctx, "projects/P/instances/I/databases/D")
if err != nil {
	log.Fatal(err)
}
```

[snip]:# (spanner-2)
```go
// Simple Reads And Writes
_, err = client.Apply(ctx, []*spanner.Mutation{
	spanner.Insert("Users",
		[]string{"name", "email"},
		[]interface{}{"alice", "a@example.com"})})
if err != nil {
	log.Fatal(err)
}
row, err := client.Single().ReadRow(ctx, "Users",
	spanner.Key{"alice"}, []string{"email"})
if err != nil {
	log.Fatal(err)
}
```

### Session Leak
A `Client` object of the Client Library has a limit on the number of maximum sessions. For example the
default value of `MaxOpened`, which is the maximum number of sessions allowed by the session pool in the
Golang Client Library, is 400. You can configure these values at the time of
creating a `Client` by passing custom `SessionPoolConfig` as part of `ClientConfig`. When all the sessions are checked
out of the session pool, every new transaction has to wait until a session is returned to the pool.
If a session is never returned to the pool (hence causing a session leak), the transactions will have to wait
indefinitely and your application will be blocked.

#### Common Root Causes
The most common reason for session leaks in the Golang client library are:
1. Not stopping a `RowIterator` that is returned by `Query`, `Read` and other methods. Always use `RowIterator.Stop()` to ensure that the `RowIterator` is always closed.
2. Not closing a `ReadOnlyTransaction` when you no longer need it. Always call `ReadOnlyTransaction.Close()` after use, to ensure that the `ReadOnlyTransaction` is always closed.

As shown in the example below, the `txn.Close()` statement releases the session after it is complete.
If you fail to call `txn.Close()`, the session is not released back to the pool. The recommended way is to use `defer` as shown below.
```go
client, err := spanner.NewClient(ctx, "projects/P/instances/I/databases/D")
if err != nil {
  log.Fatal(err)
}
txn := client.ReadOnlyTransaction()
defer txn.Close()
```

#### Debugging and Resolving Session Leaks

##### Logging inactive transactions
This option logs warnings when you have exhausted >95% of your session pool. It is enabled by default.
This could mean two things; either you need to increase the max sessions in your session pool (as the number
of queries run using the client side database object is greater than your session pool can serve), or you may
have a session leak. To help debug which transactions may be causing this session leak, the logs will also contain stack traces of
transactions which have been running longer than expected if `TrackSessionHandles` under `SessionPoolConfig` is enabled.

```go
sessionPoolConfig := spanner.SessionPoolConfig{
    TrackSessionHandles: true,
    InactiveTransactionRemovalOptions: spanner.InactiveTransactionRemovalOptions{
      ActionOnInactiveTransaction: spanner.Warn,
    },
}
client, err := spanner.NewClientWithConfig(
	ctx, database, spanner.ClientConfig{SessionPoolConfig: sessionPoolConfig},
)
if err != nil {
	log.Fatal(err)
}
defer client.Close()

// Example Log message to warn presence of long running transactions
// session <session-info> checked out of pool at <session-checkout-time> is long running due to possible session leak for goroutine
// <Stack Trace of transaction>

```

##### Automatically clean inactive transactions
When the option to automatically clean inactive transactions is enabled, the client library will automatically detect
problematic transactions that are running for a very long time (thus causing session leaks) and close them.
The session will be removed from the pool and be replaced by a new session. To dig deeper into which transactions are being
closed, you can check the logs to see the stack trace of the transactions which might be causing these leaks and further
debug them.

```go
sessionPoolConfig := spanner.SessionPoolConfig{
    TrackSessionHandles: true,
    InactiveTransactionRemovalOptions: spanner.InactiveTransactionRemovalOptions{
      ActionOnInactiveTransaction: spanner.WarnAndClose,
    },
}
client, err := spanner.NewClientWithConfig(
	ctx, database, spanner.ClientConfig{SessionPoolConfig: sessionPoolConfig},
)
if err != nil {
log.Fatal(err)
}
defer client.Close()

// Example Log message for when transaction is recycled
// session <session-info> checked out of pool at <session-checkout-time> is long running and will be removed due to possible session leak for goroutine 
// <Stack Trace of transaction>
```

## Metrics

Cloud Spanner client supports [client-side metrics](https://cloud.google.com/spanner/docs/view-manage-client-side-metrics) that you can use along with server-side metrics to optimize performance and troubleshoot performance issues if they occur.

Client-side metrics are measured from the time a request leaves your application to the time your application receives the response.
In contrast, server-side metrics are measured from the time Spanner receives a request until the last byte of data is sent to the client.

These metrics are enabled by default. You can opt out of using client-side metrics with the following code:

```go
client, err := spanner.NewClientWithConfig(
	ctx, database, spanner.ClientConfig{DisableNativeMetrics: true},
)
if err != nil {
log.Fatal(err)
}
defer client.Close()
```

You can also disable these metrics by setting `SPANNER_DISABLE_BUILTIN_METRICS` to `true`.

> Note: Exporting client-side metrics requires the `monitoring.timeSeries.create` IAM permission. To grant this, ask your administrator to assign the [Monitoring Metric Writer](https://cloud.google.com/iam/docs/roles-permissions/monitoring#monitoring.metricWriter) (`roles/monitoring.metricWriter`) IAM role to your application's service account.

### Exporting client metrics to OpenTelemetry

To additionally export the same built-in client metrics through a caller-owned
OpenTelemetry pipeline, configure a dedicated meter provider and set
`ClientMetricsProvider`:

```go
reader := sdkmetric.NewPeriodicReader(exporter) // OTLP, Prometheus, or another exporter.
providerOptions := spanner.ClientMetricsMeterProviderOptions()
providerOptions = append(providerOptions, sdkmetric.WithReader(reader))
provider := sdkmetric.NewMeterProvider(providerOptions...)
defer provider.Shutdown(ctx)

client, err := spanner.NewClientWithConfig(ctx, database, spanner.ClientConfig{
	ClientMetricsProvider: provider,
})
if err != nil {
	log.Fatal(err)
}
defer client.Close()
```

The caller owns the meter provider. Spanner instruments use the
`spanner/client/` prefix; `ClientMetricsMeterProviderOptions` also maps the gRPC
instruments into that namespace. Use a dedicated meter provider because these
views select gRPC instrument names shared by other clients.

The OpenTelemetry Prometheus exporter renders counters as
`spanner_client_<name>_total` and millisecond histograms as
`spanner_client_<name>_milliseconds_{bucket,count,sum}`. For example, it exports
`spanner_client_operation_count_total` and
`spanner_client_operation_latencies_milliseconds_bucket`.

The gRPC-layer instruments are included only when gRPC built-in metrics are
enabled through `SPANNER_DISABLE_DIRECT_ACCESS_GRPC_BUILTIN_METRICS=false` or
DirectPath. AFE latency instruments are included only when AFE server timing is
enabled.

This caller-owned export is independent of the Cloud Monitoring export.
`DisableNativeMetrics` and `SPANNER_DISABLE_BUILTIN_METRICS` control only Cloud
Monitoring, so both sinks can be enabled together. Neither sink records metrics
when the client targets the Spanner emulator. `OpenTelemetryMeterProvider`
configures the older Spanner metrics surface and does not enable this built-in
client-metrics export.

On Spanner Omni, the Cloud Monitoring export is unavailable and always off.
Use `ClientMetricsProvider` to export client metrics from an Omni client.

## Experimental faster decoding with the `spanner_vtproto` build tag

Applications that read many rows can build with the `spanner_vtproto` build tag,
and set `ExperimentalBorrowRows` on the queries and reads that read many rows,
to spend less CPU and allocate less memory per row:

```sh
go build -tags spanner_vtproto ./...
```

```go
queryIter := client.Single().QueryWithOptions(ctx, stmt, spanner.QueryOptions{ExperimentalBorrowRows: true})
readIter := client.Single().ReadWithOptions(ctx, "Singers", keys, columns, &spanner.ReadOptions{ExperimentalBorrowRows: true})
```

A query or read that sets the option decodes its results with
[vtprotobuf](https://github.com/planetscale/vtprotobuf) generated code. The
strings in the rows reference the gRPC receive buffer instead of being copied,
the client reuses the decoded values once they are released, and the
`RowIterator` returns the same `*Row` from every call to `Next`.

Every other query and read behaves as without the tag, even in a binary that
is built with it:

- The option is only used when it is set on the call. It is not inherited from
  `ClientConfig.QueryOptions` or `ClientConfig.ReadOptions`, so other code that
  shares the client is not affected.
- `Query`, `QueryWithStats`, `Read` and `ReadUsingIndex` never borrow rows.
- `ReadRow`, `ReadRowWithOptions`, `ReadRowUsingIndex` and `SelectAll` always
  return rows and values that stay valid.
- Partitioned queries and reads ignore the option.

Without the tag, the option is ignored.

Only the streaming RPCs that the client reads rows with, `ExecuteStreamingSql`
for queries and `StreamingRead` for reads, use vtprotobuf. Their cost grows with
every value in every row. The other RPCs, such as `Commit`, `ExecuteSql`,
`BeginTransaction` and `BatchWrite`, are unchanged: the client builds and reads
their messages with the public protobuf types, so decoding them with
vtprotobuf would mean converting every request and response, which costs more
than it saves.

### Rows that are only valid until the iterator moves on

With the tag and the option, a row, and every string decoded from it, is only
valid until the next call to `Next` or `Stop` on its `RowIterator`, or, with
`Do`, until the function passed to `Do` returns. After that the client may
reuse their memory for later rows. The data can then silently change to the
contents of another row, or still look correct by chance. Nothing panics and
no error is returned.

An application that sets the option must not:

- Keep a row after the iterator moves on, for example with
  `rows = append(rows, row)`, or by storing it in a variable, map or channel.
- Decode a string with `row.Column(0, &str)` and use `str` after the iterator
  moves on, without first copying it with `strings.Clone`. This also applies to
  `*string`, `NullString`, `PGNumeric`, and arrays of them, and to strings in
  `STRUCT` values and in `NullRow` values decoded from arrays of `STRUCT`.
- Fill a struct with `row.ToStruct(&item)` or `row.Columns(...)` when the
  struct has string fields, and keep `item` after the iterator moves on, for
  example with `items = append(items, item)`, without copying those fields.
- Decode a `GenericColumnValue` with `row.Column(0, &genVal)`, or take a value
  with `row.ColumnValue(0)`, and use it after the iterator moves on.
  `proto.Clone` does not help: it copies the message, but not the bytes of its
  strings.
- Use a decoded string as a map key or set entry that outlives the row,
  without copying it.
- Keep the string that the `DecodeSpanner` method of a custom `Decoder`
  receives, without copying it.
- Pass rows or decoded strings to another goroutine that uses them after the
  iterator moves on.

All of the above also applies inside `iter.Do(func(row *spanner.Row) error
{...})`: the row and its strings are only valid until the function returns.
Use `Detach` or `strings.Clone` inside the function for everything you keep.

Values decoded into `[]byte`, `NullJSON`, `big.Rat`, integer, floating point,
boolean, `time.Time`, `civil.Date` and `uuid.UUID` destinations are copies and
stay valid. The `Metadata`, `QueryPlan`, `QueryStats` and `RowCount` fields of
a `RowIterator` also stay valid.

To keep a row, call `Detach` before the iterator moves on, and keep the row it
returns. With the tag, it copies the row and its strings. Without the tag, it
returns the row itself:

```go
var rows []*spanner.Row
err := iter.Do(func(row *spanner.Row) error {
	rows = append(rows, row.Detach())
	return nil
})
```

To keep only some values, copy their strings:

```go
iter := client.Single().QueryWithOptions(ctx, spanner.NewStatement("SELECT Name FROM Singers"),
	spanner.QueryOptions{ExperimentalBorrowRows: true})
defer iter.Stop()
var names []string
for {
	row, err := iter.Next()
	if err == iterator.Done {
		break
	}
	if err != nil {
		return err
	}
	var name string
	if err := row.Column(0, &name); err != nil {
		return err
	}
	// name points into a buffer that the client may reuse after the next call
	// to Next. Copying it with = keeps pointing there; strings.Clone copies the
	// bytes.
	names = append(names, strings.Clone(name))
}
```

### Testing that an application keeps no released data

Set the environment variable `SPANNER_VTPROTO_POISON_RELEASED_BUFFERS=true`
when you run your tests with the tag. The client then overwrites each receive
buffer with `0xAA` bytes when it releases it, so strings that are used after
they were released read as garbage, and tests that check values fail instead
of passing by chance. The client reads the variable once, when the program
starts.

The client releases a receive buffer once all the rows of the response that
it holds were returned, not after every row, and one response can hold many
rows. The check therefore finds data kept past the end of a response, and a
kept `*Row` shows the data of a later row after every call to `Next`. It can
miss a string or `GenericColumnValue` that is only kept until a later row of
the same response. Tests that read small results, or results in many small
responses, find more. A passing test is not a proof that an application uses
rows correctly.

The variable is for tests only. It costs CPU for every response.

### Other differences

The values of a query or read that sets the option are decoded like the
vtprotobuf `UnmarshalVTUnsafe` methods. Unlike the default decoding, they do
not check that strings are valid UTF-8, they do not enforce a recursion limit,
and they drop the unknown fields of the values.

Without the tag nothing changes, and no vtprotobuf code is linked into your
binary. The tag and the option are experimental and unsupported: they can
change or be removed in any release.
