# Masonry relayout follow-up and remaining delivery failures, 2026-10-31

## Decision

Keep the running-thumbnail preservation fix under validation. Its repeated cold
capture completes the thumbnails and has no subsequent reverse-pass thumbnail
activity. Verified warm wheel and quarter-height middle captures are also free
of new thumbnail work. The native middle camera meets its velocity bound.

Constant 160 Hz without spikes remains **unachieved**. Every scope still fails
the original full acceptance. A one-frame presentation queue does not rescue
the fully warmed wheel case; do not change the default queue depth. No new
production code, normal application binary, Qt SDK, OS setting, external-app
affinity, commit or push changes belong to this follow-up.

## Captures

All runs use the same corrected executable and its verified build receipt:
`artifacts/thumbnail-relayout-host-20261031/probe-build-receipt.json`.
Source files still match that receipt. The actual Downloads folder has 192
entries, Masonry density is 152, refresh is 160 Hz and actual DPR is 1.75.
Cold captures start with fresh isolated caches at the normal initial viewport.
Warm and middle captures first verify all indices 0..191 displayed and
quiescent, with normal visibility preserved. Foreground remains stable, scene
registry mismatches and bounded-observer overflows are zero. No build, offline
analysis, screenshot or accessibility inspection runs during measured input.

| Scope | Case prefix / arm | New requests, passes 0/1 | Preparation p99, passes 0/1 | Missed display intervals, passes 0/1 |
| --- | --- | --- | --- | --- |
| Cold, previous candidate | thumbnailrelayoutcold / b1 | 146 / 0 | 2.0674 / 1.6548 ms | 2 / 0 |
| Cold, repeated candidate | thumbnailrelayoutcold / b2 | 137 / 0 | 1.9876 / 1.7897 ms | 4 / 4 |
| Verified warm wheel | thumbnailrelayoutwarm / b1 | 0 / 0 | 1.6906 / 1.6660 ms | 0 / 1 |
| Verified warm middle | thumbnailrelayoutmiddle / b1 | 0 / 0 | 1.6473 / 1.5823 ms | 1 / 3 |
| Verified warm wheel, queue depth 1 | thumbnailrelayoutlatencyonewarm / b1 | 0 / 0 | 1.6567 / 1.6181 ms | 2 / 0 |

Complete IDs are `real-scroll-postsync-<prefix>-<arm>-20261031-wheel-masonry`,
with `middle-masonry` for the middle case. Each capture contains two 30-second
measurement windows. The previous candidate is retained for context; the four
additional captures were performed in this follow-up.

Candidate cold B2 first-window requests have 70 exact queued releases,
71 new publications, nine pending coalescences and three reused publications.
The reverse window has no timestamped non-hit cache events and pending count
zero. Both windows of all warm/middle controls also have no such events and
pending count zero. Hits are aggregated, so this does not mean the cache was
not consulted. No source family or difficult file is removed from the folder.

## Native camera versus displayed motion

The unchanged strict middle analyzer fails at `Missing presented motion frame`.
That failure remains recorded; no missing pair is dropped to calculate a pass.
The independent native-camera check uses all native render samples, including
samples whose frames did not display. Each window has 4,789 comparable pairs,
11 direction-reversal boundary pairs and zero geometry-rebase exclusions.
The maximum native velocity error is **0.0009806821 physical px** in both
windows, below the existing 0.002 px bound. This is a native camera result,
not a replacement for failed displayed continuity or the full goal.

Wheel maximum physical errors remain:

- Repeated cold B2: 92.3822 / 115.5245 px.
- Verified warm: 3.6719 / 129.9287 px.
- Queue-depth-one warm: 114.1369 / 4.3530 px.

Coverage holds are zero. The original maximum display interval 9.375 ms,
maximum motion error 2 px, native middle velocity 0.002 px and zero-hold
requirements remain unchanged. No outlier is removed to manufacture success.

## Remaining delivery attribution

All 15 over-budget intervals across the two default-depth cold captures,
verified warm wheel and middle captures join to exact Qt submission brackets.
For those rows the affected F4 frame is reported GPU-ready **11.734–12.507 ms**
before the nominal next refresh. Their Qt preparation costs range from
0.25 to 2.5417 ms. The warm/middle failures occur without pending thumbnails.

The nominal deadline is the preceding reported display timestamp plus 6.25 ms;
it is not a directly observed DWM latch deadline. PresentMon GPU/display events
also do not establish physical scanout or optical image presentation. These
joins supplement the failed acceptance checks rather than reclassifying them
as harmless timing noise. The preceding scheduler/driver work already shows
both compositor refresh misses and inflated display notifications; these new
packets do not contain the driver trace needed to distinguish each occurrence.

Changing only `QT_D3D_MAX_FRAME_LATENCY` from 2 to 1 in the same host's warmed
wheel control still produces two over-budget intervals. Their reported GPU
ready leads are 10.931 and 5.600 ms. This control does not establish improvement
and is not a reason to change production defaults. It is one exploratory
control, not a replicated queue-depth performance comparison.

Existing opacity/fullscreen controls have already verified opaque window
painting and do not justify repeating a fullscreen/alpha shortcut. Continue
with evidence about remaining presentation/compositor work while preserving
normal-window behavior and the original cold/warm/middle requirements.

## Evidence recovery and final state

The queue-depth-one capture completes successfully before analysis fills C:.
The complete raw copy and its hash survive. Initial compression attempts mark
the files compressed but reclaim no useful space; those before/after receipts
are retained rather than claimed as savings. The raw packet is then copied to
an explicit workspace preservation folder, with its original D: source and C:
duplicate all hash-equal. Only the verified redundant C: copy is retired.
Analysis resumes on the same capture with its preservation path updated.

The already-running analyzer still invokes the old C:-only retirement guard,
which rejects the new location after metrics and owned-process cleanup finish.
The corrected workspace-specific guard independently verifies hashes and exact
parent paths, then retires the redundant source copy. The preserved packet
remains at `artifacts/preserved-relayout-latency1-20261031/`. No capture is
restarted, and neither analysis error is a capture-timing failure.

Owned diagnostic hosts are stopped. Normal Go PID 23488 and Qt PID 84780 remain
alive at their original paths and hashes. The portable payload is not updated;
the normal app still uses the last working build. The cancellation regression
and rendering verification from the preceding report remain valid.

> AI assistance has been used to create this output.
