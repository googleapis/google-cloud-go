#!/bin/bash

# Fail on any error
set -e

# Display commands being run
set -x

go version

# Set $GOPATH
export GOPATH="$HOME/go"
GOCLOUD_HOME=$GOPATH/src/cloud.google.com/go
mkdir -p $GOCLOUD_HOME

# Move code into $GOPATH and get dependencies
cp -R ./* $GOCLOUD_HOME
cd $GOCLOUD_HOME
go get -v ./...

# Run tests
GCLOUD_TESTS_GOLANG_PROJECT_ID="dulcet-port-762" go test -race -v ./...