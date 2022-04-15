# go-bench-gcs
**This is not an officially supported Google product**

## Run example:
This runs 1000 iterations on 0 to 2Gib files in the background, sending program output (errors and such) to `output.log`:

`go run main -t 72h -max_size 2097152 -max_samples 1000 -o results/2GibFiles1k.csv -c &> output.log &`



## CLI parameters

| Parameter | Description | Possible values | Default |
| --------- | ----------- | --------------- |:-------:|
| -api | which API to use | `JSON`: use JSON to upload and XML to download <br> `XML`: use JSON to upload and XML to download <br> `GRPC`: use GRPC <br> `MIXED`: select an API at random for each upload/download  | `MIXED` |
| -r | bucket region for benchmarks | any GCS region | `US-WEST1` |
| -conn_pool | GRPC connection pool size | any positive integer | 4 |
| -workers | number of goroutines to run at once; set to 1 for no concurrency | any positive integer | `16` |
| -creds | path to credentials file | any path | * |
| -gc_f | whether to force garbage collection <br> before every write or read benchmark |  `true` or `false` (present/not present) | `false` |
| -min_cs | minimum ChunkSize in kib | any positive integer | `16384` |
| -max_cs | maximum ChunkSize in kib | any positive integer | `16384` |
| -min_size | minimum object size in kib | any positive integer | `0` |
| -max_size | maximum object size in kib | any positive integer | `16` |
| -t | timeout (maximum time running benchmarks) | any [time.Duration](https://pkg.go.dev/time#Duration) | `1h` |
| -min_samples | minimum number of objects to upload | any positive integer | `10` |
| -max_samples | maximum number of objects to upload | any positive integer | `10 000` |
| -o | file to output results to | any file path | `res.csv` |
| -p | projectID | a project ID | * |
| -q_read | download quantum | any positive integer | 1 |
| -q_write | upload quantum | any positive integer | 1 |
| -min_r_size | minimum read size in bytes | any positive integer | 4000 |
| -max_r_size | maximum read size in bytes | any positive integer | 4000 |
| -min_w_size | minimum write size in bytes | any positive integer | 4000 |
| -max_w_size | maximum write size in bytes | any positive integer | 4000 |

\* required values

Note: while the default read/write size for HTTP clients is 4Kb 
(the default for this benchmarking), the default for GRPC is 32Kb.
If you want to capture performance using the defaults for GRPC, run the script 
separately, setting the read and write sizes to 32KB.