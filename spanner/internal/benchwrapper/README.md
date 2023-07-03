# Benchwrapper

A small gRPC wrapper around the spanner client library. This allows the
benchmarking code to prod at spanner without speaking Go.

## Running

```
cd spanner/internal/benchwrapper
export SPANNER_EMULATOR_HOST=localhost:9010
go run *.go --port=8081
```
