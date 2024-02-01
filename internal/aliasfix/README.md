# aliasfix

A tool to migrate client library imports from go-genproto to the new stubs
located in google-cloud-go.

## Usage

Make sure you dependencies for the cloud client library you depend on and
go-genproto are up to date.

```bash
go install cloud.google.com/go/internal/aliasfix/cmd/aliasfix@latest
aliasfix .
go mod tidy
```
