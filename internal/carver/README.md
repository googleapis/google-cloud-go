# carver

This is a tool used to carve out new modules in cloud.google.com/go.

## Usage

### Flags

```text
-parent string
    The path to the parent module.
-child string
    The relative path to the child module from the parent module.
-repo-metadata string
    The full path to the repo metadata file.
-dry-run
    If true no files or tags will be created. Optional.
-name string
    The name used to identify the API in the README. Optional
-parent-tag string
    The newest tag from the parent module. If not specified the latest tag will
    be used. Optional.
-child-tag-version string
    The tag version of the carved out child module. Should be in the form of
    vX.X.X with no prefix. Optional.
-parent-tag-prefix string
    The prefix for a git tag, should end in a '/'. Only required if parent is
    not the root module. Optional.
```

```bash
go run cmd/main.go \
  -parent=/path/to/google-cloud-go \
  -child=asset \
  -repo-metadata=/path/to/google-cloud-go/internal/.repo-metadata-full.json
```
