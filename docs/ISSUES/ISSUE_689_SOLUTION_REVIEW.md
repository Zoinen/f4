# Issue #689 — solution review

## Problem model

The editor's `fadeSyntax` path blends RGB syntax attributes from the base
editor colour over 400 ms and registers a frame-heartbeat animation. This is
separate from background syntax parsing: the latter remains useful for large
files, while the fade only delays the final colours and creates the reported
visual animation when a file is opened.

## Three candidate fixes

### A — Remove the fade implementation

Delete the interpolation and heartbeat. This is the smallest runtime change,
but removes the behavior for users who prefer it and leaves no way to restore
it without rebuilding f4.

### B — Add a persistent editor setting (selected)

Add `Editor.SyntaxAnimation` to `settings.ini` and the Editor Settings dialog.
The default is off, so opening a file no longer animates syntax highlighting;
setting it to `1` preserves the existing fade for users who want it. The
background highlighter and its incremental work budget are unchanged.

### C — Make the duration configurable

Expose a numeric duration or terminal animation profile. This offers more
control, but introduces validation and UI complexity for a preference whose
reported desired state is simply disabled. It also makes timing behavior less
predictable across terminals.

## Three-pass review

1. **Correctness:** `fadeSyntax` returns the computed attributes directly when
   `EditorSyntaxAnimation` is false. It therefore does not allocate a fade
   buffer, start a timer, or register a heartbeat. Enabling the setting keeps
   the existing RGB interpolation path; indexed colours retain their current
   behavior.
2. **Adversarial cases:** The setting is read from the same `[Editor]` section
   as the highlighter configuration, survives Save/Load, and is reachable in
   the existing Editor Settings dialog. Missing keys default to `0`, so old
   configurations get the non-animated behavior. The setting affects only the
   visual fade, not parsing, syntax engine selection, or background catch-up.
3. **Regression/scope:** Tests cover the disabled fast path, config round-trip,
   editor-dialog layout, and the existing editor/highlighter suite. A native
   Linux ARM64 smoke run will open a syntax-highlighted file with the default
   setting and verify that no fade heartbeat is registered; a second run with
   the setting enabled will verify that the legacy animation path remains
   available.

## Decision

Use B. It fixes the reported default behavior while preserving an explicit
opt-in for the previous visual effect and avoids changing the asynchronous
syntax-highlighting algorithm.
