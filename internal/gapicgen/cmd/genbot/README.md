# genbot

genbot is a binary for generating gapics and creating CLs/PRs with the results.
It is intended to be used as a bot, though it can be run locally too.

## Getting certs

1. Grab github personal access token (see: https://help.github.com/en/github/authenticating-to-github/creating-a-personal-access-token-for-the-command-line)
1. Grab a gerrit HTTP auth cookie at https://code-review.googlesource.com/settings/#HTTPCredentials > Obtain password > `git-your@email.com=SomeHash....`

## Running locally

Note: this may change your ~/.gitconfig, ~/.gitcookies, and use up non-trivial
amounts of space on your computer.

1. Make sure you have all the tools installed listed in genlocal's README.md
1. Create a fork of genproto for whichever github user you're going to be
  running as: https://github.com/googleapis/go-genproto/
1. Run:

```
cd /path/to/internal/gapicgen
go run cloud.google.com/go/internal/gapicgen/genbot \
    --accessToken=11223344556677889900aabbccddeeff11223344 \
    --githubUsername=jadekler \
    --githubName="Jean de Klerk" \
    --githubEmail=deklerk@google.com \
    --githubSSHKeyPath=/path/to/.ssh/github_rsa \
    --gerritCookieName=o \
    --gerritCookieValue=<cookie>
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
    -v /path/to/your/ssh/key/directory:/.ssh \
    -e "ACCESS_TOKEN=11223344556677889900aabbccddeeff11223344" \
    -e "GITHUB_USERNAME=jadekler" \
    -e "GITHUB_NAME=\"Jean de Klerk\"" \
    -e "GITHUB_EMAIL=deklerk@google.com" \
    -e "GITHUB_SSH_KEY_PATH=/.ssh/name_of_your_github_rsa_file" \
    -e "GERRIT_COOKIE_NAME=o" \
    -e "GERRIT_COOKIE_VALUE=<cookie>" \
    genbot
```
