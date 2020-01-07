# genlocal

genlocal is a binary for generating gapics locally. It may be used to test out
new changes, test the generation of a new library, test new generator tweaks,
run generators against googleapis-private, and various other local tasks.

## Required tools

1. Install [docker](https://www.docker.com/get-started)
1. Install [protoc](https://github.com/protocolbuffers/protobuf/releases)
1. Install [Go](http://golang.org/dl)
1. Install python3, pip3
1. Install virtualenv `pip3 install virtualenv`
1. Install Go tools:

    ```
    go get \
        github.com/golang/protobuf/protoc-gen-go \
        golang.org/x/lint/golint \
        golang.org/x/tools/cmd/goimports \
        honnef.co/go/tools/cmd/staticcheck \
        golang.org/x/review/git-codereview \
        github.com/googleapis/gapic-generator-go/cmd/protoc-gen-go_gapic
    ```

## Running

```
cd /path/to/internal/gapicgen
go run cloud.google.com/go/internal/gapicgen/cmd/genlocal
```