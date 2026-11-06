# Cached physical-pixel UI layout migration

Approved in conversation on 2026-10-17. Baseline checkpoint: parent dc35c5eef,
gallery 2dcba89. Implement on zoin_branch/cached-pixel-grid in both repositories.

## Contract

One shared per-window grid service; physical integer edges and cached local
geometry; dirty visible scopes only; no per-frame ancestor traversal/sorting.
Scroll through one shared fractional transform, fit it once when settling.
Preserve selected text renderer throughout motion (native pixel steps accepted).
Qt 6.11, no glyph patch, new setting, or Go protocol changes. Retain exact source
revision/cache/decode priorities. Preserve shaping, hit testing and selection.

## Tasks

- [x] Shared GridMetrics, PixelGridScope, dirty alignment context and host adapter;
      deterministic lifecycle/idle-work tests and real leaf 175% checks.
- [x] Masonry cached physical geometry/shared translation, continuous gesture
      ownership and unchanged text renderer; make wheel regression pass.
- [x] Grid, Icons, Details, Columns and previews/group headers use the same contract.
- [x] Shared controls, shell/tabs/keybar, breadcrumbs, split/status/header chrome.
- [x] Dialogs/settings/menus/help/queue and document/terminal rows, viewer chrome.
- [x] Remove obsolete walkers, update subsystem docs, run native/software and
      physical-grid tests; measure actual wheel/middle display cadence.
- [x] Portable audits/import/extraction checks, embed fresh host, build and restart
      only this worktree's own Qt app. Verify running child hash/branch identity.

## Verification

Actual leaves at 175%: whole physical origins, unit basis, rendered text/icons/
clip edges. Include odd dimensions, split, resize/font/DPR/parent changes, hidden
tabs, reuse, new leaves during motion, interruptions and nested overlays. Check
1/1.25/1.5/2 DPR too. Idle 200 unrelated frames: zero alignment visits/maps/sorts.
Retained scrolling: zero existing-leaf fitting/caption measurement/renderer change.
Target warm 160Hz cadence and wheel p95 <1ms; document cold rendering separately.
Opt-in diagnostic counters only; no unconditional hot-path log allocations.

## Progress

Implementation and portable restart completed. No automatic commits or push requested.

Evidence: profiler/reports/cached-pixel-grid-2026-10-17.md.
Wheel-handler p95 0.796 ms; retained motion has zero alignment visits/maps.
Median frame production is near 6.25 ms, but p95/p99 outliers remain; a strict
all-frame 160 Hz guarantee is not claimed. Selected suites pass; full-suite
baseline failures were not silently declared fixed.
