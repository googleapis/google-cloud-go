FROM docker:stable-dind

ENV GO111MODULE on

RUN apk update && \
    apk add ca-certificates wget git unzip
# Install bash and ssh tools (needed to run regen.sh etc).
RUN apk add bash openssh openssh-client
RUN which bash

# Install libc compatibility (required by protoc and go)
RUN apk add libc6-compat

# Use edge for protoc 3.12.x
# Remove when main upgrades from 3.11.x
RUN sed -i -e 's/v[[:digit:]]\..*\//edge\//g' /etc/apk/repositories
RUN apk update

# Install protoc and protobuf-dev (common protos).
RUN apk add protoc protobuf-dev
RUN protoc --version

# Install Go.
RUN wget -O /tmp/go.tgz https://dl.google.com/go/go1.15.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf /tmp/go.tgz && \
    rm /tmp/go.tgz
ENV PATH /usr/local/go/bin:$PATH
RUN go version

# Install Go tools.
RUN GO111MODULE=on go get \
    github.com/golang/protobuf/protoc-gen-go@v1.4.3 \
    golang.org/x/lint/golint@latest \
    golang.org/x/tools/cmd/goimports@latest \
    honnef.co/go/tools/cmd/staticcheck@latest \
    github.com/googleapis/gapic-generator-go/cmd/protoc-gen-go_gapic@v0.17.0
ENV PATH="${PATH}:/root/go/bin"

# Source: http://debuggable.com/posts/disable-strict-host-checking-for-git-clone:49896ff3-0ac0-4263-9703-1eae4834cda3
RUN mkdir /root/.ssh && echo -e "Host github.com\n\tStrictHostKeyChecking no\n" >> /root/.ssh/config

RUN echo -e '#!/bin/bash\n\
    set -ex\n\
    go mod download \n\
    go run cloud.google.com/go/internal/gapicgen/cmd/genbot \
    ' >> /run.sh
RUN chmod a+x /run.sh

WORKDIR /gapicgen
CMD ["bash", "-c", "/run.sh"]
