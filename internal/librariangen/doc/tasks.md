# release-init: Production-Ready Task List

This document outlines the comprehensive set of tasks required to implement, test, document, and integrate the `release-init` command into the `librariangen` binary, ensuring it is production-ready.

---

### Phase 1: Complete Core `release-init` Implementation

This phase focuses on completing and perfecting the file modification logic within the `release` package.

*   [X] **`updateChangelog` Function**
    *   **Status:** Completed.

*   [X] **`updateVersion` Function**
    *   **Status:** Completed.

*   [X] **`updateSnippetVersion` Function**
    *   **Status:** Completed.

*   [X] **Finalize Error Handling and `Init` Function Logic**
    *   **Status:** Completed.

---

### Phase 2: Implement a Robust Testing Suite

This phase ensures the implementation is correct, reliable, and will not cause regressions. It combines high-level integration testing with focused unit testing.

*   [X] **Tier 1: Unit Testing (Go Packages)**
    *   [X] **Create `release/release_test.go`:**
        *   [X] Add `TestUpdateChangelog` to verify correct formatting, prepending, and file creation.
        *   [X] Add `TestUpdateVersion` to verify correct version replacement and handling of missing files.
        *   [X] Add `TestUpdateSnippetVersion` to verify correct placeholder replacement and directory traversal.
    *   [X] **Extend `main_test.go`:**
        *   [X] Add a new test case to verify the CLI flag parsing for the `release-init` command.
        *   [X] Test for both successful invocation and failure on missing/invalid flags.

*   [X] **Tier 2: Binary Integration Testing**
    *   [X] **Create `run-binary-release-init-test.sh`:**
        *   [X] Model the new script after `run-binary-generate-test.sh`.
        *   [X] The script invokes the compiled `librariangen` binary with the `release-init` subcommand.
        *   [X] It uses `diff -r` to compare the output against the `golden` directory.

*   [X] **Tier 3: Hermetic Container Integration Testing**
    *   [X] **Create "Golden" Output Fixtures:**
        *   [X] Create a new directory: `testdata/release-init/golden/`.
        *   [X] Create `golden/secretmanager/CHANGES.md` with the expected prepended release notes for version `1.16.0`.
        *   [X] Create `golden/secretmanager/internal/version.go` with the version constant updated to `"1.16.0"`.
        *   [X] Create `golden/internal/generated/snippets/secretmanager/snippet_metadata.google.cloud.secretmanager.v1.json` with `"$VERSION"` replaced by `"1.16.0"`.
    *   [X] **Create `run-container-release-init-test.sh`:**
        *   [X] Model the new script after `run-container-integration-test.sh`.
        *   [X] The script invokes `docker run` with `--mount` flags mapping the `testdata/release-init/` fixtures to `/librarian` and `/repo`.
        *   [X] The `docker run` command passes the `release-init` subcommand to the container's entrypoint.
        *   [X] After execution, the script uses `diff -r` to compare the output directory with the `testdata/release-init/golden/` directory and fail the test on any difference.

*   [X] **Tier 4: End-to-End Librarian CLI Integration Testing**
    *   **Note:** A new, parallel test script is required for this tier rather than extending the existing `run-librarian-integration-test.sh`. The setup, execution, and verification steps for `release init` (which requires pre-existing commits and checks for file modifications) are fundamentally different from those for `generate` (which starts from a clean repository and checks for newly created files). A separate script ensures clarity, maintainability, and prevents the complex test setups from interfering with each other.
    *   [X] **Create `run-librarian-release-init-test.sh`:**
        *   [X] Model the new script after `run-librarian-integration-test.sh`.
        *   [X] The script requires a local checkout of `google-cloud-go` (`LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR`).
        *   [X] It creates a series of temporary commits (e.g., with `feat:` messages) to provide a realistic history for the release command to process.
        *   [X] It invokes the `librarian` CLI with the `release init` subcommand.
        *   [X] Verification is performed using `git status` and `git diff --staged` to check that the correct files in the `google-cloud-go` repository were modified.
        *   [X] The script includes `git reset --hard HEAD && git clean -fd` to ensure the repository is left in a clean state.

---

### Phase 3: Update Documentation

This phase ensures that users and future maintainers can understand and use the `release-init` command.

*   [X] **Update `README.md`:**
    *   [X] Add a new section titled "## `release-init` Command".
    *   [X] Document the command's purpose.
    *   [X] List and explain the command-line flags: `--librarian`, `--repo`, and `--output`.
    *   [X] Provide a clear example invocation.

*   [X] **Finalize Design Documents:**
    *   [X] Perform a final review of `doc/design-release-init.md`.
    *   [X] Update the "Open Questions" section to reflect the decisions made during implementation, transforming questions into statements of behavior.

---

### Phase 4: Finalization and Integration

This final phase addresses the remaining items to ensure the `release-init` feature is delivered as a complete and polished piece of software.

*   [X] **Code Cleanup:**
    *   [X] Remove all `// TODO(...)` comments related to `release-init`.

*   [X] **Dependency Management:**
    *   [X] Run `go mod tidy` to ensure `go.mod` and `go.sum` are clean.

---

### Phase 5: Polish and Harden

This final phase addresses the subtle bugs and gaps in test coverage identified during the final review.

*   [X] **Refactor for Testability:**
    *   [X] Modify the `updateChangelog` function to accept the date as a parameter, allowing tests to pass in a fixed date and making them deterministic.

*   [X] **Enhance Unit Test Coverage:**
    *   [X] Add a test case to `TestUpdateVersion` that verifies its behavior when a library has multiple `source_roots`, only some of which contain a `version.go` file.
    *   [X] Add a test case to `TestUpdateSnippetVersion` that verifies the recursive file walk finds multiple snippet files in nested directories.
    *   [X] Add a test case to `TestUpdateSnippetVersion` that verifies the function does not error and does not modify a snippet file that is missing the `"$VERSION"` placeholder.