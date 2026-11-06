# Masonry: admitted-node vector-owner lookup

The previous goal turn made progress by testing and rejecting converter-resource
reuse as a proven scrolling improvement. This continuation implements and tests
a direct lookup of the vector owner already recorded for admitted geometry.
Constant 160 Hz remains unachieved; this revision is not promoted.

The preceding CPU trace has 183 preparation self samples in vector-gate setup.
The existing gate setup repeatedly walks ancestors of batch-first nodes, even
when the owner of admitted geometry is already recorded. The diagnostic adds a
topology revision to that record and uses it directly when valid. New gates,
source generations and gate removal invalidate revisions. Existing recursive
shadow-node removal erases the corresponding records before reparenting/address
reuse. Revision wraparound resets stamps. Full batch membership and GPU
residency checks remain intact; ownership does not imply admission.

Six helper results pass. A native topology fixture passes three results at
actual DPR 1.75: inserting a nested hidden gate, removing it, reparenting the
geometry between two roots, and changing source generation all preserve the
expected owner and rendered pixels. It checks the geometry leaf's physical
scene origin and unit transforms, and saves initial/hidden/reparented captures.
The control passes its ownership/pixel checks and fails only the final assertion
that the direct lookup ran. The rendered output was inspected. The broader
native suite passes 48 results in each arm, shader tests 27, embedded-only import
tests three, and the Windows system-DLL dependency audit. No static product UI
layout changes. An initial build script incorrectly required an existing
renderer object in the import-test response file; it was fixed to insert that
object when absent. No SDK library or portable payload is overwritten.

Four clean cold-wheel captures use one host with only
`F4_PROFILE_VECTOR_OWNER_LOOKUP` changing. Both earlier diagnostic AV features
remain enabled in both arms; converter-resource reuse is absent. Actual
Downloads, 191 entries, Masonry density152, 3840×2160/160Hz/DPR1.75, fresh gallery
caches zzn/zzo/zzp/zzq and two ten-second rapid-wheel blocks are retained. No
compiler, tests, offline analysis, scheduler sampler or UI Automation observer
runs during gestures. Content, display, input/focus, renderer/resource and error
guards pass. Cases are `real-scroll-vector-owner-{a1,b1,b2,a2}-20261029-wheel-masonry`.

| Arm | Block | Preparation max ms | Display max ms | Intervals >9.375ms | Max motion error physical px |
| --- | ---: | ---: | ---: | ---: | ---: |
| A1 control | 0 | 5.8600 | 31.2566 | 3 | 118.625 |
| A1 control | 1 | 9.2594 | 18.7410 | 3 | 110.730 |
| B1 lookup | 0 | 5.6815 | 25.0671 | 5 | 93.806 |
| B1 lookup | 1 | 6.8582 | 13.4530 | 2 | 60.073 |
| B2 lookup | 0 | 6.0903 | 12.5219 | 2 | 6.483 |
| B2 lookup | 1 | 11.4156 | 18.7104 | 6 | 138.654 |
| A2 control | 0 | 4.7867 | 18.7551 | 1 | 15.231 |
| A2 control | 1 | 6.2488 | 12.5564 | 5 | 83.187 |

All blocks have zero coverage holds and fail the maximum two-physical-pixel
motion limit. Controls have 12 late display intervals; candidates 15. No stable
display improvement is established. The clean captures do not attribute the
long display intervals to specific waits; no scheduling claim is made.

Process-cumulative owner counters, including startup:

| Arm | Direct hits | Ancestor walks | Ancestor steps |
| --- | ---: | ---: | ---: |
| A1 | 0 | 904,456 | 21,503,965 |
| B1 | 152,287 | 751,562 | 20,078,121 |
| B2 | 148,014 | 720,575 | 19,198,272 |
| A2 | 0 | 910,506 | 21,621,440 |

This demonstrates limited coverage of the admitted-node shortcut. Remaining
queries include nodes without an admission record; the current counters do not
separate unresolved vector nodes from nodes with no vector owner. The next
experiment will cache bounded first-node resolution results, including negative
results, with topology/source invalidation and recursive removal. It must keep
admission proof separate, test the invalidation paths, and measure positive and
negative hits independently. The earlier per-ancestor hash cache remains rejected.

The diagnostic host, source, tests, reversible hooks, build receipts, capture
packets, presentation CSVs and analysis are preserved in the experiment snapshot.
The normal worktree application remains alive unchanged. No commit or push.

AI assistance has been used to create this output.
