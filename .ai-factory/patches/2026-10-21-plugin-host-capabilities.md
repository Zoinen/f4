# Preserve optional host APIs during plugin loading

**Date:** 2026-10-21
**Severity:** high
**Files:** internal/plughost/manager.go, host.go, manager_coverage_test.go

## Problem

Environment Manager and other contribution-based plugins disappeared from F11.
They were compiled into the binary but failed initialization.

## Root Cause

The deferred URI host wrapped the host behind the narrow vfs.HostAPI interface.
Embedding that interface does not promote the concrete host's optional methods.
Plugin assertions for ContributionHost, ProcessEnvironmentHost and settings
therefore failed. RPC command/panel registration had the same exposure problem.

## Solution

Pass the original host to in-process plugin initialization. Keep the decorator
for RPC/WASM initialization but unwrap it for session contributions and callback
handlers; register drives through the decorator as before. Do not synthesize
unsupported optional capabilities or weaken plugin capability checks.

## Verification

The host-identity regression test failed before the fix and passes after it.
A real Environment Manager instance now initializes against an isolated fake
host, publishing both commands and its Settings provider without changing the
process environment. The plughost and envman suites pass with CGO_ENABLED=0.

## Prevention

Audit optional interface assertions whenever decorating a host interface. Retain
the original object for consumers that need capabilities beyond the base API.

## Icon investigation

No icon-rendering changes are included. GalleryEntryDelegateBase selects the
cursor/marked foreground for tinted icons; GalleryEntryPreview additionally uses
0.78 opacity outside Details/Columns. This explains the faded cursor appearance
independently of plugin initialization.

## Tags

`#go` `#plugins` `#interfaces` `#decorator`
