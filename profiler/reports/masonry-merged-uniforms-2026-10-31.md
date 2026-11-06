# Merged uniform sharing: census, validation and live results

Measurements started on 2026-10-30 and continued on 2026-10-31.

## Outcome

The broader prototype serves identical uniform bytes to merged draws across
material classes while preserving their shader/sampler bindings and draw order.
It removes roughly 47% of measured material-uniform uploads. Native pixel tests
pass, but A/B/B/A scrolling does not establish a repeatable improvement and
still fails the original display/motion gates. The feature remains disabled
and unpromoted. Constant 160 Hz has not been achieved.

A synthetic lookup benchmark identifies a faster hash implementation, but its
estimated absolute saving is tens of microseconds per frame, below the measured
millisecond stalls. The next useful attribution is a CPU/scheduling trace of
the remaining scene preparation, image/geometry transfer and synchronization
work. Do not claim a spike fix from fewer calls or a faster microbenchmark.

## Full-byte census

Case `real-scroll-mergedcontent-a1-20261030-wheel-masonry` uses actual
Downloads/192 entries, density 152, 3840×2160, 160 Hz/DPR 1.75, frame latency 2,
fresh process and empty isolated cache. The second direction is not a
verified-warm claim. Original capture-validity checks pass; all 1773 wheel
events are active and native foreground ownership is stable. No screenshot or
accessibility observation occurs during gestures.

The observer compares complete payloads after hashing, per renderer preparation
pass, with fixed 512-key tables and 256-byte maximum payloads. It does not change
GPU updates. Queries include all material classes; unmerged and direct-update
paths remain excluded from sharing eligibility. Eleven helper results and all
44 native rendering results pass, zero skips, with static import isolation and
the Windows system-DLL audit.

| Census | Whole capture | First window | Second window |
| --- | ---: | ---: | ---: |
| Material-uniform queries | 939214 | 431772 | 454720 |
| Eligible merged queries / queued updates | 678950 | 310562 | 322068 |
| Identical queued payload repetitions | 441228 | 203269 | 206846 |
| Distinct merged payloads, summed per pass | 237722 | 107293 | 115222 |
| Median distinct payloads per pass | 67 | 67 | 68 |
| Maximum distinct payloads per pass | 95 | 91 | 95 |

All queries in this capture queue updates. Thus per-pass deduplication could
avoid 441228 of 939214 wrapped material-uniform calls (46.98%). The observer
records 260264 unmerged queries, zero direct/oversized/depth/key-capacity
bypasses, and 3444 complete preparation scopes. Those counts do not include
every possible QRhi update outside the wrapped material path, such as stencil
setup. The observed maximum fits the prototype's 128-value bound; fallback
remains necessary for other workloads or multiple passes within one epoch.

The census itself still misses 11 / 9 refreshes, with maximum display gaps
12.5750 / 12.5275 ms and motion errors 108.8373 / 117.6421 physical pixels.
It is attribution evidence, not a performance comparison with another host.

## Prototype and native validation

The experimental flag is `F4_PROFILE_SHARED_MERGED_UNIFORMS=1`; default is off.
The existing immutable-within-epoch content book, 128-buffer limit, renderer/RHI
ownership, original fallback and original-batch invalidation are retained from
the text-only prototype. The eligibility restriction to native text is removed
for merged queued updates at offset zero, up to 256 bytes. Key equality includes
size and every payload byte. The original binding point, stage flags, size and
sampler updates remain per draw. No class-dependent interpretation or partial
matrix comparison is used. Unmerged/direct paths retain original behavior.

Source: [SharedMergedUniform20261030.h](../../artifacts/shared-merged-uniform-20261030/SharedMergedUniform20261030.h).
The epoch still uses diagnostic D3D11 frame tracking and is not a production
portable Qt frame API. Device-loss and standalone cleanup coverage remain
required before rollout, as documented for the predecessor. No Qt SDK, shipped
payload or production default is changed.

Fifteen helper QtTest results pass. All 45 native rendering results pass with
sharing disabled, then all 45 pass enabled; zero skips at actual DPR 1.75/D3D11.
Fifteen captured window PNGs match exactly between runs, as do the two generated
PNG inputs. The expanded fixture checks every one of 16 named text/image/effect
leaves for whole physical scene origins and unscaled unit vectors. It checks
painted pixels in every small text label, opaque-image colors, shader color
permutation and opacity, and the distinct transparent source. It changes font,
colors, window size, forces fallback, resumes sharing and replaces the window.
Enabled execution asserts non-text shared uploads occurred, as well as hits.
The shader is baked with the installed Qt 6.11.1 qsb tool; no new dependency.

The first expanded fixture waited on both ShaderEffect statuses being Compiled
and failed with sharing off. Diagnostics showed `opaque=1 alpha=0 enum=0`,
empty logs, while the rendered effects already produced the correct colors.
The final fixture rejects shader errors and waits for the expected actual
shader pixels instead of relying on that status. The failure log and diagnostic
image are retained. This is not evidence of a sharing failure or a diagnosed
root cause inside Qt's status implementation.

