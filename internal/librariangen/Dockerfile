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
FROM marketplace.gcr.io/google/debian12:latest AS builder

# Set environment variables for tool versions for easy updates.
ENV GO_VERSION=1.24.0

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
ENV PATH=/usr/local/go/bin:$PATH
RUN go version

WORKDIR /src

# Copy go.mod and go.sum to download dependencies before copying all the source.
# This allows Docker to cache the dependencies layer.
COPY go.mod go.sum ./
RUN go mod download

# Copy all source code.
COPY . .

# Build the librariangen binary.
RUN CGO_ENABLED=0 GOOS=linux go build -v -o /librariangen .


# --- Final Stage ---
# This stage creates the final, minimal image with the compiled binary and
# all required runtime dependencies pinned to specific versions for compatibility.
FROM marketplace.gcr.io/google/debian12:latest

# Set environment variables for tool versions for easy updates.
ENV GO_VERSION=1.24.0
ENV PROTOC_VERSION=25.7
ENV GO_PROTOC_PLUGIN_V1_VERSION=1.5.4
ENV GO_PROTOC_PLUGIN_V2_VERSION=1.35.2
ENV GO_GRPC_PLUGIN_VERSION=1.3.0
ENV GAPIC_GENERATOR_VERSION=0.53.1
ENV STATICCHECK_VERSION=2023.1.6

# Set up a home directory for the non-root user that will be running this container.
# Go tools will use this directory for caches, which resolves permission issues.
ENV HOME=/home/nonroot
# GOPATH defaults to $HOME/go, so we need to add that to the PATH.
ENV PATH=$HOME/go/bin:/usr/local/go/bin:/usr/local/bin:$PATH

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

# Note: This is needed for the go install commands below.
RUN wget https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz -O go.tar.gz && \
    tar -C /usr/local -xzf go.tar.gz && \
    rm go.tar.gz

# Install the specific protoc (Protobuf Compiler) version required for compatibility.
RUN wget https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-x86_64.zip -O protoc.zip && \
    unzip protoc.zip -d /usr/local && \
    rm protoc.zip && \
    protoc --version

# Build the "old" Go protoc plugin manually; using go install just redirects to the new one.
# Note that the "old" plugin uses the internal code in the "new" plugin, so we need to use
# "go mod edit" to end up with a consistent version.
# The plugin is built as protoc-gen-go_v1, so that we can use "go_v1" as the name in protoc options.
# This is ghastly, and should all be removed as soon as possible by migrating all packages to go_grpc.
RUN (export GOTOOLCHAIN='auto' && \
    git clone --depth 1 --branch=v${GO_PROTOC_PLUGIN_V1_VERSION} https://github.com/golang/protobuf protoc-gen-go-v1 && \
    cd protoc-gen-go-v1 && \
    go mod edit -require google.golang.org/protobuf@v${GO_PROTOC_PLUGIN_V2_VERSION} && \
    go mod tidy && \
    go build -o $HOME/go/bin/protoc-gen-go_v1 ./protoc-gen-go)

# Install required Go tools for protoc and the post-processor.
RUN (export GOTOOLCHAIN='auto' && \
    go install google.golang.org/protobuf/cmd/protoc-gen-go@v${GO_PROTOC_PLUGIN_V2_VERSION} && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v${GO_GRPC_PLUGIN_VERSION} && \
    go install github.com/googleapis/gapic-generator-go/cmd/protoc-gen-go_gapic@v${GAPIC_GENERATOR_VERSION} && \
    go install golang.org/x/tools/cmd/goimports@latest && \
    go install honnef.co/go/tools/cmd/staticcheck@${STATICCHECK_VERSION})

# Create and set permissions for the home directory so it is writable by any user.
RUN mkdir -p ${HOME} && chmod -R 777 ${HOME}

# Copy the compiled librariangen binary from the builder stage.
COPY --from=builder /librariangen /usr/local/bin/librariangen

# Set the entrypoint for the container to run the compiled librariangen.
# The Librarian will provide commands like 'generate' as arguments.
ENTRYPOINT [ "/usr/local/bin/librariangen" ]
