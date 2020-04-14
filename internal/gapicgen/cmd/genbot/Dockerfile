FROM docker:stable-dind

ENV GO111MODULE on

RUN apk update && \
    apk add ca-certificates wget git unzip
# Install bash and ssh tools (needed to run regen.sh etc).
RUN apk add bash openssh openssh-client
RUN which bash
# Install protoc and protobuf-dev (common protos).
RUN apk add protoc protobuf-dev
RUN protoc --version
# Install libc compatibility (required by protoc and go)
RUN apk add libc6-compat

# Install Go.
RUN wget -O /tmp/go.tgz https://dl.google.com/go/go1.13.4.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf /tmp/go.tgz && \
    rm /tmp/go.tgz
ENV PATH /usr/local/go/bin:$PATH
RUN go version

# Install Go tools.
RUN GO111MODULE=on go get \
  github.com/golang/protobuf/protoc-gen-go@v1.3.5 \
  golang.org/x/lint/golint@latest \
  golang.org/x/tools/cmd/goimports@latest \
  honnef.co/go/tools/cmd/staticcheck@latest \
  github.com/googleapis/gapic-generator-go/cmd/protoc-gen-go_gapic@v0.13.3 \
  golang.org/x/review/git-codereview@latest
ENV PATH="${PATH}:/root/go/bin"

# Source: http://debuggable.com/posts/disable-strict-host-checking-for-git-clone:49896ff3-0ac0-4263-9703-1eae4834cda3
RUN mkdir /root/.ssh && echo -e "Host github.com\n\tStrictHostKeyChecking no\n" >> /root/.ssh/config

RUN echo -e '#!/bin/bash\n\
set -ex\n\
echo "https://$GITHUB_USERNAME:$GITHUB_ACCESS_TOKEN@github.com" > ~/.git-credentials \n\
go mod download \n\
go run cloud.google.com/go/internal/gapicgen/cmd/genbot \
    --githubAccessToken=$GITHUB_ACCESS_TOKEN \
    --githubUsername=$GITHUB_USERNAME \
    --githubName="$GITHUB_NAME" \
    --githubEmail=$GITHUB_EMAIL \
    --gerritCookieValue=$GERRIT_COOKIE_VALUE \n\
' >> /run.sh
RUN chmod a+x /run.sh

WORKDIR /gapicgen
CMD ["bash", "-c", "/run.sh"]
