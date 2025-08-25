# Go GAPIC Generator (`librariangen`)

This directory contains the source code for `librariangen`, a containerized Go application that serves as the Go-specific code generator within the Librarian pipeline. Its responsibility is to generate release-ready Go GAPIC client libraries from `googleapis` API definitions, replacing the legacy `bazel-bot` and `OwlBot` toolchain.

## How it Works (The Container Contract)

The `librariangen` binary is designed to be run inside a Docker container orchestrated by the central Librarian tool. It adheres to a specific "container contract" by accepting commands and expecting a set of mounted directories for its inputs and outputs.

The primary commands are `generate` and `release-init`.

### `generate` Command

This command is responsible for the core work of code generation. The container is expected to generate the library code and write it to the `/output` mount.

**Example `generate` command:**
`bash
librariangen generate \
    --source /source \
    --librarian /librarian \
    --output /output
`

### `release-init` Command

This command is the core of the release workflow. It applies version and changelog updates to an existing library's files.

**Example `release-init` command:**
`bash
librariangen release-init \
    --repo /repo \
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

### `release-init` Command Workflow

1.  **Inputs:** The container is provided with the following mounted directories:
    *   `/repo`: A complete checkout of the `google-cloud-go` repository containing the library to be updated.
    *   `/librarian`: Contains a `release-init-request.json` file, which specifies the library, the new version, and the changelog entries.
    *   `/output`: An empty directory where the modified library files will be written.

2.  **Execution:**
    *   The `librariangen` binary parses the `release-init-request.json`.
    *   It reads the specified library files from the `/repo` directory.
    *   It updates the `version.go` file with the new version number.
    *   It prepends the new entries to the `CHANGES.md` file.

3.  **Output:** The modified files (e.g., `version.go`, `CHANGES.md`) are written to the `/output` directory. The Librarian tool then copies these files back into the `google-cloud-go` repository.

## Running

There are three primary ways to run the generator, with varying levels of setup complexity.

### Run Librarian with the prebuilt container image

This is the standard way to run the generator, using a pre-built image from Google's Artifact Registry. This method orchestrates the code generation using the `librarian` CLI, which pulls and runs the container for you.

1.  **Prerequisites:**
    *   You must have `docker` and `git` installed.
    *   Set the `GOOGLE_CLOUD_GO_DIR` environment variable to the absolute path of your `google-cloud-go` repository checkout.
    *   Set the `GOOGLEAPIS_DIR` environment variable to the absolute path of your `googleapis` repository checkout.

2.  **Prepare the Target Repository:**
    Before running the generator, ensure your target `google-cloud-go` repository is clean. It must also contain a committed `.librarian/state.yaml`.
    ```bash
    pushd $GOOGLE_CLOUD_GO_DIR && git reset --hard HEAD && git clean -fd && popd
    ```

3.  **Execute:**
    Run the `librarian` command from the `internal/librariangen` directory. This command will generate the `secretmanager` client library using the public container image.
    ```bash
    go run github.com/googleapis/librarian/cmd/librarian@HEAD generate \
      --image="gcr.io/cloud-devrel-public-resources/librarian-go:infrastructure-public-image-latest" \
      --repo="$GOOGLE_CLOUD_GO_DIR" \
      --library=secretmanager \
      --api=google/cloud/secretmanager/v1,google/cloud/secretmanager/v1beta2 \
      --api-source="$GOOGLEAPIS_DIR"
    ```

    To run the `release-init` command for the `secretmanager` library:
    ```bash
    go run github.com/googleapis/librarian/cmd/librarian@HEAD release-init \
      --image="gcr.io/cloud-devrel-public-resources/librarian-go:infrastructure-public-image-latest" \
      --repo="$GOOGLE_CLOUD_GO_DIR" \
      --library=secretmanager
    ```


### Build the container yourself, and run Librarian with your image

If you have made local changes to `librariangen` and want to test them in a containerized environment, you can build the Docker image locally and run it directly.

1.  **Prerequisites:**
    *   You must have `docker` and `git` installed.
    *   Set the `GOOGLEAPIS_DIR` environment variable to the absolute path of your `googleapis` repository checkout.
    *   You may need to authenticate with Google Artifact Registry to pull the base image. See instructions in Building the Container, below.

2.  **Build the image:**
    The `build-docker-and-test.sh` script will build the image and tag it as `gcr.io/cloud-go-infra/librariangen:latest`.
    ```bash
    ./build-docker-and-test.sh
    ```

3.  **Prepare Inputs:**
    The container requires a directory with a `generate-request.json` file and an empty output directory.
    ```bash
    # Create the required directories
    mkdir -p /tmp/librariangen-run/librarian /tmp/librariangen-run/output

    # Copy the sample request file
    cp testdata/generate/librarian/generate-request.json /tmp/librariangen-run/librarian/
    ```

4.  **Execute:**
    Run the `librarian` command from the `internal/librariangen` directory. This command will generate the `secretmanager` client library using the public container image.
    ```bash
    go run github.com/googleapis/librarian/cmd/librarian@HEAD generate \
      --image="gcr.io/cloud-go-infra/librariangen:latest" \
      --repo="$GOOGLE_CLOUD_GO_DIR" \
      --library=secretmanager \
      --api=google/cloud/secretmanager/v1,google/cloud/secretmanager/v1beta2 \
      --api-source="$GOOGLEAPIS_DIR"
    ```


### Run the librariangen binary as a CLI

This method runs the generator directly as a Go binary, without any Docker containerization. It is the fastest way to iterate on the generator's code but requires all `protoc` plugins and dependencies to be installed on your local machine.

1.  **Prerequisites:**
    *   Install all tools listed in the **Local Development Dependencies** section.
    *   Set the `GOOGLEAPIS_DIR` environment variable to the absolute path of your `googleapis` repository checkout.

2.  **Prepare Inputs:**
    The binary requires a directory with a `generate-request.json` file and an empty output directory.
    ```bash
    # Create the required directories
    mkdir -p /tmp/librariangen-run/librarian /tmp/librariangen-run/output

    # Copy the sample request file
    cp testdata/generate/librarian/generate-request.json /tmp/librariangen-run/librarian/
    ```

3.  **Execute:**
    Run the `go run` command with flags pointing to your `googleapis` checkout and the I/O directories you just created.
    ```bash
    go run . generate \
      --source="$GOOGLEAPIS_DIR" \
      --librarian="/tmp/librariangen-run/librarian" \
      --output="/tmp/librariangen-run/output"
    ```
    The generated Go client library will be in the `/tmp/librariangen-run/output` directory.

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

2.  **Binary Integration Tests:** Shell scripts provide full, end-to-end tests of the compiled binary. This is the primary way to validate changes to the core logic.
    *   **`generate` command:** (`run-binary-generate-test.sh`)
        *   **Setup:** The test requires local checkouts of the `googleapis` and `googleapis-gen` repositories. You must set the `LIBRARIANGEN_GOOGLEAPIS_DIR` and `LIBRARIANGEN_GOOGLEAPIS_GEN_DIR` environment variables to point to these checkouts.
        *   **Execution:**
            ```bash
            ./run-binary-generate-test.sh
            ```
        This script will compile the binary and run it against realistic API fixtures, verifying that the correct Go files are generated.
    *   **`release-init` command:** (`run-binary-release-init-test.sh`)
        *   **Setup:** The test requires a local checkout of the `google-cloud-go` repository. You must set the `LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR` environment variable to point to this checkout.
        *   **Execution:**
            ```bash
            ./run-binary-release-init-test.sh
            ```
        This script will compile the binary and run it against a test library, verifying that the version and changelog files are updated correctly.

## Docker Container

The `Dockerfile` packages the `librariangen` binary and all its dependencies into a MOSS-compliant container for use in the Librarian pipeline.

### Building the Container

1.  **Authenticate with Google Artifact Registry:** The official build process requires pulling a base image from `marketplace.gcr.io`.

    **Note:** Access to this base image may require special access.

    You must authenticate your Docker client with Google Cloud before building with some combination of the following commands:

    ```bash
    gcloud auth login
    go install github.com/GoogleCloudPlatform/docker-credential-gcr/v2@latest
    docker-credential-gcr configure-docker
    gcloud auth configure-docker
    gcloud auth configure-docker gcr.io
    ```

2.  **Build and Test:** The `build-docker-and-test.sh` script builds the Docker image and then runs a verification container to ensure all dependencies are correctly installed.
    ```bash
    bash build-docker-and-test.sh
    ```
