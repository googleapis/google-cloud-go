# go-bench-gcs
**This is not an officially supported Google product**

## Run example:
This runs 1000 iterations on 512kib to 2Gib files in the background, sending output to `out.log`:

`go run main -p {PROJECT_ID} -t 72h -max_samples 1000 -o {RESULTS_FILE_NAME}.csv &> out.log &`


## CLI parameters

| Parameter | Description | Possible values | Default |
| --------- | ----------- | --------------- |:-------:|
| -project | projectID | a project ID | * |
| -o | file to output results to <br> if empty, will output to stdout | any file path | stdout |
| -output_type | output results as csv records or cloud monitoring | `csv`, `cloud-monitoring` | `cloud-monitoring` |
| -api | which API to use | `JSON`: use JSON to upload and XML to download <br> `XML`: use JSON to upload and XML to download <br> `GRPC`: use GRPC <br> `MIXED`: select an API at random for each upload/download  <br> `DirectPath`: use GRPC with direct path | `Mixed` |
| -region | bucket region for benchmarks | any GCS region | `US-WEST1` |
| -workers | number of goroutines to run at once; set to 1 for no concurrency | any positive integer | `16` |
| -t | timeout (maximum time running benchmarks) <br> the program may run for longer while it finishes running processes | any [time.Duration](https://pkg.go.dev/time#Duration) | `1h` |
| -min_samples | minimum number of objects to upload | any positive integer | `10` |
| -max_samples | maximum number of objects to upload | any positive integer | `10 000` |
| -gc_f | whether to force garbage collection <br> before every write or read benchmark |  `true` or `false` (present/not present) | `false` |
| -object_size | object size in bytes | any positive integer | `1 048 576` (1 MiB) |
| -min_size | minimum object size in bytes <br> ignored if object_size is set | any positive integer | `512` |
| -max_size | maximum object size in bytes <br> ignored if object_size is set | any positive integer | `2 147 483 648` (2 GiB) |
| -range_read_size | size of the range to read in bytes | any positive integer <br> <=0 reads the full object | `0` |
| -min_read_offset | minimum offset for the start of the range to be read in bytes | any integer >0 | `0` |
| -max_read_offset | maximum offset for the start of the range to be read in bytes | any integer >0 | `0` |
| -defaults | use default settings for the client <br> (conn_pool, read, write and chunk size parameters will be ignored) | `true` or `false` | `false`
| -conn_pool | GRPC connection pool size | any positive integer | 4 |
| -min_cs | minimum ChunkSize in bytes | any positive integer | `16 384` (16 MiB) |
| -max_cs | maximum ChunkSize in bytes | any positive integer | `16 384` (16 MiB) |
| -q_read | download quantum | any positive integer | 1 |
| -q_write | upload quantum | any positive integer | 1 |
| -min_r_size | minimum read size in bytes | any positive integer | `4000` |
| -max_r_size | maximum read size in bytes | any positive integer | `4000` |
| -min_w_size | minimum write size in bytes | any positive integer | `4000` |
| -max_w_size | maximum write size in bytes | any positive integer | `4000` |
| -labels | labels added to cloud monitoring output (ignored when outputting as csv) | any string; should be in the format: <br> `stringKey=\"value\",intKey=3,boolKey=true` | empty |

\* required values

Note: while the default read/write size for HTTP clients is 4Kb 
(the default for this benchmarking), the default for GRPC is 32Kb.
If you want to capture performance using the defaults for GRPC run the script 
separately setting the read and write sizes to 32Kb, or run with the `defaults`
parameter set.