# Folder preview restoration in the first frame

Status: implemented, verified and delivered; the real 4A-route p95 remains above the 50 ms target. Source: user-approved plan, 2026-09-16.
Docs: update the existing Qt host documentation. Keep this worktree and its existing changes.

## Contract

- Shared 8 MiB RAM snapshots keyed by directory source identity, independent of child models and leases.
- Unknown / Empty / HasImages state, selected children, dimensions and existing pixel-cache keys.
- Restore all incoming rows before catalog publication; no disk or VFS on a RAM hit.
- Paint retained folder appearance from the visual snapshot before deferred ImageFile creation.
- Keep original ZoinGallery design, physical-pixel alignment and square outer geometry.
- Refresh in the background using existing authority and worker limits, with completion-driven dispatch.
- Empty success clears; failure retains; unrelated source replacement cannot inherit another snapshot.
- Existing disk formats and settings remain unchanged.

## Tasks

- [x] 1. Add correlated instrumentation, capture 20 baseline cycles on each real route, and reproduce the first-frame regression.
- [x] 2. Add the bounded shared snapshot cache and atomic catalog restoration; cover ownership, eviction and invalidation.
- [x] 3. Connect snapshot presentation and stable child lookup; remove polling from directory admission.
- [x] 4. Verify 38-folder first frames, mode changes, scrolling, failures, paging and 175% leaves/rendering; repeat real-route profiles. The real 4A-route frame-time target remains above 50 ms; see results below.
- [x] 5. Update docs and pinned Gallery revision, rebuild the static payload and worktree binary, then launch --gui=qt.

## Acceptance

Two real routes, 20 measured cycles each: P:\\ <-> P:\\[Year 2026], and P:\\[Year 2026] <-> P:\\[Year 2026]\\[2026] 4A.
Warm restoration of 38 states: p95 <= 2 ms. Known visible folder appearance: first frame after catalog publication, p95 <= 50 ms.
Blocked VFS and deferred ImageFile creation must not postpone known appearance.
Every affected text/image leaf must have an objectName and pass scene-origin and unit-transform checks at DPR 1.75, plus rendered-image inspection.

## Recorded baseline

- Both isolated real-route runs completed 20 measured cycles and 2 warmups. Traces: `.diagnostics/folder-first-frame-before-{year,4a}-{nav,media,go}.jsonl`.
- From navigation dispatch to last visible folder appearance: root -> Year p95 407.4 ms; 4A -> Year p95 279.1 ms. These measurements do not establish a full one-second delay.
- Failing regression at DPR 1.75: first frame restores 0/38 appearances with 0 ImageFile facades; `.diagnostics/folder-first-frame-failing.txt`.

## Verification so far

- Twenty warm first-frame regressions restore 38/38 appearances with zero ImageFile facades and blocked VFS. Evicted models restore identical geometry and pixel URLs without decoding again.
- Recycled delegates retain the lightweight frame while releasing their grid. The added regression first failed on the destroyed frame, then passed with the retained shell; its DPR 1.75 first-frame p95 is 32.9 ms for all 38 folders.
- GallerySession, MasonryLayoutModes, GalleryLayoutEngine, QtMediaClient and GalleryPanelController suites pass. The controller test target needed the same static Qt platform imports as the existing core test targets.
- MasonryVisualDpr175Test and compiled static-QML smoke pass; captures for Masonry, Grid and Icons were inspected.
- Relevant Go directory/media broker and panel authority checks pass.
- Full F4GalleryBridge: 55 pass, 3 fail, 1 skip. Failures are the previously established baseline cases: panelCatalogPatchLeavesOtherSessionUntouched, inactiveGalleryDoesNotStealFocus, viewerOwnsEscapeAndZoom. Paging coverage passes.

## Final measured results

The final host completed 20 measured cycles plus two warmups for each route in an
isolated profile. No build or test ran concurrently. Full navigation/media tracing
was enabled. Traces and analyses: `.diagnostics/folder-first-frame-verified-*`.

| Measurement | P root -> Year 2026 | 4A -> Year 2026 |
| --- | ---: | ---: |
| Last visible appearance from navigation dispatch, baseline p95 | 407.4 ms | 279.1 ms |
| Last visible appearance from navigation dispatch, final p95 | 36.7 ms | 49.6 ms |
| Publication to presented first frame, final p95 | 43.0 ms | 64.3 ms |
| RAM state lookup of the 38 incoming folders, p95 | 0.052 ms | 0.025 ms |
| Visible appearances present in first frame | 33/33, all 20 cycles | 33/33, all 20 cycles |

The 50 ms presented-frame target passes on the root route but is **not achieved**
on the 4A route. The 4A median is 43.8 ms; its p95 includes GUI commit/render work
after RAM restoration. Do not describe this as a cache miss or claim the complete
frame-time target passed. Actual visibility is 33 folders at this viewport and
scroll position; the separate regression exercises all 38 at once.

Across the measured warm cycles: zero pixel decodes, zero disk metadata batches,
and 12,927/12,927 ready-image provider requests hit RAM (median 0.014–0.015 ms).
Background enumeration/resolution still validates freshness after restoration.

The all-38 blocked-VFS/deferred-ImageFile test passes, with a 23.1 ms first-sync
p95 in its focused run. A second regression captured activation at the wrong
197x150 geometry before the 150x150 square was installed; it now checks atomic
geometry/appearance activation. Failing evidence is in
`.diagnostics/folder-geometry-before.txt`.

Final full CTest run: six relevant suites passed (88.2 seconds), including
MasonryLayoutModes and MasonryVisualDpr175. Static QML smoke passed on the freshly
built host; all three final rendered captures were inspected. Go media/panel
regressions and the complete semantic/extui contract suites passed. Full bridge
results remain the same three known baseline failures, with no added failures.

## Delivery

- Gallery pinned to `7c2083f6c05ea0c124bd61438a2e36cc4509bc0b`.
- Static Windows host imports audited: Windows system DLLs only.
- Compressed host regenerated; launcher built with `CGO_ENABLED=0`, the system
  Go build cache, and `f4_embedded_qt_host`. Embedded payload/extraction tests passed.
- Only this worktree's `D:/Code/f4-zoin/f4-zoin.exe` was replaced. Its previous
  executable was backed up. The canonical `D:/Code/f4/f4.exe` remained untouched.
- Direct `--gui=qt` launch verified: Go PID 30132, Qt PID 70992, window title
  `f4 [zoin]`. The extracted host's SHA256 matches the tested static host:
  `715645899b10fd91d120a697410dd56d948cd0d5171079ee249bd02f17cc44dc`.
- Launcher SHA256:
  `bd2d5f40ee7dce22e2eef2f769abcccfc32f3e0f359a434ba0f97820422f60ba`.
- An initial launch-command issue was corrected: on this PowerShell/.NET build,
  `SetEnvironmentVariable(name, $null, 'Process')` exports an empty value into
  child processes, which Qt still treats as a set smoke-only flag. A direct
  launch from the normal environment succeeds; no product change was required.
