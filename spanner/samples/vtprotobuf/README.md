# High-Throughput Cloud Spanner with `vtprotobuf`

This sample demonstrates how an application developer can use [`vtprotobuf`](https://github.com/planetscale/vtprotobuf) as an opt-in, high-performance serialization codec with the standard `cloud.google.com/go/spanner` client.

## Why Use `vtprotobuf`?

By default, the Go Spanner client uses the standard `google.golang.org/protobuf` runtime, which relies on reflection-based deserialization. For high-QPS streaming reads (`ExecuteStreamingSql`) returning large result sets, `vtprotobuf` generates unrolled, reflection-free parsers that provide:
- **~2.6x faster** protobuf unmarshaling (~445 MB/s vs ~164 MB/s).
- **Lower CPU overhead** and reduced memory pressure.

## How It Works

1. **Zero Changes to the Official SDK:** The official `cloud.google.com/go/spanner` module remains standard and clean.
2. **Build-Time Augmentation via Vendoring:** The developer runs `./generate.sh` during their build/CI to generate `*_vtproto.pb.go` companion files into their `vendor/cloud.google.com/go/spanner/apiv1/spannerpb` directory.
3. **Opt-In via gRPC Dial Option:** The application passes `grpc.ForceCodec(vtgrpc.Codec{})` when creating the Spanner client.
4. **Safe Fallback:** If `vendor/` is not present (or if the generated files are omitted), the gRPC codec automatically falls back to standard `proto.Unmarshal` without errors.

---

## Code Example: How Little Code You Need

In your application code, creating the high-throughput Spanner client requires adding only **one dial option**:

```go
package main

import (
	"context"

	"cloud.google.com/go/spanner"
	vtgrpc "github.com/planetscale/vtprotobuf/codec/grpc"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

func NewHighThroughputSpannerClient(ctx context.Context, database string) (*spanner.Client, error) {
	return spanner.NewClient(ctx, database,
		option.WithGRPCDialOption(
			grpc.WithDefaultCallOptions(
				// Force the vtprotobuf codec:
				grpc.ForceCodec(vtgrpc.Codec{}),
			),
		),
	)
}
```

Everything else (queries, mutations, transactions, row decoding) uses the standard `spanner.Client` APIs unchanged.

---

## Build & Benchmark Instructions

### 1. Generate the `vtproto` bindings
Run the generator script to vendor dependencies and generate the `vtprotobuf` methods:
```bash
./generate.sh
```

### 2. Run the automated tests
```bash
go test -v ./...
```

### 3. Run the benchmarks
Run the built-in benchmarks to compare standard protobuf vs `vtprotobuf`:
```bash
go test -bench=. -benchmem
```

#### Benchmark Results:
```text
BenchmarkUnmarshal_PartialResultSet/Standard_proto.Unmarshal-8    2451   469804 ns/op   163.60 MB/s
BenchmarkUnmarshal_PartialResultSet/VTProtobuf_UnmarshalVT-8     6062   172513 ns/op   445.54 MB/s (2.7x faster)

BenchmarkSpannerClient_EndToEnd/Standard_Protobuf-8               210   5523292 ns/op
BenchmarkSpannerClient_EndToEnd/VTProtobuf_Codec-8                217   5252172 ns/op
```
