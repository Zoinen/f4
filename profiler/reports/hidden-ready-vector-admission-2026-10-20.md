# Revalidate hidden vector residency before upload — 2026-10-20

## Confirmed bug and correction

A renderer gate can remain ready while present=false, even after its current
batches are replaced. Ownership already covers this state and removes stale
warmed entries, but readiness was not invalidated. The ordinary upload loops
therefore uploaded the new batches without the pending-admission budget.

The gate constructor now clears ready when its validated warmed map differs from
current membership. Existing admission limits, priority ordering, one-oversize
progress, complete-surface visibility, and visible prepared surfaces are unchanged.
The generated production renderer differs by exactly this conditional and its
two explanatory comments. The source of truth is ci/qt_quick_upload_policy.py.

## Regression

Extend preparedVectorReentryUsesUploadBudget with hidden ready clip/transform
cases. Keep the gate registered and ready, set present=false, replace its 24
geometry leaves, and inspect newly resident GPU batches on the render thread.
Before: 24 batches admitted in one frame on D3D11 and OpenGL; maximum allowed by
this fixture's byte estimate is five. After: both cases obey the bound and take
multiple frames. Every captured image is either background or the entire final
surface. Presentation then produces the exact reference image. All leaves retain
175% scene origins and unit transforms, and rendered output was inspected.

The first test iteration counted Qt's drawing-preparation log, which misses
hidden uploads; the next inspected only alpha batches although this fixture is
opaque. Neither zero-count result was used as proof of the bypass. The corrected
regression enumerates both batch classes and reproduces 24 before fixing code.

## Verification

- Renderer suite: 32 passed/backend, zero failed/skipped. Covers four reentry
  cases, deferred complete drawing, unrelated roots, painter order, unchanged
  geometry, geometry pool settings and available-capacity reuse.
- Gallery session checks: 12 passed/backend, zero failed/skipped. All panel modes,
  viewer swap, gradients/transparency/embedded image, prepared reuse and progress.
- Native-camera/culling checks: five passed/backend, zero failed/skipped.
- Qt patch tests: 34 passed. Import isolation, DLL audit, embedded-host extraction
  and packaging passed. git diff --check passed with preexisting CRLF warnings.

## Performance evidence and limits

Same material-scope diagnostic configuration as the preceding experiment:
Downloads186, Masonry152, screen160Hz, DPR1.75, wheel30ms, two cold and two warm
10-second input windows, default GC5000us, geometry pools16MiB. No build, test,
analysis, UIA or scheduler trace ran during input. Probe arrays stayed bounded.

Post-fix displayed long intervals (>9.375ms): zero over40seconds, window maxima
6.3681,6.3670,6.4218,6.3477ms. This does not prove an exact6.25ms bound.
Ordinary alpha-upload maxima:1.0873,0.9954,1.0803,1.0888ms. The preceding repeat
contained a3.4349ms ordinary upload phase. Pending admission maxima after:
2.6654,1.5364,1.9488,1.9975ms. The soft budget permits one oversize batch and
cannot preempt a single driver call. This comparison is one after run versus
prior runs, not a statistically isolated speedup claim.

Within input windows, thresholded uploads marked ready=true,present=false:
preceding repeat cold9 events/4.0876ms, warm0; post-fix cold0,warm0. Samples are
thresholded at0.1ms, so zero does not prove absence of faster events. The failing
and passing renderer regression supplies the direct budget-behavior evidence.

The older displayed missed refresh remains unresolved evidence. Native scrolling
is still disabled by default; GUI scheduling and cold startup remain rollout work.
The overall constant160Hz objective is not achieved.

## Deployment

Fresh single Go launcher directly runs --gui=qt and extracts the matching embedded
host without an override. Responsive live host SHA256:
79ebee199c5a89a0a340c4060a75e36c50babc359bece4856ac3d2b77ed62f50.
Launcher SHA256:
70a0331a0cff232f692671b13e6f1b8574572a733d68d4ad52e2a4f53b016a6b.
Only exact worktree/diagnostic launcher trees were terminated. Canonical build
untouched. No commit or push. Evidence archived with round-trip SHA256 checks.

AI assistance has been used to create this output.
