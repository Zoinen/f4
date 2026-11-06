# Curve preprocessing attribution and production candidate, 2026-10-20

Constant 160 Hz without spikes remains unproven. This work reduces recurring
renderer preprocessing overhead; it does not resolve all previously measured
CPU spikes or establish constant frame delivery across all workloads.

## Attribution

The middle-button stress capture previously missed one refresh on a 14.2992 ms
producer frame. Public preparation took 4.1507 ms, but measured batch preparation
accounted for only 1.8284 ms. No upload chunk ran in that frame. This justified
instrumenting the outer QSGRenderer entry rather than blaming upload chunking.

The exact Qt 6.11.1 qsgrenderer.cpp was fetched and hash-pinned. Bounded probes
separate render-context preprocessing, registered-node callbacks, and node state
updates. A subsequent middle-button capture did not reproduce the original
2.29 ms unexplained gap. It did reveal regular traversal of hundreds of nodes.
Untextured curve fills have no work in their callback but register UsePreprocess.
A later native stroke fixture showed that ordinary curve strokes are already
unregistered; the final patch therefore omits the redundant stroke condition.

## Change

The standalone Conan source patch now filters registration for the exact Qt
curve fill type without a texture provider. Derived
classes retain their callbacks. DirtyMaterial refreshes registration, so adding,
removing, destroying, or replacing a texture provider retains dynamic-texture
updates. Removal never inspects the dynamic type after derived destruction.
Without compiler RTTI the original behavior remains unchanged.

The production patch has no diagnostic switches. Dynamic-type checks occur on
node addition or relevant flag/material changes, not during stable traversal.
It leaves bitmap handling, SVG geometry, upload budgets and scrolling unchanged.
The experimental SVG upload chunking remains excluded.

The new native F4QtCurvePreprocessTests checks actual renderer membership,
provider transitions, dynamic texture updates, and preservation of a custom
subclass callback. Its fixture checks scene origin and unit transforms at 175%.
The unoptimized library failed the original two membership regressions on both
D3D11 and OpenGL; the optimized candidate passes all five test checks per backend.

## Controlled measurement

real-scroll-noop-control and real-scroll-noop-filter used the same diagnostic
executable and matched receipts. SVG upload chunking was disabled in both; only
the registration filter changed. Downloads, Masonry, DPR 1.75, 160 Hz display,
quarter-window middle-button offset with bounces, four 10-second input windows.
No build, tests, offline analysis or UIA observer ran during input.

| Scope | Cold control p99 | Cold filter p99 | Warm control p99 | Warm filter p99 |
|---|---:|---:|---:|---:|
| Whole preprocessing | 0.3766 ms | 0.2679 ms | 0.3540 ms | 0.2824 ms |
| Registered node loop | 0.3294 ms | 0.2145 ms | 0.3128 ms | 0.2300 ms |

Warm whole-preprocessing median fell from 0.1077 to 0.0776 ms. Registered-node
processed count p99 fell from 423 to 291. Node state updates were essentially
unchanged. All four windows in each run passed strict middle-motion gates and
had zero missed display intervals. Full public-preparation p99 improved in
three of four corresponding windows; one cold window worsened. These finite
captures show headroom, not elimination of every possible spike.

## Verification and deployment status

The strict recipe serialization/source-drift suite passes 37 tests. The actual
production Qt Quick archive was compiled from the recipe replacements with no
profiling code. SHA256:

8082fc3f8b0accf91f495e65de79eaf9d9b63d5c938950b3a02c04d7b6880ddd

The host and native regression target build successfully. On that library,
The final expanded native suite passes 6 curve checks, 12 camera checks, and
14 SVG presentation checks per backend at DPR 1.75. The first OpenGL SVG suite terminated with
heap-corruption status 0xC0000374 during the catalog viewer swap. Three subsequent
paired runs pass 14 checks each with both previous and candidate libraries.
The initial failure is retained; clean reruns do not explain it.

Two more full-suite paired debugger repetitions and six paired swap-only
stress repetitions passed without reproducing the heap failure. This failure
remains unexplained; these passes do not establish its root cause. The initial debugger setup
caused an existing 5-second QML loading timeout on the control library; this is
not evidence that the original heap failure occurs in the control. Debug heap
instrumentation was removed for a more representative repeat, with exception
stack capture retained.

Import isolation, Windows system-only DLL audit and CGO_ENABLED=0 embedded-host
extraction tests passed. The final portable launcher is now running directly with --gui=qt. Its responsive
content-addressed child matches the newly built host hash. No host, pool-budget,
frame-latency or scale override was supplied.
No commit or push was made; the canonical checkout binary is untouched.

## Evidence

The immutable attribution snapshot is in
profiler/experiments/curve-preprocess-attribution-2026-10-20/manifest.json.
It preserves source hashes, bounded probes, test logs (including the initial
OpenGL failure), comparison data, input captures and the original link receipt.
Later debugger/packaging/deployment evidence will be archived separately.


## Running final build

- Launcher SHA256: 500be90b2c254dbebc50f8fd6d01e1ee88ecb5fabba63e1cb0b8adc926d347fa
- Host SHA256: 2e58e03b092333ec68691324cd81f80a903992c16585f4ff402115446873fc6f
- Go PID 11952; Qt PID 70332, responsive at restart verification.
- Launch receipt: artifacts/curve-preprocess-live-identity.json.
- Previous launcher preserved as artifacts/f4-zoin-before-curve-preprocess.exe.

## Further scrolling captures and remaining work

real-scroll-curve-production-middle and real-scroll-curve-production-rapid used
production source behavior with bounded probes, without upload chunking. They
used the immediately preceding e340fa58 library, whose only extra condition was
the redundant exact-stroke exclusion removed from the final 8082fc3f library.
Do not describe these as measurements of the final binary. The immutable first
snapshot preserves the earlier source and package hashes; per-case link receipts
identify the exact measured library.

Middle-button scrolling passed all four strict motion windows, with zero missed
display intervals. Display maxima were 6.3411/6.4857/6.8913/6.3525 ms. Worst motion
step error was 1.8366 physical pixels. Rapid 10 ms wheel input also had no missed
display intervals, maxima 6.3930/6.3675/6.3456/6.3818 ms, but failed the unchanged
2 physical pixel motion threshold with 2.4061 px in cold block 0. The descriptive
rerun is not a passing strict gate. That step paired a 6.393 ms displayed interval
with a 6.25 ms animation-clock step and consecutive presentation indices; small
phase variation is amplified by rapid scrolling. The failed strict log is retained.

Public preparation still reached 5.7142 ms in warm middle block 0. Its producer
frame was 12.5926 ms although buffered display delivery did not miss a refresh.
Correlated scopes show 5.0814 ms inside batch preparation, including 1.7392 ms in
the ordinary alpha upload loop, 1.1151 ms in gated admission and 1.0664 ms preparing
alpha batches. One recorded upload of 1,344,420 vertex bytes plus 122,220 index
bytes took 1.1049 ms. Those scopes nest; their times must not simply be summed.
CPU cycle probes were not enabled in this last capture, so it does not distinguish
execution cost from descheduling. See curve-production-warm-preparation-outlier.json.

Next work must address the remaining preparation/large-upload tail and rapid-input
motion phase error, and measure the final binary. Upload chunking is still an
unpromoted prototype. The isolated OpenGL heap incident also needs stronger
attribution if it recurs. No full-goal completion is claimed.
