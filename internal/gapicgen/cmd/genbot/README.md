# genbot

genbot is a binary for generating gapics and creating PRs with the results.
It is intended to be used as a bot, though it can be run locally too.

## Getting certs

### Github

For Github, you need to generate/supply a Personal Access Token.  More information on how that's done is here:
https://help.github.com/en/github/authenticating-to-github/creating-a-personal-access-token-for-the-command-line

## Running locally

Note: this may change your `~/.gitconfig`, `~/.gitcookies`, and use up
non-trivial amounts of space on your computer.

1. Make sure you have all the tools installed listed in genlocal's README.md
2. Run:

```
export GITHUB_USERNAME=jadekler
export GITHUB_ACCESS_TOKEN=11223344556677889900aabbccddeeff11223344
echo "https://$GITHUB_USERNAME:$GITHUB_ACCESS_TOKEN@github.com" > ~/.git-credentials
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

```
cd /path/to/internal/gapicgen/cmd/genbot
docker build . -t genbot
docker run -t --rm --privileged \
    -v `pwd`/../..:/gapicgen \
    -e "GITHUB_ACCESS_TOKEN=11223344556677889900aabbccddeeff11223344" \
    -e "GITHUB_USERNAME=jadekler" \
    -e "GITHUB_NAME=\"Jean de Klerk\"" \
    -e "GITHUB_EMAIL=deklerk@google.com" \
    genbot
```

## FAQ

#### How do I bump to a later version of the microgenerator?

```
cd /path/to/internal/gapicgen
go get -u github.com/googleapis/gapic-generator-go/cmd/protoc-gen-go_gapic
```

(it's just based on the go.mod entry)
