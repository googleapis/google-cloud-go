# Google Cloud Go (`google-cloud-go`) Context

## Repository Overview
This repository contains the Cloud Client Libraries for Go.

*   **Primary Distinction:** This repo contains both **hand-written** (or hybrid) clients and **auto-generated** (GAPIC) clients.
*   **Generated Code:** The majority of top-level directories (e.g., `aiplatform`, `secretmanager`, `kms`) are auto-generated GAPIC clients.
    *   **Do not edit these manually** to fix logic bugs, as changes will be overwritten.
    *   Only edit them if you are debugging the generation process itself.

## Hand-Written & Hybrid Clients
The following modules contain significant hand-written code and are actively maintained. These are high-priority targets for reading and editing, as opposed to pure generated code:

*   `auth/` (and `auth/oauth2adapt`)
*   `bigquery/`
*   `bigtable/`
*   `compute/metadata/`
*   `datastore/`
*   `errorreporting/`
*   `firestore/`
*   `logging/`
*   `longrunning/`
*   `profiler/`
*   `pubsub/` (and `pubsublite/`)
*   `spanner/`
*   `storage/`
*   `vertexai/`

## Internal Package (`internal/`)
The `internal/` directory contains both critical shared utilities and code generation tools.

*   **Useful Utilities (Good to Browse):**
    *   `retry.go`: Shared retry logic.
    *   `trace/`, `tracecontext/`: Internal tracing support.
    *   `version/`: Versioning logic.
    *   `optional/`: Optional primitive types.
    *   `fields/`: Field handling.
    *   `testutil/`: Common testing helpers.
*   **Ignorable (Tools & Generated):**
    *   `generated/`: Contains generated snippets and other artifacts. **Do not edit.**
    *   `gapicgen/`: Tools for running the GAPIC generator.
    *   `godocfx/`: Documentation generator tools.
    *   `aliasgen/`, `aliasfix/`: Alias management tools.

## Architecture & Wiring
*   **Dependency Flow:** Generated clients in this repo depend heavily on:
    *   **`google.golang.org/api/option`** (from `google-api-go-client`) for configuration.
    *   **`github.com/googleapis/gax-go/v2`** for retries and operation management.
*   **Transport:** Most clients here use gRPC by default but can fallback to HTTP/JSON. The transport setup logic is generated into the client constructors.

## Development
*   **Tests:** Run tests using `go test ./...`.
*   **Linting:** Adhere strictly to standard Go style (`gofmt`).
