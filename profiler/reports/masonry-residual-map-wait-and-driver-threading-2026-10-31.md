# Masonry: residual Map wait and driver-threading control

## Outcome

Constant 160 Hz remains unachieved. A new scheduling trace proves that a small
pooled geometry update can block in D3D11 Map while a worker in the same process
executes NVIDIA driver code. A documented device-threading control is implemented
and tested as a diagnostic intervention. Its live pair does not improve delivery.
The flag-enabled scheduling capture has much less blocked preparation time but
still shows an 8.13 ms CPU spike, with dependency-graph hashing on the sampled stack.
Do not promote the flag or conclude that one wait explains the entire problem.

The preceding turn made progress by implementing and testing a clean inherited
transform skip, then measuring a repeatable node-visit reduction. This turn keeps
that skip and geometric snapshot storage enabled while investigating residual
costs. No product source, installed Qt archive, portable payload, normal app,
commit or push changes.

## Exact-host scheduling capture

Case `real-scroll-coldresidual-b1-20261031-wheel-masonry` uses host SHA256
`18a1d6931ccedf64bbfe831ec289d4a01eb36c41d2ec433da7111ab7e877c6cc`.
Actual Downloads contains 192 entries including parent; Masonry density is 152.
Desktop is 3840×2160 at 160 Hz and actual DPR 1.75. A fresh isolated application
cache and normal viewport loading are used. The second direction is not described
as verified warm. All 1725 wheel inputs have active native foreground ownership.
Buffer prewarming and PresentMon are disabled. Kernel scheduling/profile capture
and a narrow graphics logger remain intrusive; this recording is attribution,
not an independent frame-delivery acceptance test.

The owned loggers have zero event/buffer loss and schema errors. CPU sample-prefix
decoding agrees with 431 xperf reference records. The exact host map comes from an
analysis-only relink with byte-identical executable code and unwind sections; it
is never launched. Windows symbols are checked against trace module identity.
Across 3201 frames, preparation sums are 2399.8956 ms wall, 2394.4486 scheduled,
1.6560 runnable, 3.7910 blocked and zero unknown. These are summed spans, not
capture duration. There are 2286 preparation profile samples; one lacks a joined
stack and is recorded as such.

The worst preparation frame lasts 6.8655 ms: 3.2591 scheduled, 0.0491 runnable,
3.5573 blocked. Uploads account for 4.7739 ms of its wall duration. Two observed
unmap scopes update just 2880 bytes and 80 bytes in pooled vertex buffers. Their
data-update spans are 3.3379 and 0.3865 ms; neither executes native creation.

Three waits of 2.8602, 0.3717 and 0.2384 ms are woken by thread 17816 in the same
host process 32708. The render-thread resume stack is:

```
RtlEnterCriticalSection / RtlpEnterCriticalSectionContended
NVIDIA user-mode driver
d3d11!CResource<ID3D11Resource>::Map<0,4>
F4GeometryReuse::map
QD3D11Buffer::beginFullDynamicBufferUpdateForCurrentFrame
QRhiBuffer::fullDynamicBufferUpdateForCurrentFrame
QSGBatchRenderer::Renderer::unmap / uploadBatch / prepareRenderPass
```

The waking thread's stack includes `RtlLeaveCriticalSection`, NVIDIA driver
addresses and the Windows thread-entry path. Private NVIDIA function names, the
lock address and the exact internal worker operation are unavailable. The trace
does not prove that transfer size, GPU saturation or a particular driver task is
the root cause. It does prove blocked time on this call path, without buffer
prewarming or PresentMon. This extends earlier upload-stage evidence with an
exact Map call and caller/readier attribution.

## Device-threading intervention

