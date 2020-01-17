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

This tool relies on API authentication for both github (which hosts the `google.golang.org/genproto/...` libraries) and the
Gerrit code review instance `code-review.googlesource.com` which is where the `cloud.google.com/go/...` libraries are maintained.

You need to supply credentials for both systems to successfully manage the generation of gapic libraries.

### Github

For Github, you need to generate/supply a Personal Access Token.  More information on how that's done is here:
https://help.github.com/en/github/authenticating-to-github/creating-a-personal-access-token-for-the-command-line

### Gerrit

For Gerrit, you need a gerrit authentication cookie.  The authentication flow in gerrit to fetch this information begins here:
https://code-review.googlesource.com/settings/#HTTPCredentials

At the end, it provides a shell script for injecting the cookies into your .gitcookies file.  There's typically two cookies in the provided shell script, one for code.googlesource.com and one for code-review.googlesource.com (though there is an option to generate a single cookie for the entire domain as well).  You want the cookie correspnding to code-review.googlesource.com.

The relevant portion which needs to be passed to this tool looks similar to this (you need both the identity and the secret):
**git-yourlogin.yourdomain.com=1//abcdef01010209202-cbdef010102-skdjskljsdkjsksjd**

## Running locally

Note: this may change your `~/.gitconfig` and `~/.gitcookies`.

```
cd /path/to/internal/gapicgen
go run cloud.google.com/go/internal/gapicgen/cmd/genmgr \
    --githubAccessToken=11223344556677889900aabbccddeeff11223344 \
    --githubName="Jean de Klerk" \
    --githubEmail=deklerk@google.com \
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
    -e "GERRIT_COOKIE_VALUE=<cookie>" \
    genmgr
```
