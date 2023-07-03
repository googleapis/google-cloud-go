# carver

This is a tool used to carve out new modules in cloud.google.com/go.

## carver Usage

```bash
go run cmd/carver/main.go \
  -parent=/path/to/google-cloud-go \
  -repo-metadata=/path/to/google-cloud-go/internal/.repo-metadata-full.json \
  -child=asset
```

## stabilizer Usage

```bash
go run cmd/stabilizer/main.go \
  -base=/path/to/google-cloud-go \
  -child=asset
```
