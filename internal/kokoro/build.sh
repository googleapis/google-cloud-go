#!/bin/bash

# Fail on any error
set -eo pipefail

# Display commands being run
set -x

# cd to project dir on Kokoro instance
cd git/gocloud

go version

# Set $GOPATH
export GOPATH="$HOME/go"
GOCLOUD_HOME=$GOPATH/src/cloud.google.com/go
mkdir -p $GOCLOUD_HOME

# Move code into $GOPATH and get dependencies
cp -R ./* $GOCLOUD_HOME
cd $GOCLOUD_HOME
go get -v -t ./...

# Run tests and tee output to log file, to be pushed to GCS as artifact.
go test -race -v -short ./... 2>&1 | tee $KOKORO_ARTIFACTS_DIR/$KOKORO_GERRIT_CHANGE_NUMBER.txt