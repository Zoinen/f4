# Masonry: bounded word sets for dependency IDs

## Outcome

Constant 160 Hz remains unachieved. A bounded word-set prototype reduces the
allocation regression from 4416 to 320 allocations and improves graph helper
timings while preserving differential, invalidation, mutation and native pixel
tests. Cold A/B/B/A does not establish a delivery benefit; the repeated candidate
is worse. The verified-warm pair has zero long displayed intervals but still
fails the original motion limit. Keep the prototype unpromoted.

The previous goal turn made progress by separating a D3D11 Map wait from a
scheduled CPU spike and locating dependency-set hashing on the latter's stack.
This turn implements and measures that specific hypothesis. No product source,
installed Qt archive, embedded payload, normal app, commit or push changes.

## Representation and bounds

The existing implementation keeps four IDs inline and spills larger sets into
QSet<int>. The candidate retains four inline IDs, then stores sorted words:
each word identifies a 64-ID block and contains its membership bits. IDs are the
existing graph record indices, 0 through 16383. The vector grows geometrically,
with at most 256 words and 4096 bytes of word payload per set on this build.
Small removals demote to inline storage; larger sparse reductions use a fourfold
slack threshold to avoid retaining a full vector for a tiny set. Explicit squeeze
preserves contents. Out-of-range IDs retain the existing hash semantics.

Membership, removal, duplicate suppression and iteration are exact. Sorted
iteration changes internal invalidation order but does not change which entries
are invalidated; tests exercise that contract. No graph dependency, ancestor walk,
preprocess callback or renderer node is omitted. Graph entry/ancestor/link limits,
revision handling, structural invalidation and capacity fallback are unchanged.

The first implementation used 80 bytes per set. A fixed four-element inline
container reduces the final object to 56 bytes, versus the previous 48 bytes.
This extra eight-byte cost per object is explicit. Both live arms use the same
56-byte object, with F4_PROFILE_DEPENDENT_WORDS choosing hash or word spill.
The original 48-byte helper is also measured separately. Capacity statistics
have different units: hash entry slots versus 16-byte word slots. Their numeric
values must not be compared as total memory or process footprint.

The exact host SHA256 is
`9da3cce6261df59f66abfcb8b7c7f4354be1a3f5a4cdde456c619389fa23f914`.
Both QSGRenderer and QSGNode notification objects and the host snapshot consumer
are rebuilt with consistent definitions. The batch renderer, inherited-transform
skip and geometric snapshot implementation remain fixed. Driver-threading
prevention stays disabled, as the previous experiment did not justify it.

## Regression, correctness and native checks

Before implementation, 64 sets grow from empty to 1024 IDs each. Global new/new[]
hooks count 4416 allocations, exceeding the 384 regression budget. Contents and
checksum pass. The final candidate needs 320 counted allocations, with the same
checksum 33521664 and valid contents. These hooks do not count every possible
direct CRT allocation or establish a frame-time improvement.

Final helper results per policy:

- Seven set checks pass, including 100000 mixed signed-ID operations and 200000
  positive operations over the complete supported range, boundary bits, sparse
  and dense sets, copies, moves, independent mutations, promotion/demotion and
  out-of-range fallback.
- Fourteen bounded graph checks and nine eligibility checks pass, preserving
  invalidation, retirement, rollback and capacity behavior.
- The real in-flight mutation fixture has three passing QtTest results.
- All 46 native rendering results pass at actual DPR 1.75, without skips. Native
  counters record eight word promotions, zero recorded word-to-hash fallbacks,
  and zero graph failures. All 25 image files match between arms: 23 rendered
  captures and two fixture inputs. Static visual leaf origins/unit transforms
  remain covered by the existing fixtures. The small native captions were
  visually inspected in the final capture.
- Three embedded import-isolation results and the new host's Windows dependency
  audit pass. These diagnostic checks do not constitute a portable rollout.

One new test initially required an empty hash fallback to have zero capacity.
The original Qt behavior retains a minimum capacity of 64. The failed output is
preserved, and the test now verifies empty contents, no word payload and that
existing bound. The allocation budget and performance gates are unchanged.
An initial helper compile also collided with Qt's `slots` macro; the local
counter was renamed. Neither is counted as a successful regression run.

The final imports link initially failed with LNK1180 (disk space). Completed
objects and inputs were copied into the named C: archive with hash verification
and original-path junctions. The resume script verifies unchanged object/header
hashes, relinks, then runs all native/mutation/import checks. The normal app
remained alive throughout. No installed SDK or Qt archive was modified.

## Helper benchmark

