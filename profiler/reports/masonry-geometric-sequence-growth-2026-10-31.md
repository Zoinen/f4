# Masonry: bounded geometric growth for retained snapshots

## Outcome

Constant 160 Hz remains unachieved. This experiment fixes an allocation-growth
regression in the experimental retained render-order storage. It proves fewer
allocations, but establishes no reliable scrolling improvement. Keep the
prototype experimental; the normal application and portable payload are unchanged.

The original user acceptance gates remain: actual Downloads, all 192 entries,
Masonry density 152, 3840×2160 desktop at 160 Hz and actual DPR 1.75, cold wheel,
verified full warm tour then wheel, quarter-height middle scrolling, zero displayed
intervals above 9.375 ms, maximum motion error at most two physical pixels and no
coverage holds. This turn does not satisfy those gates or measure middle scrolling.

## Regression and implementation

Before changing storage, a regression ramps four simultaneous snapshots from
zero through 4096 elements. The original exact-reserve implementation performs
16436 heap allocations over 16388 snapshots, exceeding the test bound of 128.
Lookup correctness and memory bounds pass. The unchanged original header remains
available, and the new header's exact-growth control produces the identical result.

The candidate grows the ordered vector to the next power of two, retaining the
existing power-of-two position table. It performs 104 allocations on the same
regression, with the same 655360 retained bytes and valid positions. The existing
four-lease, 16384-element, 2.5 MiB-per-thread bounds and fallback are preserved.
The full helper passes 1048576 lookup comparisons, generation/move/nesting/thread
checks and 4000 warmed snapshots with zero allocations. This is allocation
evidence, not evidence of constant frame delivery.

Both policies run in the same diagnostic host, selected once per process by
`F4_PROFILE_GEOMETRIC_SEQUENCE`. Retained snapshots are enabled in both arms;
this comparison is exact versus geometric retained storage, not original QHash
versus retained storage. Host SHA256:
`76ed2c6df61d05ed2ce681fc6a7c9638f692aec556d2389822c02bbacfe991c6`.

## Native verification

Both policies pass all 45 native renderer tests, without skips, at DPR 1.75.
Explicit counters confirm the intended path executes. All 17 PNG outputs match
between arms and the frozen previous prototype (15 rendered captures and two
fixture inputs). The import-isolation checks and DLL dependency audit pass.
No SVG raster fallback, quality reduction or replacement of static leaf alignment
is introduced. Source/header/object hashes and test logs accompany the experiment.

## Live results

Cold captures use fresh isolated caches and normal viewport loading. Warm captures
first verify all 192 entries displayed and quiescent through the existing tour;
the second direction of a cold test is not relabeled as verified warm. All runs
have zero inactive wheel inputs. No screenshots, UI Automation queries, builds or
offline analyses occur during gestures. PresentMon and existing diagnostic probes
remain enabled, so measurements retain their established observer limitations.
Buffer prewarming stays off; all other flags and the host are fixed.

Each row lists the two ten-second wheel blocks. Preparation and display values are
milliseconds; motion values are maximum physical-pixel errors using the original
metric. Long intervals exceed 9.375 ms. Coverage holds are zero in every block.

| Workload / arm | Storage growths, whole capture | Preparation max | Display max | Long intervals | Motion max |
| --- | ---: | --- | --- | --- | --- |
| Cold A1 exact | 118 | 5.965 / 5.572 | 12.542 / 12.6653 | 32 / 37 | 104.898 / 122.333 |
| Cold B1 geometric | 18 | 6.5256 / 6.2001 | 18.6922 / 12.7085 | 31 / 45 | 114.530 / 122.950 |
| Cold A2 exact | 120 | 7.4908 / 8.0949 | 25.0354 / 18.7634 | 31 / 53 | 114.843 / 129.997 |
| Cold B2 retry geometric | 18 | 8.0032 / 7.6159 | 12.5724 / 12.7187 | 34 / 42 | 123.568 / 118.707 |
| Verified warm A1 exact | 124 | 2.8788 / 2.5691 | 6.3925 / 6.4124 | 0 / 0 | 2.0373 / 3.1813 |
| Verified warm B1 geometric | 18 | 2.4014 / 2.3050 | 6.4572 / 6.4240 | 0 / 0 | 2.0392 / 2.7606 |

Cold order is A1, B1, rejected B2, A2, fresh B2 retry. The original B2 timed out
waiting for activation before gestures started. Its logs are retained. This is
not a complete A/B/B/A series. Warm is one A/B pair, not a repeatability claim.
In warm A1/B1 the preparation medians are approximately 0.42–0.43 ms for both.
Growth counters cover the entire capture; they do not establish which allocations
occurred inside a measured frame. Both warm arms still fail the two-pixel maximum.

The prototype-specific validity check now bounds growths by twice requests: each
begin can grow at most two arrays. The former fixed 128 check was inappropriate
for the exact-growth control as its maximum increased. No performance, quality,
foreground, warm-tour, or motion acceptance threshold was relaxed.

## Disposition and next evidence

The allocation regression is fixed and tested in the prototype. Cold preparation
spikes and missed delivery remain; this change alone is insufficient. Do not
promote it as a proven FPS improvement. The remaining investigation must attribute
the cold material/scene/upload peaks and distinguish application CPU work from
driver or scheduling waits, using the prior symbol-verified scheduling workflow.
Warm motion errors and the separate middle-scroll acceptance remain outstanding.

Raw packets are hash-verified on C: with location receipts; compact results and
sources are frozen separately. The owned diagnostic tree is stopped, the display
hold is terminal, and the normal worktree application remains alive. No installed
Qt archive, product source, embedded payload, commit or push changes in this turn.
