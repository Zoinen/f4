# Device-qualified public paths

## Regression

iOS and Android mounts exposed native paths without the device namespace.
The iOS regression expected `ios://iPhone/DCIM/100APPLE` but received
`/DCIM/100APPLE`. QML editable-path navigation also bypassed URI restoration.

## Fix

Use the shared `vfs.DevicePath` codec at the transport boundary. Public paths
carry scheme and discovery-resolved device name; native operations retain
export-relative POSIX paths and stable serial/UDID session keys. Register URI
providers for both device plugins, reject ambiguous authorities, preserve
case-sensitive device identities, and route QML URI navigation through the
existing general navigation lifecycle. Do not apply local path helpers to
qualified paths or double-unescape literal filenames.

## Verification

Regression tests cover escaping, foreign devices/exports, duplicate names,
URI restoration, transport paths, and semantic navigation. The opt-in real
iPhone integration test passed using qualified paths for its unique temporary
folder and file operations; cleanup completed. Android and FISH+ unit tests
passed. Native service details remain hidden behind existing VFS interfaces.
