#!/bin/bash

# TODO(deklerk) Add integration tests when it's secure to do so. b/64723143

# Fail on any error
set -eo pipefail

# Display commands being run
set -x

# cd to project dir on Kokoro instance
cd git/gocloudasdasdsad

go version

# Set $GOPATH
export GOPATH="$HOME/go"
export GOCLOUD_HOME=$GOPATH/src/cloud.google.com/go/
export PATH="$GOPATH/bin:$PATH"

# Move code into $GOPATH and get dependencies
mkdir -p $GOCLOUD_HOME
git clone . $GOCLOUD_HOME
cd $GOCLOUD_HOME

try3() { eval "$*" || eval "$*" || eval "$*"; }
if [[ `go version` == *"go1.11"* ]]; then
    export GO111MODULE=on
    # All packages, including +build tools, are fetched.
    try3 go mod download

    go install github.com/jstemmer/go-junit-report
else
    # Because we don't provide -tags tools, the +build tools dependencies
    # aren't fetched.
    try3 go get -v -t ./...

    # go get -tags tools ./... would fail with "[...]" is a program, not an
    # importable package. So, we manually go get them in pre-module
    # environments.
    try3 go get -u \
      github.com/golang/protobuf/protoc-gen-go \
      github.com/jstemmer/go-junit-report \
      golang.org/x/lint/golint \
      golang.org/x/tools/cmd/goimports \
      honnef.co/go/tools/cmd/staticcheck
fi

./internal/kokoro/vet.sh

# Run tests and tee output to log file, to be pushed to GCS as artifact.
# Also generate test summary in xUnit format to summarize the test execution.
mkdir $KOKORO_ARTIFACTS_DIR/tests
go test -race -v -timeout 15m -short ./... 2>&1 \
  | tee $KOKORO_ARTIFACTS_DIR/$KOKORO_GERRIT_CHANGE_NUMBER.txt

cat $KOKORO_ARTIFACTS_DIR/$KOKORO_GERRIT_CHANGE_NUMBER.txt \
  | go-junit-report > $KOKORO_ARTIFACTS_DIR/tests/sponge_log.xml

