# updateall

`updateall` will update all submodules that depend on the target dep to the
target version, and tidy the module afterwards. The program output is the set of
nested commit statements needed to correctly trigger release-please releases for
the updates. (See example output below.)

The available flags are as follows:

* `-q`: Optional. Enables quiet mode with no logging or program output. In the
event of an error while in quiet mode, all logs that were surpressed are dumped
with the error, defaults to `false` (i.e. "verbose").
* `-dep=[module]`: Required. The module dependency to be updated.
* `-version=[version]`: Optional. The module version to update to, defaults to
`latest`.
* `-commit-level=[type]`: Optional. The nested commit conventional commits type
for the program output, defaults to `fix`.
* `-msg=[message]`: Optional. The nested commit message for the program output,
defaults to `update <dep> to <version>`.
* `-no-indirect`: Optional. Exclude updating submodules with only an indirect
dependency on the target, defaults to false.

Example usages from this repo root:

```sh
# update the google.golang.org/api dependency to latest including indirect deps
go run ./internal/actions/cmd/updateall -dep google.golang.org/api

# quiet mode, update google.golang.org/api dependency to v0.122.0 including indirect deps
go run ./internal/actions/cmd/updateall -q -dep google.golang.org/api -version=v0.122.0

# update google.golang.org/api dependency to v0.122.0 excluding indirect deps
go run ./internal/actions/cmd/updateall -dep google.golang.org/api -version=v0.122.0 -no-indirect

# update the google.golang.org/api dependency to latest including indirect deps, with custom commit type and message
go run ./internal/actions/cmd/updateall -dep google.golang.org/api -commit-level chore -msg "bump apiary"
```

Example output:

```sh
BEGIN_NESTED_COMMIT
fix(accessapproval): update golang.org/x/net to v0.17.0
END_NESTED_COMMIT
BEGIN_NESTED_COMMIT
fix(accesscontextmanager): update golang.org/x/net to v0.17.0
END_NESTED_COMMIT
...
```
