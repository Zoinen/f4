#!/bin/bash

# Every path below is relative to the repository root, so go there rather
# than requiring the caller to be standing in it.
cd "$(dirname "$0")/.." || exit 1

OUTPUT="docs/FILELIST.md"

# The list is built from `git ls-files`, not from a walk of the working
# directory. A walk has to be told what to leave out — build output, editor and
# tool state, caches — and that exclusion list goes stale silently: it says
# nothing when a new ignored directory appears, it just lists it. What git
# tracks is the same question this file claims to answer, and it needs no list.
if ! files=$(git ls-files); then
    echo "not a git repository, or git is unavailable" >&2
    exit 1
fi

{
    echo "# Project Structure"
    echo ""
    echo "Every file tracked in the repository. Regenerate with"
    echo "\`scripts/filelist_update.sh\` after adding or moving files."
    echo ""
    if command -v tree &> /dev/null && tree --fromfile . </dev/null &> /dev/null; then
        printf '%s\n' "$files" | grep -vFx "$OUTPUT" | tree --fromfile . | sed 's/^/    /'
    else
        printf '%s\n' "$files" | grep -vFx "$OUTPUT" | sed 's|^|    ./|'
    fi
} > "$OUTPUT"

echo "File list updated in $OUTPUT"
