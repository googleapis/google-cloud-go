# genlocal

genlocal is a binary for generating gapics locally. It may be used to test out
new changes, test the generation of a new library, test new generator tweaks,
run generators against googleapis-private, and various other local tasks.

## Required tools

*Note*: If you are on a Windows platform you will need to install these tools
in a linux docker container: [Install docker](https://www.docker.com/get-started)

1. Install [protoc](https://github.com/protocolbuffers/protobuf/releases)
2. Install [Go](http://golang.org/dl)
3. Install python3, pip3
4. Install virtualenv `pip3 install virtualenv`
5. Install Go tools:

```bash
go get \
    github.com/golang/protobuf/protoc-gen-go \
    golang.org/x/lint/golint \
    golang.org/x/tools/cmd/goimports \
    honnef.co/go/tools/cmd/staticcheck \
    github.com/googleapis/gapic-generator-go/cmd/protoc-gen-go_gapic
```

## Running

`git clone` this project if you don't already have it checked-out locally.

```bash
cd /path/to/google-cloud-go/internal/gapicgen
go run cloud.google.com/go/internal/gapicgen/cmd/genlocal
```
