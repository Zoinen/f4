# Hidden debug URL resolution

## Result

Hidden layout-debug loaders now leave their source empty. Qt.resolvedUrl runs
only when a debug overlay is active. A live GUI selector probe records debug
paint URL resolutions falling from 364 to zero during scrolling. The regression
fails before the fix and passes afterward, including activation, hiding and
reactivation. Clean display captures do not establish a cadence improvement;
constant 160 Hz remains unfinished.

## Selector attribution and rejected larger change

The preceding CPU trace found synchronous filesystem checks under
VectorSourcePin::loadComponent. Qt 6.11.1 source confirms that constructing a
type blob invokes URL interception before asynchronous compilation. This does
not establish the size of the cost. Relevant upstream sources are
[QQmlComponent](https://github.com/qt/qtdeclarative/blob/v6.11.1/src/qml/qml/qqmlcomponent.cpp),
[QQmlDataBlob](https://github.com/qt/qtdeclarative/blob/v6.11.1/src/qml/qml/qqmldatablob.cpp)
and [QQmlFileSelector](https://github.com/qt/qtdeclarative/blob/v6.11.1/src/qml/qml/qqmlfileselector.cpp).

An artifact-only full translation-unit probe times the existing selector method,
recording its input/output URL and data type on the GUI thread. It uses the
matching original compile command, installed Qt 6.11.1 headers and matching
generated moc source, replacing one archive member without duplicate-symbol
overrides. The SDK, normal Qt library and source behavior are unchanged. Native
records reserve capacity 65536, serialize after measurement and report zero
dropped records. Recording/copy/timer overhead remains; these are attribution
captures, not clean cadence results.

Before: 3806 GUI records including startup, 1051 inside separate native scroll
direction windows. Nine generated Prepared.qml lookups total 1.4863 ms, max
0.2742 ms. They return the original URL. This is real synchronous filesystem
work but does not justify a larger loading architecture change from this trace.
No global selectors are disabled, no synchronous setData compilation is added,
and no custom URL scheme/network backend is introduced.

The larger unnecessary repeated category is GalleryLayoutDebugPaint.qml:
364 calls, 10.2177 ms total, max 0.1463 ms, p95 0.0495 ms. Inactive Loader
objects still evaluated their unconditional Qt.resolvedUrl source binding.
Other categories include 356 resource calls (11.8811 ms total) and 322 image
provider URLs (0.0228 ms total). These categories must not be interpreted as
independent CPU-exclusive contributions to a frame.

After: the same instrumented selector library records zero debug paint calls
inside scrolling. Generated component calls remain nine, max 0.1673 ms, with
their ordinary selection behavior intact. Row work differs slightly between
captures (322 versus 338 image-provider URL calls), so no total-selector-time
delta is presented as causal. The focused regression and disappearance of this
specific category establish the removed work.

## Regression and rendering

layoutDebugZonesMaterializeOnlyWhenShown now installs a test URL interceptor
before creating its fixture. It counts only GUI-thread debug paint URL lookups.
The old implementation fails with three lookups while all three overlays are
hidden (expected zero). The fixed implementation checks zero hidden lookups,
successful activation, empty source after hiding, no lookup on a hidden resize,
and successful reactivation. The interceptor removes itself while the engine
is still alive.

Direct3D 11 and OpenGL each pass four Gallery results (including shared factory
lifetime coverage), three mode results and three resource-only import results.
The mode regression loops through all five presentation modes. At DPR 1.75 it
checks actual debug paint leaves' scene origins, extents and unit transforms,
and captures rendered output. The standalone capture and Masonry capture were
visually inspected, including thin overlay borders and captions. No layout or
rounding arithmetic changes.

## Clean display comparison

Exact before/after diagnostic hosts are built from the same current core and
resources except this source binding. The build helper temporarily restores the
old binding for the control, restores the fixed bytes in a finally block, then
builds the candidate and normal host. The final source remains fixed. Both
clean hosts use the original QtQml SDK library and combined QtQuick renderer;
neither includes selector or GC phase probes.

A1/B1/B2/A2 are fresh processes, active 160 Hz/DPR 1.75, Downloads 186 entries,
density 152, extent 4101.142857 and actual wheel input every 30 ms. No profiling
client, WPR, UI Automation observer, selector probe, object census, role counter,
build or graphics test runs during capture. Caption measurements are zero.
Strict display intervals have both endpoints inside each direction window.

| Run | Down p99 ms | Down max ms | Down >9.375 ms | Up p99 ms | Up max ms | Up >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 | 6.3453 | 12.5026 | 3 | 6.3036 | 12.4828 | 1 |
| B1 | 6.3218 | 18.7274 | 5 | 6.3100 | 12.4930 | 1 |
| B2 | 6.3493 | 12.5053 | 3 | 6.3231 | 12.4867 | 1 |
| A2 | 6.3362 | 12.4980 | 4 | 6.2947 | 6.3824 | 0 |

Candidate ten long intervals versus eight controls is not a repeatable gain;
candidate maximum is worse. Four runs and small outlier counts also do not
establish a regression. The change is retained for the directly proven removal
of hidden work. Do not claim the 10.22 ms removed across scrolling equals a
frame-time saving or fixes remaining GC/native scheduling stalls.

## Portable rollout and next work

The normal static host passes the Windows system-DLL audit and resource-only
import test. Embedded payload generation, CGO_ENABLED=0 launcher build and
payload/extraction/reuse/concurrent extraction/corruption/lookup tests pass.
The worktree's own launcher is replaced only after successful checks and started
directly with --gui=qt. Its extracted host hash matches the built normal host;
stderr is empty. No canonical application process is touched. No commit/push.

Normal host SHA256: 15440686B36148594758C7E09793C1145451BE441D3554C9B638B0080347EC79.
Launcher SHA256: CA1EA48D4D8DFDB467063E2065209EE8CA3663169EDF604FCB869D96E8E29257.

Evidence is archived with SHA256 manifest under
profiler/experiments/hidden-debug-url-resolution-2026-10-19. Binaries, raw
PresentMon CSVs, compiler output and upstream Qt source copies remain outside
the compact evidence bundle. Existing GC/native attribution remains the basis
for the next work; do not redesign SVG loading around the ruled-out selector
cost or mistake this local reduction for completion of the 160 Hz goal.
