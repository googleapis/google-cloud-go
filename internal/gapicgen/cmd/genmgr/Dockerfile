FROM golang:1.13.4-alpine

RUN apk update && \
    apk add ca-certificates wget git unzip bash

RUN go version

# Install Go tools.
RUN GO111MODULE=on go get \
  golang.org/x/lint/golint@latest \
  golang.org/x/tools/cmd/goimports@latest \
  honnef.co/go/tools/cmd/staticcheck@latest \
  golang.org/x/review/git-codereview@latest
ENV PATH="${PATH}:/root/go/bin"

RUN echo -e '#!/bin/bash\n\
set -ex\n\
go run cloud.google.com/go/internal/gapicgen/cmd/genmgr \
    --githubAccessToken=$GITHUB_ACCESS_TOKEN \
    --githubName="$GITHUB_NAME" \
    --githubEmail=$GITHUB_EMAIL \
    --gerritCookieValue=$GERRIT_COOKIE_VALUE \n\
' >> /run.sh
RUN chmod a+x /run.sh

WORKDIR /gapicgen
CMD ["bash", "-c", "/run.sh"]
