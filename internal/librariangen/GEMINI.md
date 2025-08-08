# Gemini Code Assistant Context

This document provides context for the Gemini code assistant to understand the `librariangen` project.

## Project Overview

`librariangen` is a Go command-line application responsible for generating Go GAPIC (Google API Client) libraries. It is designed to be run within a Docker container as part of a larger "Librarian" pipeline.

The tool takes a checkout of the `googleapis` repository and a `generate-request.json` file as input. It then invokes the `protoc` compiler with the `protoc-gen-go` and `protoc-gen-go_gapic` plugins to generate the Go client library. After generation, a post-processing step is applied to create a release-ready Go module.

## Building and Running

### Building the Binary

To compile the `librariangen` binary from source, run:
```bash
go build .
```

### Running Tests

There are three primary ways to test the project:

1.  **Unit Tests:** To run the Go unit tests for all packages:
    ```bash
    go test ./...
    ```

2.  **Binary Integration Test:** This is a comprehensive end-to-end test of the compiled binary. It requires local checkouts of the `googleapis` and `googleapis-gen` (or `google-cloud-go`) repositories.
    ```bash
    # Set these environment variables to point to your local checkouts
    export LIBRARIANGEN_GOOGLEAPIS_DIR=/path/to/googleapis
    export LIBRARIANGEN_GOOGLEAPIS_GEN_DIR=/path/to/googleapis-gen

    # Run the test script
    ./run-binary-integration-test.sh
    ```

3.  **Docker Container Test:** This script builds the Docker image and verifies that all dependencies are correctly installed within the container.
    ```bash
    ./build-docker-and-test.sh
    ```

## Development Conventions

*   The application logic is primarily written in Go.
*   The main entrypoint is in `main.go`, which handles command-line argument parsing and invokes the appropriate sub-commands.
*   The core generation logic resides in the `generate` package.
*   Shell scripts (`.sh`) are used for automation, particularly for testing and Docker image management.
*   The project follows standard Go conventions for code style and project structure.
*   Dependencies are managed with Go modules (`go.mod`). The runtime dependencies for code generation (`protoc`, plugins) are expected to be present in the execution environment (i.e., the Docker container).
*   After any dependency change (`go get` or manual edit to `go.mod`), always run `go mod tidy` to ensure the `go.sum` file is updated and unused dependencies are removed.
