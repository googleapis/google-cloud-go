# carver

This is a tool used to carve out new modules in cloud.google.com/go.

## Usage

```bash
go run cmd/main.go \
  -parent=/path/to/google-cloud-go \
  -repo-metadata=/path/to/google-cloud-go/internal/.repo-metadata-full.json \
  -child=asset
```
