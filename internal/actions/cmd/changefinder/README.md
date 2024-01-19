# changefinder

`changefinder` will compare the current branch to the `origin/master` branch
then determine which submodules, excluding `internal` submodules, had changes,
and list them on `stdout`. The default is emit them in a simple newline
delimited list.

The available flags are as follows:
 * `-dir=[absolute path]`: The directory to diff, defaults to current working
 directory. 
 * `-q`: Enables quiet mode with no logging. In the event of an error while in
 quiet mode, all logs that were surpressed are dumped with the error. Defaults
 to `false` (i.e. "verbose").
 * `-format=[plain|github|commit]`: The `stdout` output format. Default is `plain`.
 * `-gh-var=[variable name]`: The variabe name to set output for in `github`
 format mode. Defaults to `submodules`.
 * `-base=[ref name]`: The base ref to compare `HEAD` to. Default is
 `origin/main`.
 * `-path-filter=[path filter]`: The path filter to diff for.
 * `-content-pattern=[regex]`: A regex to match on diff contents.
 * `-commit-message=[commit message]`: Message to use in the nested commit block
 * `-commit-scope=[conventional commit scope]`: Scope to use for the commit e.g. `fix`

Example usages from this repo root:

```sh
# targeting a git repository other than current working directory
go run ./internal/actions/cmd/changefinder -dir /path/to/your/go/repo

# quiet mode, github format
go run ./internal/actions/cmd/changefinder -q -format=github

# quiet mode, github format, github var name "foo"
go run ./internal/actions/cmd/changefinder -q -format=github -gh-var=foo
```
