# Coalescing panel icon raster work

## Fresh attribution

The previous checkpoint saved authoritative source and measurements, which
is progress; constant 160 Hz remains unproven. A fresh native sampled capture
uses the integrated child-window stacking fix, not the upload-gate prototype.
Local evidence is delegate-cold-wheel-masonry under artifacts/modes-profile.
The capture has no lost events or buffers and resolves local PDB symbols.

There are 1352 GUI samples, 1351 with matched stacks. Inclusive counts include
prepareNearbyDelegates 385, prepareDelegateRow 100, popBrickItem 70 and
child-window stacking only 2. Within the 385 nearby-preparation samples,
68 include createComponent, 110 retirement, 82 visual binding, 53 layout,
127 QQuickIconImage::load and 121 QQuickImageBase::loadPixmap. These are nested
inclusive samples and cannot be summed or treated as elapsed milliseconds.
The native frame-period p99 is 10.1188 ms, maximum 18.8381 ms under sampling;
these are diagnostic periods, not clean display-cadence measurements.

Full stacks confirm icon loads during width/height propagation from
GalleryDelegateItem::setGeometry and during row rebinding. Qt 6.11.1's
QQuickIconImage calls load on geometry, source and color changes; its tint
path creates a QPainter over the pixmap. Synchronous row preparation therefore
does redundant raster/tint work before the final properties settle. This is
not solely a delegate allocation problem.

The sample parser was corrected to locate the sampled module/function field
from the end: xperf can emit an unquoted comma in a thread-start template
name. Timestamp/thread matching and inclusive stacks were unaffected; the
correction prevents contaminating render leaf names with that template suffix.

## Change and regression

GalleryIconImage subclasses the existing private Qt icon renderer, coalescing
load requests into the item's polish pass. Source changes remain observable
immediately through its requested-source property; final raster work uses the
settled size and tint before scene synchronization. Reentrant geometry changes
during the actual load preserve Qt's recursion guard. No timer, per-frame
rounding, new setting or bitmap conversion for SVG files is introduced.

An empty source with no theme name clears pixels immediately and cancels the
pending load. Qt's normal IconImage path can retain its old internal URL when
the source becomes empty, so this path uses the base empty-image reset.
The native companion module exposes the new type; its private Qt dependency
is explicit. Redundant QML assignments of false to mipmap/retainWhileLoading
are omitted because those inherited revisioned properties are not exposed by
the companion module's 1.0 import; the Qt defaults remain false.

The regression changes source, width, height and tint synchronously and counts
uncached image-provider requests before drawing. It fails before with 6
requests versus expected 1. The final implementation passes with exactly one
request for the final source at its final DPR-scaled size, correct blue pixels,
whole physical-pixel leaf origin and scene unit vectors at 175%, and immediate
Null status after clearing the source. Initial iterations caught both the
empty-source behavior and inherited QML property availability; those failures
are fixed before the final checks. The embedded test body is excluded from
moc parsing because its lexer misparses provider URI strings in embedded QML.

Final focused checks at DPR 1.75: 9 passes, no failures, including the new
regression, nearby preparation in both modes, compact Lucide parity, preserved
non-Lucide colors, pending-thumbnail fallback and physical-grid placement.
The focused SVG selection has 26 passes with upload-gate capability explicitly
absent, including all five panel modes, cache, source replacement, cancellation,
resize, animation and viewer swaps. The new raster fixture capture and the
real Downloads panel capture were inspected; captions, icons and SVG outlines
remain visible. These focused checks do not establish the entire suite green.

## Clean display cadence

A is the preceding child-stacking host; B adds icon coalescing. Both use the
same guarded Downloads folder: 184 entries, density 152, extent 4161.142857,
actual DPR 1.75, active window, 160 Hz. Wheel events repeat every 30 ms with
delta 120, about four seconds per direction. State is restored between fresh
processes. No build, tests, sampler, ETW recording, expensive observer or debug
logging runs during captures. Caption measurements remain zero. The headline
is PresentMon display-change intervals, not the harness's native periods.

| Run | Down p99 ms | Down max ms | Down >9.375 ms | Up p99 ms | Up max ms | Up >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 | 6.3234 | 12.4994 | 3 | 6.3098 | 12.4994 | 1 |
| B1 | 6.3352 | 18.7457 | 3 | 6.2857 | 18.7582 | 2 |
| B2 | 6.3120 | 18.7562 | 5 | 6.2925 | 12.5009 | 1 |
| A2 | 6.3300 | 18.7591 | 5 | 6.2835 | 12.4999 | 1 |

The long-interval counts tie on downward scrolling; p99 differences are small,
and B1's maximum and upward long count are worse. No reliable scrolling gain
is established. The change removes reproduced redundant work, but does not
eliminate the remaining frame spikes or prove constant 160 Hz. Evidence stems
are icon-coalesce-{a1,b1,b2,a2}-wheel-masonry; compact local evidence is
artifacts/icon-coalesce-summary.json. Generated binaries and raw traces are
outside the source checkpoint.

## Remaining work

Ready geometry uploads and driver resource creation still appear on the
render critical path. A 16.7888 ms native period also contains 13 GUI samples
inside QV4 GC; this establishes GC activity during that frame, not proof that
all its wall time is GC. Attribute remaining allocation/rebinding and renderer
resource work before selecting another change. Neither hiding incomplete SVGs
nor deferring visible work indefinitely meets the objective.

## Portable worktree verification

The normal host links the integrated child-stacking/stable-interval library;
experimental upload admission remains disabled. Import isolation and compiled
host loading pass 4 results. The Windows DLL audit succeeds. The compressed
payload was regenerated, the launcher built with CGO_ENABLED=0 and GOWORK=off,
and tagged payload/extraction/reuse/concurrency/lookup tests pass (0.654 s).
The system Go cache remains in use. Cross-platform release gates were not run.

Only this worktree's process tree was replaced, followed by a direct --gui=qt
launch with diagnostic overrides cleared. The actual extracted child matches
the freshly built host hash, title is f4 [zoin_branch/cached-pixel-grid], and
startup stderr is empty. Canonical D:/Code/f4/f4.exe was untouched. Evidence is
artifacts/icon-coalescing-normal-identity.json.

- Host SHA256: 7eedd3f9f8c75259a830cced02706baa4c162980132f14d309723639f9a44dc2
- Launcher SHA256: 3bbcc033ff2c1b6cb97a19a6da3eb20d7a018aa31dcd9cb1521783cc1764b9ca

This continuation leaves changes uncommitted and makes no push. The preceding
checkpoint remains intact; the 160 Hz goal remains active and unfinished.
