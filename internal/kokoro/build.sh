#!/bin/bash

# Fail on any error
set -e

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
go get -v ./...

cd internal/kokoro
# Don't print out encryption keys, etc
set +x
key=$(cat $KOKORO_ARTIFACTS_DIR/keystore/*_encrypted_ba2d6f7723ed_key)
iv=$(cat $KOKORO_ARTIFACTS_DIR/keystore/*_encrypted_ba2d6f7723ed_iv)
pass=$(cat $KOKORO_ARTIFACTS_DIR/keystore/*_encrypted_ba2d6f7723ed_pass)

openssl aes-256-cbc -K $key -iv $iv -pass pass:$pass -in kokoro-key.json.enc -out key.json -d
set -x

export GCLOUD_TESTS_GOLANG_KEY="$(pwd)/key.json"
cd $GOCLOUD_HOME

# Run tests
GCLOUD_TESTS_GOLANG_PROJECT_ID="dulcet-port-762" go test -race -v ./...