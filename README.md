# Google Cloud for Go

Go packages for Google Cloud Platform services. Supported APIs include:

 * Google Cloud Datastore
 * Google Cloud Storage
 * Google Cloud Pub/Sub
 * Google Cloud Container Engine

``` go
import "google.golang.org/cloud"
```

> Note: This package is a work-in-progress, and may occasionally
> make backwards-incompatible changes.

Documentation and examples are available at
[https://godoc.org/google.golang.org/cloud](https://godoc.org/google.golang.org/cloud).

## Authorization

Authorization, throughout the package, is delegated to the godoc.org/golang.org/x/oauth2.
Refer to the [godoc documentation](https://godoc.org/golang.org/x/oauth2)
for examples on using oauth2 with the Cloud package.

## Google Cloud Datastore

[Google Cloud Datastore][cloud-datastore] ([docs][cloud-datastore-docs]) is a fully
managed, schemaless database for storing non-relational data. Cloud Datastore
automatically scales with your users and supports ACID transactions, high availability
of reads and writes, strong consistency for reads and ancestor queries, and eventual
consistency for all other queries.

Follow the [activation instructions][cloud-datastore-activation] to use the Google
Cloud Datastore API with your project.

[https://godoc.org/google.golang.org/cloud/datastore](https://godoc.org/google.golang.org/cloud/datastore)


```go
// snippet to show how easy it is
```

## Google Cloud Storage

[Google Cloud Storage][cloud-storage] ([docs][cloud-storage-docs]) allows you to store
data on Google infrastructure with very high reliability, performance and availability,
and can be used to distribute large data objects to users via direct download.

[https://godoc.org/google.golang.org/cloud/storage](https://godoc.org/google.golang.org/cloud/storage)


```go
// snippet
```

## Google Cloud Pub/Sub (Alpha)

> Google Cloud Pub/Sub is in **Alpha status**. As a result, it might change in
> backward-incompatible ways and is not recommended for production use. It is not
> subject to any SLA or deprecation policy.

[Google Cloud Pub/Sub][cloud-pubsub] ([docs][cloud-pubsub-docs]) allows you to connect
your services with reliable, many-to-many, asynchronous messaging hosted on Google's
infrastructure. Cloud Pub/Sub automatically scales as you need it and provides a foundation
for building your own robust, global services.

[https://godoc.org/google.golang.org/cloud/pubsub](https://godoc.org/google.golang.org/cloud/pubsub)


```go
```

## Contributing

Contributions are welcome. Please, see the
[CONTRIBUTING](https://github.com/GoogleCloudPlatform/gcloud-golang/blob/master/CONTRIBUTING.md)
document for details. We're using Gerrit for our code reviews. Please don't open pull
requests against this repo, new pull requests will be automatically closed.

[cloud-datastore]: https://cloud.google.com/datastore/
[cloud-datastore-docs]: https://cloud.google.com/datastore/docs
[cloud-datastore-activation]: https://cloud.google.com/datastore/docs/activate

[cloud-pubsub]: https://cloud.google.com/pubsub/
[cloud-pubsub-docs]: https://cloud.google.com/pubsub/docs

[cloud-storage]: https://cloud.google.com/storage/
[cloud-storage-docs]: https://cloud.google.com/storage/docs/overview
[cloud-storage-create-bucket]: https://cloud.google.com/storage/docs/cloud-console#_creatingbuckets
