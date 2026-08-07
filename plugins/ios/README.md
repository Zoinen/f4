# iPhone filesystem

The built-in **iOS** drive discovers iPhone and iPad devices through the
platform Apple mobile-device service. Open `iOS` from the panel drive menu,
select a trusted device, and choose one of the capability rows:

- **Media** is the device's writable AFC media export. Reads and writes stream
  directly, and random-access reads use native AFC seek operations.
- **Applications** lists user applications. f4 first requests the complete
  developer container or the Documents export with House Arrest. iOS decides
  which applications are accessible; f4 does not bypass that decision.
- **App Groups** uses the iOS 17.4+ CoreDevice FileService and is read-only.
- **Crash Reports** uses the classic crash-report AFC service when available,
  with a read-only CoreDevice fallback.

This is not access to the device root filesystem. A stock iPhone exposes only
Apple-defined service domains, and application access remains subject to code
signing, File Sharing, pairing, and sandbox policy.

## Requirements

- Go 1.26 or newer when building f4 from source;
- a device that has trusted this computer and is unlocked while a new service
  connection is established;
- macOS: the built-in Apple mobile-device service;
- Linux: a running `usbmuxd` service with USB permissions for the current user;
- Windows: Apple Mobile Device Support, installed by Apple Devices or iTunes.

Developer Mode is shown in the information panel but is not required for the
Media export. CoreDevice application and group access requires iOS 17.4 or
newer and a Developer Disk Image. f4 establishes its own unprivileged
userspace tunnel and mounts an already installed Developer Disk Image itself.
On macOS it first uses Xcode's system image. On other systems, or when that
image is absent, the first CoreDevice access downloads and caches the universal
image used by go-ios. Set `F4_IOS_DEVELOPER_IMAGE` to an existing Restore
directory to override that location. f4 does not unmount a shared image when it
exits, because Xcode or another debugger may still be using it.

No `devicectl`, `ifuse`, `go-ios`, `ios tunnel` daemon, kernel TUN interface, or
separately installed command-line utility is required. On iOS 26.5.x Apple has
a device-side FileService regression that rejects container paths (Apple's
`devicectl` is affected for app groups as well); f4 leaves **App Groups**
visible but disables entry and skips CoreDevice application fallback on that
OS line.

The currently documented FileService directory response contains names but no
portable size, timestamp, or symlink metadata. CoreDevice views therefore use
directory probes for navigation and materialize an opened file into a private
temporary file for accurate random access. AFC and House Arrest views retain
their full native metadata and do not need this fallback.

USB is preferred when the same UDID is visible over both USB and Wi-Fi. One AFC
metadata connection and a small transfer pool are reused per mounted domain;
idle sessions are bounded and released when f4 exits.

## Real-device test

The integration test is opt-in. It mutates only a unique temporary child of the
Media export and removes that child during cleanup:

```powershell
$env:F4_IOS_TEST_UDID = "your-device-udid"
go test -run TestIOSDeviceIntegration -v ./plugins/ios
```

Without `F4_IOS_TEST_UDID`, the test is skipped. To additionally open one
application container read-only, set its exact bundle identifier:

```powershell
$env:F4_IOS_TEST_APP_BUNDLE = "com.example.app"
go test -run TestIOSDeviceIntegration -v ./plugins/ios
```

To exercise the embedded iOS 17.4+ CoreDevice userspace tunnel directly, set
`F4_IOS_TEST_CORE_BUNDLE` to an installed application bundle identifier. This
probe only lists the exported root:

```powershell
$env:F4_IOS_TEST_CORE_BUNDLE = "com.example.app"
go test -run TestIOSDeviceIntegration -v ./plugins/ios
```

Alternatively, set `F4_IOS_TEST_CORE_CRASH=1` to list the system crash-log
domain through CoreDevice instead of an application domain.

The transport and CoreDevice integration use the MIT-licensed
[go-ios](https://github.com/danielpaulus/go-ios) Go module. The AFC client in
this directory is embedded and protocol-native; no external executable is
launched. See [LICENSE.go-ios](LICENSE.go-ios) for the upstream notice.
