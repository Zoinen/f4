# macOS information panel data parity

**Date:** 2026-10-13
**Severity:** medium
**Files:** internal/sysinfo/{native_darwin,mem_darwin,fs_darwin,mem_other}.go

## Problem
Ctrl+L omitted physical memory, swap and volume metadata on macOS.

## Root Cause
Darwin shared the unsupported-platform memory stub. Its filesystem collector
used only statfs, which does not expose a volume name, UUID or name limit.

## Solution
Use cgo-free read-only libSystem bindings for Mach VM statistics and getattrlist,
and x/sys sysctl/pathconf for capacity, swap and filename limits. Collect volume
attributes independently so unavailable fields do not suppress disk capacity.
Parse signed relative volume-name references with bounds checks and respect
volume capability validity bits. Release the Mach host send right after every
sample. Dynamic memory values are never cached. Diagnostics use VTUI_DEBUG and
[FIX:darwin-info]. Serial displays the volume UUID on macOS.

## Verification
Live API regressions failed before implementation (Mem unavailable, empty volume
identity/flags and zero filename limit) and passed after. Unit tests cover memory
page accounting, clamping, malformed relative strings and mount flag names.
RAM availability includes free and inactive pages, without double counting
speculative pages. See Apple's vm_statistics64 definition:
https://github.com/apple-oss-distributions/xnu/blob/main/osfmk/mach/vm_statistics.h

## Prevention
Test collectors through live OS calls in addition to synthetic buffer parsers.
Further coverage should include external case-sensitive and read-only volumes.

## Tags
`#darwin` `#sysinfo` `#memory` `#filesystem` `#ffi` `#cgo-free`
