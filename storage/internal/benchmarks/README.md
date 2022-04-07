# go-bench-gcs

## Run example:
This runs 1000 iterations on 0 to 2Gib files in the background, sending program output (errors and such) to `output.log`:

`go run main -t 72h -max_size 2097152 -max_samples 1000 -o results/2GibFiles1k.csv -c &> output.log &`



## CLI parameters

| Parameter | Description | Possible values | Default |
| --------- | ----------- | --------------- |:-------:|
| -api | which API to use | `JSON`: use JSON to upload and XML to download <br> `XML`: use JSON to upload and XML to download <br> `GRPC`: use GRPC <br> `RANDOM`: select an API at random  | `RANDOM` |
| -r | bucket region for benchmarks | any GCS region | `US-WEST1` |
| -c | whether to run benchmarks concurrently | `true` or `false` (present/not present) | `false` |
| -creds | path to credentials file | any path | * |
| -gc_f | whether to force garbage collection <br> at the beginning of each upload |  `true` or `false` (present/not present) | `false` |
| -min_cs | minimum ChunkSize in kib | any positive integer | `16384` |
| -max_cs | maximum ChunkSize in kib | any positive integer | `16384` |
| -min_size | minimum object size in kib | any positive integer | `0` |
| -max_size | maximum object size in kib | any positive integer | `16` |
| -t | timeout (maximum time running benchmarks) | any [time.Duration](https://pkg.go.dev/time#Duration) | `1h` |
| -min_samples | minimum number of objects to upload | any positive integer | `10` |
| -max_samples | maximum number of objects to upload | any positive integer | `10 000` |
| -o | file to output results to | any file path | `res.csv` |
| -p | projectID | a project ID | * |
| -q_read | download quantum | any positive integer | 16 |
| -q_write | upload quantum | any positive integer | 16 |

\* required values