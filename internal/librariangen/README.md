# Go GAPIC Generator (`librariangen`)

This directory contains the source code for `librariangen`, a containerized Go application that serves as the Go-specific code generator within the Librarian pipeline. Its responsibility is to generate release-ready Go GAPIC client libraries from `googleapis` API definitions, replacing the legacy `bazel-bot` and `OwlBot` toolchain.

## How it Works (The Container Contract)

The `librariangen` binary is designed to be run inside a Docker container orchestrated by the central Librarian tool. It adheres to a specific "container contract" by accepting commands and expecting a set of mounted directories for its inputs and outputs.

The primary command is `generate`.

### Usage

The `librariangen` binary expects the command to be the first argument, followed by flags.

**Example `generate` command:**
`bash
librariangen generate \
    --source /source \
    --librarian /librarian \
    --output /output
`

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

## Running

There are three primary ways to run the generator, with varying levels of setup complexity.

### EASY: Run Librarian with the prebuilt container

This is the standard way to run the generator, using a pre-built image from Google's Artifact Registry.

1.  **Prerequisites:**
    *   You must have `docker` and `git` installed.
    *   Set the `LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR` environment variable to the absolute path of your `google-cloud-go` repository checkout.
    *   Set the `LIBRARIANGEN_GOOGLEAPIS_DIR` environment variable to the absolute path of your `googleapis` repository checkout.

2.  **Execute:**
    Run the `run-librarian-integration-test.sh` script, which invokes the `librarian` CLI to pull the image and run the generator.
    ```bash
    ./run-librarian-integration-test.sh
    ```
    The script uses this public image: `gcr.io/cloud-devrel-public-resources/librarian-go:infrastructure-public-image-latest`

### MEDIUM: Build the Dockerfile yourself, and run Librarian with your image

If you have made local changes to `librariangen` and want to test them in a containerized environment, you can build the Docker image locally.

1.  **Prerequisites:**
    *   Same as the "EASY" method.
    *   You may need to authenticate with Google Artifact Registry to pull the base image: `gcloud auth configure-docker`.

2.  **Build the image:**
    The `build-docker-and-test.sh` script will build the image and tag it as `gcr.io/cloud-devrel-public-resources/librarian-go:infrastructure-public-image-latest`.
    ```bash
    ./build-docker-and-test.sh
    ```

3.  **Execute:**
    Once the image is built and tagged, run the same script as the "EASY" method. The `librarian` tool will find and use your local image instead of pulling a remote one.
    ```bash
    ./run-librarian-integration-test.sh
    ```

### SUPER EASY: Invoke the librariangen binary as a CLI

This method runs the generator directly as a Go binary, without any Docker containerization. It is the fastest way to iterate on the generator's code but requires all `protoc` plugins and dependencies to be installed on your local machine.

1.  **Prerequisites:**
    *   Install all tools listed in the **Local Development Dependencies** section.
    *   Create three directories for I/O, for example:
        ```bash
        mkdir -p /tmp/librariangen-test/source /tmp/librariangen-test/librarian /tmp/librariangen-test/output
        ```
    *   Copy your `googleapis` checkout into the `source` directory.
    *   Create a `generate-request.json` file inside the `librarian` directory (see `testdata/librarian/generate-request.json` for an example).

2.  **Execute:**
    Run the `main.go` program with the `generate` command and flags pointing to your I/O directories.
    ```bash
    go run . generate \
      --source /tmp/librariangen-test/source \
      --librarian /tmp/librariangen-test/librarian \
      --output /tmp/librariangen-test/output
    ```
    The generated Go client library will be in the `/tmp/librariangen-test/output` directory.

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
    **Note:** Access to this base image may require special access.

2.  **Build and Test:** The `build-docker-and-test.sh` script builds the Docker image and then runs a verification container to ensure all dependencies are correctly installed.
    ```bash
    bash build-docker-and-test.sh
    ```

## Lines of code in this module

To get an approximate count of the non-commented, non-blank lines of production Go code in the `librariangen` module, you can run the following command from the root of the `google-cloud-go` repository:

```bash
find ./internal/librariangen -type f -name "*.go" ! -name "*_test.go" -exec cat {} + | sed -e 's://.*::' -e '/^\s*$/d' | wc -l
```

This command works by:
1.  Finding all files ending in `.go` within the `internal/librariangen` directory.
2.  Excluding all test files (`*_test.go`).
3.  Removing single-line comments and blank lines.
4.  Counting the remaining lines.

**Note:** This provides a good estimate but does not handle multi-line `/* ... */` comments. For a more precise count, consider using a dedicated tool like `cloc`.
