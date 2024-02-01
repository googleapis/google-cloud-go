# go-bench-gcs
**This is not an officially supported Google product**

## Run example:
This runs 1000 iterations on 512kib to 2Gib files in the background, sending output to `out.log`:

`go run main -project {PROJECT_ID} -samples 1000 -object_size=524288..2147483648 -o {RESULTS_FILE_NAME} &> out.log &`


## CLI parameters

| Parameter | Description | Possible values | Default |
| --------- | ----------- | --------------- |:-------:|
| -project | projectID | a project ID | * |
| -bucket | bucket supplied for benchmarks <br> must be initialized | any GCS region | will create a randomly named bucket |
| -bucket_region | bucket region for benchmarks <br> ignored if bucket is explicitly supplied | any GCS region | `US-WEST1` |
| -o | file to output results to <br> if empty, will output to stdout | any file path | stdout |
| -output_type | output results as csv records or cloud monitoring | `csv`, `cloud-monitoring` | `cloud-monitoring` |
| -samples | number of samples to report | any positive integer | `8000` |
| -workers | number of goroutines to run at once; set to 1 for no concurrency | any positive integer | `16` |
| -clients | total number of Storage clients to be used; <br> if Mixed APIs, then x3 the number are created | any positive integer | `1` |
| -api | which API to use | `JSON`: use JSON <br> `XML`: use JSON to upload and XML to download <br> `GRPC`: use GRPC without directpath enabled <br> `Mixed`: select an API at random for each object <br> `DirectPath`: use GRPC with directpath | `Mixed` |
| -object_size | object size in bytes; can be a range min..max <br> for workload 6, a range will apply to objects within a directory | any positive integer | `1 048 576` (1 MiB) |
| -range_read_size | size of the range to read in bytes | any positive integer <br> <=0 reads the full object | `0` |
| -minimum_read_offset | minimum offset for the start of the range to be read in bytes | any integer >0 | `0` |
| -maximum_read_offset | maximum offset for the start of the range to be read in bytes | any integer >0 | `0` |
| -read_buffer_size | read buffer size in bytes | any positive integer | `4096` for HTTP <br> `32768` for GRPC |
| -write_buffer_size | write buffer size in bytes | any positive integer | `4096` for HTTP <br> `32768` for GRPC |
| -min_chunksize | minimum ChunkSize in bytes | any positive integer | `16 384` (16 MiB) |
| -max_chunksize | maximum ChunkSize in bytes | any positive integer | `16 384` (16 MiB) |
| -connection_pool_size | GRPC connection pool size | any positive integer | 4 |
| -force_garbage_collection | whether to force garbage collection <br> before every write or read benchmark |  `true` or `false` (present/not present) | `false` |
| -timeout | timeout (maximum time running benchmarks) <br> the program may run for longer while it finishes running processes | any [time.Duration](https://pkg.go.dev/time#Duration) | `1h` |
| -timeout_per_op | timeout on a single upload or download | any [time.Duration](https://pkg.go.dev/time#Duration) | `5m` |
| -workload | `1` will run a w1r3 (write 1 read 3) benchmark <br> `6` will run a benchmark uploading and downloading (once each) <br> a single directory with `-directory_num_objects` number of files (no subdirectories) <br> `9`** will run a benchmark that does continuous reads on a directory with `directory_num_objects` | `1`, `6`, `9` | `1` |
| -directory_num_objects | total number of objects in a directory (directory will only contain files, <br> no subdirectories); only applies to workload 6 and 9 | any positive integer | `1000` |
| -warmup | time to spend warming the clients before running benchmarks <br> w1r3 benchmarks will be run for this duration without recording any results <br> this is compatible with all workloads; however, w1r3 benchmarks are done regardless of workload <br> the warmups run with the number of logical CPUs usable by the current process  | any [time.Duration](https://pkg.go.dev/time#Duration) | `0s` |

\* required values

\*\* Note that this workload is experimental and will not work under certain conditions. Here's a non-comprehensive list of notes on workload 9:
 - output type must be `cloud-monitoring`
 - it continues reading until the timeout is reached - samples should be set to 1
 - `directory_num_objects` must be larger than `workers`
