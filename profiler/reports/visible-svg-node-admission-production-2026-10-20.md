# Visible SVG node admission in production, 2026-10-20

The tested geometry-node admission candidate is now in the portable Qt source-patch
recipe and the running worktree application. Constant 160 Hz without spikes remains
unproven; large single-batch copy costs and the earlier 6.9 ms preparation spike remain
open. This implements the next action from svg-residency-and-visible-admission-2026-10-20.md.

## Change and verification

A ready, visible SVG root previously bypassed admission when it published additional
geometry. The regression appended 24 large batches after initial readiness; both
backends uploaded all 24 in one frame when the fixture budget allowed at most five.
The renderer now tracks geometry-node membership. New members enter the existing
bounded admission queue. Rebatching or animation of known nodes keeps readiness.
Node removal, gate replacement, and renderer destruction clear membership through
a reverse index. Budgets, ordering and animation behavior are preserved.

The portable patch retains strict original Qt anchors and modifies their emitted
replacement bodies. Recipe serialization remains standalone and includes no artifact
paths or diagnostic trace flags. The content-aware cache and failed observer edit
are absent. Generated production renderer matched the previously tested candidate
with inactive diagnostic trace blocks removed.

Validation on the actual production static library at DPR 1.75:

- 36 recipe tests passed, including serialization, refresh and source drift checks.
- 44 renderer checks passed per backend, D3D11 and OpenGL, including the new regression.
- 30 camera checks and 14 SVG presentation checks passed per backend.
- Embedded-QML import isolation: three checks passed, no failure/skip.
- Windows static dependency audit passed; only permitted system DLLs.
- CGO_ENABLED=0 embedded-host extraction tests passed using the system Go cache.
- The packaged launcher started directly with --gui=qt. Its responsive child was
  extracted from the new embedded payload and matched the built host hash.

The import test retains a QPointF undefined-assignment warning; it did not fail.
These Windows checks do not assert untested cross-platform release results.

## Running build identity

Qt Quick library SHA256:
e66e1c1c16f60a8509d893d16a80b75d45d9f651811e753926bf567ced8a1f19

Worktree launcher D:/Code/f4-zoin/f4-zoin.exe SHA256:
552e2f7322e04f34caf9ebc131dfdc7a648c51f056339c26010b9a9f338e20d8

Extracted Qt host SHA256:
234013dd2e9ae27642e673f23a11ca55c41f128186a62f27e0d03457023df295

Launch receipt records Go PID 52416 and Qt PID 125140. No explicit host, frame-latency,
or pool-budget override. The canonical D:/Code/f4/f4.exe was untouched. The preceding
worktree launcher is retained as artifacts/f4-zoin-before-visible-node-admission.exe.

## Exact production-source scrolling measurement

real-scroll-visible-node-production: Downloads, 186 entries, Masonry density 152,
DPR 1.75, 160 Hz screen, 30 ms bouncing wheel, four 10-second input windows.
Finite OutSine scrolling and precise animation clock use production defaults.
Bounded renderer instrumentation and independent PresentMon were enabled; no build,
test, offline analysis or UIA observer ran during input. The receipt pins actual
production source, library, instrumentation, patch recipe and successful link.

All four blocks had zero missed display intervals (>9.375 ms), zero camera commits
outside presentation, and passed motion-error p95 <=0.5 / maximum <=2 physical pixels.
Display maxima: 6.3598 / 6.3665 / 6.3570 / 6.3685 ms.
Maximum motion errors: 0.7916 / 0.8443 / 0.7969 / 0.7098 physical pixels.
No slow upload >=100 us from a ready+visible gated SVG bypassed admission during input.

Public preparation p99 cold 2.4219/2.2111 ms; warm 2.1296/2.0704 ms.
Public preparation maximum 4.0084 ms; render-to-frame-end maximum 5.0526 ms.
Internal renderer preparation p99 cold/warm 2.0322/1.8316 ms.
Membership/admission classification p99 0.0636/0.0642 ms, maximum 0.1611/0.1392 ms.
These scopes differ and inclusive scopes must not be added. Some producer-present
intervals exceed 9.375 ms despite no missed display intervals; retain both metrics.

## Remaining spike evidence

In the preceding candidate capture, the largest pending upload was one batch with
2,054,448 vertex bytes and 186,768 index bytes. It took 3.9301 ms, with 3.0696 ms in
mapped copy, 0.0518 ms driver map and 0.0018 ms driver unmap; buffer creation also
contributed. Warm reentry of the same-sized payload took 1.4481 ms, including 1.0626
ms copy. This is not eliminated by limiting batch count: the existing 2 ms admission
limit is soft and one oversized batch can exceed it.

The latest 40-second run did not reproduce that outlier. It does not invalidate it
or prove constant 160 Hz. Next: address bounded handling/residency of large geometry
without changing authored SVG/animation semantics, then measure cold/warm wheel,
rapid wheel and middle-button scrolling on the exact packaged build.
