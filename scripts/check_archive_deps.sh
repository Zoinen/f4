#!/bin/bash

# Checks the archive libraries f4 is built with, as described in
# docs/ARCHIVE_DEPENDENCIES.md. It prints the version of each one and a
# PROBLEM line for every broken rule:
#
#   - a library pinned to a commit (a pseudo-version) instead of a tag;
#   - a replace directive for one of the libraries in f4's go.mod;
#   - a library that asks for a different version of another library in the
#     chain than the one f4 is built with.
#
# Exit status: 0 when there is no problem, 1 otherwise.

cd "$(dirname "$0")/.." || exit 1

# The libraries of the chain, as a regular expression over module paths.
chain='^github[.]com/(unxed/(zipper|zip|tar|xz|sevenzip|archives|par2|zipcharset|zlib4go|localecp)|nwaples/rardecode/v2)$'

# A pseudo-version ends in a 14-digit time and a 12-digit commit hash:
# v0.0.0-20260925074956-1fd8e92986d5, v0.1.140-0.20260924053511-cd3c7523a2c8.
pseudo='[0-9]{14}-[0-9a-f]{12}$'

if ! all=$(go list -m -f '{{.Path}} {{.Version}}' all); then
    echo "go list failed" >&2
    exit 1
fi
selected=$(echo "$all" | awk -v re="$chain" '$1 ~ re')

echo "f4 is built with:"
echo "$selected" | sed 's/^/  /'
echo

problems=0

while read -r path version; do
    if [[ $version =~ $pseudo ]]; then
        echo "PROBLEM: f4 builds $path from a commit ($version), not from a tag"
        problems=$((problems + 1))
    fi
done <<< "$selected"

if ! replaced=$(go list -m -f '{{if .Replace}}{{.Path}}{{end}}' all); then
    echo "go list failed" >&2
    exit 1
fi
for path in $(echo "$replaced" | awk -v re="$chain" '$1 ~ re'); do
    echo "PROBLEM: go.mod has a replace directive for $path"
    problems=$((problems + 1))
done

if ! graph=$(go mod graph); then
    echo "go mod graph failed" >&2
    exit 1
fi
# Each line of the graph is "module@version requirement@version". Only the
# versions f4 is built with matter: older versions in the graph are not used.
mismatches=$(awk -v re="$chain" '
    NR == FNR { built[$1] = $2; next }
    {
        split($1, from, "@")
        split($2, to, "@")
        if (from[1] !~ re || to[1] !~ re) next
        if (built[from[1]] != from[2]) next
        if (to[2] != built[to[1]]) {
            print "PROBLEM: " from[1] "@" from[2] " asks for " to[1] "@" to[2] \
                ", but f4 is built with " built[to[1]]
        }
    }' <(echo "$selected") <(echo "$graph"))
if [ -n "$mismatches" ]; then
    echo "$mismatches"
    problems=$((problems + $(echo "$mismatches" | wc -l)))
fi

if [ "$problems" -gt 0 ]; then
    echo
    echo "$problems problem(s). See docs/ARCHIVE_DEPENDENCIES.md for how to fix them."
    exit 1
fi
echo "OK: every library asks for the versions f4 is built with, all by tag."
