_October 10, 2016_

Breaking changes to cloud.google.com/go/storage:

* AdminClient replaced by methods on Client.
    Replace
    ```go
    adminClient.CreateBucket(ctx, bucketName, attrs)
    ```
    with 
    ```go
    client.Bucket(bucketName).Create(ctx, projectID, attrs)
    ```

* BucketHandle.List replaced by BucketHandle.Objects.
    Replace
    ```go
    for query != nil {
        objs, err := bucket.List(d.ctx, query)
        if err != nil { ... }
        query = objs.Next
        for _, obj := range objs.Results {
            fmt.Println(obj)
        }
    }
    ```
    with
    ```go
    iter := bucket.Objects(d.ctx, query)
    for {
        obj, err := iter.Next()
        if err == iterator.Done {
            break
        }
        if err != nil { ... }
        fmt.Println(obj)
    }
    ```
    (The `iterator` package is at `google.golang.org/api/iterator`.)

    Replace `Query.Cursor` with `ObjectIterator.PageInfo().Token`.
    
    Replace `Query.MaxResults` with `ObjectIterator.PageInfo().MaxSize`.


* ObjectHandle.CopyTo replaced by ObjectHandle.CopierFrom.
    Replace
    ```go
    attrs, err := src.CopyTo(ctx, dst, nil)
    ```
    with
    ```go
    attrs, err := dst.CopierFrom(src).Run(ctx)
    ```

    Replace
    ```go
    attrs, err := src.CopyTo(ctx, dst, &storage.ObjectAttrs{ContextType: "text/html"})
    ```
    with
    ```go
    c := dst.CopierFrom(src)
    c.ContextType = "text/html"
    attrs, err := c.Run(ctx)
    ```

* ObjectHandle.ComposeFrom replaced by ObjectHandle.ComposerFrom.
    Replace
    ```go
    attrs, err := dst.ComposeFrom(ctx, []*storage.ObjectHandle{src1, src2}, nil)
    ```
    with
    ```go
    attrs, err := dst.ComposerFrom(src1, src2).Run(ctx)
    ```

* ObjectHandle.Update's ObjectAttrs argument replaced by ObjectAttrsToUpdate.
    Replace
    ```go
    attrs, err := obj.Update(ctx, &storage.ObjectAttrs{ContextType: "text/html"})
    ```
    with
    ```go
    attrs, err := obj.Update(ctx, storage.ObjectAttrsToUpdate{ContextType: "text/html"})
    ```

* ObjectHandle.WithConditions replaced by ObjectHandle.If.
    Replace
    ```go
    obj.WithConditions(storage.Generation(gen), storage.IfMetaGenerationMatch(mgen))
    ```
    with
    ```go
    obj.Generation(gen).If(storage.Conditions{MetagenerationMatch: mgen})
    ```

    Replace
    ```go
    obj.WithConditions(storage.IfGenerationMatch(0))
    ```
    with
    ```go
    obj.If(storage.Conditions{DoesNotExist: true})
    ```

* `storage.Done` replaced by `iterator.Done` (from package `google.golang.org/api/iterator`).

_October 6, 2016_

Package preview/logging deleted. Use logging instead.

_September 27, 2016_

Logging client replaced with preview version (see below).

_September 8, 2016_

* New clients for some of Google's Machine Learning APIs: Vision, Speech, and
Natural Language.

* Preview version of a new [Stackdriver Logging][cloud-logging] client in
[`cloud.google.com/go/preview/logging`](https://godoc.org/cloud.google.com/go/preview/logging).
This client uses gRPC as its transport layer, and supports log reading, sinks
and metrics. It will replace the current client at `cloud.google.com/go/logging` shortly.

