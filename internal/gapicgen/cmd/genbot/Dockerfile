FROM golang:1.22-alpine as godist
RUN go version

FROM docker:25.0

# Log the version of Alpine in use.
RUN cat /etc/alpine-release

ENV GO111MODULE on

RUN apk update && \
    apk add ca-certificates wget git unzip
# Install bash and ssh tools (needed to run regen.sh etc).
RUN apk add bash openssh-client build-base
RUN which bash

# Install libc compatibility (required by protoc and go)
RUN apk add libc6-compat

# Install protoc and protobuf-dev (common protos).
RUN apk add protoc protobuf-dev
RUN protoc --version

# Copy pre-built alpine Go
COPY --from=godist /usr/local/go /usr/local/go
ENV PATH /usr/local/go/bin:$PATH
RUN go version

# Install Go tools.
RUN go install github.com/golang/protobuf/protoc-gen-go@v1.5.3 && \
    go install golang.org/x/lint/golint@latest && \
    go install golang.org/x/tools/cmd/goimports@latest && \
    go install honnef.co/go/tools/cmd/staticcheck@latest && \
    go install github.com/googleapis/gapic-generator-go/cmd/protoc-gen-go_gapic@v0.38.0
ENV PATH="${PATH}:/root/go/bin"

# Source: http://debuggable.com/posts/disable-strict-host-checking-for-git-clone:49896ff3-0ac0-4263-9703-1eae4834cda3
RUN mkdir /root/.ssh && echo -e "Host github.com\n\tStrictHostKeyChecking no\n" >> /root/.ssh/config

RUN echo -e '#!/bin/bash\n\
    set -ex\n\
    cd internal/gapicgen\n\
    go run cloud.google.com/go/internal/gapicgen/cmd/genbot' >> /run.sh
RUN chmod a+x /run.sh

WORKDIR /google-cloud-go
CMD ["bash", "-c", "/run.sh"]
