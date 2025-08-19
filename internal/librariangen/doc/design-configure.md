# configure Design

`#begin-approvals-addon-section`

**Author(s):** [Chris Smith](mailto:chrisdsmith@google.com)| **Last Updated**: Aug 19, 2025  | **Status**: Draft   
**Self link:** [go/librarian:go-librarian-configure](http://goto.google.com/librarian:go-librarian-configure) | **Project Issue**: [b/432276586](http://b/432276586)

# Objective

Add support for the Librarian `configure` command to the Go `librariangen` binary and container. This command is responsible for onboarding new libraries into the repository by updating global configuration, dependency, and manifest files.

# Background

The `configure` command is invoked by Librarian when a new API is being added to a library for the first time. Its primary role is to integrate this new module into the repository's overall structure and build system. This process replaces legacy behavior that was previously handled by a monolithic post-processing script.

For the purpose of this design, a global file is defined as a file that exists outside of a specific client library module directory (e.g., `/asset`). The manipulation of these repository-level files is essential for integrating new modules into the monorepo.

The primary global files to be managed by the `configure` command are:

### `go.work` and `go.work.sum`

When a new module is generated, it must be registered within the Go workspace to be discoverable by the Go compiler and developer tools across the entire monorepo. The `configure` command will add the new module to the root `go.work` file.

### `internal/generated/snippets/go.mod`

The shared snippets module needs to reference the actual client library modules to compile correctly. For a newly generated module, the `configure` command will add a `replace` directive to this `go.mod` file. This directive points the snippet module to the local, on-disk version of the client library module, resolving the dependency without requiring a published version.

### `internal/.repo-metadata-full.json`

This file serves as a comprehensive manifest of all releasable modules within the repository. The `configure` command is responsible for completely regenerating this file to ensure it accurately reflects the current set of modules after a new one has been added. This metadata is consumed by downstream tooling.

# Detailed Design

## Repository Configuration

The `configure` command relies on a `config.yaml` file in the `.librarian` directory to grant it permission to modify specific global files.

```yaml
# .librarian/config.yaml
global_files_allowlist:
  # Allow the container to read and write the root go.work and go.work.sum
  # files to add new modules.
  - path: "go.work"
    permissions: "read-write"
  - path: "go.work.sum"
    permissions: "read-write"

  # Allow the container to read and write the snippets go.mod file to add new
  # replace directives.
  - path: "internal/generated/snippets/go.mod"
    permissions: "read-write"
  
  # Allow the container to read and write the repo metadata file.
  - path: "internal/.repo-metadata-full.json"
    permissions: "read-write"
```

## Container Contract

Librarian invokes the `configure` command with the following contract:

*   **/librarian**: Contains `configure-request.json`. The container must write back `configure-response.json`.
*   **/input**: The `.librarian/generator-input` directory.
*   **/repo**: A partial checkout of the repository containing files specified in the `global_files_allowlist`.
*   **/source**: A complete checkout of the API definitions repository (e.g., googleapis/googleapis).
*   **/output**: An empty directory where all modified global files must be written.

## Librarian Go generator binary (librariangen)

The `librariangen` binary will be updated to handle the `configure` command.

### Execution

1.  **Command and Flag Parsing:**
    *   The existing `configure` case in `main.go` will be implemented.
    *   A `handleConfigure` function will parse the required flags: `--librarian`, `--input`, `--repo`, `--source`, and `--output`.

2.  **Request and Response Handling:**
    *   Go structs will be defined to represent the `configure-request.json` and `configure-response.json` schemas as defined in the official container contract.

3.  **Core `configure` Logic:**
    *   A new `configure` package will house the primary logic.
    *   The main entry point will parse the `configure-request.json` to identify the new library.
    *   It will then orchestrate the modification of global files and the creation of the response file.

4.  **Global File Handling:**
    *   **`updateGoWork(cfg *Config, libs []request.Library) error`**: This function will read `/repo/go.work`, parse it, and add the `source_roots` for the new library. The updated file will be written to `/output/go.work`. After the `go.work` file is written, this function will execute `go work sync` in the `/output` directory to generate the corresponding `go.work.sum` file.
    *   **`updateSnippetsGoMod(cfg *Config, libs []request.Library) error`**: This function will read `/repo/internal/generated/snippets/go.mod`, parse it, and add the necessary `replace` directive for the new library. The updated file will be written to `/output/internal/generated/snippets/go.mod`.
    *   **`updateRepoMetadata(cfg *Config, libs []request.Library) error`**: This function will generate a new `internal/.repo-metadata-full.json` file based on the full list of libraries provided in the request and write it to `/output/internal/.repo-metadata-full.json`.

5.  **Response Generation:**
    *   The `configure` logic will generate a `configure-response.json` for the new library.
    *   Following convention, it will propose a starting `version` of `0.1.0`.
    *   It will also propose a `tag_format` of `{id}/v{version}`.

# Risks

*   **State Corruption:** An error during the regeneration of a global file like `.repo-metadata-full.json` could leave the repository in an inconsistent state. The process must be robust and have clear error reporting.
*   **Command Execution:** The reliance on executing the `go work sync` command introduces a dependency on the Go toolchain being present and correctly configured within the container.

# Open questions

*   How should the tool behave if a `go.work` file does not exist at the root? Should it create one?
*   Is it possible for the `configure-request.json` to contain more than one "new" library? The design assumes only one, but this should be verified against the Librarian specification.
