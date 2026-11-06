# macOS mounted volumes in the drive menu

**Date:** 2026-10-31
**Severity:** medium
**Files:** internal/sysinfo/mounts_darwin.go, mounts_darwin_test.go, mounts_other.go, mounts_linux.go, drives_unix.go

## Problem

Mounted removable drives did not appear in the main drive list on macOS.

## Root Cause

The shared Unix menu queried UserMounts, but the non-Linux implementation always
returned nil. macOS therefore had only Root and Home despite visible volumes.

## Solution

Read Getfsstat with MNT_NOWAIT on each menu opening, retrying a growing table.
Include mounts under /Volumes except MNT_DONTBROWSE entries, deduplicate paths,
and preserve the native device, path and filesystem strings without shell parsing.
Keep unmount capability Linux-only. Diagnostics use [FIX:drives].

## Verification

Before the fix a live regression failed for Yandex, PMHOME, Untitled and
Untitled 1. It passes after the fix. Deterministic fixtures cover Unicode,
spaces, read-only and network mounts, system exclusions, duplicates and removal.

## Prevention

Test Unix platform stubs against real mount tables and keep navigation separate
from optional destructive device operations.

## Tags

`#macos` `#mounts` `#drives` `#go`
