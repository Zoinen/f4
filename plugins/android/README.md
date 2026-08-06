# Android filesystem

The built-in **Android** drive exposes devices already known to the local ADB
server. Open the drive from the panel drive menu and select an online row such
as `Pixel 9 (emulator-5554)`. Offline and unauthorized devices stay visible,
but cannot be opened until ADB reports the `device` state.

## Requirements

- Android 7.0 / API 24 or newer;
- USB debugging enabled and the host key authorized on the device;
- Android SDK Platform Tools installed on the host.

f4 talks to the ADB smart socket at `127.0.0.1:5037` directly. If the server is
not running, it starts the installed `adb` executable. Executable discovery is,
in order: `F4_ADB_PATH`, `PATH`, `ANDROID_SDK_ROOT/platform-tools`, then
`ANDROID_HOME/platform-tools`. Platform Tools are not downloaded or bundled.
Wireless devices that have already been connected to the same ADB server are
listed in exactly the same way as USB devices.

## Backend selection

One backend is selected when a device row is opened and remains fixed for that
panel session:

1. f4 starts a raw ADB shell-v2 stream and uploads the FISH+ helper in one
   verified base64 command. If that Android userspace has no compatible
   decoder, it retries the portable line bootstrap on a fresh shell.
2. The FISH+ backend is accepted only when the device shell provides working
   listing, read, and write modes.
3. If the helper is incompatible with that Android userspace, f4 opens the ADB
   Sync backend instead.

FISH+ keeps its server-side search, line indexing, delta writes, scans, duplicate
search and command jobs whenever the device tools support them. Its job scratch
directory candidates include Android's shell-writable `/data/local/tmp`.
An established FISH+ shell is pooled per ADB serial for the lifetime of the
plugin. Leaving the device closes the panel view but keeps a parentless anchor;
entering it again validates the helper with one `noop` and reuses the same
session. Closing f4 releases all anchors.

The Sync fallback implements v1/v2 LIST, STAT/LSTAT, RECV and SEND, including
the `stat_v2`, `ls_v2`, and `sendrecv_v2` feature variants. Sequential reads are
streamed directly; the first random-access read materializes one temporary host
file. Directory creation, removal, rename, permissions, ownership and times use
quoted, unprivileged shell commands because the Sync protocol has no mutation
messages for them.

## Permissions and scope

The drive has exactly the access of the ordinary Android `shell` user. It never
runs `su`, requests `adb root`, remounts a partition, or bypasses scoped storage
and Android filesystem permissions. A permission error is returned as-is.

Pairing, `adb connect`, root/remount controls and hot-plug tracking are outside
this drive. Pair or connect a device with Platform Tools first, then refresh the
Android drive. Device discovery itself is refreshed on every panel refresh.

## Real-device test

The integration test is opt-in and mutates only a unique child of
`/data/local/tmp`:

```powershell
$env:F4_ADB_TEST_SERIAL = "your-device-serial"
go test -run TestADBDeviceIntegration -v ./plugins/android
```

Without `F4_ADB_TEST_SERIAL`, the test is skipped.

Set `F4_ADB_TEST_FISH=1` to require FISH+ instead of allowing the Sync
fallback. This mode also verifies the negotiated backend, positioned writes
that preserve the existing tail, and reads through symbolic links:

```powershell
$env:F4_ADB_TEST_FISH = "1"
go test -run TestADBDeviceIntegration -v ./plugins/android
```

The user-facing workflow follows the device-list model of
[FarDroid](https://github.com/dimfishr/FarDroid). The implementation is an
independent Go client built against the documented ADB smart-socket, shell-v2,
and Sync protocols and shares no FarDroid source code.
