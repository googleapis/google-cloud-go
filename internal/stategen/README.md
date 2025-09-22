# Librarian state file generator (MIGRATION TOOL)

This is a state file generator for Librarian, for google-cloud-go.

Command line arguments: repo-root module1 module2...

- repo-root: Path to the repository root
- module1, module2, ...: Modules to add to the state file

Example from the root directory:

```sh
$ go run ./internal/stategen . spanner apphub
```

It is expected that the state file (`.librarian/state.yaml`) already
exists. Any libraries that already exist within the state file are
ignored, even if they're listed in the command line.

**NOTE:** This is a one-time migration tool to assist in moving modules from the legacy OwlBot/release-please workflow to the new Librarian workflow.

In addition to adding new modules to the `.librarian/state.yaml` file, this tool will also **remove the legacy configuration** for each migrated module from the following files:
- `.github/.OwlBot.yaml`
- `internal/postprocessor/config.yaml`
- `release-please-config.json`
- `release-please-config-individual.json`
- `release-please-config-yoshi-submodules.json`
- `.release-please-manifest.json`
- `.release-please-manifest-individual.json`
- `.release-please-manifest-submodules.json`