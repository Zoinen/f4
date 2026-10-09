#!/usr/bin/env bash
# Usage: check_release_version.sh <command that runs f4 --version>...
#
# Runs the command and prints what it printed. On a release tag it then fails
# unless the version is exactly that tag.
#
# The tag reaches the binary through -X "$VERSION_SYMBOL=<tag>" (see the env
# block of .github/workflows/build.yml), and the linker skips an -X whose name
# resolves to nothing without a word: the build passes, the binary runs, and
# it reports a commit hash instead of the release, which the updater then
# offers as an update to itself. Only the output shows it, so the output is
# what gets checked.
set -euo pipefail

out=$("$@")
printf '%s\n' "$out"

case "${GITHUB_REF:-}" in
  refs/tags/v*) ;;
  *) exit 0 ;;
esac

# "--version" prints "<version> [<build time>]".
read -r got _ <<<"$out" || true
if [ "$got" != "${GITHUB_REF_NAME:-}" ]; then
  echo "::error::the binary reports version '$got', the tag is '${GITHUB_REF_NAME:-}'" >&2
  exit 1
fi
