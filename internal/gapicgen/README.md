# gapicgen

gapicgen contains three binaries:

- `cloud.google.com/go/internal/gapicgen/cmd/genlocal`: Generates
  genproto+gapics locally. Intended to be run by humans - for example, when
  testing new changes, or adding a new gapic, or generating from
  googleapis-private.
- `cloud.google.com/go/internal/gapicgen/cmd/genbot`: Generates genproto+gapics
  locally, and creates CLs/PRs for them and assigns to the appropriate folks.
  Intended to be run periodically as a bot, but humans can use it too.
- `cloud.google.com/go/internal/gapicgen/cmd/genmgr`: Checks for an outstanding
  gapic regen CL that needs to have reviewers added and go.mod update, and then
  does so. Intended to be run periodically as a bot, but humans can use it too.

See the README.md in each folder for more specific instructions.
