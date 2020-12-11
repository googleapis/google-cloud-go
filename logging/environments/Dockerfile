# This Dockerfile is moved to repo root dir upon build
# Use the offical golang image to create a binary.
# This is based on Debian and sets the GOPATH to /go.
# https://hub.docker.com/_/golang
FROM golang:1.15-buster as builder

# Create and change to the app directory.
WORKDIR /app

# Copy google-cloud-go/* into app/
COPY . ./
# cd into directory containing e2e tests
WORKDIR /app/logging/environments

# Build the binary
RUN go mod download
RUN go build -v -o server

# Use the official Debian slim image for a lean production container.
# REQUIRED as Cloud Run is capped at 256M memory limit
FROM debian:buster-slim
RUN set -x && apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y \
    ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Copy the binary to the production image from the builder stage.
COPY --from=builder /app/logging/environments/server /app/server

# Run the web service on container startup.
CMD ["/app/server"]