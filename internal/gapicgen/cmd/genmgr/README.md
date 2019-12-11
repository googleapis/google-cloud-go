# genmgr

genmgr is a binary used to apply reviewers and update go.mod in a gocloud regen
CL once the corresponding genproto PR is submitted.

## Required tools

1. Install [Go](http://golang.org/dl)
1. Install Go tools:

    ```
    go get \
        golang.org/x/lint/golint \
        golang.org/x/tools/cmd/goimports \
        honnef.co/go/tools/cmd/staticcheck \
        golang.org/x/review/git-codereview
    ```

## Getting certs

1. Grab github personal access token (see: https://help.github.com/en/github/authenticating-to-github/creating-a-personal-access-token-for-the-command-line)
2. Grab a gerrit HTTP auth cookie at https://code-review.googlesource.com/settings/#HTTPCredentials > Obtain password > `git-your@email.com=SomeHash....`

## Running locally

Note: this may change your `~/.gitconfig` and `~/.gitcookies`.

```
cd /path/to/internal/gapicgen
go run cloud.google.com/go/internal/gapicgen/cmd/genmgr \
    --githubAccessToken=11223344556677889900aabbccddeeff11223344 \
    --githubName="Jean de Klerk" \
    --githubEmail=deklerk@google.com \
    --gerritCookieName=o \
    --gerritCookieValue=<cookie>
```

## Running with docker

Note: this may leave a lot of docker resources laying around. Use
`docker system prune` to clean up after runs.

```
cd /path/to/internal/gapicgen/cmd/genmgr
docker build . -t genmgr
docker run -t --rm --privileged \
    -v `pwd`/../..:/gapicgen \
    -e "GITHUB_ACCESS_TOKEN=11223344556677889900aabbccddeeff11223344" \
    -e "GITHUB_NAME=\"Jean de Klerk\"" \
    -e "GITHUB_EMAIL=deklerk@google.com" \
    -e "GERRIT_COOKIE_NAME=o" \
    -e "GERRIT_COOKIE_VALUE=<cookie>" \
    genmgr
```
