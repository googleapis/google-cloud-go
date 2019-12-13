# Build protoc. Source: https://gist.github.com/rizo/513849f35178d19a13adcddf2d045a19.
FROM golang:1.8.3-alpine3.6 as protoc-builder

# System setup
RUN apk update && apk add git curl build-base autoconf automake libtool

# Install protoc
ENV PROTOBUF_URL https://github.com/google/protobuf/releases/download/v3.6.1/protobuf-cpp-3.6.1.tar.gz
RUN curl -L -o /tmp/protobuf.tar.gz $PROTOBUF_URL
WORKDIR /tmp/
RUN tar xvzf protobuf.tar.gz
WORKDIR /tmp/protobuf-3.6.1
RUN mkdir /export
RUN ./autogen.sh && \
    ./configure --prefix=/export && \
    make -j 3 && \
    make check && \
    make install

# Export dependencies
RUN cp /usr/lib/libstdc++* /export/lib/
RUN cp /usr/lib/libgcc_s* /export/lib/

FROM docker:stable-dind

RUN apk update && \
    apk add ca-certificates wget git unzip

# Install Go.
RUN cd $(mktemp -d) && \
    wget https://dl.google.com/go/go1.13.4.linux-amd64.tar.gz && \
    tar xvzf go1.13.4.linux-amd64.tar.gz && \
    mv go /go
ENV PATH="${PATH}:/go/bin"
# Source: https://stackoverflow.com/questions/34729748/installed-go-binary-not-found-in-path-on-alpine-linux-docker
RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2
RUN go version

# Install python and pip.
RUN apk add python python3 py-pip py3-pip py-virtualenv py3-virtualenv python2-dev python3-dev libffi-dev
RUN python --version
RUN python3 --version
RUN pip --version
RUN pip3 --version
RUN virtualenv --version

# Install protoc.
COPY --from=protoc-builder /export /usr
RUN protoc --version

# Install Go tools.
RUN go get \
  github.com/golang/protobuf/protoc-gen-go \
  golang.org/x/lint/golint \
  golang.org/x/tools/cmd/goimports \
  honnef.co/go/tools/cmd/staticcheck \
  github.com/googleapis/gapic-generator-go/cmd/protoc-gen-go_gapic \
  golang.org/x/review/git-codereview
ENV PATH="${PATH}:/root/go/bin"

# Install bash and ssh tools (needed to run regen.sh etc).
RUN apk add bash openssh openssh-client
# Source: http://debuggable.com/posts/disable-strict-host-checking-for-git-clone:49896ff3-0ac0-4263-9703-1eae4834cda3
RUN mkdir /root/.ssh && echo -e "Host github.com\n\tStrictHostKeyChecking no\n" >> /root/.ssh/config
RUN which bash
# Install tools necessary for artman.
RUN apk add alpine-sdk build-base openssl-dev
RUN which gcc

RUN echo -e '#!/bin/bash\n\
set -ex\n\
echo "https://$GITHUB_USERNAME:$GITHUB_ACCESS_TOKEN@github.com" > ~/.git-credentials \n\
dockerd-entrypoint.sh &\n\
unset DOCKER_HOST\n\
go mod download & # download deps whilst we wait for dockerd to start \n\
sleep 15 # wait for docker to start\n\
docker ps\n\
docker pull googleapis/artman:latest & # download latest artman whilst we wait for genbot to start \n\
go run cloud.google.com/go/internal/gapicgen/cmd/genbot \
    --githubAccessToken=$GITHUB_ACCESS_TOKEN \
    --githubUsername=$GITHUB_USERNAME \
    --githubName="$GITHUB_NAME" \
    --githubEmail=$GITHUB_EMAIL \
    --gerritCookieName=$GERRIT_COOKIE_NAME \
    --gerritCookieValue=$GERRIT_COOKIE_VALUE \n\
' >> /run.sh
RUN chmod a+x /run.sh

WORKDIR /gapicgen
CMD ["bash", "-c", "/run.sh"]