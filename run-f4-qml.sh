#!/usr/bin/env bash

set -euo pipefail

repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

# The QML frontend has one supported panel renderer. Keep the historical
# launcher name as a compatibility alias, but route it through the canonical
# integrated build so it cannot accidentally start a stale alternate host.
exec "$repo_dir/run-f4-gallery.sh" "$@"
