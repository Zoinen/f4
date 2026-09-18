# Immediate native list cursor during key repeat

**Date:** 2026-09-18
**Severity:** medium
**Files:** qt/host/qml/SemanticWidgetDelegate.qml, qt/host/tests/F4OperationsQueueTests.cpp, plugins/envman/manager_semantic_test.go

## Problem

Holding an arrow in the profile list made the cursor appear faint and delayed.

## Root Cause

A 70 ms ColorAnimation applied to selection as well as hover. Keyboard repeat
changed rows before their highlight finished appearing. A 60-update regression
retained every observed row and footer object, but all 60 selected colors lagged.
There was no evidence of delegate rebuilding in this reproduction.

## Solution

Remove the list-row color animation so authoritative selection appears immediately.
Keep existing models and protocol. The Go test exercises 200 arrow events, console
draws and semantic serialization with 40 profiles; list presentation stays unchanged.
Measured p95 was about 0.5 ms and mean dialog JSON was 5305 bytes. Native replay
took 92 ms before and 87 ms after for 60 updates; the fix targets highlight latency,
not a claim of major throughput improvement or an end-to-end live-app benchmark.

## Prevention

Test visual selection immediately after each acknowledged cursor update, faster
than the animation duration, and preserve delegate identity during snapshot replay.
Avoid fades for keyboard cursors in lists that support held-arrow navigation.

## Tags

`#qml` `#animation` `#keyboard-repeat` `#envman`
