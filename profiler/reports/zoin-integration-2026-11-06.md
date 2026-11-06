# Integration into zoin — 2026-11-06

## Revisions and conflict decisions

F4 merges feature checkpoint `2f64b38aa` into `zoin` at `5058b374e`.
ZoinGallery merges checkpoint `0542874` into `zoin/f4-integration` at
`04487cd`; the resulting gallery merge is `ce8064d`.

Keep upstream's shared file-group/icon factories instead of the feature's
obsolete manifest matcher. Preserve native icon resolution and transparent
padding trimming. Retained fallback hosts honor upstream's tint policy and
full opacity; update the drawing-gate test accordingly. The prepared-icon
resource test resolves canonical chrome aliases and trims CRLF manifest lines.

Keep upstream precise touchpad input, bounded overscroll and 150 ms wheel
animation, together with retained motion controllers and scene visibility
gates. Merge both Masonry overscroll and scroll-animation properties.

## Verification on the merged sources

- Fresh Release static Qt host and selected test executables built successfully.
- Go editor, chroma, navtrace, nativeui and dirwatch suites passed; selected
  panel/plughost navigation and listing regressions passed. Plain Go build passed.
- CI Python tests: 93 tests, one skipped, no failures.
- Icon provider: 40 passed, two platform/render-environment skips, no failures.
- Native editor 175% regression selection: nine passed, no failures or skips.
  Checked the captured synthetic editor image, including its small status text.
- QML import isolation passed. Windows GUI-subsystem check passed. Portable
  DLL audit passed: application libraries and MSVC runtime remain static.
- Native gallery selection: 51 passed initially; one obsolete opacity assertion
  failed. After updating it to upstream's full opacity, its rerun passed
  (three including setup/cleanup). Selection covers opaque PNG rendering,
  shared fallback factories, vector source admission and vector resizing.
- Gesture selection exited successfully but reports three passed and ten
  environment skips. This is not evidence that skipped touchpad cases ran.
- Embedded payload regenerated; tagged payload/materialization/host-preference
  Go tests passed. Worktree-specific embedded executable built and launched
  directly with `--gui=qt`; observed the `zoin` header and populated Masonry view.

## Limits and existing failures

This is an integration check, not another exhaustive performance study or a
claim of 160 Hz across all workloads. Earlier checkpoint failures and mixed-DPI
limits remain documented in `production-checkpoint-validation-2026-11-06.md`.

The archive consistency script reports 12 dependency mismatches. Archive
requirements/replacements are unchanged relative to the target `zoin` commit;
the only go.mod delta is the checkpoint's test-related indirect WinTUN entry.
Do not interpret the failed archive check as a passed release gate.

Raw logs, captures, generated embedded payload and executables remain local.
Original feature checkout and its running application were preserved.
