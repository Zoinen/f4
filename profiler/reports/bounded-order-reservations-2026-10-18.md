# Bounded drawing-order reservation trial

## Input-mode correction (2026-10-18)

A later metadata audit found that the resource/reservation captures in this
report used continuous linear animation: wheelIntervalMs is 0 and wheelEvents
is empty. The -wheel- filename suffix was misleading. Their timing tables and
native regression results remain as recorded, but these captures must not be
cited as wheel-event measurements. The runner now rejects wheel-named captures with a zero wheel interval.

## Result

The preceding turn established reservation exhaustion as an initiating cause of
large upload bursts and saved its evidence. That was progress. This turn builds
a bounded alternative, reproduces its benefit in real native regressions, and
tests clean display cadence. The prototype is not promoted: preventing the cold
rebuild does not establish a reliable overall display improvement. The full
constant-160-Hz objective remains active and unfinished.

## Prototype policy

Only full render-list reconstruction performs an extra visible-node count,
stopping at 2^20 geometry/render nodes. For smaller scenes and no explicit Qt
minimum-padding override, a stack-local padding budget is the lesser of 2^18
and 2^20 minus actual geometry/render-node count. Each root receives at least
2048 orders where the budget permits; its ordinary padding request is clamped
to the remaining budget. Once spent, later roots receive zero speculative
padding. Partial updates consume already-owned intervals as before.

This bounds total speculative padding to 262144 and total order space to
1048576 for scenes using the policy. Larger scenes and explicit Qt diagnostic
overrides retain the original policy; those paths do not claim the new bound.
The scoped thread-local pointer restores its previous value on return, with no
node/renderer ownership, persistent map, buffer handoff or resource cache.
Counting is confined to full reconstruction, not ordinary scrolling frames.
The policy does not delay source loading, rasterize SVG content or skip visuals.

The local native Qt library starts from the current integrated 6.11.1 source,
including buffer fit, stable intervals and native-child stacking. Its source
delta and the local builder are archived alongside the test patch; no production
dependency recipe or portable payload consumes this experiment.

## Native regression evidence

The cold fixture first shows an empty clipped geometry branch alongside an
unrelated red branch. Publishing 512 static green geometry nodes changes the
normal renderer's order range from 9 to 645; the new regression fails at the
range comparison before the policy. With bounded reservations, four growth/
removal cycles keep the original order range, do not increase the unrelated
branch's observed upload count, and produce the expected red/green/black pixels.
The stable red item and cold geometry item have stable object names; both scene
origins and unit transforms are checked at actual DPR 1.75. The resulting
bounded-order-cold-175.png was inspected.

A second fixture renders 512 overlapping clipped roots. It verifies global
padding bounds, every actual geometry element's interval membership, float
and 24-bit depth separation for adjacent orders, and the final green frontmost
pixel. All 512 named visual leaves have scene-space origin/unit checks. Sixteen
late additions to the final root exercise fallback when its speculative budget
has run out; range/depth assertions and rendered order remain correct.

Both new tests plus buffer-pool selection, 32-cycle scissor/stencil lifecycle,
visible/blocked opacity, ancestor reservation return and merged-retention/order
change rows pass: 18 results on D3D11 and 18 on OpenGL at 175%. The buffer fixture
asserts the actual RHI selection and actual DPR. These are focused native checks,
not a full-suite or cross-platform release claim. Experimental admission/unmerged
rows in the fixture remain outside the tested production subset.

Evidence under artifacts: bounded-orders-before-175.txt (3 pass/1 fail),
bounded-orders-after-175.txt (4 pass), bounded-orders-native-175.txt and
bounded-orders-opengl-175.txt (18 pass each). The before and after source/library
identities are preserved by the archived exact deltas.

## Clean live comparison

A uses the current normal buffer-fit renderer; B changes only the native Qt
library to the bounded prototype. Fresh processes use Downloads, 184 entries,
density 152, extent 4161.142857, actual DPR 1.75, active window and 160 Hz. Continuous
linear animation runs for four seconds each direction. Folder state is
restored between runs. No build, tests, sampler, ETW capture, owner traversal,
resource observer or debug logging runs during clean capture. Caption
measurements remain zero. Actual scene order ranges verify the policy takes
effect: A 1401-2126; B remains 223606 or 223640 throughout each run.

PresentMon display intervals require both display endpoints inside the same
scrolling observation window. This applies equally to every run and retains
all interior stalls. Native frame periods are not substituted for display time.

| Run | Down p99 ms | Down max ms | Down >9.375 ms | Up p99 ms | Up max ms | Up >9.375 ms | Max render wall ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 | 6.3274 | 24.9875 | 5 | 6.3084 | 12.4990 | 2 | 13.1167 |
| B1 | 6.3954 | 18.7417 | 6 | 6.2823 | 12.5274 | 2 | 12.4507 |
| B2 | 6.3939 | 12.5125 | 4 | 6.3673 | 12.4952 | 3 | 9.6362 |
| A2 | 12.4568 | 18.8136 | 7 | 6.3112 | 12.4910 | 2 | 14.1209 |

B1 slightly worsens the first pair's long count and p99 while reducing its
maximum; the reverse pair has fewer downward gaps but one extra upward gap.
All candidate directions total 15 long intervals against 16 for the controls.
This small mixed difference does not establish a reliable overall improvement.
The lower render maximum in B2 is real but does not remove the B1 12.45 ms render
wall or display gaps. No run is discarded on an unsupported scheduling theory.

Local stems are bounded-orders-{a1,b1,b2,a2}-wheel-masonry. Compact summaries
including actual order ranges are archived under
profiler/experiments/bounded-order-reservations-2026-10-18/display-summary.json;
raw CSV/JSON, generated binaries and logs stay untracked under artifacts.

## Restored state and next investigation

The tested renderer/test deltas and builder are archived. The tracked native
test source is restored from HEAD, its timestamp refreshed, recompiled and
linked against the normal buffer-fit library. Focused normal native rerun:
16 pass at actual DPR 1.75, including both clip lifecycles and buffer selection.
The local imported Qt library remains qt-buffer-pool-fit/Qt6Quick.lib.

The panel state is restored before restart. Only the process tree whose
executable resolves to this worktree's f4-zoin.exe is replaced. The normal
portable launcher starts directly with --gui=qt after clearing host, backend,
GC, scale and reservation overrides. Its extracted child matches the normal
host build hash, title identifies zoin_branch/cached-pixel-grid and startup
stderr is empty. Canonical D:/Code/f4/f4.exe is untouched. Evidence:
artifacts/bounded-orders-normal-identity.json.

- Host SHA256: 549f063f2f8e3252f5784f0b017d6500b719102ff799fb49b279964e44383bed
- Launcher SHA256: b9c479436f6fbc954f0eabccf67594844a1e35fd962a3c7eab40eb6e4e7f9d58

No production policy change, commit or push is made. The evidence rules out
simply shipping a bounded larger padding default as a complete solution.
The next capture should attribute the remaining long frames with stable order
ranges, separating actual GUI/render work from scheduling or backend waits.
Use native samples/context switches and displayed-frame correlation, without
per-ancestor scene observers. Preserve source-to-visible latency and complete
vector rendering as invariants; a smaller p99 alone is not the end state.
