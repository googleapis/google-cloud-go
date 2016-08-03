# Cloud Bigtable on App Engine Flexible using Go
# (Hello World for Cloud Bigtable)

This app counts how often each user visits.

## Prerequisites

1. Set up Cloud Console.
  1. Go to the [Cloud Console](https://cloud.google.com/console) and create or select your project.
     You will need the project ID later.
  1. Go to **Settings > Project Billing Settings** and enable billing.
  1. Select **APIs & Auth > APIs**.
  1. Enable the **Cloud Bigtable API** and the **Cloud Bigtable Admin API**.
  (You may need to search for the API).
1. Set up gcloud.
  1. `gcloud init`
1. Download App Engine SDK for Go.
  1. `go get -u google.golang.org/appengine/...`
1. In helloworld.go, change the constants `project`, `zone` and `cluster`

## Running locally

1. From the sample project folder, `go run *.go`

## Deploying on Google App Engine Flexible

1. From the sample project folder, `aedeploy gcloud app deploy`
