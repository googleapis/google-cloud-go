# Google Cloud for Go

[![Build Status](https://travis-ci.org/GoogleCloudPlatform/google-cloud-go.svg?branch=master)](https://travis-ci.org/GoogleCloudPlatform/google-cloud-go)
[![GoDoc](https://godoc.org/cloud.google.com/go?status.svg)](https://godoc.org/cloud.google.com/go)

``` go
import "cloud.google.com/go"
```

Go packages for Google Cloud Platform services.

**NOTE:** These packages are under development, and may occasionally make
backwards-incompatible changes.

**NOTE:** Github repo is a mirror of [https://code.googlesource.com/gocloud](https://code.googlesource.com/gocloud).

## News

_November 17, 2016_

Change to BigQuery: values from INTEGER columns will now be returned as int64,
not int. This will avoid errors arising from large values on 32-bit systems.

_November 8, 2016_

New datastore feature: datastore now encodes your nested Go structs as Entity values,
instead of a flattened list of the embedded struct's fields.
This means that you may now have twice-nested slices, eg.
```
type State struct {
  Cities  []struct{
    Populations []int
  }
}
```

See [the announcement](https://groups.google.com/forum/#!topic/google-api-go-announce/79jtrdeuJAg) for
more details.

_November 8, 2016_

Breaking changes to datastore: contexts no longer hold namespaces; instead you
must set a key's namespace explicitly. Also, key functions have been changed
and renamed.

* The WithNamespace function has been removed. To specify a namespace in a Query, use the Query.Namespace method:
  ```go
  q := datastore.NewQuery("Kind").Namespace("ns")
  ```

* All the fields of Key are exported. That means you can construct any Key with a struct literal:
  ```go
  k := &Key{Kind: "Kind",  ID: 37, Namespace: "ns"}
  ```

* As a result of the above, the Key methods Kind, ID, d.Name, Parent, SetParent and Namespace have been removed.

* `NewIncompleteKey` has been removed, replaced by `IncompleteKey`. Replace
  ```go
  NewIncompleteKey(ctx, kind, parent)
  ```
  with
  ```go
  IncompleteKey(kind, parent)
  ```
  and if you do use namespaces, make sure you set the namespace on the returned key.

* `NewKey` has been removed, replaced by `NameKey` and `IDKey`. Replace
  ```go
  NewKey(ctx, kind, name, 0, parent)
  NewKey(ctx, kind, "", id, parent)
  ```
  with
  ```go
  NameKey(kind, name, parent)
  IDKey(kind, id, parent)
  ```
  and if you do use namespaces, make sure you set the namespace on the returned key.

* The `Done` variable has been removed. Replace `datastore.Done` with `iterator.Done`, from the package `google.golang.org/api/iterator`.

* The `Client.Close` method will have a return type of error. It will return the result of closing the underlying gRPC connection.

See [the announcement](https://groups.google.com/forum/#!topic/google-api-go-announce/hqXtM_4Ix-0) for
more details.

_October 27, 2016_

Breaking change to bigquery: `NewGCSReference` is now a function,
not a method on `Client`.

New bigquery feature: `Table.LoaderFrom` now accepts a `ReaderSource`, enabling
loading data into a table from a file or any `io.Reader`.

_October 21, 2016_

Breaking change to pubsub: removed `pubsub.Done`.

Use `iterator.Done` instead, where `iterator` is the package
`google.golang.org/api/iterator`.


_October 19, 2016_

Breaking changes to cloud.google.com/go/bigquery:

* Client.Table and Client.OpenTable have been removed.
    Replace
    ```go
    client.OpenTable("project", "dataset", "table")
    ```
    with
    ```go
    client.DatasetInProject("project", "dataset").Table("table")
    ```

* Client.CreateTable has been removed.
    Replace
    ```go
    client.CreateTable(ctx, "project", "dataset", "table")
    ```
    with
    ```go
    client.DatasetInProject("project", "dataset").Table("table").Create(ctx)
    ```
    
* Dataset.ListTables have been replaced with Dataset.Tables.
    Replace
    ```go
    tables, err := ds.ListTables(ctx)
    ```
    with
    ```go
    it := ds.Tables(ctx)
    for {
        table, err := it.Next()
        if err == iterator.Done {
            break
        }
        if err != nil {
            // TODO: Handle error.
        }
        // TODO: use table.
    }
    ```

* Client.Read has been replaced with Job.Read, Table.Read and Query.Read. 
    Replace
    ```go
    it, err := client.Read(ctx, job)
    ```
    with
    ```go
    it, err := job.Read(ctx)
    ```
  and similarly for reading from tables or queries.

* The iterator returned from the Read methods is now named RowIterator. Its
  behavior is closer to the other iterators in these libraries. It no longer
  supports the Schema method; see the next item.
    Replace
    ```go
    for it.Next(ctx) {
        var vals ValueList
        if err := it.Get(&vals); err != nil {
            // TODO: Handle error.
        }
        // TODO: use vals.
    }
    if err := it.Err(); err != nil {
        // TODO: Handle error.
    }
    ```
    with
    ```
    for {
        var vals ValueList
        err := it.Next(&vals)
        if err == iterator.Done {
            break
        }
        if err != nil {
            // TODO: Handle error.
        }
        // TODO: use vals.
    }
    ```
    Instead of the `RecordsPerRequest(n)` option, write
    ```go
    it.PageInfo().MaxSize = n
    ```
    Instead of the `StartIndex(i)` option, write
    ```go
    it.StartIndex = i
    ```

* ValueLoader.Load now takes a Schema in addition to a slice of Values.
    Replace
    ```go
    func (vl *myValueLoader) Load(v []bigquery.Value)
    ```
    with
    ```go
    func (vl *myValueLoader) Load(v []bigquery.Value, s bigquery.Schema)
    ```


* Table.Patch is replace by Table.Update.
    Replace
    ```go
    p := table.Patch()
    p.Description("new description")
    metadata, err := p.Apply(ctx)
    ```
    with
    ```go
    metadata, err := table.Update(ctx, bigquery.TableMetadataToUpdate{
        Description: "new description",
    })
    ```

* Client.Copy is replaced by separate methods for each of its four functions.
  All options have been replaced by struct fields.

  * To load data from Google Cloud Storage into a table, use Table.LoaderFrom.

    Replace
    ```go
    client.Copy(ctx, table, gcsRef)
    ```
    with
    ```go
    table.LoaderFrom(gcsRef).Run(ctx)
    ```
    Instead of passing options to Copy, set fields on the Loader:
    ```go
    loader := table.LoaderFrom(gcsRef)
    loader.WriteDisposition = bigquery.WriteTruncate
    ```

  * To extract data from a table into Google Cloud Storage, use
    Table.ExtractorTo. Set fields on the returned Extractor instead of
    passing options.

    Replace
    ```go
    client.Copy(ctx, gcsRef, table)
    ```
    with
    ```go
    table.ExtractorTo(gcsRef).Run(ctx)
    ```

  * To copy data into a table from one or more other tables, use
    Table.CopierFrom. Set fields on the returned Copier instead of passing options.

    Replace
    ```go
    client.Copy(ctx, dstTable, srcTable)
    ```
    with
    ```go
    dst.Table.CopierFrom(srcTable).Run(ctx)
    ```

  * To start a query job, create a Query and call its Run method. Set fields
  on the query instead of passing options.

    Replace
    ```go
    client.Copy(ctx, table, query)
    ```
    with
    ```go
    query.Run(ctx)
    ```

* Table.NewUploader has been renamed to Table.Uploader. Instead of options,
  configure an Uploader by setting its fields.
    Replace
    ```go
    u := table.NewUploader(bigquery.UploadIgnoreUnknownValues())
    ```
    with
    ```go
    u := table.NewUploader(bigquery.UploadIgnoreUnknownValues())
    u.IgnoreUnknownValues = true
    ```


[Older news](https://github.com/GoogleCloudPlatform/google-cloud-go/blob/master/old-news.md)

## Supported APIs

Google API                     | Status       | Package
-------------------------------|--------------|-----------------------------------------------------------
[Datastore][cloud-datastore]   | beta         | [`cloud.google.com/go/datastore`][cloud-datastore-ref]
[Storage][cloud-storage]       | beta         | [`cloud.google.com/go/storage`][cloud-storage-ref]
[Pub/Sub][cloud-pubsub]        | experimental | [`cloud.google.com/go/pubsub`][cloud-pubsub-ref]
[Bigtable][cloud-bigtable]     | beta         | [`cloud.google.com/go/bigtable`][cloud-bigtable-ref]
[BigQuery][cloud-bigquery]     | experimental | [`cloud.google.com/go/bigquery`][cloud-bigquery-ref]
[Logging][cloud-logging]       | experimental | [`cloud.google.com/go/logging`][cloud-logging-ref]
[Vision][cloud-vision]         | experimental | [`cloud.google.com/go/vision`][cloud-vision-ref]
[Language][cloud-language]     | experimental | [`cloud.google.com/go/language/apiv1beta1`][cloud-language-ref]
[Speech][cloud-speech]         | experimental | [`cloud.google.com/go/speech/apiv1beta`][cloud-speech-ref]


> **Experimental status**: the API is still being actively developed. As a
> result, it might change in backward-incompatible ways and is not recommended
> for production use.
>
> **Beta status**: the API is largely complete, but still has outstanding
> features and bugs to be addressed. There may be minor backwards-incompatible
> changes where necessary.
>
> **Stable status**: the API is mature and ready for production use. We will
> continue addressing bugs and feature requests.

Documentation and examples are available at
https://godoc.org/cloud.google.com/go

Visit or join the
[google-api-go-announce group](https://groups.google.com/forum/#!forum/google-api-go-announce)
for updates on these packages.

## Go Versions Supported

We support the two most recent major versions of Go. If Google App Engine uses
an older version, we support that as well. You can see which versions are
currently supported by looking at the lines following `go:` in
[`.travis.yml`](.travis.yml).

## Authorization

By default, each API will use [Google Application Default Credentials][default-creds]
for authorization credentials used in calling the API endpoints. This will allow your
application to run in many environments without requiring explicit configuration.

To authorize using a
[JSON key file](https://cloud.google.com/iam/docs/managing-service-account-keys),
pass
[`option.WithServiceAccountFile`](https://godoc.org/google.golang.org/api/option#WithServiceAccountFile)
to the `NewClient` function of the desired package. For example:

```go
client, err := storage.NewClient(ctx, option.WithServiceAccountFile("path/to/keyfile.json"))
```

You can exert more control over authorization by using the
[`golang.org/x/oauth2`](https://godoc.org/golang.org/x/oauth2) package to
create an `oauth2.TokenSource`. Then pass
[`option.WithTokenSource`](https://godoc.org/google.golang.org/api/option#WithTokenSource)
to the `NewClient` function:
```go
tokenSource := ...
client, err := storage.NewClient(ctx, option.WithTokenSource(tokenSource))
```

## Cloud Datastore [![GoDoc](https://godoc.org/cloud.google.com/go/datastore?status.svg)](https://godoc.org/cloud.google.com/go/datastore)

- [About Cloud Datastore][cloud-datastore]
- [Activating the API for your project][cloud-datastore-activation]
- [API documentation][cloud-datastore-docs]
- [Go client documentation](https://godoc.org/cloud.google.com/go/datastore)
- [Complete sample program](https://github.com/GoogleCloudPlatform/golang-samples/tree/master/datastore/tasks)

### Example Usage

First create a `datastore.Client` to use throughout your application:

```go
client, err := datastore.NewClient(ctx, "my-project-id")
if err != nil {
	log.Fatal(err)
}
```

Then use that client to interact with the API:

```go
type Post struct {
	Title       string
	Body        string `datastore:",noindex"`
	PublishedAt time.Time
}
keys := []*datastore.Key{
	datastore.NewKey(ctx, "Post", "post1", 0, nil),
	datastore.NewKey(ctx, "Post", "post2", 0, nil),
}
posts := []*Post{
	{Title: "Post 1", Body: "...", PublishedAt: time.Now()},
	{Title: "Post 2", Body: "...", PublishedAt: time.Now()},
}
if _, err := client.PutMulti(ctx, keys, posts); err != nil {
	log.Fatal(err)
}
```

## Cloud Storage [![GoDoc](https://godoc.org/cloud.google.com/go/storage?status.svg)](https://godoc.org/cloud.google.com/go/storage)

- [About Cloud Storage][cloud-storage]
- [API documentation][cloud-storage-docs]
- [Go client documentation](https://godoc.org/cloud.google.com/go/storage)
- [Complete sample programs](https://github.com/GoogleCloudPlatform/golang-samples/tree/master/storage)

### Example Usage

First create a `storage.Client` to use throughout your application:

```go
client, err := storage.NewClient(ctx)
if err != nil {
	log.Fatal(err)
}
```

```go
// Read the object1 from bucket.
rc, err := client.Bucket("bucket").Object("object1").NewReader(ctx)
if err != nil {
	log.Fatal(err)
}
defer rc.Close()
body, err := ioutil.ReadAll(rc)
if err != nil {
	log.Fatal(err)
}
```

## Cloud Pub/Sub [![GoDoc](https://godoc.org/cloud.google.com/go/pubsub?status.svg)](https://godoc.org/cloud.google.com/go/pubsub)

- [About Cloud Pubsub][cloud-pubsub]
- [API documentation][cloud-pubsub-docs]
- [Go client documentation](https://godoc.org/cloud.google.com/go/pubsub)
- [Complete sample programs](https://github.com/GoogleCloudPlatform/golang-samples/tree/master/pubsub)

### Example Usage

First create a `pubsub.Client` to use throughout your application:

```go
client, err := pubsub.NewClient(ctx, "project-id")
if err != nil {
	log.Fatal(err)
}
```

```go
// Publish "hello world" on topic1.
topic := client.Topic("topic1")
msgIDs, err := topic.Publish(ctx, &pubsub.Message{
	Data: []byte("hello world"),
})
if err != nil {
	log.Fatal(err)
}

// Create an iterator to pull messages via subscription1.
it, err := client.Subscription("subscription1").Pull(ctx)
if err != nil {
	log.Println(err)
}
defer it.Stop()

// Consume N messages from the iterator.
for i := 0; i < N; i++ {
	msg, err := it.Next()
	if err == iterator.Done {
		break
	}
	if err != nil {
		log.Fatalf("Failed to retrieve message: %v", err)
	}

	fmt.Printf("Message %d: %s\n", i, msg.Data)
	msg.Done(true) // Acknowledge that we've consumed the message.
}
```

## Cloud BigQuery [![GoDoc](https://godoc.org/cloud.google.com/go/bigquery?status.svg)](https://godoc.org/cloud.google.com/go/bigquery)

- [About Cloud BigQuery][cloud-bigquery]
- [API documentation][cloud-bigquery-docs]
- [Go client documentation][cloud-bigquery-ref]
- [Complete sample programs](https://github.com/GoogleCloudPlatform/golang-samples/tree/master/bigquery)

### Example Usage

First create a `bigquery.Client` to use throughout your application:
```go
c, err := bigquery.NewClient(ctx, "my-project-ID")
if err != nil {
    // TODO: Handle error.
}
```
Then use that client to interact with the API:
```go
// Construct a query.
q := c.Query(`
    SELECT year, SUM(number)
    FROM [bigquery-public-data:usa_names.usa_1910_2013]
    WHERE name = "William"
    GROUP BY year
    ORDER BY year
`)
// Execute the query.
it, err := q.Read(ctx)
if err != nil {
    // TODO: Handle error.
}
// Iterate through the results.
for {
    var values bigquery.ValueList
    err := it.Next(&values)
    if err == iterator.Done {
        break
    }
    if err != nil {
        // TODO: Handle error.
    }
    fmt.Println(values)
}
```


## Stackdriver Logging [![GoDoc](https://godoc.org/cloud.google.com/go/logging?status.svg)](https://godoc.org/cloud.google.com/go/logging)

- [About Stackdriver Logging][cloud-logging]
- [API documentation][cloud-logging-docs]
- [Go client documentation][cloud-logging-ref]
- [Complete sample programs](https://github.com/GoogleCloudPlatform/golang-samples/tree/master/logging)

### Example Usage

First create a `logging.Client` to use throughout your application:

```go
ctx := context.Background()
client, err := logging.NewClient(ctx, "my-project")
if err != nil {
    // TODO: Handle error.
}
```
Usually, you'll want to add log entries to a buffer to be periodically flushed
(automatically and asynchronously) to the Stackdriver Logging service.
```go
logger := client.Logger("my-log")
logger.Log(logging.Entry{Payload: "something happened!"})
```
Close your client before your program exits, to flush any buffered log entries.
```go
err = client.Close()
if err != nil {
    // TODO: Handle error.
}
```

## Contributing

Contributions are welcome. Please, see the
[CONTRIBUTING](https://github.com/GoogleCloudPlatform/google-cloud-go/blob/master/CONTRIBUTING.md)
document for details. We're using Gerrit for our code reviews. Please don't open pull
requests against this repo, new pull requests will be automatically closed.

Please note that this project is released with a Contributor Code of Conduct.
By participating in this project you agree to abide by its terms.
See [Contributor Code of Conduct](https://github.com/GoogleCloudPlatform/google-cloud-go/blob/master/CONTRIBUTING.md#contributor-code-of-conduct)
for more information.

[cloud-datastore]: https://cloud.google.com/datastore/
[cloud-datastore-ref]: https://godoc.org/cloud.google.com/go/datastore
[cloud-datastore-docs]: https://cloud.google.com/datastore/docs
[cloud-datastore-activation]: https://cloud.google.com/datastore/docs/activate

[cloud-pubsub]: https://cloud.google.com/pubsub/
[cloud-pubsub-ref]: https://godoc.org/cloud.google.com/go/pubsub
[cloud-pubsub-docs]: https://cloud.google.com/pubsub/docs

[cloud-storage]: https://cloud.google.com/storage/
[cloud-storage-ref]: https://godoc.org/cloud.google.com/go/storage
[cloud-storage-docs]: https://cloud.google.com/storage/docs
[cloud-storage-create-bucket]: https://cloud.google.com/storage/docs/cloud-console#_creatingbuckets

[cloud-bigtable]: https://cloud.google.com/bigtable/
[cloud-bigtable-ref]: https://godoc.org/cloud.google.com/go/bigtable

[cloud-bigquery]: https://cloud.google.com/bigquery/
[cloud-bigquery-docs]: https://cloud.google.com/bigquery/docs
[cloud-bigquery-ref]: https://godoc.org/cloud.google.com/go/bigquery

[cloud-logging]: https://cloud.google.com/logging/
[cloud-logging-docs]: https://cloud.google.com/logging/docs
[cloud-logging-ref]: https://godoc.org/cloud.google.com/go/logging

[cloud-vision]: https://cloud.google.com/vision/
[cloud-vision-ref]: https://godoc.org/cloud.google.com/go/vision

[cloud-language]: https://cloud.google.com/natural-language
[cloud-language-ref]: https://godoc.org/cloud.google.com/go/language/apiv1beta1

[cloud-speech]: https://cloud.google.com/speech
[cloud-speech-ref]: https://godoc.org/cloud.google.com/go/speech/apiv1beta1

[default-creds]: https://developers.google.com/identity/protocols/application-default-credentials