Microsoft documents
[D3D11_CREATE_DEVICE_PREVENT_INTERNAL_THREADING_OPTIMIZATIONS](https://learn.microsoft.com/en-us/windows/win32/api/d3d11/ne-d3d11-d3d11_create_device_flag)
as a request to avoid additional runtime/video-driver threads for profiling or
debugging, and advises against general use. The diagnostic adds that flag only
to the Qt QRhi D3D11 creation calls. It does not add SINGLETHREADED, change normal
thread-safety rules, disable hardware media decoding, alter presentation, or
change SVG quality. External media-device creation remains outside this hook.

Both arms use one host, selected by `F4_PROFILE_NO_DRIVER_THREADS`:
`adcd0b726fcb4e3b5089211c7a69fc28e86f081cbfeca08940a60deaa9ec3138`.
A fixed 256-entry recorder stores creation results and GetCreationFlags after
creation, with no per-frame recording. Returned flags confirm the request is
present, not that the vendor creates no workers or honors every intended effect.

Both policies pass 46 native tests, zero failures/skips. All 25 PNG files match
exactly, including 23 rendered captures and two input fixtures. The same static
QML resource graph passes the three import-isolation checks and the newly linked
host passes the system-DLL dependency audit. These are diagnostic checks, not a
portable payload rollout.

Two build-preflight guards were corrected before testing: the source contains a
D3D11CreateDevice mention in a documentation comment, and the native fixture's
response file uses an older backend-object path than the live host. Only the two
actual creation calls are rewritten; the native fixture explicitly links the
same new backend object as the host. The initial 32-entry recorder then filled
during the multi-window suite (45 pass, cleanup audit fails). That failed native
directory is retained separately. The 256-entry version records 46 device
creations per suite and passes both complete runs; it is not counted as a product
failure or hidden from verification.

## Live A/B: no promotion

The same Downloads workload and two ten-second wheel blocks use a fresh process
and isolated cache per arm. All 1729/1765 inputs are active. No screenshot, UI
Automation, build, test or offline analyzer runs during gestures. Other diagnostic
flags are identical, including the inherited-transform skip, retained geometric
snapshots and disabled buffer prewarming. PresentMon is enabled for this pair.

| Arm | Preparation p99 ms | Preparation max ms | Display max ms | Intervals >9.375 ms | Maximum motion error px |
| --- | --- | --- | --- | --- | --- |
| A1 ordinary flags | 3.1833 / 3.1732 | 6.1938 / 5.0877 | 12.5629 / 12.8340 | 13 / 29 | 99.457 / 102.067 |
| B1 prevention flag | 3.6193 / 3.7284 | 6.4527 / 5.6315 | 18.5751 / 18.7326 | 30 / 43 | 128.988 / 168.399 |

Coverage holds are zero. Each arm records four successful Qt device creations,
with returned/requested flags zero for A1 and eight for B1. This is one A/B pair,
not A/B/B/A or a repeatable regression claim. It provides no reason to promote
the candidate; the original timing/motion gates remain unchanged and failing.
No verified-warm or middle-scroll comparison is claimed for this intervention.

## Flag-enabled scheduling check and next target

`real-scroll-driverwait-b1-20261031-wheel-masonry` uses the same flag-enabled host,
PresentMon absent, and the narrow scheduler/profile workflow. All 1698 wheel
inputs are active. Logs have zero loss/decoder errors and cover both complete
windows; all 402 xperf reference sample prefixes match. The analysis-only symbol
relink again proves exact code/unwind equality.

Across 3192 frames, summed preparation is 2781.8033 ms wall, 2779.0983 scheduled,
2.5304 runnable, 0.1746 blocked and zero unknown. Maximum blocked preparation is
0.0882 ms; no frame exceeds 0.1 ms of blocked preparation. This capture has no
comparable multi-millisecond Map wait. One trace per configuration cannot prove
that the flag eliminated the rare wait or that workers are absent.

It still has preparation maxima of 8.1291 and 7.3877 ms. The worst frame is
8.1278 ms scheduled, 0.0013 runnable, zero blocked. Its before-body phase lasts
4.4775 ms while 1412 nodes are added and 1423 removed. Of the five recorded
before-body profile samples, three have dependency-cache operations at the
instruction pointer: QSet<int> rehash, Scope::blocked hash insertion (ICF aliases
retained), and Graph::addPath dependent-map lookup. Their joined stacks pass
through QSGRenderer::preprocess. The other two samples are preprocess and updater
transform traversal. Sampling does not assign an exact millisecond cost to each.

This identifies a concrete next target: rebuilding the reverse dependency graph
and its spilled integer sets during scene churn. The current four-inline-ID
optimization still uses QSet for larger sets. A replacement must preserve exact
invalidation, mutation/deletion handling, capacity fallback and bounded memory;
it must not skip preprocessing work merely to pass timing checks.

The active goal remains the full cold/warm/middle workload at actual 160 Hz and
DPR 1.75, zero intervals above 9.375 ms, at most two physical-pixel motion error,
zero coverage holds and the original native middle-velocity accuracy. These
captures do not complete it. Both diagnostic hosts and owned trace sessions are
stopped, display holds are terminal and the normal worktree app remains alive.
