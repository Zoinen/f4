# Masonry: clean batch roots under inherited matrix changes

## Outcome

Constant 160 Hz remains unachieved. A new renderer prototype removes an unnecessary
traversal of clean batch-root subtrees when only their ancestor matrix changes.
Native tests prove the intended path and pixel equivalence. Cold A/B/B/A captures
show about 45% fewer updater node visits and consistently lower before-body p99,
but do not show reliable elimination of long displayed intervals. The verified-warm
pair still fails the original maximum-motion gate. Keep this experimental.

The preceding turn was progress: it fixed and tested snapshot geometric growth,
then established that fewer allocations did not remove the cold stalls. This turn
uses that evidence to examine scene traversal instead of continuing allocation
tuning. No product source, installed Qt archive, portable payload, normal app,
commit or push changes.

## Mechanism and correctness conditions

The existing Qt updater already skips a batch root's clean descendants when only
that root's own matrix changes. In `Updater::visitTransformNode`, its combined
matrix is updated first; descendant batch roots receive `updateRootTransforms`.
Geometry expressed in the root's local coordinate system remains unchanged.

The prototype adds the equivalent path for a clean batch root reached under an
inherited transform change. It requires an existing batch root, no node additions,
no forced update, no opacity change, an inherited matrix change and no dirty
subtree state beyond the root's own matrix. Descendant dirty bits veto the skip.
It updates nested root transforms and returns; it does not omit geometry from
render lists, change admission, rasterize vectors, reduce image quality or modify
pixel alignment. Existing behavior is selected by default; the diagnostic flag
`F4_PROFILE_INHERITED_TRANSFORM_SKIP=1` enables the candidate.

Both arms have identical instrumentation. Node counters are thread-local during
the updater pass and aggregated through five relaxed atomics once per pass; there
is no atomic increment or timer per node. They still have observer overhead, and
whole-capture totals are not a per-frame timing attribution. Candidate counts can
be lower because skipping an outer clean root avoids visiting eligible descendants.
No claim of equal candidate counts is made.

## Regression and native checks

The focused regression moves an ancestor while modifying a sibling, retaining a
clean 400-tile subtree and a nested clip. Later phases change an internal color,
clip position and opacity, verifying that descendant changes still render. Actual
DPR is 1.75. Named leaves' scene-space origins and effective unit transforms are
checked after settling; rendered images verify every phase.

The control fails the explicit `F4_REQUIRE_TRANSFORM_SKIP` assertion before the
candidate is enabled. In this fixture, the control visits 4069 nodes; the candidate
visits 3265, skipping one eligible subtree. Both policies pass all 46 native tests,
without skips. All 25 PNG files are byte-identical (23 rendered captures and two
input fixtures), including small native-text captions and nested clip captures.
The small-caption image was visually inspected. The unchanged static QML import
graph passes the three import-isolation checks; the new host passes its Windows
dependency audit. These diagnostic checks do not constitute a portable rollout.

The single host is pinned by `transform-traversal-20261031/probe-build-receipt.json`.
Only the inherited-transform flag differs between live arms. Retained snapshots
and geometric growth are enabled in both; asynchronous buffer prewarming stays
off. All other flags, presentation mode and resource graph are fixed.

## Live workload and results

Actual Downloads has 192 entries including parent, Masonry density 152, desktop
3840×2160, refresh 160 Hz and actual DPR 1.75. Cold tests use fresh isolated caches
and normal viewport loading. Verified-warm tests first tour all 192 entries through
normal visibility and require quiescent previews. Both ten-second wheel blocks
have zero inactive inputs. PresentMon and the existing native diagnostic probes
remain enabled. No screenshots, UI Automation calls, builds or offline analyses
run during gestures. Native foreground and display guards pass.

Cold capture order is A1/B1/B2/A2, without retries. Values in paired columns refer
to the two scrolling directions. Times are milliseconds, motion is physical pixels.
All coverage-hold counts are zero.

| Arm | Visits, whole capture | Before-body p99 | Preparation max | Long displayed intervals | Maximum motion error |
| --- | ---: | --- | --- | --- | --- |
| Cold A1 control | 3017836 | 0.9171 / 0.8883 | 9.7403 / 4.8400 | 29 / 38 | 123.259 / 131.479 |
| Cold B1 candidate | 1663078 | 0.8054 / 0.6030 | 5.8026 / 4.4862 | 36 / 9 | 124.150 / 111.223 |
| Cold B2 candidate | 1634828 | 0.7063 / 0.6510 | 6.8547 / 7.1968 | 25 / 45 | 248.224 / 128.283 |
| Cold A2 control | 3034964 | 1.0138 / 0.9888 | 5.5510 / 6.7785 | 17 / 41 | 121.591 / 114.501 |
| Verified warm A1 control | 3053515 | 0.4625 / 0.4132 | 2.4348 / 2.2370 | 0 / 0 | 3.3211 / 3.6756 |
| Verified warm B1 retry candidate | 1721687 | 0.4958 / 0.4493 | 2.5284 / 1.9991 | 0 / 0 | 1.9419 / 2.5928 |

Before-body includes updater traversal and preprocessing. Its cold p99 improves
consistently across these four runs; warm p99 does not. Whole preparation medians
and long-interval counts do not support a reliable overall delivery improvement.
The original gates remain zero intervals above 9.375 ms, maximum motion error at
most two physical pixels and zero coverage holds. Neither cold arm passes them;
the warm candidate passes motion only in its first direction. The second direction
of a cold run is not described as verified warm.

The original warm B1 attempt failed the free-space preflight before launching its
application or starting gestures. Its logs are retained. Completed build folders
were copied to the named C: archive with hash checks and original-path junctions;
a fresh `transformwarmretry` namespace then captured the candidate. Warm is one
A/B pair, not A/B/B/A. Middle scrolling was not measured in this experiment; the
full goal still requires it, including native velocity accuracy.

## Disposition and next work

This is an independently tested reduction in redundant updater work. It does not
resolve the dominating cold material/upload/scene-admission spikes or establish
constant 160 Hz. Do not promote it as a verified FPS fix. Further work must isolate
the residual slow frames with scheduling and symbol-verified CPU attribution,
then measure changes against the original full workload and gates. More aggressive
dirty-state propagation changes require new invalidation regressions before use.

Raw captures are preserved on C: with hash receipts, compact evidence is frozen
under `profiler/experiments/masonry-inherited-transform-traversal-2026-10-31`, the
owned diagnostic process tree is stopped, and the display hold is released.
The normal worktree Go and Qt application processes remain alive. No commit/push.
