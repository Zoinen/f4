#!/usr/bin/env bash

set -euo pipefail

repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
f4_bin="$repo_dir/f4"

cd "$repo_dir"
go build -o "$f4_bin" ./cmd/f4

exec "$f4_bin" --gui=gogpu "$@"
