# Shared thumbnail factories and nearby-preparation experiment

## Result

Three thumbnail factories (vector, shader bitmap and compact bitmap) now belong
to each panel instead of each preview row. Their renderer instances still belong
to their row hosts. Cross-row rebinding/lifetime, actual pixel placement and
rendered-source regressions pass. A matched 293-row census records row-local
preview factories falling from 2051 to 1172: seven to four per row, with twelve
new shared factories across four panels. The normal portable worktree app is
rebuilt and running.

Clean display results remain mixed and retain missed refresh intervals. No
reliable cadence gain or GC-pause reduction is claimed. Constant 160 Hz is not
achieved. A separate experiment disabling nearby preparation did not help and
was not promoted.

## Rejected preparation ablation

An artifact-only copy of MasonryLayoutDelegateCommit.cpp adds a startup flag
return to scheduleNearbyPreparation. Visible commits, thumbnail plans and normal
retention logic remain intact. One diagnostic executable is used for both flag
states, with a fresh process per capture. Its link map resolves the method to
the direct replacement object, without duplicate-symbol overrides. The normal
core source and packaged executable do not contain this ablation.

Downloads, 186 entries, density 152, extent 4101.142857, active 160 Hz/DPR 1.75,
Direct3D 11 and 30 ms actual wheel input match all valid captures. No profiler,
WPR, UI Automation observer, census, role observer, build or graphics test runs
inside clean measurement intervals. Caption measurements are zero. Strict
display intervals require both endpoints inside the direction window.

| Run | Nearby prep | Down p99/max ms | Down >9.375 | Up p99/max ms | Up >9.375 |
| --- | --- | ---: | ---: | ---: | ---: |
| A1 | normal | 6.3519 / 12.5146 | 3 | 6.2938 / 12.4617 | 1 |
| B1 replacement | disabled | 6.3483 / 12.5064 | 4 | 6.2919 / 12.4716 | 2 |
| B2 | disabled | 6.3444 / 12.5485 | 5 | 6.2770 / 12.4900 | 1 |
| A2 | normal | 6.3250 / 12.5005 | 2 | 6.3210 / 12.4883 | 1 |
| A3 | normal | 6.3344 / 18.7295 | 5 | 6.3030 / 12.4933 | 1 |
| B3 | disabled | 6.3578 / 12.5432 | 4 | 6.2961 / 12.4816 | 1 |

The disabled runs have seventeen long intervals versus thirteen controls. This
does not establish a performance benefit; the sample also does not isolate a
statistically reliable regression. The original B1 had a command-file reset
sharing failure before launch and was excluded because stale command replay was
possible. B1 replacement was collected after the other runs. The launcher now
retries only that IOException and fails if reset cannot complete.

Two separate post-capture censuses record 294 rows/29262 objects with normal
preparation and 288 rows/28897 objects with it disabled. Those are whole-engine
reachable QObject counts, not peak JS allocations or measured GC time. The small
row difference does not support disabling useful preparation as a solution for
the large retained graph. The first attempted census pair had an empty census
because its flag was missing from the driver environment; those snapshots are
excluded and replaced.

## Factory implementation and regression

GalleryEntryVisualFactories gains the three factories. A new typed
GalleryPreviewVisualHost carries each GalleryEntryPreview into its loaded item.
The existing preview-content Loader keeps its activity, deferred component
exchange and instance lifetime behavior. Factory roots read their own host's
preview; they cannot capture a particular row from the panel's context. The
four remaining fallback/icon/marker factories stay row-local in this change.
No geometry, sampling, source acquisition, animation, renderer/GC budget or
prefetch policy changes.

thumbnailFactoriesAreSharedWithoutCapturingPreviews first fails on the old code
because two rows' sourceComponent pointers differ. The fixed regression checks:
all five modes use the correct shared shader/compact factory; instances remain
distinct and bind to their own entry; replacing one row's source leaves its
sibling unchanged; vector instances share the factory and retain independent
preview owners; deleting one row preserves the sibling/factory; destroying the
panel destroys its factory.

