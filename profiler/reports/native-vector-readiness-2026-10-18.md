# Native SVG readiness traversal — 2026-10-18

## Change and result

The previous goal turn established CPU marking and ordinary heap sweeping as
remaining GC costs. This turn removes one measured producer: the recursive
JavaScript walk used to check asynchronous SVG Shapes. `VectorSourcePin::sceneReady`
now walks QQuickItem children in C++, checks QQuickShape::Ready, and applies
CurveRendering to nested QQuickText only when needed. It visits children even
when an ancestor is not ready, preserving the old text preparation behavior.
Null roots return false. The caller, polling interval, source lifetime,
cancellation, upload admission and presentation timing are otherwise unchanged.

The private QuickShapes dependency is explicit on the existing Core target. The
module already belonged to the vector pipeline; no new runtime format or setting
is introduced. Built-in interface icons are unaffected. No rounding or static
layout arithmetic changes.

The comparable QML traces show the recursive JS call count dropping from 420
to zero inside the scrolling activity windows. Its attributed positive small
allocation proxy was 1268 events / 160960 bytes before, absent afterward.
Vector-surface proxy bytes drop 318336 -> 155552; total proxy bytes drop
5906560 -> 5678464. `resolveState` stays nearly unchanged (13088 -> 13056 bytes),
so the removed walk has not merely shifted its allocation attribution there.
These are temporal QML attribution proxies, not exact native allocation stacks
or a complete heap-size measurement. The removed helper's inclusive time was
1.6501 ms across the interval, not a per-frame saving of that magnitude.

## Regressions and presentation

The new native readiness regression first fails against the old implementation
because the invokable is absent. An initial test draft needed to wait for the
inline component to finish loading; that fixture issue is corrected before the
recorded expected failure. The final test verifies an unpolished asynchronous
Shape is not ready, nested Text switches to curves, readiness completes after
window exposure, 100 repeated calls preserve actual Text/Shape object identity,
and querying an unrelated pin does not acquire a source or component.

- New regression: 3 passed including init/cleanup.
- D3D11 at DPR 1.75: 28 passed, zero failures.
- OpenGL at DPR 1.75: 28 passed, zero failures.
- Both runs cover SVG/SVGZ local/virtual acquisition, failure skipping, successful
  prefetch counting, cancellation/pinning, viewer swaps, recycling, bounded scene
  retention, resizing, five-mode alignment, gradients/alpha/embedded raster and
  authored animation/visibility behavior.
- Import isolation and GLSL resource audit: 4 passed on each backend.
- Static Windows system-DLL audit passes after the build.
- Generated payload/materialization/reuse/concurrency/corrupt-payload/lookup
  tests pass.

The existing five-mode scene-coordinate checks cover vector and caption leaves,
including effective unit transforms at 175%. The newly generated Masonry capture
is inspected, including its small caption and thin vector border. This change
does not alter production visual geometry or the text-rendering policy; it moves
the existing policy's traversal out of JS.

## Clean displayed-cadence comparison

Separate fresh processes use the same combined renderer and Downloads folder:
184 entries, density 152, DPR 1.75, active 160 Hz, 30 ms actual wheel events,
default GC budget, D3D11. No QML profiler, GC instrumentation, WPR sampler,
scene observer, tests or builds run during these captures. Captures are A1,
B1, B2, A2, with A being the previously deployed source. PresentMon displayed
intervals include both endpoints only inside the same direction window.

| Run | Down p99 ms | Down max ms | Down >9.375 ms | Up p99 ms | Up max ms | Up >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 | 6.2960 | 12.5161 | 4 | 6.2790 | 12.4760 | 1 |
| B1 | 6.2949 | 18.7385 | 3 | 6.3137 | 12.5016 | 2 |
| B2 | 6.3079 | 12.5325 | 3 | 6.3280 | 12.4768 | 2 |
| A2 | 6.3540 | 12.5183 | 4 | 6.3102 | 12.4982 | 3 |

All captures stay active. Results are mixed: B1 retains an 18.74 ms outlier,
and some candidate p99 values worsen. No environmental explanation or consistent
frame-cadence improvement is inferred. The change removes verified allocation
work but does not establish lower GC pauses or constant 160 Hz. Larger
allocation producers and retained QML graph traversal remain to optimize.

## Rollout and evidence

The normal Qt host is rebuilt with the already promoted combined renderer,
the static payload is regenerated and a CGO_ENABLED=0 portable launcher is
built. Panel state is restored via the live diagnostic. Only the worktree's
own launcher tree is replaced, and the fresh launcher runs directly with
`--gui=qt`, clearing diagnostic overrides. The extracted Qt child matches the
new host hash; startup stderr is empty. Canonical F4 is untouched.

- Host SHA256: `3916514DCBE1302AD6C73ECDB2B53F755C82E3DCC9DBA713820FF1A43E866662`
- Launcher SHA256: `794B40D0FF0AB3343EB7C47D17CEED2633331CCB5A2DA3E57E9050B3F9C611C6`

`profiler/experiments/native-vector-readiness-2026-10-18/` preserves expected
failure and passing results, complete native frame JSON, strict display
summaries for every comparison run, allocation-site/window summaries, scripts
and a manifest. Full raw QML/PresentMon traces and build outputs stay in
untracked artifacts. The QML filter uses two sustained contentY-change clusters,
not exact QPC alignment. Clients exit 0 with complete XML. Raw traces:
`profiler/traces/vector-check-{before,after}-qml-memory-2026-10-18.qtd`.
Live deployment identity is `artifacts/vector-native-normal-identity.json`.
No commit or push is made; the full goal remains active.
