# Copyright 2025 Google LLC
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

# This Dockerfile creates a MOSS-compliant image for the Go librariangen,
# which is invoked by the Librarian tool. It uses a multi-stage build to
# create a minimal final image.

# --- Builder Stage ---
# This stage builds the librariangen binary using the MOSS-compliant base image.
# TODO(quartzmo): For production builds, this must be switched back to the MOSS-compliant
# base image.
# See https://github.com/googleapis/librarian/issues/1019.
# FROM marketplace.gcr.io/google/debian12:latest as builder
FROM debian:12-slim as builder

# Set environment variables for tool versions for easy updates.
ENV GO_VERSION=1.23.0

# Install build dependencies.
RUN apt-get update && \
    apt-get install -y \
    build-essential \
    ca-certificates \
    curl \
    wget && \
    rm -rf /var/lib/apt/lists/*

# Install the specific Go version required for compatibility.
RUN wget https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz -O go.tar.gz && \
    tar -C /usr/local -xzf go.tar.gz && \
    rm go.tar.gz
ENV PATH /usr/local/go/bin:$PATH

WORKDIR /src

# Copy all source code.
COPY . .

# Build the librariangen binary.
RUN CGO_ENABLED=0 go build -o /librariangen .


# --- Final Stage ---
# This stage creates the final, minimal image with the compiled binary and
# all required runtime dependencies pinned to specific versions for compatibility.
# TODO(quartzmo): For production builds, this must be switched back to the MOSS-compliant
# base image.
# See https://github.com/googleapis/librarian/issues/1019.
# FROM marketplace.gcr.io/google/debian12:latest
FROM debian:12-slim

# Set environment variables for tool versions for easy updates.
ENV GO_VERSION=1.23.0
ENV PROTOC_VERSION=25.7
ENV GO_PROTOC_PLUGIN_VERSION=1.35.2
ENV GO_GRPC_PLUGIN_VERSION=1.3.0
ENV GAPIC_GENERATOR_VERSION=0.53.1
ENV STATICCHECK_VERSION=2023.1.6

# Set up PATH for Go and other tools.
ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/local/go/bin:/usr/local/bin:$PATH

# Install essential system packages.
# Clean up apt cache to keep the image smaller.
RUN apt-get update && \
    apt-get install -y \
    ca-certificates \
    curl \
    git \
    unzip \
    wget && \
    rm -rf /var/lib/apt/lists/*

# Install the specific Go version required for compatibility.
RUN wget https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz -O go.tar.gz && \
    tar -C /usr/local -xzf go.tar.gz && \
    rm go.tar.gz

# Install the specific protoc (Protobuf Compiler) version required for compatibility.
RUN wget https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-x86_64.zip -O protoc.zip && \
    unzip protoc.zip -d /usr/local && \
    rm protoc.zip && \
    protoc --version

# Install required Go tools for protoc and the post-processor.
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v${GO_PROTOC_PLUGIN_VERSION} && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v${GO_GRPC_PLUGIN_VERSION} && \
    go install github.com/googleapis/gapic-generator-go/cmd/protoc-gen-go_gapic@v${GAPIC_GENERATOR_VERSION} && \
    go install golang.org/x/tools/cmd/goimports@latest && \
    go install honnef.co/go/tools/cmd/staticcheck@${STATICCHECK_VERSION}

# Copy the compiled librariangen binary from the builder stage.
COPY --from=builder /librariangen /usr/local/bin/librariangen

# Set the entrypoint for the container to run the compiled librariangen.
# The Librarian will provide commands like 'generate' as arguments.
ENTRYPOINT [ "/usr/local/bin/librariangen" ]
