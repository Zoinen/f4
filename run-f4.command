#!/bin/bash
set -euo pipefail

# Run the matching local Go core and Qt frontend from this checkout.
app_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
app_binary="$app_dir/f4-zoin"
qt_host="$app_dir/qt/host/build-dev/bin/Release/f4-qt-host.app/Contents/MacOS/f4-qt-host"

for executable in "$app_binary" "$qt_host"; do
    if [[ ! -x "$executable" ]]; then
        printf 'Cannot launch f4: executable not found: %s\n' "$executable" >&2
        exit 1
    fi
done

cd "$app_dir"
export F4_EXT_UI_PATH="$qt_host"
qt_runtime="$app_dir/.codex-build/dependencies/qt-color-space"
if [[ -d "$qt_runtime/plugins" && -d "$qt_runtime/qml" ]]; then
    export QT_PLUGIN_PATH="$qt_runtime/plugins"
    export QML_IMPORT_PATH="$qt_runtime/qml"
fi
exec "$app_binary" --gui=qt --attached "$@"