The unchanged tree has 384 leaves, 2386 unique ancestor keys and 9600 reverse
links. Each run records 200 rebuild samples and 2000 eight-path churn samples;
the checksum is identical across policies. The final comparison order is
original 48-byte / hash 56-byte / word 56-byte / word / hash / original.

| Policy | Rebuild medians, ms | Eight-path churn medians, µs |
| --- | --- | --- |
| Original 48-byte hash spill | 0.8678 / 0.9045 | 18.6 / 18.4 |
| Same-host 56-byte hash spill | 0.9463 / 0.9031 | 19.5 / 20.1 |
| Final 56-byte word spill | 0.7069 / 0.7163 | 14.3 / 14.4 |

The word payload at the benchmark's sampled full graph is 3392 bytes, excluding
wrappers, hash tables, graph records, allocation headers and other process data.
An earlier 80-byte representation and its benchmark are preserved separately;
those numbers are not substituted for the final implementation.

## Live cold A/B/B/A

Actual C:\Users\xs\Downloads contains 192 entries including parent. Masonry
density is 152; desktop 3840×2160, actual refresh 160 Hz and actual DPR 1.75.
Each process has a fresh isolated application cache and uses normal viewport
loading. Two ten-second bouncing wheel windows are measured. No screenshot,
UI Automation query, compiler, test or offline analyzer runs during gestures.
PresentMon and the existing native probes remain enabled. All input, foreground,
display, bounded-record and feature-use guards pass. Inputs per arm are
1773 / 1734 / 1696 / 1706; inactive inputs are zero. Graph failures, capacity
bypasses and recorded word-to-hash fallbacks are zero. Candidate promotions are
550 and 538 over the complete captures.

Each paired value corresponds to one wheel direction. Times are milliseconds,
motion is physical pixels, and all coverage holds are zero.

| Arm | Before-body p99 | Preparation max | Display max | Intervals >9.375 ms | Maximum motion |
| --- | --- | --- | --- | --- | --- |
| A1 hash | 0.7263 / 0.5455 | 7.9671 / 4.2170 | 12.6684 / 18.7533 | 29 / 10 | 116.667 / 99.121 |
| B1 words | 0.7612 / 0.7054 | 5.0781 / 5.9003 | 18.7191 / 12.6491 | 23 / 35 | 114.253 / 110.659 |
| B2 words | 0.8949 / 0.8496 | 5.5931 / 9.0808 | 12.6480 / 18.6978 | 40 / 60 | 114.369 / 120.604 |
| A2 hash | 0.9415 / 0.7066 | 5.7345 / 6.7311 | 18.7691 / 12.5888 | 31 / 27 | 167.646 / 114.436 |

These four captures do not demonstrate a consistent cold improvement. This
workload includes source completion and scene churn, whose timing differs
between runs. There is no new scheduler trace in this turn; wall-time phase
results must not be labeled pure CPU time. The second direction of a cold test
is not relabeled as verified warm.

## Verified-warm pair

The existing tour verifies all 192 entries displayed through normal visibility
and quiescent before measurement. All 1807/1812 inputs are active. This is one
A/B pair, not a repeatability or A/B/B/A claim. Word promotions are 563 in B1.

| Arm | Before-body p99 ms | Preparation p99 ms | Display max ms | Long intervals | Maximum motion px |
| --- | --- | --- | --- | --- | --- |
| A1 hash | 0.5548 / 0.5300 | 1.8987 / 1.9402 | 6.3780 / 6.6940 | 0 / 0 | 1.9752 / 7.2543 |
| B1 words | 0.3710 / 0.3767 | 1.7066 / 1.7700 | 6.4400 / 6.5137 | 0 / 0 | 3.6701 / 2.8492 |

Both arms have zero coverage holds. B1 still fails the maximum-two-pixel motion
limit in both directions. No middle-scroll measurement or full-goal acceptance
is claimed. The original display, motion, coverage and native middle-velocity
gates remain intact.

## Disposition and next hypothesis

The representation is tested and the helper improves; that is insufficient to
promote it as a scrolling fix. The broader graph still stores and rebuilds a
complete ancestor vector and reverse links for every preprocessing entry. The
benchmark alone duplicates 9600 links across 2386 unique ancestor keys. Sharing
ancestor topology is a justified next experiment, not an implemented change.
It must preserve structural mutation, address reuse, invalidation and fallback
semantics before any performance claim. Neither another integer-set tweak nor
changing the success thresholds is justified by these captures.

Sources, summaries, raw-data locations and hashes are frozen under
`profiler/experiments/masonry-dependency-word-sets-2026-10-31`. Owned diagnostic
processes/loggers are stopped, the display hold is released and the normal
worktree app remains alive. The full 160 Hz objective stays active.
