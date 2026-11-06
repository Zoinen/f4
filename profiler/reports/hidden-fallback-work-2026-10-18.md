# Avoid hidden thumbnail fallback work — 2026-10-18

## Production change

The previous goal turn removed the JS SVG readiness walk and measured its
allocation effect. This turn finds and fixes another allocation producer:
`GalleryEntryPreview` computes/rasterizes a fallback icon even when its drawing
gate already suppresses it because thumbnail metadata/pixels are available.
The Loader also creates an unused fallback primitive for such initial rows.

The fallback Loader now starts only when presentation is active and a thumbnail
is not expected, while retaining any previously created compatible primitive.
Its raster source is empty while presentation is inactive or a thumbnail is
expected. Component selection is not committed while the fallback is suppressed;
this prevents an empty suppressed source from replacing an existing aspect-fit
primitive with another primitive. This preserves useful reuse while avoiding
unnecessary initial objects and provider requests. The drawing gate, thumbnail
error predicate, actual geometry, pixel fitting and normal icon rasterization
policy are unchanged.

## Regression evidence

A counting native provider reproduces two unnecessary raster requests per row
after thumbnail readiness in each of the five modes. All five old-code rows
fail the source-empty assertion. The final counter mutates plain C++ state and
does not expose a notifying property to the binding. An initial QML-counter
draft itself introduced a dependency/binding loop; it is replaced before the
recorded expected failure and is not used as causal evidence.

The fixed tests verify zero provider requests for the thumbnail row, an empty
hidden fallback source, restoration when thumbnails are disabled, preservation
of an existing compatible icon when thumbnails are reenabled, and fallback
restoration after an image decode error. A separate construction within every
data row begins with thumbnail metadata: it creates no fallback image leaf and
makes zero provider calls. These are concrete request/object assertions rather
than timing thresholds.

- Gallery tests at DPR 1.75: 33 passed D3D11, 33 passed OpenGL.
- The five new mode cases pass on both backends, alongside all prior SVG,
  failure, recycling, lifetime, animation, resize and alignment regressions.
- Native mode/motion tests: 11 passed, covering the fallback drawing gate,
  opacity-node preservation, wheel bursts in five modes, alignment settling in
  Grid/Masonry and Columns text alignment.
- Import isolation and GLSL resources: 4 passed on each backend.
- Static DLL audit and generated payload/materialization/reuse/concurrency/
  corrupt payload/lookup tests pass.

The drawing-gate regression checks actual icon, thumbnail and caption origins
and unit scene transforms at 175%, then captures the result. The rendered orange
thumbnail and small caption are inspected. Existing vector alignment checks
cover all five modes. No layout change or new rounding is introduced.

## Allocation measurement

Both fresh-process QML captures use Downloads, 184 entries, density 152, active
160 Hz, DPR 1.75, D3D11, normal GC budget and 30 ms wheel events. The current
native SVG readiness helper is present on both sides. Clients exit 0 with
complete XML. Scrolling activity is filtered by the same two sustained
contentY-change clusters; this is not exact QPC alignment.

`sourceColorIconAtSize` calls fall 447 -> 429; its positive-small-allocation
proxy falls 118912 -> 114208 bytes. Inclusive helper time is 6.1762 -> 6.2712 ms,
so no CPU-time improvement is claimed. `isLucideIconSource` calls fall
1023 -> 1005, proxy bytes 123648 -> 121344. Whole-trace small-allocation proxy
bytes fall 5802816 -> 5770080. These are small effects in this mixed folder;
do not attribute all variation to the change or reinterpret the temporal
allocation proxy as exact native allocation counts. Other image creation
counts vary between runs. The controlled five-mode request/object regressions
provide stronger evidence for the specific suppressed-work contract.

## Clean scrolling comparison

Fresh A1/B1/B2/A2 processes use identical workload, input, scale, refresh rate,
GC budget and the promoted renderer. A is the previously deployed native-SVG-
readiness build. There is no QML profiler, WPR, observer, test or build during
clean captures. All windows remain active. Strict displayed intervals have
both endpoints inside the same direction window.

| Run | Down p99 ms | Down max ms | Down >9.375 ms | Up p99 ms | Up max ms | Up >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 | 6.3287 | 12.4990 | 4 | 6.3337 | 12.4908 | 2 |
| B1 | 6.3054 | 12.4751 | 1 | 6.2809 | 12.4912 | 1 |
| B2 | 6.3532 | 12.5281 | 3 | 6.3070 | 12.5123 | 2 |
| A2 | 6.3400 | 12.5068 | 5 | 6.2942 | 12.5035 | 1 |

Controls total 12 long displayed intervals; candidates total 7. B2 p99 worsens
in some directions and maximum displayed gaps remain around two refreshes.
This small comparison does not establish a reliable frame-time gain or explain
all remaining spikes. The objective of constant 160 Hz remains unachieved.

## Rollout and remaining work

The current worktree Qt host is rebuilt, audited, compressed into the embedded
payload and used to build a CGO_ENABLED=0 launcher with the system Go cache.
The live diagnostic restores panel state. Only this worktree's launcher tree is
replaced, then launched directly with `--gui=qt` and diagnostic overrides cleared.
The extracted child hash matches the normal host; startup stderr is empty.
Canonical F4 is untouched. No commit or push is made.

- Host SHA256: `28D96B1FD4DE622CDC67844D6E900AD0217249C99F52241FC37F48673AEC443A`
- Launcher SHA256: `13F852A90D233CC7C0A89E5AFA9573016DE72A668C7977DAE665292BCA50208D`

The experiment directory `profiler/experiments/hidden-fallback-work-2026-10-18/`
contains expected-failure/passing logs, both QML summaries and windows, complete
native frame JSON and strict display summaries for all four runs, scripts,
allocation comparison and manifest. Raw QML/PresentMon captures and build
outputs remain untracked. Live identity is
`artifacts/hidden-fallback-normal-identity.json`.

This fix removes verified waste but the mixed Downloads workload is dominated
by other delegate/binding work. Continue reducing those larger producers and
the retained object graph; do not infer that GC pauses have been solved from
these tests or the lower small-allocation proxy.
