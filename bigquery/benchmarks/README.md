# BigQuery Benchmark
This directory contains benchmarks for BigQuery client, used primarily by library maintainers to measure changes that may affect library performance.


## Usage
`go run bench.go`

### Flags
`--reruns` can be used to override the default number of times a query is rerun.

`--projectid` can be used to run benchmarks in a different project.  If unset, the GOOGLE_CLOUD_PROJECT
 environment variable is used.

`--queryfile` can be used to override the default file which contains queries to be instrumented.

`--table` can be used to specify a table to which benchmarking results should be streamed.  The format for this string is in BigQuery standard SQL notation without escapes, e.g. `projectid.datasetid.tableid`

`--create_table` can be used to have the benchmarking tool create the destination table prior to streaming.

`--tag` allows arbitrary key:value pairs to be set.  This flag can be specified multiple times.


### Example invocations

Setting all the flags
```
go run bench.go \
  --reruns=5 \
  --projectid=test_project_id \
  --table=logging_project_id.querybenchmarks.measurements \
  --create_table \
  --tag=source:myhostname \
  --tag=somekeywithnovalue \
  --tag=experiment:special_environment_thing
```

Or, a more realistic invocation using shell substitions:
```
go run bench.go \
  --reruns=5 \
  --table=$BENCHMARK_TABLE \
  --tag=origin:$(hostname) \
  --tag=branch:$(git branch --show-current) \
  --tag=latestcommit:$(git log --pretty=format:'%H' -n 1)
```