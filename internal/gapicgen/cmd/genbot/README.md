# genbot

genbot is a binary for generating gapics and creating PRs with the results.
It is intended to be used as a bot, though it can be run locally too.

## Prerequisites for running locally

Note that only step one, listed below, is required if you plan to run the code
in docker.

1. Clone this project: `git clone https://github.com/googleapis/google-cloud-go.git`
1. Install [protoc](https://github.com/protocolbuffers/protobuf/releases)
1. Install [Go](http://golang.org/dl)
1. Add `$GOPATH/bin` to `PATH`
1. Create a [GitHub access token](https://help.github.com/en/github/authenticating-to-github/creating-a-personal-access-token-for-the-command-line).
1. Install Go tools:

```bash
go get \
    github.com/golang/protobuf/protoc-gen-go \
    golang.org/x/lint/golint \
    golang.org/x/tools/cmd/goimports \
    honnef.co/go/tools/cmd/staticcheck \
    github.com/googleapis/gapic-generator-go/cmd/protoc-gen-go_gapic
```

## Generating code and PRs(bot mode)

### Run genbot locally

Note: this may change your `~/.gitconfig`, `~/.gitcookies`, and use up
non-trivial amounts of space on your computer.

1. Make sure you are on a non-Windows platform. If you are using windows
   continue on to the docker instructions.
1. Make sure you have all the tools installed listed in genlocal's README.md
1. Run:

```shell
cd /path/to/internal/gapicgen
go run cloud.google.com/go/internal/gapicgen/cmd/genbot \
    --githubAccessToken=$GITHUB_ACCESS_TOKEN \
    --githubUsername=$GITHUB_USERNAME \
    --githubName="Jean de Klerk" \
    --githubEmail=deklerk@google.com \
```

### Run genbot with docker

Note: this can be quite slow (~10m).

Note: this may leave a lot of docker resources laying around. Use
`docker system prune` to clean up after runs.

```shell
cd /path/to/internal/gapicgen/cmd/genbot
docker build . -t genbot
docker run -t --rm --privileged \
   -v `pwd`/../..:/gapicgen \
   -e GITHUB_ACCESS_TOKEN \
   -e GITHUB_USERNAME \
   -e GITHUB_NAME \
   -e GITHUB_EMAIL \
   genbot
```

## Generating code (local mode)

Sometimes you may want to just generate gapic sources to test out
new changes, test the generation of a new library, test new generator tweaks,
run generators against googleapis-private, and various other local tasks. The
local mode in genbot allows you to do just that.

### Run genbot(local mode) locally

```shell
cd /path/to/internal/gapicgen
go run cloud.google.com/go/internal/gapicgen/cmd/genbot \
   -local \
   -only-gapics \
   -gocloud-dir=/path/to/google-cloud-go \
   -gapic=cloud.google.com/go/foo/apiv1
```

### Run genbot(local mode) with docker

```shell
cd /path/to/internal/gapicgen
docker build -t genbot -f cmd/genbot/Dockerfile .
docker run --rm \
   -v `pwd`/../..:/gapicgen \
   -e GENBOT_LOCAL_MODE=true \
   -e ONLY_GAPICS=true \
   -e GOCLOUD_DIR=/gapicgen \
   -e GAPIC_TO_GENERATE=cloud.google.com/go/foo/apiv1 \
   genbot
```

Note you can optionally mount in your Go module cache if you have Go installed.
This will speed up the build a bit:

```shell
-v `go env GOMODCACHE`:/root/go/pkg/mod
```

### Generating new stubs

Flip status in aliasfix for gapics being migrated to in progress.

```shell
cd /path/to/internal/gapicgen
go run cloud.google.com/go/internal/gapicgen/cmd/genbot \
   -local \
   -only-gapics \
   -gocloud-dir=/path/to/google-cloud-go \
   -gapic=cloud.google.com/go/foo/apiv1
```

## FAQ

### How to bump to a later version of the microgenerator

```shell
cd /path/to/internal/gapicgen
go get -d github.com/googleapis/gapic-generator-go/cmd/protoc-gen-go_gapic
```

(it's just based on the go.mod entry)
