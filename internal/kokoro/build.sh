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
cp -R . $GOCLOUD_HOME
cd $GOCLOUD_HOME
go get -v ./...

# Don't print out encryption keys, etc
set +x
# TODO(shadams): uncomment after keystore is set up
# set:
# 	- encrypted_ba2d6f7723ed_key
# 	- encrypted_ba2d6f7723ed_iv
# 	- encrypted_ba2d6f7723ed_pass
# from keystore.
# openssl aes-256-cbc -K $encrypted_ba2d6f7723ed_key -iv $encrypted_ba2d6f7723ed_iv -pass pass:$encrypted_ba2d6f7723ed_pass -in kokoro-key.json.enc -out key.json -d
set -x

# Run tests
# GCLOUD_TESTS_GOLANG_PROJECT_ID="dulcet-port-762" GCLOUD_TESTS_GOLANG_KEY="$(pwd)/key.json" go test -race -v ./...

# Run tests (only unit until keystore is set up)
go test -race -short -v ./...