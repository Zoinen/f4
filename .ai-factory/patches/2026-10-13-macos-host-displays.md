# macOS host and display information

## Problem
The general Ctrl+L section omitted model, chip, operating system, architecture
and displays in both console and Qt.

## Solution
Collect system_profiler JSON with a bounded, cancellable process off the UI
thread; share the cached snapshot and row visitor across presentations. Refresh
at most once per 30 seconds. Host and VFS-provider tasks have separate lifetimes;
resetting an absent VFS provider must not cancel local hardware collection.
Use physical display pixels, not Retina desktop points. Read approximate screen
dimensions with CGDisplayScreenSize and label them explicitly as approximate.
Do not hardcode model mappings, display diagonals or OS marketing codenames.
Probe hardware ARM capability instead of the executable architecture (Rosetta).

## Verification
New collector tests first failed because the API was absent. Live collection
returns MacBook Pro (Mac17,9), Apple M5 Pro, macOS 27.0.1 (26A434), ARM64 and
Color LCD 3024x1964, approximately 14.1 inches. Parser tests cover Retina physical
pixels, multiple displays, Intel CPU fallback, partial data and malformed JSON.
Semantic rows cover all fields and omission of unknown diagonals. Cancellation
has a regression test. Additional useful coverage: live monitor hotplug and
Rosetta execution on an Apple Silicon host.

Core Graphics dimensions can be estimated without EDID:
https://developer.apple.com/documentation/coregraphics/cgdisplayscreensize(_:)

Tags: #macos #sysinfo #displays #async #semantic-parity
