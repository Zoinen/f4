# Moving-scope grid deferral and delegate attribution

## Outcome

The cached pixel grid now retains a moving scope's last leaf corrections and
defers dirty-leaf fitting until that scope stops. The native regression passes
at DPR 1.75, including rendered small text and thin raster strokes. This removes
verified unnecessary scene mapping, but it does **not** establish a scrolling
performance win or achieve the constant-160-Hz goal.

One candidate-only Downloads capture still had 54 coverage holds and two missed
display intervals. It is not a controlled before/after comparison. The normal
embedded application was preserved; this candidate ran in its own worktree
diagnostic host and has not been promoted into the normal embedded payload.

## Implementation and regression

Changed ZoinGallery's `PixelGrid.cpp` and `PixelGrid.h`. A cached boundary flag
suppresses fitting for dirty leaves inside an active motion scope. Each deferred
alignment retains its previous correction. Settlement invalidates only those
deferred alignments. Removal clears deferred references; ordinary stationary
scopes still fit. Boundary discovery reads the private motion marker once; this
does not introduce per-frame ancestor traversal, depth sorting, or rounding.

`pixelGridScopeDefersDirtyLeavesDuringMotionAt175Percent` covers both explicit
and attached scopes, two named native-text leaves (including a 9-pixel caption),
a named thin-stroke raster leaf, a stationary sibling, and deletion of a
deferred alignment. It asserts actual scene-space origins, dimensions and unit
transforms after settlement, rather than checking only wrappers.

With the old grid implementation and the same final test object, both data cases
fail: **6 scene maps during motion**, where zero are required. The candidate
performs zero maps for these dirty moving leaves and retains their offsets.
The separate stationary sibling still performs two maps.

The first fixture used an invalid base URL and is retained as invalid evidence.
The corrected fixture and expanded two-case negative tests are the regression
evidence. The first positive run used software rendering: 4 passes, 3 skips.
The hardware rerun (`ZOIN_TEST_NATIVE_RENDERER=1`) has **7 passes, 0 failures,
0 skips**, including existing retarget, native-camera culling and SVG tests.
Both settled 175% captures were inspected, including the small caption and
one-physical-pixel raster strokes. The explicit/attached RGB and alpha outputs
are equal. No production QML presentation was changed by this fix.

## Live candidate scope

Case `cold-l131`, host SHA-256
`e8c9f54a8ebd10a3892c871d7768c186ce7e2ebcf5e664fe7c44f7fcbb874972`.
Actual display: 3840x2160, 160 Hz, DPR 1.75. Downloads: 192 entries, Masonry
density 152, 30 seconds of wheel input. The existing diagnostic baseline keeps
two asynchronous roots, stable source identity, ready-first handoff and the
directional leading preparation budget enabled. These diagnostic mechanisms
must not be mistaken for changes newly promoted into production.

The original controller reached its foreground-observation timeout. The existing
initialized Qt process was preserved and released by a recovery controller;
navigation/input had not started before recovery. No case or application was
restarted. Native foreground was verified for all 2,832 input samples, and the
Computer Use helper was absent before release and after capture.

| Measurement | Result |
| --- | ---: |
| Display intervals | 4,799 |
| Display p99 | 6.304 ms |
| Maximum display interval | 12.512 ms |
| Display intervals above 9.375 ms | 2 |
| Dropped presents | 0 |
| Render-signal interval p99 / max | 2.459 / 4.295 ms |
| Synchronization p99 / max | 0.752 / 1.253 ms |
| Input-handler p99 / max | 2.858 / 8.767 ms |
| Coverage holds / attributed holds | 54 / 54 |
| Unmatched motion commits / presents | 0 / 0 |

At the readiness snapshot, blocking-cell occurrences were 75 constructing and
72 queued. At held render time: 52 constructing, 46 queued, 36 already
published, 6 ready and 7 canceled. These are occurrences, not unique roots.
The snapshot/render distinction matters: a root can complete after the coverage
decision. Neither queue wait nor construction elapsed time proves CPU residency.

The physical-motion analyzer excludes all 4,794 wheel-curve pairs and admits
**zero** physical-error samples. Zero unmatched pairs and small matrix float
error do not fill this acceptance gap. No physical-motion or full-160-Hz pass is
claimed. The capture is app-cache cold at startup, with 48 frames ready before
input; it is not an OS-cache-cold or all-entry-warm claim.

## QML attribution, before the grid change

Case `cold-k121` used the same two-root diagnostic baseline with selective QML
profiling enabled. The initial controller failed to import `profiler`; it had
not released navigation. Its client was closed and the existing initialized Qt
process was reattached. The failed attempt and its log are retained. Startup
trace coverage may therefore be incomplete. The recovered client flushed valid
XML and exited successfully; the Qt/Go processes closed normally.

The 33,286,598-byte recovered trace SHA-256 is
`d8612062a814b9772c14932d700788af3a80d01f66aa213710f34608c7f3fcb0`.
Three ordered QtInfo events map the trace clock to native navigation/input
markers; the navigation-to-input delta discrepancy is 6,400 ns. The done marker
is 75.599 ms after the packet snapshot and does not redefine the input window.

Within the first 1,500 ms of input:

| Range | Count | Inclusive elapsed total | Maximum |
| --- | ---: | ---: | ---: |
| GalleryEntryDelegateBase creation, GalleryViewport.qml:388 | 133 | 50.801 ms | 10.883 ms |
| QtQuick/Loader creation, GalleryVisualHost.qml:5 | 530 | 35.814 ms | 0.588 ms |
| Native BrickItem creation, GalleryEntryDelegateBase.qml:9 | 263 | 26.596 ms | 0.431 ms |
| activePresentation binding, GalleryEntryDelegateBase.qml:295 | 471 | 25.242 ms | 0.583 ms |
| iconPixelAlignmentActive binding, GalleryEntryDelegateBase.qml:82 | 867 | 21.875 ms | 0.198 ms |

The clipped union of all recorded ranges is 371.387 ms in this input window.
During the separate 425.974-ms entry window, `updatePanelDescriptor` has 22
ranges totaling 88.955 ms inclusive, maximum 11.928 ms. Inclusive totals overlap
and must not be summed into CPU or wall time. Profiling changes timing. The
trace did not request animation/frame, memory or pixmap events and supplies no
independent cadence or cache acceptance result.

This directs the next investigation toward delegate/Loader creation and
presentation changes during readiness, with a controlled comparison needed
before claiming improvement. Merely raising concurrent roots was already
rejected in the separate concurrency report.

## Remaining acceptance work

Both diagnostic hosts pass embedded-only QML import isolation (3 passes each),
the Windows GUI-subsystem test (3 passes each), and the system-DLL import audit.
Warnings remain in the raw logs. The first DLL-audit invocation lacked the
MSVC environment; the successful rerun used `vcvars64.bat`. These native checks
do not replace packaged extraction or an embedded-payload launch test.

Zero coverage holds and refresh misses remain unmet. The physical wheel-motion
measurement gap must be resolved; fixed early input, all-entry warm touring and
middle-button scrolling still require representative verification. Production
payload rebuilding and portable-launch verification remain pending; a linked
diagnostic host is not a completed portable rollout.

Receipts, exact responses, source copies, corrected negative logs, software and
native test logs, rendered captures and analyses are bound by the accompanying
experiment manifest. Raw captures and closed binaries may be hash-preserving
symlinks into the profiling archive. No commit or push was made.
