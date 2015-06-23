# Contributing

1. Sign one of the contributor license agreements below.
1. `go get golang.org/x/review/git-codereview` to install the code reviewing tool.
1. Get the cloud package by running `go get -d google.golang.org/cloud`.
    1. If you have already checked out the source, make sure that the remote git
       origin is https://code.googlesource.com/gocloud:

            git remote set-url origin https://code.googlesource.com/gocloud
1. Make changes and create a change by running `git codereview change <name>`,
provide a command message, and use `git codereview mail` to create a Gerrit CL.
1. Keep amending to the change and mail as your recieve feedback.

## Integration Tests

Additional to the unit tests, you may run the integration test suite.

To run the integrations tests, creating and configuration of a project in the
Google Developers Console is required. Once you create a project, set the
following environment variables to be able to run the against the actual APIs.

- **GCLOUD_TESTS_GOLANG_PROJECT_ID**: Developers Console project's ID (e.g. bamboo-shift-455)
- **GCLOUD_TESTS_GOLANG_KEY**: The path to the JSON key file.

Create a storage bucket with the same name as the project id set in **GCLOUD_TESTS_GOLANG_PROJECT_ID**.
The storage integration test will create and delete some objects in this bucket.

Install the [gcloud command-line tool][gcloudcli] to your machine and use it
to create the indexes used in the datastore integration tests with indexes
found in `datastore/testdata/index.yaml`:

From the project's root directory:

``` sh
# Install the app component
$ gcloud components update app

# Set the default project in your env
$ gcloud config set project $GCLOUD_TESTS_GOLANG_PROJECT_ID

# Authenticate the gcloud tool with your account
$ gcloud auth login

# Create the indexes
$ gcloud preview datastore create-indexes datastore/testdata

```

You can run the integration tests by running:

``` sh
$ go test -v -tags=integration google.golang.org/cloud/...
```

## Contributor License Agreements

Before we can accept your pull requests you'll need to sign a Contributor
License Agreement (CLA):

- **If you are an individual writing original source code** and **you own the
- intellectual property**, then you'll need to sign an [individual CLA][indvcla].
- **If you work for a company that wants to allow you to contribute your work**,
then you'll need to sign a [corporate CLA][corpcla].

You can sign these electronically (just scroll to the bottom). After that,
we'll be able to accept your pull requests.

[gcloudcli]: https://developers.google.com/cloud/sdk/gcloud/
[indvcla]: https://developers.google.com/open-source/cla/individual
[corpcla]: https://developers.google.com/open-source/cla/corporate
