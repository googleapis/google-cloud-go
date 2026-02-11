#!/bin/sh

# Script to facilitate syncrhonization from the main to the preview branch.

git checkout main -- \
    .gemini \
    .github/workflows/action_syntax.yml \
    .github/workflows/vet.yml \
    .github/workflows/vet.sh \
    .github/CODEOWNERS \
    .github/header-checker-lint.yml \
    internal/godocfx \
    internal/kokoro \
    internal/testutil \
    internal/uid \
    internal/README.md \
    internal/version.go \
    third_party \
    .gitignore \
    CODE_OF_CONDUCT.md \
    CONTRIBUTING.md \
    debug.md \
    doc.go \
    LICENSE \
    README.md \
    SECURITY.md

sed -i '1c\'"$(git show origin/main:.librarian/state.yaml | head -n 1)" .librarian/state.yaml
