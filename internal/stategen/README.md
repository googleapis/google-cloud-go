# Librarian state file generator

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
