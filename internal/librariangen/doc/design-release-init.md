# release-init Design

`#begin-approvals-addon-section`

**Author(s):** [Chris Smith](mailto:chrisdsmith@google.com)| **Last Updated**: Aug 13, 2025  | **Status**: Current   
**Self link:** [go/librarian:go-librarian-release-init](http://goto.google.com/librarian:go-librarian-release-init) | **Project Issue**: [b/432276586](http://b/432276586)

# Objective

Add support for the Librarian `release init` command to the Go `librariangen` binary and container.

# Background

The Go Librarian container’s support for the Librarian `release init` command replaces the following behavior in legacy components.

## post-processor

The current post-processor, implemented in the `google-cloud-go/internal/postprocessor` binary, modifies the following **global** files within the `google-cloud-go` repository.

For the purpose of this design, a global file is defined as a file that exists outside of a specific client library module directory (e.g., `/asset`) and its corresponding snippets directory (e.g., `/internal/generated/snippets/asset`).

This manipulation of these repository-level files is essential for integrating new and updated modules into the monorepo. The current post-processor operates with broad filesystem access, directly invoking `go` commands and writing to files as needed.

The primary global files manipulated by the current post-processor are:

### `go.work` and `go.work.sum`

When a new module is generated, the post-processor executes `go work use` to add the new module to the root `go.work` file. This registers the new module within the Go workspace, making it discoverable by the Go compiler and developer tools across the entire monorepo.

### `internal/generated/snippets/go.mod`

The shared snippets module needs to reference the actual client library modules to compile correctly. For newly generated or updated modules, the post-processor adds a `replace` directive to this `go.mod` file (via `go mod edit -replace`). This directive points the snippet module to the local, on-disk version of the client library module, resolving the dependency without requiring a published version.

### `internal/generated/snippets/<module>/<version>/**/*`

The post-processor updates the `$VERSION` placeholder within the `snippet_metadata.google.cloud.*.json` files within the `internal/generated/snippets/<module>/<version>` directories.

### `internal/.repo-metadata-full.json`

This file serves as a comprehensive manifest of all releasable modules within the repository. The post-processor is responsible for completely regenerating this file to ensure it accurately reflects the current set of modules. This metadata is consumed by downstream tooling.

### `.release-please-manifest-submodules.json`

To integrate a new module into the automated release process, it must be added to this `release-please` manifest. The post-processor adds an entry for the new module with a starting version of `0.0.0`, effectively bootstrapping it into the release cycle.

**Note:** This file is out-of-scope for the Librarian project, which replaces `release-please`.

### `release-please-config-yoshi-submodules.json`

This is the primary configuration file for the `release-please` automation. The post-processor regenerates this entire file to ensure that all modules, including any new ones, are included. This ensures that `release-please` tracks commits and propose releases for every module in the repository.

**Note:** This file is out-of-scope for the Librarian project, which replaces `release-please`.

## release-please

The current release process is managed by `release-please`, which uses a set of language-specific strategies to automate release pull request creation. For `google-cloud-go`, `go-yoshi.ts` is used. This strategy dictates how version numbers and changelogs are updated.

The strategy modifies two library-specific files:

### `CHANGES.md`

The strategy prepends the newly generated release notes for the new version to the top of this file.

### `internal/version.go`

The strategy parses this Go source file and replaces the value of the `Version` constant with the new semantic version number.

Note: A significant portion of the strategy is dedicated to complex commit filtering logic. It parses commit message scopes (e.g., `feat(asset): ...`) to determine which commits belong to which module. This ensures that only relevant changes are included in a module's release notes. However, in the Librarian model, this commit filtering and collation is handled by the central Librarian tool itself *before* the `release-init` command is ever invoked. The `release-init-request.json` will arrive with a pre-filtered list of changes for each library. Therefore, `librariangen` does not need to replicate this commit filtering logic.

# Overview

This document focuses on the delivery of a production-ready implementation of the `release-init` step for **existing libraries**. 

# Detailed Design

## Repository configuration

All language repos will have a single `state.yaml` file located in their language repo at the location `<repo>/.librarian/state.yaml`. This file lets Librarian know which libraries it is responsible for generating and releasing.

```textproto
# The name of the image and tag to use.
image: "image=gcr.io/cloud-go-infra/librariangen:latest"

# The state of each library which is released within this repository.
libraries:
  - # The library identifier (language-specific format). api_paths configured under a
    # given id should correspond to a releasable unit in a given language
    id: "secretmanager"
    # The commit hash (within the API definition repo) at which
    # the repository was last generated.
    last_generated_commit: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
    # APIs that are bundled as a part of this library.
    apis:
      - # The API path included in this library, relative to the root
        # of the API definition repo, e.g. "google/cloud/functions/v2".
        path: "google/cloud/secretmanager/v1"
        # The name of the service config file, relative to the path.
        service_config: "secretmanager_v1.yaml"
      - path: "google/cloud/secretmanager/v1beta2"
        service_config: "secretmanager_v1beta2.yaml"
    # Directories to which librarian contributes code to.
    source_roots:
      - "secretmanager"
      - "internal/generated/snippets/secretmanager"
    # Directories files in the local repo to leave untouched during copy and remove.
    preserve_regex:
      - "secretmanager/CHANGES.md"
      - "secretmanager/README.md"
      - "secretmanager/aliasshim/aliasshim.go"
      - "secretmanager/apiv1/iam.go"
      - "secretmanager/apiv1/iam_example_test.go"
      - "secretmanager/apiv1/version.go"
      - "secretmanager/apiv1beta2/version.go"
      - "secretmanager/internal/version.go"
    # If configured, these files/dirs will be removed before generated code is copied
    # over. A more specific `preserve_regex` takes preceidece. If not not set, defaults
    # to the `souce_paths`.
    remove_regex:
      - "secretmanager"
      - "internal/generated/snippets/secretmanager"
    # Path of commits to be excluded from parsing while calculating library changes.
    # If all files from commit belong to one of the paths it will be skipped.
    release_exclude_paths:
    # The last version that was released for the library.
    version: "1.2.3"
    # Specifying a tag format allows librarian to honor this format when creating
    # a tag for the release of the library. The replacement values of {id} and {version}
    # permitted to reference the values configured in the library. If not specified
    # the assumed format is {id}-{version}.
    tag_format: "{id}/v{version}"
```

## 

A `config.yaml` file will also be present as described in [go/librarian:global-file-edits](http://goto.google.com/librarian:global-file-edits).

```textproto
global_files_allowlist:
  # Allow the container to read and write the root go.work and go.work.sum
  # files to add new modules.
  - path: "go.work"
    permissions: "read-write"
  - path: "go.work.sum"
    permissions: "read-write"

  # Allow the container to read and write the snippets go.mod and go.sum file to add new
  # replace directives.
  - path: "internal/generated/snippets/go.mod"
    permissions: "read-write"
  - path: "internal/generated/snippets/go.sum"
    permissions: "read-write"
  
  # Allow the container to read and write the repo metadata file.
  - path: "internal/.repo-metadata-full.json"
    permissions: "read-write"
```

## Container

The same container used for the `generate` command should also be used for the `release-init` command.

Librarian provides all necessary inputs as mounted directories and also as command flags:

* `/librarian`:This mount will contain exactly one file named `release-init-request.json`. The container must write back any error in the optional file `release-init-response.json`.  
* `/output`: An empty directory.  Any files that are updated during the release phase should be moved to this directory in the correct path that they should land in the language repository.  
* `/repo`: An incomplete checkout of the `google-cloud-go` repository. This directory will contain all directories that make up a library, the `.librarian` folder, and any global file declared in the `config.yaml`.  
* `--librarian`, `--output`, `--repo` flags: Passed to each container invocation after the command with the locations of all of the mounts. Provided to aid in local testability of the container's entrypoint. E.g. `--librarian=/path/to/librarian`

## Librarian Go generator binary (librariangen)

The `release-init` command is scoped to all Librarian libraries in a release. For example, given a Librarian release for the Go `secretmanager` and `workflows` top-level submodules, the binary must write changed files to `/output/secretmanager` and `/output/workflows`. To update snippet files, it must write to the corresponding paths, e.g., `/output/internal/generated/snippets/secretmanager` and `/output/internal/generated/snippets/workflows`. Global files may be located anywhere in the repository.

The binary’s command-line requirements for `release-init`  include:

1. Accept a positional string argument containing the `release-init` command.  
2. Accept filepath (absolute or relative) flags, after the command, for **all** of the following mounted directories in the container, in order to provide a convenient development experience on a regular Google laptop (S-11 in [go/librarian-generation-lifecycle-requirements](http://goto.google.com/librarian-generation-lifecycle-requirements)).  
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
   * In the `request` package, add structs to represent the `release-init-request.json` and `release-init-response.json` as described in the design documents. This will include structs for libraries, changes, and APIs.

```go
// ReleaseInitRequest represents the data in release-init-request.json.
type ReleaseInitRequest struct {
	Libraries []*ReleaseLibrary `json:"libraries"`
}

// ReleaseLibrary represents a single library to be released.
type ReleaseLibrary struct {
	ID               string    `json:"id"`
	Version          string    `json:"version"`
	Changes          []*Change `json:"changes"`
	APIs             []*API    `json:"apis"`
	SourceRoots      []string  `json:"source_roots"`
	ReleaseTriggered bool      `json:"release_triggered"`
}

// Change represents a single conventional commit change.
type Change struct {
	Type             string `json:"type"`
	Subject          string `json:"subject"`
	Body             string `json:"body"`
	PiperCLNumber    string `json:"piper_cl_number"`
	SourceCommitHash string `json:"source_commit_hash"`
}

// API represents a single API definition.
// This struct is simplified as other fields are not needed for release-init.
type API struct {
	Path string `json:"path"`
}

// ReleaseInitResponse represents the data in release-init-response.json.
type ReleaseInitResponse struct {
	Error string `json:"error,omitempty"`
}
```

3. **Core `release-init` Logic (in the `release` package):**

   * Create a `ReleaseInit(ctx context.Context, cfg *Config)` function that will be the main entry point for the command's logic.  
   * This function will first parse the `release-init-request.json` file.  
4. **Global File Handling:**

   * For each global file identified in the "Background" section, a corresponding function will be created.  
   * **`updateGoWork(cfg *Config, libs []request.Library) error`**: This function will read `/repo/go.work`, parse it, and add the missing module, if necessary. The updated file will be written to `/output/go.work`.  After the `go.work` file is written, this function will execute `go work sync` in the `/output` directory to generate the corresponding `go.work.sum` file.  
   * **`updateSnippetsGoMod(cfg *Config, libs []request.Library) error`**: This function will read `/repo/internal/generated/snippets/go.mod`, parse it, and add any necessary `replace` directives for the libraries being released. The updated file will be written to `/output/internal/generated/snippets/go.mod`.  
   * **`updateRepoMetadata(cfg *Config, libs []request.Library) error`**: This function will generate a new `internal/.repo-metadata-full.json` file based on the full list of libraries and write it to `/output/internal/.repo-metadata-full.json`.  
5. **Library-Specific File Handling:**

   * **`updateChangelog(cfg *Config, lib request.Library) error`**: This function will read the `CHANGES.md` for a given library, prepend the new release notes, and write the updated file to the `/output` directory.  
   * **`updateVersion(cfg *Config, lib request.Library) error`**: This function will update the version in the `version.go` files for the given library and write the updated files to the `/output` directory.  
   * **`updateSnippetVersion(cfg *Config, lib request.Library) error`**: This function will update the `$VERSION` placeholders within the `snippet_metadata.google.cloud.*.json` files within the `<module>/<version>` directories, and write the updated files to the `/output` directory.

# Risks

# Open questions
