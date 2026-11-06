# Shared text uniforms: binding-action attribution

## Finding

The proposed explanation that shared-buffer slot assignment increases text
binding churn is contradicted by this capture pair. The candidate has fewer
text SRB resource updates: approximately 5.6–6.3% of text calls versus
17.2–17.6% in the control. Most candidate text draws retain their previous
bindings. Do not add a more complicated slot-affinity policy on the assumption
that extra text binding updates offset upload savings.

This establishes action counts, not CPU time in each action. It does not prove
that sharing achieves a repeatable whole-view performance benefit. The prior
A/B/B/A live comparison remained inconclusive under temporal variation and
failed the original gates. The feature remains disabled and unpromoted.

## Read-only counter variant

Same sharing algorithm, bounded pool and original Qt 6.11.1 draw order as the
tested prototype. Before Qt's existing SRB-action switch, counters classify
DoNothing, UpdateResources and Rebake separately for QSGTextMaskMaterial and
other materials. No decision, binding, buffer or synchronization behavior is
changed. Counts are plain render-thread fields within the existing bounded
preparation scope; serialization follows the gesture. Native text action
totals exactly equal the independently counted eligible text calls in both
captures. This is structural attribution, not another controlled performance
rollout trial.

Fifteen helper QtTest results pass, including scoped/bounded action counters.
All 45 native renderer results pass with sharing off and all 45 with it on,
zero skips, at actual DPR 1.75/D3D11. All 15 rendered images are identical by
SHA-256. Static import isolation (three results) and Windows system-DLL audit
pass. The static dependency graph and QML resources remain intact.

## Capture validity

Cases `real-scroll-sharedbindings-a1-20261030-wheel-masonry` and
`real-scroll-sharedbindings-b1-20261030-wheel-masonry` use the same counter host
and differ only in the sharing flag. Actual Downloads/192 entries, density
152, 3840×2160, 160 Hz/DPR 1.75, default frame latency 2, sparse strategy off,
fresh process and empty isolated cache, two ten-second wheel windows each.
The second direction is not claimed as verified warm. Original capture-validity
checks pass; all inputs are active and native foreground ownership is stable
throughout all four windows. No screenshots/accessibility observations occur
during gestures. PresentMon supplies 3400 / 3409 target rows; inputs are
1722 / 1798. No candidate fallback or uniform-buffer creation failure occurs.

## Counts within measurement windows

| Arm/window | Text calls | Text unchanged | Text resource updates | Text rebakes | Other resource updates |
| --- | ---: | ---: | ---: | ---: | ---: |
| Control/first | 120265 | 99109 | 21134 | 22 | 39312 |
| Control/second | 119858 | 99253 | 20604 | 1 | 40401 |
| Candidate/first | 120297 | 113530 | 6747 | 20 | 40789 |
| Candidate/second | 120897 | 113330 | 7563 | 4 | 46253 |

Total resource-update actions, text plus other materials, are 121451 in the
control's two windows and 101352 in the candidate's. Exact workload and input
counts differ, so do not translate that raw delta into a CPU-time saving.
Candidate rebakes remain uncommon; no evidence here supports a text-SRB
recreation storm. Whole-capture shared hits are 161925; shared uploads 94561;
29 pool buffers are created. Whole-capture total queued uniform calls,
including shared uploads, are 914784 control and 760475 candidate.

| Arm/window | Display misses >9.375 ms | Maximum display gap, ms | Maximum motion error, physical px |
| --- | ---: | ---: | ---: |
| Control/first | 35 | 12.8242 | 125.0902 |
| Control/second | 48 | 18.6107 | 128.4764 |
| Candidate/first | 26 | 18.7691 | 124.9917 |
| Candidate/second | 8 | 12.5078 | 120.9927 |

Coverage holds are zero in every window, but none passes all original gates.
This pair does not override the inconclusive repeated performance comparison.

## Next action

The text path reduces both queued uploads and binding-update counts. Measure
full-byte duplicate payloads among other merged material classes before
deciding whether a broader shared-uniform experiment is worthwhile. Keep
unmerged/direct-update paths, explicit buffer lifetime rules and original
display/motion/coverage gates intact. Continue to treat source/texture uploads
and remaining scene work as possible causes of spikes; call-count reductions
alone do not close the constant-160-Hz goal.

## Lifecycle and evidence

The owned finite display hold is explicitly released. Only the owned diagnostic
launcher tree is stopped; normal worktree app PID 43936 is retained. Raw packets
are copied after complete analysis to
`C:/Users/xs/AppData/Local/Temp/f4-shared-bindings-live-raw-20261030/`, SHA-256
verified and recorded in per-arm raw-location receipts before removing the D:
copy. No trace is discarded. Diagnostic intermediates use the verified C:
archive with original worktree paths preserved. No commit, push, Qt SDK
modification or embedded-payload promotion.

Evidence: `profiler/experiments/masonry-shared-text-bindings-2026-10-30/`.

> AI assistance has been used to create this output.
