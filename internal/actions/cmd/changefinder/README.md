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
 * `-touch`: touches the `CHANGES.md` file to elicit a submodule change - only
 works when used with `-format=commit`

Example usages from this repo root:

```sh
# targeting a git repository other than current working directory
go run ./internal/actions/cmd/changefinder -dir /path/to/your/go/repo

# quiet mode, github format
go run ./internal/actions/cmd/changefinder -q -format=github

# quiet mode, github format, github var name "foo"
go run ./internal/actions/cmd/changefinder -q -format=github -gh-var=foo
```

## How to bump all changed modules for release using `-touch`

The `-touch` flag is best used when attempting to generate nested commits for an
already submitted change. For example, a GAPIC generator change touches every
gapic module, but the PR doesn't have nested commits for each changed module.
The PR is submitted as is, perhaps because it was included with a bunch of other
changes or for timing purposes, and the nested commits were not added with
`changefinder` while the PR was open. In this case, we can do the following
using `changefinder`:

1. checkout the merged commit with the target changes: `git checkout <some commit has>`
2. "save" this to a branch: `git checkout -b changed`
3. checkout the commit just before the target commit: `git checkout <the commit just before>`
4. "save" this to a branch : `git checkout -b base`
5. checkout the changes: `git checkout changed`
6. generate the nested commits to stdout, and add a temporary blank line to each
   module. release-please should clean up the blank lines later.
```
go run ./internal/actions/cmd/changefinder -q \
  -format=commit \
  -base=base \
  -commit-scope=fix \
  -commit-message="describe the change" \
  -touch
```
7. checkout main, new branch, commit:
```
git checkout main && \
  git checkout -b bump-modules && \
  git commit -a -m 'chore: bump changed modules' && \
  git push
```
8. create a PR in GitHub from your branch.
9. copy/paste the nested commit messages from stdout to the description of your
   GitHub PR.
