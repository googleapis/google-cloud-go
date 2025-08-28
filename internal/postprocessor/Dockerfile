# Copyright 2022 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM marketplace.gcr.io/google/debian12:latest

# Set environment variables for the Go version and installation
ENV GO_VERSION 1.24.0
ENV PATH /usr/local/go/bin:/root/go/bin:$PATH

RUN apt-get update && \
    apt-get install -y wget ca-certificates git && \
    rm -rf /var/lib/apt/lists/*

RUN wget https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz -O go.tar.gz && \
    tar -C /usr/local -xzf go.tar.gz && \
    rm go.tar.gz

RUN go version

# Copy local code to the container image.
COPY . /postprocessor/
WORKDIR /postprocessor

# go build with no path looks in current directory (/postprocessor) and finds package main
RUN CGO_ENABLED=0 GOOS=linux go build -v -o post_processor

# Install tools used in build
RUN (export GOTOOLCHAIN='auto' && \
    go install honnef.co/go/tools/cmd/staticcheck@latest && \
    go install github.com/jstemmer/go-junit-report@latest && \
    go install golang.org/x/lint/golint@latest && \
    go install golang.org/x/tools/cmd/goimports@latest)

CMD /postprocessor/post_processor
