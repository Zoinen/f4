# Shared text uniforms: controlled live comparison

## Result

Sharing removes many queued uniform updates, but this A/B/B/A comparison does
not establish a repeatable scrolling improvement. Candidate uploads fall to
about 758k / 757k total versus 938k / 911k in the controls, including its own
shared updates. Both candidate runs still miss refreshes and exceed motion
limits. The final control is slower than the first, so it is also incorrect to
attribute the candidate's earlier worse result solely to the sharing flag.
The feature remains disabled and unpromoted. Constant 160 Hz is not achieved.

## Protocol

Same diagnostic executable and SHA-256 across A1/B1/B2/A2. Only
`F4_PROFILE_SHARED_TEXT_UNIFORMS` changes (0/1/1/0). All use default frame
latency 2, sparse strategy off, actual Downloads/192 entries, Masonry density
152, 3840×2160, actual 160 Hz/DPR 1.75, fresh processes and empty isolated
caches. Each has two ten-second wheel windows; the second direction is not
claimed as verified warm. No screenshot or accessibility observation runs
during gestures. All original capture-validity checks pass, with zero inactive
input and stable native foreground ownership across all eight windows.

| Arm | PresentMon rows | Wheel events | Shared hits | Shared uploads | Pool buffers created | Fallback / failures |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 | 3437 | 1770 | 0 | 0 | 0 | 258457 / 0 |
| B1 | 3421 | 1728 | 161864 | 94812 | 29 | 0 / 0 |
| B2 | 3394 | 1728 | 161077 | 93720 | 29 | 0 / 0 |
| A2 | 3404 | 1691 | 0 | 0 | 0 | 255439 / 0 |

The control's fallback count means deliberate use of the original path, not a
failure. Candidate hits are 63.1% / 63.2% of eligible text calls. Candidate
creation during the first measurement window is two buffers, then zero in
the second; most pool creation happens during preparation before scrolling.
Whole-capture update counts include startup. The snapshot observer counts
original queued updates; candidate totals explicitly add shared uploads.

## Original gates

| Arm/window | Misses (>9.375 ms) | Maximum display gap, ms | Maximum motion error, physical px | Preparation median / p99, ms |
| --- | ---: | ---: | ---: | ---: |
| A1/first | 28 | 12.7546 | 119.0123 | 0.5626 / 3.4671 |
| A1/second | 0 | 6.3920 | 2.5772 | 0.4290 / 2.0732 |
| B1/first | 34 | 18.7797 | 130.9293 | 0.5863 / 3.7663 |
| B1/second | 45 | 18.7181 | 137.3392 | 0.5290 / 3.8987 |
| B2/first | 36 | 18.8764 | 126.0328 | 0.5836 / 3.6269 |
| B2/second | 40 | 13.0734 | 131.9456 | 0.5289 / 3.8713 |
| A2/first | 50 | 18.7070 | 201.9880 | 0.6115 / 3.9778 |
| A2/second | 60 | 18.7792 | 226.3941 | 0.5331 / 4.2774 |

Coverage holds are zero in every window. None meets all original gates; even
A1's refresh-clean second window exceeds the 2-physical-pixel motion limit.
Those gates are unchanged. This comparison does not cover the separate
verified-warm tour or quarter-height middle-scroll acceptance workload.

The preceding prototype passed 14 helper results, 45 native rendering results
in each feature state, 15 identical rendered images, static import isolation
and the Windows DLL audit. Those establish compatibility within their scope,
not speed. No new production change is promoted based on the live results.

## Next evidence

Qt's per-element SRB path compares resource bindings and updates them when
their buffer identity changes. The pool currently assigns slots by first-seen
payload order each frame. Binding changes are a concrete candidate cost that
may offset upload savings, but they are not measured by the hit counter.
A separate read-only variant counts text and other-material SRB actions
(unchanged/update/rebake) before modifying slot assignment. Do not remove draw
ordering or weaken lifetime checks to pursue the update-count reduction.

## Storage and lifecycle

The D: disk filled before capture. Four inactive intermediate directories were
archived on C: after SHA-256 verification, preserving their paths with directory
junctions. Raw packets were copied after each complete capture and analysis to
`C:/Users/xs/AppData/Local/Temp/f4-shared-text-live-raw-20261030/`, hash-verified,
then their D: copies removed. Each arm's `raw-location.json` records both paths,
length and SHA-256. They remain uncompressed, with original bytes. Other capture
files and per-arm analyses remain under artifacts. The finite display hold was
released; no normal worktree app is stopped or replaced. No commit or push.

Evidence: `profiler/experiments/masonry-shared-text-uniform-live-2026-10-30/`.

> AI assistance has been used to create this output.
