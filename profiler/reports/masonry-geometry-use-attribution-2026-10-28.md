# Per-buffer geometry tracking admits no additional maps in this capture

Do not replace the existing geometry completion guard based on this result.
The new observer finds 266015 eligible of 267852 main-context geometry maps
(99.314%), exactly matching the original guard's 266015 proven maps. Both
reject 1837 unproven maps; there are zero queued-work rejections by the original
guard. Finer read tracking therefore adds no observed reuse opportunity here.
It is an unpromoted diagnostic, not a scrolling fix.

## Why this experiment

The preceding exact GPU-timing capture has a 13.9684 ms cold preparation frame
containing native buffer growth calls of 4.6871 and 3.9748 ms. Another 9.445 ms
preparation frame contains an 8.8007 ms geometry map, and a 7.7779 ms preparation
frame contains 5.0299 ms of buffer growth. These calls overlap preparation;
their durations must not be added to the complete frame duration again.

Per-buffer read epochs could theoretically allow mapping a completed geometry
buffer despite unrelated commands queued in the same frame. This capture does
not demonstrate that situation. The observed map policy remains the existing
whole-frame guarded NO_OVERWRITE/DISCARD policy throughout.

## Tracking and limitations

The bounded observer registers only exclusively vertex- or index-bound native
dynamic buffers. It retires their pointer lifetimes before release, marks
bound vertex buffers on queued draws and index buffers on queued indexed draws,
and records queued buffer readbacks before native execution. It also marks
executed copies. External access and native buffer exports conservatively
invalidate per-buffer proof until a later completed frame. Native export
invalidation covers all QRhi buffer kinds, deliberately over-conservative;
672653 export/external invalidations occur in this process. This is not a
minimal geometry-only export model. Unknown, missing, unsupported, wrong-thread,
overflow and unfinished-marker cases cannot qualify.

GPU completion remains the existing nonflushing EVENT query, not a CPU guess.
There are no extra COM references in the registry. The main context peaks at
624 tracked native buffers, with 1019 registrations and 395 retirements;
capacity is 4096. No wrong-thread, overflow or untracked map occurs. Draw hooks
cost 174.568 ms and map decisions 100.125 ms over the process lifetime; this
extra observation cost is a reason not to deploy unnecessary tracking.

## Verification

- Seven tests pass: queued reads, copies, external access, completed reuse,
  index buffers, native lifetime retirement, non-geometry rejection,
  wrong-thread rejection and registry capacity. Test totals include setup and
  cleanup. Native work/fences run on D3D11; no skipped tests.
- Twenty-seven rendered shader checks pass at actual DPR 1.75, with actual
  guarded uniform reuse and zero failed reuse maps.
- Four isolated QML import checks and the Windows dependency audit pass.
- Real Downloads, Masonry, actual 160 Hz / DPR 1.75, fresh application cache bf,
  two 10-second rapid-wheel blocks. Timing queries are disabled for this run;
  the same existing rendering-policy flags remain enabled.

No long geometry map reproduces in this run. Among main-context render-thread native spans
inside the first gesture, Present reaches 1.970 ms, uniform DISCARD reaches
1.765 ms, and a fallback texture update reaches 0.874 ms. The process-wide
21.539 ms Present and 4.101 ms staging-texture readback are outside the measured
gesture windows and are not scrolling evidence. Always filter by gesture
windows before attributing process-wide outliers.

The geometry observer admits zero additional maps in this workload, so no
per-buffer geometry reuse change was made. Cold native allocation growth and
unfinished-marker DISCARD stalls remain unresolved. Sustained 160 Hz, motion
quality and loading/fidelity gates remain unproven. The normal app is unchanged;
only this worktree-specific diagnostic process tree was stopped. No commit,
push, portable-payload replacement or production rollout occurred.

The matching experiment snapshot retains source, test receipts, capture data,
analysis and hashes of external executable/object artifacts.
