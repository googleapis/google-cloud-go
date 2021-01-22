# genbot

genbot is a binary for generating gapics and creating PRs with the results.
It is intended to be used as a bot, though it can be run locally too.

## Getting certs

### Github

For Github, you need to generate/supply a Personal Access Token.  More
information on how that's done can be found here:
[creating a personal access token](https://help.github.com/en/github/authenticating-to-github/creating-a-personal-access-token-for-the-command-line).

## Running locally

Note: this may change your `~/.gitconfig`, `~/.gitcookies`, and use up
non-trivial amounts of space on your computer.

1. Make sure you are on a non-Windows platform. If you are using windows
   continue on to the docker instructions.
2. Make sure you have all the tools installed listed in genlocal's README.md
3. Run:

```shell
cd /path/to/internal/gapicgen
go run cloud.google.com/go/internal/gapicgen/cmd/genbot \
    --githubAccessToken=$GITHUB_ACCESS_TOKEN \
    --githubUsername=$GITHUB_USERNAME \
    --githubName="Jean de Klerk" \
    --githubEmail=deklerk@google.com \
```

## Run with docker

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

## FAQ

### How to bump to a later version of the microgenerator

```shell
cd /path/to/internal/gapicgen
go get -u github.com/googleapis/gapic-generator-go/cmd/protoc-gen-go_gapic
```

(it's just based on the go.mod entry)
