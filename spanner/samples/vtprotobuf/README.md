# High-Throughput Cloud Spanner with `vtprotobuf`

This sample demonstrates how an application developer can use [`vtprotobuf`](https://github.com/planetscale/vtprotobuf) as an opt-in, high-performance serialization codec with the standard `cloud.google.com/go/spanner` client.

## Why Use `vtprotobuf`?

By default, the Go Spanner client uses the standard `google.golang.org/protobuf` runtime, which relies on reflection-based deserialization. For high-QPS streaming reads (`ExecuteStreamingSql`) returning large result sets, `vtprotobuf` generates unrolled, reflection-free parsers that provide:
- **~2.5x faster** protobuf unmarshaling.
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

## Build & Test Instructions

### 1. Generate the `vtproto` bindings
Run the generator script to vendor dependencies and generate the `vtprotobuf` methods:
```bash
./generate.sh
```

### 2. Run the application or tests
Because `vendor/` exists, standard Go commands (`go run`, `go test`, `go build`) automatically use the vendored packages:
```bash
go run main.go
go test -v ./...
```
