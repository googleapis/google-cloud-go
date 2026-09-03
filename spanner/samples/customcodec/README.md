# High-Throughput Cloud Spanner with Custom Fast Codec & Memory Pooling

This sample demonstrates how to build a **specialized, zero-reflection custom gRPC codec and memory pool** tailored specifically for Google Cloud Spanner streaming reads without requiring any third-party compiler plugins or code-generation tools.

---

## Why a Spanner-Specific Codec?

In high-throughput Cloud Spanner workloads (such as analytical streaming queries or large ETL scans via `ExecuteStreamingSql`), the vast majority of network payload bytes consist of repeated `google.protobuf.Value` items inside `PartialResultSet.values`.

The standard Go Protobuf runtime (`google.golang.org/protobuf`) uses table-driven reflection to parse each message. By writing a lightweight, specialized decoder for `PartialResultSet` and `Value`:
1. **Zero Reflection on Hot Paths:** `*sppb.PartialResultSet` and `*structpb.Value` are parsed directly from wire bytes using inline varint and fixed64 bit-shifts.
2. **Deep Memory Recycling:** `PartialResultSet` chunk containers and inner `Value` objects (strings, numbers, booleans, nulls) are recycled using `sync.Pool`.
3. **Zero External Tooling:** Works out of the box in pure standard Go code—no `protoc` plugins or code generation scripts required.
4. **Safe Fallback:** All non-streaming RPC messages (sessions, commit requests, metadata) automatically fall back to standard `proto.Unmarshal`.

---

## How It Works

### 1. The Custom gRPC Codec (`SpannerFastCodec`)
Implements `google.golang.org/grpc/encoding.Codec`:
```go
type SpannerFastCodec struct{}

func (SpannerFastCodec) Name() string { return "proto" }

func (SpannerFastCodec) Marshal(value any) ([]byte, error) {
    return proto.Marshal(value.(proto.Message))
}

func (SpannerFastCodec) Unmarshal(data []byte, value any) error {
    if partialResultSet, ok := value.(*sppb.PartialResultSet); ok {
        return FastUnmarshalPartialResultSet(data, partialResultSet)
    }
    return proto.Unmarshal(data, value.(proto.Message))
}
```

### 2. Memory Pooling (`CustomPartialResultSetPool`)
Implements `spanner.PartialResultSetPool`:
```go
type CustomPartialResultSetPool struct{}

func (p *CustomPartialResultSetPool) Get() *sppb.PartialResultSet {
    return partialResultSetPool.Get().(*sppb.PartialResultSet)
}

func (p *CustomPartialResultSetPool) Put(partialResultSet *sppb.PartialResultSet) {
    // Recycles nested *structpb.Value objects and resets the chunk slice
    ...
}
```

### 3. Creating the Client
```go
func NewCustomOptimizedSpannerClient(ctx context.Context, database string, opts ...option.ClientOption) (*spanner.Client, error) {
    codecOption := option.WithGRPCDialOption(
        grpc.WithDefaultCallOptions(grpc.ForceCodec(SpannerFastCodec{})),
    )
    allOpts := append([]option.ClientOption{codecOption}, opts...)
    return spanner.NewClientWithConfig(ctx, database,
        spanner.ClientConfig{
            SessionPoolConfig:    spanner.DefaultSessionPoolConfig,
            PartialResultSetPool: &CustomPartialResultSetPool{},
        },
        allOpts...,
    )
}
```

---

## Running Tests and Benchmarks

### Run Automated Tests
```bash
go test -v ./...
```

### Run End-to-End Client Benchmarks
```bash
go test -bench=BenchmarkSpannerClient -benchmem -benchtime=2s
```

#### Benchmark Results (1,000 rows / 4,000 columns per query):
```text
BenchmarkSpannerClient/Standard_Protobuf-8           385   6203632 ns/op   5892991 B/op   115213 allocs/op
BenchmarkSpannerClient/CustomCodec_With_Pooling-8    415   5777385 ns/op   5571052 B/op   103241 allocs/op
```
