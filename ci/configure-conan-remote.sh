#!/usr/bin/env bash
set -euo pipefail

# The Artifactory virtual repository is deliberately optional.  This keeps
# local builds and forked pull requests working when the repository variables
# or credentials are not present, while making the remote the first source in
# the CI jobs that have it configured.
remote_url="${F4_CONAN_REMOTE_URL:-}"
if [[ -z "${remote_url}" ]]; then
    echo "Conan binary remote is not configured; using the existing remotes"
    exit 0
fi

remote_name="${F4_CONAN_REMOTE_NAME:-f4-conan}"
conan remote add "${remote_name}" "${remote_url}" --index 0 --force

if [[ -n "${F4_CONAN_TOKEN:-}" ]]; then
    conan remote login "${remote_name}" "${F4_CONAN_USERNAME:-admin}" \
        --password "${F4_CONAN_TOKEN}"
else
    echo "Conan binary remote configured without credentials (read-only mode)"
fi