Three static import-isolation results and the system-DLL audit pass. No claim
of completed portable rollout follows; the Go embedded payload is unchanged.

## Controlled live comparison

Cases `real-scroll-sharedmerged-{a1,b1,b2,a2}-20261030-wheel-masonry` use the
same executable/hash and differ only in the flag (0/1/1/0). Dataset, density,
display, default frame latency 2, sparse mode off, fresh process/isolated empty
cache, active input and foreground checks match the census protocol. All four
original capture-validity checks pass. The two ten-second directions are not
substitutes for the separate verified-warm and middle-scroll acceptance runs.

| Arm | PresentMon rows | Inputs | Shared hits | Shared uploads | Buffers created | Fallback / failures | Total wrapped + shared uploads |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 | 3460 | 1775 | 0 | 0 | 0 | 687926 / 0 | 946363 |
| B1 | 3408 | 1720 | 431915 | 243053 | 94 | 0 / 0 | 502515 |
| B2 | 3407 | 1785 | 431642 | 246217 | 98 | 0 / 0 | 504173 |
| A2 | 3404 | 1727 | 0 | 0 | 0 | 669471 / 0 | 925836 |

The control fallback counts denote deliberate use of the original path.
Candidate hits include 263616 / 263501 non-text draws and 168299 / 168141 text
draws. Upload and buffer counts include preparation before measurement. B1
creates 19 buffers in the first window and zero in the second; B2 creates
18 / 5. Those counts alone cannot explain the stalls: B1's zero-creation second
window misses 39 refreshes, while B2's five-creation second window misses none.

| Arm/window | Misses >9.375 ms | Maximum display gap, ms | Maximum motion error, physical px | Preparation median / p99, ms |
| --- | ---: | ---: | ---: | ---: |
| A1/first | 22 | 12.5445 | 115.5169 | 0.6036 / 3.6361 |
| A1/second | 0 | 6.4271 | 2.3853 | 0.4378 / 2.0307 |
| B1/first | 18 | 12.5516 | 111.3420 | 0.5744 / 3.5716 |
| B1/second | 39 | 12.7351 | 117.6883 | 0.5764 / 3.8033 |
| B2/first | 33 | 12.7371 | 119.2073 | 0.5979 / 3.9316 |
| B2/second | 0 | 6.4465 | 4.0848 | 0.4639 / 2.0219 |
| A2/first | 32 | 12.6542 | 115.4080 | 0.5720 / 3.6617 |
| A2/second | 41 | 18.7083 | 116.1896 | 0.5287 / 4.3807 |

Coverage holds are zero throughout. None meets all original gates. Even the
refresh-clean windows exceed the unchanged 2-pixel motion-error limit. This
comparison neither proves a consistent gain nor a consistent regression from
sharing. It establishes upload reduction and the remaining failure scope.

## Lookup microbenchmark

The independent CPU-only benchmark compares the current FNV-1a hash with Qt's
`qHashBits(data,size,0)`, both raw and with the actual bounded content-book
lookup. It uses one runtime-generated synthetic corpus, repeated A/B/B/A order,
64/76/96/112/160/256-byte payloads, aligned and unaligned raw reads, and 70
distinct keys per simulated 200-query pass. Results contain 72 records. This
is not a replay of captured uniforms or a frame-time benchmark.

| Payload bytes | FNV hash + lookup, ns/query | Qt hash + lookup, ns/query | Ratio |
| --- | ---: | ---: | ---: |
| 64 | 71.31 | 30.94 | 2.30× |
| 76 | 78.51 | 30.40 | 2.58× |
| 96 | 96.15 | 36.39 | 2.64× |
| 112 | 110.95 | 39.86 | 2.78× |
| 160 | 161.63 | 51.45 | 3.14× |
| 256 | 241.74 | 68.04 | 3.55× |

At 200 operations the synthetic saving is roughly 8–35 microseconds. Real
acquisition also includes resource bookkeeping and queued updates; this table
does not time those. The lookup outcomes/checksums must agree across hash
choices, and full-byte collision checks remain necessary. They agree in the
recorded run with reproducible corpus seed `0x351085c1`. An initial benchmark
included the warm-up hash sum in its reported lookup digest; verification
rejected that evidence snapshot. The corrected run resets the digest before
the timed loop and asserts equal lookup outcomes. The initial source/results
are retained separately. No hash replacement
has been promoted based on these numbers. The next trace must account for the
much larger remaining preparation/display stalls and the motion errors in
refresh-clean windows.

## Lifecycle and evidence

All owned finite display holds are released and their sessions completed.
Only the diagnostic launcher tree is stopped; normal worktree app PID 43936
remains alive. Raw packets are preserved with SHA-256/location receipts on C:
after each completed analysis. Inactive intermediate directories are archived
with full hash verification and their original paths preserved by junctions;
the later space-reclamation script also rejects nested reparse points. No trace
is discarded. No commit or push is performed.

Evidence: `profiler/experiments/masonry-merged-uniforms-2026-10-31/`.

> AI assistance has been used to create this output.
