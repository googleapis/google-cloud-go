# go-bench-gcs

## Run example:
This runs 1000 iterations on 0 to 2Gib files in the background, sending program output (errors and such) to `output.log`:

`go run main -t 72h -max_size 2097152 -max_samples 1000 -o results/2GibFiles1k.csv -c &> output.log &`



## CLI parameters:
```
  -api string
        api used to upload/download objects; JSON or XML values will use JSON to upload and XML to download (default "RANDOM")
  -c    concurrent
  -creds string
        path to credentials file
  -gc_f
        force garbage collection at the beginning of each upload
  -max_cs int
        max chunksize in kib (default 16384)
  -max_samples int
        maximum number of objects to upload (default 10000)
  -max_size int
        maximum object size in kib (default 16)
  -min_cs int
        min chunksize in kib (default 16384)
  -min_samples int
        minimum number of objects to upload (default 10)
  -min_size int
        minimum object size in kib
  -o string
        file to output results to (default "res.csv")
  -p string
        projectID
  -q_read int
        read quantum (default 16)
  -q_write int
        write quantum (default 16)
  -r string
        region (default "US-WEST1")
  -t duration
        timeout (default 1h0m0s)
```
