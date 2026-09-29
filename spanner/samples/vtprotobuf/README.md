# High-Throughput Cloud Spanner with `vtprotobuf`

This sample demonstrates how an application developer can use [`vtprotobuf`](https://github.com/planetscale/vtprotobuf) as an opt-in, high-performance serialization codec with Cloud Spanner in Go.

## Why Use `vtprotobuf`?

By default, the Go Spanner client uses the standard `google.golang.org/protobuf` runtime, which relies on reflection-based deserialization. For high-QPS streaming reads (`ExecuteStreamingSql`) returning large result sets, `vtprotobuf` generates unrolled, reflection-free parsers and memory recycling hooks that provide:
- **Faster deserialization throughput**.
- **Lower CPU overhead** and reduced garbage collection pressure.

## How It Works

1. **Build-Time Augmentation via Vendoring:** The developer runs `./generate.sh` during their build/CI to generate `*_vtproto.pb.go` companion files with memory pooling into their `vendor/cloud.google.com/go/spanner/apiv1/spannerpb` directory.
2. **Opt-In gRPC Codec:** The application passes `grpc.ForceCodec(vtgrpc.Codec{})` from `github.com/planetscale/vtprotobuf/codec/grpc` when creating the client.
3. **Memory Pooling via `PartialResultSetPool`:** The application adapts `vtprotobuf`'s generated pool (`PartialResultSetFromVTPool` / `ReturnToVTPool`) to Spanner's generic `PartialResultSetPool` interface in `ClientConfig`.
4. **Decoupled SDK:** The core `cloud.google.com/go/spanner` client requires zero third-party dependencies or duck-typing—it simply fetches and recycles chunk structs via the injected pool.

---

## Code Example: How Little Code You Need

In your application code, configuring the high-throughput Spanner client with `vtprotobuf` and memory pooling requires only configuring the gRPC codec and `PartialResultSetPool`:

```go
package main

import (
	"context"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	vtgrpc "github.com/planetscale/vtprotobuf/codec/grpc"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

// VTPartialResultSetPool adapts vtprotobuf's generated pool to spanner.PartialResultSetPool.
type VTPartialResultSetPool struct{}

func (p VTPartialResultSetPool) Get() *sppb.PartialResultSet {
	return sppb.PartialResultSetFromVTPool()
}

func (p VTPartialResultSetPool) Put(m *sppb.PartialResultSet) {
	if m != nil {
		m.ReturnToVTPool()
	}
}

func NewHighThroughputSpannerClient(ctx context.Context, database string, opts ...option.ClientOption) (*spanner.Client, error) {
	vtprotoOpt := option.WithGRPCDialOption(
		grpc.WithDefaultCallOptions(
			grpc.ForceCodec(vtgrpc.Codec{}),
		),
	)

	allOpts := append([]option.ClientOption{vtprotoOpt}, opts...)
	return spanner.NewClientWithConfig(ctx, database,
		spanner.ClientConfig{
			SessionPoolConfig:    spanner.DefaultSessionPoolConfig,
			PartialResultSetPool: VTPartialResultSetPool{},
		},
		allOpts...,
	)
}
```

Everything else (queries, mutations, transactions, row decoding) uses the standard `spanner.Client` and `spanner.RowIterator` APIs unchanged.

---

## Build & Benchmark Instructions

### 1. Generate the `vtproto` bindings
```bash
./generate.sh
```

### 2. Run automated tests
```bash
go test -v ./...
```

### 3. Run the end-to-end client benchmark
```bash
go test -bench=BenchmarkSpannerClient -benchmem -benchtime=2s
```

#### Benchmark Results (1,000 rows / 4,000 columns per query):
```text
BenchmarkSpannerClient/Standard_Protobuf-8         405   5930605 ns/op   4806085 B/op   101734 allocs/op
BenchmarkSpannerClient/VTProtobuf_With_Pooling-8   466   5201633 ns/op   4930335 B/op   102729 allocs/op
```
