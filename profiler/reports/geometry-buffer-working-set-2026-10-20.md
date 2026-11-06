# Geometry buffer working set, 2026-10-20

Qt's existing 2 MiB retirement pool repeatedly discarded and recreated geometry
buffers during Masonry scrolling. Raising the bounded pool to 16 MiB per vertex
and index category nearly eliminated recreation after warm-up. This change is
implemented, tested, packaged, and running in the worktree application.
Constant 160 Hz remains incomplete.

## Evidence and experiment

Downloads, 186 entries, Masonry density 152, DPR 1.75, 160 Hz screen, wheel input
every 30 ms. Each capture has two 10-second cold and two 10-second warm input
windows. The diagnostic native camera is enabled; production default remains
disabled. GC budget 5000 us, diagnostic pixmap cache 2 MiB. Builds, tests,
scheduler tracing, UI Automation, and offline analysis were absent during input.

New diagnostic counters distinguish new buffers, pool reuse, capacity growth,
Immutable-to-Dynamic conversion, creation, dynamic update, static upload, and
reuse without recreation. Slow-upload samples retain both requested sizes and
selected pooled capacities. Total counters are cumulative from process startup.

One ordinary 960-byte UI vertex allocation selected a 2,054,448-byte pooled
buffer. The SVG stroke allocation of that size later selected 80-, 880-, or
960-byte buffers and had to grow them. Qt already has best-fit selection, but
insufficient retained capacity forces these exchanges. The D3D11 implementation
releases the old resource on recreation and uses CreateBuffer; dynamic writes
use WRITE_DISCARD mapping. Increasing the existing pool addresses the observed
working-set pressure without adding a second pool or changing lifetimes.

The two larger-pool runs use the same diagnostic binary as the baseline, with
only QSG_RENDERER_BUFFER_POOL_LIMIT set to 16777216. Case identity files record
the override. This measures a configuration experiment; native tests separately
prove the compiled production default has the same value without an override.

Counter differences between cold and warm snapshots include the inter-stage
gap and the warm capture, not only the 20-second warm input windows:

| Counter | 2 MiB baseline | 16 MiB | 16 MiB repeat |
| --- | ---: | ---: | ---: |
| New buffers | 8592 | 0 | 0 |
| Capacity growth | 1204 | 0 | 0 |
| Type conversions | 2501 | 1 | 1 |
| Buffer creates | 12212 | 1 | 1 |

Frame comparisons use only producer intervals with both endpoints inside input
windows. Long means greater than 9.375 ms, not the stricter 6.25 ms target.

| Run | Input seconds | Long producer intervals | Long display intervals | Max display gap |
| --- | ---: | ---: | ---: | ---: |
| 2 MiB baseline | 40 | 31 | 0 | 6.5164 ms |
| 16 MiB | 40 | 6 | 0 | 6.3883 ms |
| 16 MiB repeat | 40 | 5 | 0 | 6.5960 ms |

Warm preparation p99 fell from 2.4599/2.2241 ms to 1.9474/1.8779 ms in the
first larger-pool run. Repeated larger-pool warm maxima for render-to-frame-end
were 2.9311/4.4909 ms. Remaining long producer intervals combine approximately
5–7 ms in the pre-render pacing/sync bracket with 3–4.5 ms rendering. That bracket
includes deliberate pacing; these observations alone do not establish GUI
blocking or justify removing the wait. Next investigate frame pacing and
presentation age, then validate the ordinary production scrolling path.

## Implementation and limits

- The portable Qt upload-policy source patch sets the existing renderer pool
  default to 16 MiB per category. This bounds retired vertex/index capacity at
  32 MiB per renderer; it does not bound live GPU resources or driver overhead.
- Existing explicit overrides, including smaller budgets and zero retention,
  remain effective. Existing releaseCachedResources empties both pools.
- No new setting, raster conversion, SVG fidelity reduction, draw-order change,
  wider materialization window, or new per-frame layout work.
- Dependency patch reapplication refreshes the existing policy payload instead
  of retaining a stale serialized recipe. Strict boundary checks preserve
  neighboring patches and reject ambiguous input.

## Verification and deployment

Regression tests failed against the old renderer default on both D3D11 and
OpenGL while the explicit override cases passed. After rebuilding:

- 34 Python dependency-patch checks pass, including payload upgrade and drift.
- 36 renderer checks/backend pass at DPR 1.75, including the default budget,
  explicit 2 MiB and zero budgets, ordering, and complete SVG presentation.
- 12 session checks/backend pass: cache reentry, background-free viewer swaps,
  gradients/alpha/embedded images, and all five panel modes at 175%.
- 9 native camera checks/backend pass, including retargets, metadata rewrap,
  shrinking geometry, culling, and SVG/raster independence.
- Resource-only QML import isolation passes (3 checks); Windows system-DLL audit
  passes; CGO_ENABLED=0 embedded-host extraction tests pass using the system Go
  cache. The rebuilt host was compressed and embedded in the launcher.

Quick library SHA256:
`dde7429f021d886549eca4f821fa6c77367477b79602e4bc1bcb23b82083e67a`.
Deployed Qt host SHA256:
`cc59ce7f9aa7bf6d2e40154bee453666b75424b6df079b725fd127697851756d`.
Deployed launcher SHA256:
`4fd507da9a6f168031b37bd80a2d60dc30fc346444c7fbb789bf3eb9a7e9553a`.
The content-addressed embedded child matches the rebuilt host and is responsive.
No host override is used. The canonical checkout executable was not touched.
No commit or push was made.

The accompanying experiment manifest retains verified source, build receipts,
capture data, lifecycle counters, PresentMon data, tests, and deployment receipt.
Older large profiling CSV evidence was losslessly compressed and hash-verified
to make disk space; capture inputs were complete before this housekeeping.

AI assistance has been used to create this output.