The fixture waits for deferred component exchange rather than accepting the old
mode's renderer merely because its source URL already matches. It checks loaded
bitmap status and actual named image/shader leaves. An initial pre-render Grid
check caught physical coordinate 38.5. After waiting for three actual rendered
frames, the cached alignment pass finishes and all origins/unit transforms pass
at DPR 1.75. No production rounding was added to hide this pre-polish state.

Direct3D and OpenGL each pass 36 Gallery cases, eleven mode cases and four
resource-only import/shader cases. These include SVG failure/cancellation,
surface reuse, recycling, hidden fallback work, all-mode SVG pixels/alignment,
folder previews, pending thumbnail presentation and resting alignment. New
bitmap captures and existing real-panel vector/caption captures were inspected.
No new TypeError, ReferenceError, binding-loop or invalid-assignment messages
appear in the selected Gallery/import logs; existing font/PinchArea warnings
remain.

## Counts and clean scrolling

Separate native censuses run after measurement has stopped and frames are
copied. Both use 293 retained rows across the engine:

| Count | Before | After |
| --- | ---: | ---: |
| GalleryEntryPreview-origin Components | 2051 (7/row) | 1172 (4/row) |
| Panel factory-origin Components | 24 (6/panel) | 36 (9/panel) |
| All QQmlComponents | 2276 | 1411 |
| Reachable QObjects | 29158 | 28333 |

The per-row factory change is directly proved by pointers and normalized
counts. Other class/lifetime differences mean the entire object-total delta
must not be attributed to this change. The census is not a complete JS heap
walk and does not establish a GC-duration improvement.

Four clean fresh-process A1/B1/B2/A2 runs compare the current hidden-debug-URL
fix with the shared-thumbnail version. They retain normal nearby preparation,
original QtQml library, combined renderer, normal GC budget and the same
160 Hz/DPR 1.75/186-entry/30 ms wheel contract. No census or other listed
observer runs during these four clean intervals.

| Run | Down p99/max ms | Down >9.375 | Up p99/max ms | Up >9.375 |
| --- | ---: | ---: | ---: | ---: |
| A1 | 6.3226 / 18.7583 | 2 | 6.3085 / 12.5121 | 1 |
| B1 | 6.3132 / 12.5149 | 4 | 6.2975 / 12.4904 | 1 |
| B2 | 6.3388 / 12.5017 | 3 | 6.2911 / 12.4798 | 1 |
| A2 | 12.4948 / 24.9687 | 11 | 12.4180 / 12.5170 | 7 |

Candidate nine long intervals versus twenty-one controls is dominated by the
last slow control. A1/B1/B2 do not show a consistent improvement; A2 is retained
and unexplained rather than discarded. These data cannot support a reliable
speedup, much less constant 160 Hz. Existing correlated GC/native evidence still
identifies remaining stalls. The next work must bound those costs, not claim
success from a smaller object census.

The startup metadata helper could not autoload Get-FileHash in the nested
Windows PowerShell sessions. Its sidecar hash was unavailable; reset, launch,
capture IDs and actual measurement metadata still completed. This did not replay
commands or invalidate the measured intervals. The helper now uses .NET SHA256
directly; archived binary identities are computed independently with Python.

## Portable rollout and evidence

The new QML host is part of the existing qt_add_qml_module file list. AOT stays
enabled. Windows system-DLL audit and resource-only import isolation pass. The
compressed payload is regenerated; the CGO_ENABLED=0 embedded launcher and its
payload/extraction/reuse/concurrent-extraction/corruption/lookup tests pass.
The worktree launcher is replaced after successful checks, launched directly
with --gui=qt and verified against the newly extracted child hash. Stderr is
empty. Only the worktree process tree is stopped. No commit or push.

Normal host SHA256: 486BF5D18F306A408C24B0B393665C096D42F3E54B59B6CFB3DC29D15D30A687.
Launcher SHA256: 46039CA7688430AE768AA3DCE15592CE2A3B2D2FCD47B920E91FB66D5C8A027A.

The evidence bundle profiler/experiments/shared-thumbnail-factories-2026-10-19
contains compact captures, census/measurement summaries, passing and expected
failing tests, rendered images, source snapshots and reproducibility scripts
with SHA256 manifest. Binaries, raw PresentMon CSVs and build outputs remain
outside it. Earlier failed/invalid attempts are explicitly identified above.
