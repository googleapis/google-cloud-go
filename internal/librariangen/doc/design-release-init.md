# release-init Design

`#begin-approvals-addon-section`

**Author(s):** [Chris Smith](mailto:chrisdsmith@google.com)| **Last Updated**: Aug 19, 2025  | **Status**: Draft   
**Self link:** [go/librarian:go-librarian-release-init](http://goto.google.com/librarian:go-librarian-release-init) | **Project Issue**: [b/432276586](http://b/432276586)

# Objective

Add support for the Librarian `release-init` command to the Go `librariangen` binary and container. The command is responsible for applying version and changelog updates to existing, library-specific files.

# Background

The Go Librarian container’s support for the Librarian `release-init` command replaces legacy behavior previously managed by `release-please`.

The current release process is managed by `release-please`, which uses a set of language-specific strategies to automate release pull request creation. For `google-cloud-go`, `go-yoshi.ts` is used. This strategy dictates how version numbers and changelogs are updated.

The strategy modifies two library-specific files:

### `CHANGES.md`

The strategy prepends the newly generated release notes for the new version to the top of this file.

### `internal/version.go`

The strategy parses this Go source file and replaces the value of the `Version` constant with the new semantic version number.

### `snippet_metadata.google.cloud.*.json`

The post-processor updates the `$VERSION` placeholder within the `snippet_metadata.google.cloud.*.json` files within the `internal/generated/snippets/<module>/<version>` directories.

**Note:** A significant portion of the legacy strategy is dedicated to complex commit filtering logic. It parses commit message scopes (e.g., `feat(asset): ...`) to determine which commits belong to which module. In the Librarian model, this commit filtering and collation is handled by the central Librarian tool itself *before* the `release-init` command is ever invoked. The `release-init-request.json` will arrive with a pre-filtered list of changes for each library. Therefore, `librariangen` does not need to replicate this commit filtering logic.

# Overview

This document focuses on the delivery of a production-ready implementation of the `release-init` step. The command's scope is strictly limited to updating files with new version numbers and changelog content for **existing libraries** that are undergoing a release.

Global file modifications related to onboarding new libraries (such as updating `go.work` or `internal/.repo-metadata-full.json`) are explicitly out of scope for this command and are handled by the `configure` command.

# Detailed Design

## Repository configuration

The `release-init` command relies on two primary configuration files located in the `.librarian` directory.

### `.librarian/state.yaml`

This file provides the complete configuration for all libraries managed by Librarian. The `release-init` command uses this to understand the context of the library being released, such as its `source_roots`.

```yaml
image: image=gcr.io/cloud-go-infra/librariangen:latest
libraries:
    - id: secretmanager
      version: "1.15.0"
      last_generated_commit: 8fe8f9f460fe5dd1df95057659d2520ceca3c9c6
      apis:
        - path: google/cloud/secretmanager/v1
          service_config: secretmanager_v1.yaml
        - path: google/cloud/secretmanager/v1beta2
          service_config: secretmanager_v1beta2.yaml
      source_roots:
        - secretmanager
        - internal/generated/snippets/secretmanager
      preserve_regex:
        - secretmanager/CHANGES.md
        - secretmanager/internal/version.go
        - internal/generated/snippets/secretmanager/snippet_metadata.google.cloud.secretmanager.v1.json
        - secretmanager/aliasshim/aliasshim.go
        - secretmanager/apiv1/iam.go
        - secretmanager/apiv1/iam_example_test.go
      remove_regex:
        - secretmanager
        - internal/generated/snippets/secretmanager
      release_exclude_paths: []
      tag_format: "{id}/v{version}"
```

### `.librarian/config.yaml`

This file may be present as described in [go/librarian:global-file-edits](http://goto.google.com/librarian:global-file-edits). As `release-init` primarily modifies files within a library's `source_roots`, a `global_files_allowlist` may not be necessary. However, it is required if a version-dependent global file (like a root `README.md`) needs to be modified.

```yaml
global_files_allowlist:
  # Allow the container to read a template.
  - path: "internal/README.md.template"
    permissions: "read-only"
  
  # Allow publishing the updated root README.md
  - path: "README.md"
    permissions: "write-only"
```

## Container

The same container used for the `generate` command should also be used for the `release-init` command.

Librarian provides all necessary inputs as mounted directories and also as command flags:

* `/librarian`:This mount will contain exactly one file named `release-init-request.json`. The container must write back any error in the optional file `release-init-response.json`.  
* `/output`: An empty directory.  Any files that are updated during the release phase should be moved to this directory in the correct path that they should land in the language repository.  
* `/repo`: An incomplete checkout of the `google-cloud-go` repository. This directory will contain all directories that make up a library, the `.librarian` folder, and any global file declared in the `config.yaml`.  
* `--librarian`, `--output`, `--repo` flags: Passed to each container invocation with the locations of all of the mounts. Provided to aid in local testability of the container's entrypoint. E.g. `--librarian=/path/to/librarian`

## Librarian Go generator binary (librariangen)

The `release-init` command is scoped to a single Librarian library. For example, given a Librarian library configuration for the Go `workflows` top-level submodule, the binary must write changed files to `/output/workflows`. To update snippet files, it must write to the corresponding path, e.g., `/output/internal/generated/snippets/workflows`.

The binary’s command-line requirements for `release-init`  include:

1. Accept a positional string argument containing the `release-init` command.  
2. Accept filepath (absolute or relative) flags for **all** of the following mounted directories in the container.
   1. `/librarian`  
   2. `/output`  
   3. `/repo`  
3. Write error details to `release-init-response.json` if there was an error.  
4. Return exit code 0 if successful, or a non-zero error code if there was an error.

### Execution

The execution of the `release-init` command will be broken down into the following phases:

1. **Command and Flag Parsing:**
   * Add a `release-init` case to the `run` function in `main.go`.  
   * Create a `handleReleaseInit` function to parse the required flags: `--librarian`, `--repo`, and `--output`.  
   * Define a `release.Config` struct to hold the flag values.  
2. **Request and Response Handling:**
   * Create a `release` package.  
   * In the `release` package, add structs that conform to the official container contract for `release-init-request.json` and `release-init-response.json`.
3. **Core `release-init` Logic (in the `release` package):**
   * Create a `Init(ctx context.Context, cfg *Config)` function that will be the main entry point for the command's logic.  
   * This function will first parse the `release-init-request.json` file.  
   * It will then iterate through the libraries where `release_triggered` is `true`.
4. **Library-Specific File Handling:**
   * For each library being released, the following functions will be called:
   * **`updateChangelog(cfg *Config, lib request.Library) error`**: This function will read the `CHANGES.md` for a given library. It will then format the incoming changes from the request by grouping them under conventional commit types (e.g., "Features", "Bug Fixes") and adding a new version and date header. This formatted content will be prepended to the existing changelog, and the result will be written to the `/output` directory.
   * **`updateVersion(cfg *Config, lib request.Library) error`**: This function will update the version in the `internal/version.go` file for the given library and write the updated file to the `/output` directory.  
   * **`updateSnippetVersion(cfg *Config, lib request.Library) error`**: This function will update the `$VERSION` placeholders within the `snippet_metadata.google.cloud.*.json` files within the `<module>/<version>` directories, and write the updated files to the `/output` directory.

# Risks

*   **File Parsing Errors:** The `CHANGES.md` or `internal/version.go` files might be in an unexpected format due to manual edits, causing parsing to fail. The implementation must include robust error handling and report clear messages back in the `release-init-response.json`.
*   **Idempotency:** If the `release-init` command is run twice for the same release, it could result in duplicated changelog entries. The process should be designed to be idempotent where possible, or the risks of non-idempotent operations should be noted.
*   **Partial Failure:** If updating `CHANGES.md` succeeds but updating `internal/version.go` fails for a given module, the repository could be left in an inconsistent state. The operation should be atomic per-library, meaning that if any file update fails, no files for that library should be written to `/output`.

# Implementation Details & Decisions

*   **File Handling Strategy:** To ensure idempotency and preserve existing file content, the `release-init` command adopts a staging-based approach. For each library being released, the command first performs a full, recursive copy of all directories listed in that library's `source_roots` from the `/repo` mount to the `/output` mount. All subsequent file modifications (for changelogs, version files, etc.) are then performed *in-place* on the files that have been staged in the `/output` directory. This strategy ensures that the command operates on a complete and consistent view of the library's files and correctly preserves content from the previous state.

*   **Handling Multiple `version.go` Files:** The `updateVersion` function iterates through all `source_roots` for a given library. For each root, it looks for a version file at the conventional path (`<source_root>/internal/version.go`). This strategy ensures that all relevant `version.go` files within a library's scope are discovered and updated.

*   **Changelog Formatting:** The `updateChangelog` function does not simply prepend raw commit messages. It creates a formatted markdown section that includes a version and date header (e.g., `### 1.16.0 (2025-08-19)`) and groups commit subjects under standardized headers (e.g., "Features", "Bug Fixes") based on their conventional commit type.
