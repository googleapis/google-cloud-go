# Go DocFX YAML Generator

This tool generates DocFX YAML for Go modules.

Only a single module will be processed at once.

By default, the output files are stored at `./obj/api`. You can convert them to
HTML using [doc-templates](https://github.com/googleapis/doc-templates) and/or
[doc-pipeline](https://github.com/googleapis/doc-pipeline).

Example usage:

```
cd module && godocfx ./...
godocfx cloud.google.com/go/...
godocfx -print cloud.google.com/go/storage/...
godocfx -out custom/output/dir cloud.google.com/go/...
godocfx -rm custom/output/dir cloud.google.com/go/...
```

## Testing

You can run the tests with `go test`.

If you need to update the golden files, add the `-update-goldens` flag:

```
go test -v -update-goldens
```