# Dataflux for Google Cloud Storage Go client library

## Overview
The purpose of this client is to quickly list data stored in GCS.

## Fast List
The fast list component of this client leverages GCS API to parallelize the listing of files within a GCS bucket. It does this by implementing a workstealing algorithm, where each worker in the list operation is able to steal work from its siblings once it has finished all currently stated listing work. This parallelization leads to a significant real world speed increase than sequential listing. Note that paralellization is limited by the machine on which the client runs. 

Benchmarking has demonstrated that the larger the object count, the better Dataflux performs when compared to a linear listing. Around 100k objects, users will see improvemement in listing speed.

### Example Usage

First create a `storage.Client` to use throughout your application:

[snip]:# (storage-1)
```go
ctx := context.Background()
client, err := storage.NewClient(ctx)
if err != nil {
    log.Fatal(err)
}
```

[snip]:# (storage-2)
```go

// storage.Query to filter objects that the user wants to list.
query := storage.Query{}
// Input for fast-listing.
dfopts := dataflux.ListerInput{
    BucketName:		"bucket",
    Parallelism:	500,
    BatchSize:		500000,
    Query:			query,
}

// Construct a dataflux lister.
df, close = dataflux.NewLister(sc, dfopts)
defer close()

// List objects in GCS bucket.
for {
    objects, err := df.NextBatch(ctx)

    if err == iterator.Done {
        // No more objects in the bucket to list.
        break
        }
    if err != nil {
        log.Fatal(err)
        }
    // TODO: process objects
}
```

### Fast List Benchmark Results
VM used : n2d-standard-48
Region: us-central1-a
NIC type: gVNIC
|File Count|VM Core Count|List Time Without Dataflux  |List Time With Dataflux|
|------------|-------------|--------------------------|-----------------------|
|5000000 Obj |48 Core      |319.72s                   |17.35s                 |
|1999032 Obj |48 Core      |139.54s                   |8.98s                  |
|578703 Obj  |48 Core      |32.90s                    |5.71s                  |
|10448 Obj   |48 Core      |750.50ms                  |637.17ms               |