# Go GAPIC Generator (`librariangen`)

This directory contains the source code for `librariangen`, a containerized Go application that serves as the Go-specific code generator within the Librarian pipeline. Its responsibility is to generate release-ready Go GAPIC client libraries from `googleapis` API definitions, replacing the legacy `bazel-bot` and `OwlBot` toolchain.

## How it Works (The Container Contract)

The `librariangen` binary is designed to be run inside a Docker container orchestrated by the central Librarian tool. It adheres to a specific "container contract" by accepting commands and expecting a set of mounted directories for its inputs and outputs.

The primary command is `generate`.

### `generate` Command Workflow

1.  **Inputs:** The container is provided with several mounted directories:
    *   `/source`: A complete checkout of the `googleapis` repository. This is the primary include path (`-I`) for `protoc`.
    *   `/librarian`: Contains a `generate-request.json` file, which specifies the library and the specific API protos to be generated.
    *   `/output`: An empty directory where all generated Go files will be written.
    *   `/input`: A directory for future use (e.g., templates, scripts).

2.  **Execution (`gapicgen`):**
    *   The `librariangen` binary parses the `generate-request.json`.
    *   For each API specified in the request, it locates the corresponding `BUILD.bazel` file within the `/source` directory.
    *   It parses this `BUILD.bazel` file to extract the necessary options for the `protoc` command (e.g., import paths, transport, service config paths).
    *   It constructs and executes a `protoc` command, invoking the `protoc-gen-go` and `protoc-gen-go_gapic` plugins.

3.  **Post-processing:** After generation, the `postprocessor` component runs to make the code a complete, release-ready Go module. This includes formatting, linting, and generating module files like `go.mod` and `README.md`.

4.  **Output:** All generated files (`*.pb.go`, `*_gapic.go`, etc.) are written to the `/output` directory. The Librarian tool is then responsible for copying these files to their final destination in the `google-cloud-go` repository.

## Development & Testing

### Local Development Dependencies

To build and test `librariangen` locally, you must have the following tools installed and available in your `PATH`:

*   **Go Toolchain:** (Version `1.23.0` is used in the container)
*   **`protoc`:** (Version `25.7` is used in the container)
*   **`protoc-gen-go`:** `go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.35.2`
*   **`protoc-gen-go-grpc`:** `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.3.0`
*   **`protoc-gen-go_gapic`:** `go install github.com/googleapis/gapic-generator-go/cmd/protoc-gen-go_gapic@v0.53.1`
*   **`goimports`:** `go install golang.org/x/tools/cmd/goimports@latest`
*   **`staticcheck`:** `go install honnef.co/go/tools/cmd/staticcheck@2023.1.6`

### Building the Binary

To compile the binary locally:
```bash
go build .
```

### Running Tests

The project has a multi-layered testing strategy.

1.  **Unit Tests:** Each Go package has its own unit tests. To run all of them:
    ```bash
    go test ./...
    ```

2.  **Binary Integration Test:** A shell script (`run-binary-integration-test.sh`) provides a full, end-to-end test of the compiled binary. This is the primary way to validate changes to the core generation logic.
    *   **Setup:** The test requires local checkouts of the `googleapis` and `googleapis-gen` repositories. You must set the `LIBRARIANGEN_GOOGLEAPIS_DIR` and `LIBRARIANGEN_GOOGLEAPIS_GEN_DIR` environment variables to point to these checkouts.
    *   **Execution:**
        ```bash
        bash run-binary-integration-test.sh
        ```
    This script will compile the binary and run it against realistic API fixtures, verifying that the correct Go files are generated and comparing them against the golden files.

## Docker Container

The `Dockerfile` packages the `librariangen` binary and all its dependencies into a MOSS-compliant container for use in the Librarian pipeline.

### Building the Container

1.  **Authenticate with Google Artifact Registry:** The official build process requires pulling a base image from `marketplace.gcr.io`. You must authenticate your Docker client with Google Cloud before building.
    ```bash
    gcloud auth configure-docker
    ```
    **Note:** Access to this base image may require special IAM permissions. If you encounter an `unauthorized` error, you can temporarily switch the `Dockerfile` to use a public base image (`debian:12-slim`) to unblock local development. See the `TODO` comments in the `Dockerfile` for details.

2.  **Build and Test:** The `build-docker-and-test.sh` script builds the Docker image and then runs a verification container to ensure all dependencies are correctly installed.
    ```bash
    bash build-docker-and-test.sh
    ```

### Container Dependencies

The container environment is built with the following pinned tool versions to ensure backward compatibility:

*   **Base Image:** `marketplace.gcr.io/google/debian12:latest`
*   **Go:** `1.23.0`
*   **protoc:** `25.7`
*   **protoc-gen-go:** `v1.35.2`
*   **protoc-gen-go-grpc:** `v1.3.0`
*   **protoc-gen-go_gapic:** `v0.53.1`
*   **staticcheck:** `2023.1.6`

## Future Work

This implementation currently focuses only on the core `generate` command. Future work includes:

*   **`configure` and `build` commands:** Implementing the remaining commands from the Librarian container contract.
*   **Finalizing the `newModule` logic:** The post-processor's `newModule` parameter is currently hardcoded to `false`. This needs to be driven by configuration from the orchestrating Librarian tool.
*   **Bazel Fallback:** Implementing a contingency plan to invoke Bazel directly for APIs with highly complex or unusual configurations.