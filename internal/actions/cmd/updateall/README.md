# updateall

`updateall` will update all submodules that depend on the target dep to the
target version, and tidy the module afterwards.

The available flags are as follows:
 * `-q`: Optional. Enables quiet mode with no logging. In the event of an error
 while in quiet mode, all logs that were surpressed are dumped with the error,
 defaults to `false` (i.e. "verbose").
 * `-dep=[module]`: Required. The module dependency to be updated
 * `-version=[version]`: Optional. The module version to update to, defaults to
`latest`.
 * `-no-indirect`: Optional. Exclude updating submodules with only an indirect
dependency on the target, defaults to false.

Example usages from this repo root:

```sh
# update the google.golang.org/api dependency to latest including indirect deps
go run ./internal/actions/cmd/updateall -dep google.golang.org/api

# quiet mode, update google.golang.org/api dependency to v0.122.0 including indirect deps
go run ./internal/actions/cmd/updateall -q -dep google.golang.org/api -version=v0.122.0

# quiet mode, update google.golang.org/api dependency to v0.122.0 excluding indirect deps
go run ./internal/actions/cmd/updateall -q -dep google.golang.org/api -version=v0.122.0 -no-indirect
```
