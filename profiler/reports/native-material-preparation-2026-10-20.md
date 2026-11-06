# Native scrolling material preparation attribution — 2026-10-20

The 160 Hz without spikes objective remains incomplete. This experiment changes
only diagnostic tooling, not production rendering or the default motion mode.
The normal embedded worktree build from the interruption fix remains installed.

## Method

Added optional --material-parts scopes to the diagnostic renderer: merged and
unmerged preparation, static/dynamic material data, pipeline state, clip state,
and both shader material lookup variants. Individual scopes record only wall
durations at least 0.1 ms into the existing bounded, pre-touched buffer. Nested
scopes overlap and must not be added to their parent. No CPU/scheduler attribution
is implied. Probes add overhead; these runs are not uninstrumented production.

Two fresh-process captures each contain two cold and two warm 10-second wheel
windows. Downloads 186 entries, Masonry density 152, physical screen 160 Hz,
DPR 1.75, wheel every 30 ms, bounded bounce, default GC 5000 us, geometry pools
16 MiB. Native motion is explicitly enabled; general frame recorder is disabled.
No build, test, analysis, UIA or scheduler trace ran during the input windows.
Actual successful-link hashes and bounded-buffer checks pass.

## Results

All eight windows have zero displayed intervals above 9.375 ms; maxima range
6.3361–6.4182 ms. This threshold identifies a missed refresh, not compliance with
an exact 6.25 ms deadline. The earlier 12.4787 ms displayed miss remains evidence;
these runs do not erase it or prove constant 160 Hz.

The earlier delayed frame had 3.9939 ms sync, 5.6345 ms preparation, including
2.3511 ms alpha preparation; its recorded small upload was only 0.1001 ms. That
particular event did not recur, so its material cause is not established.

The repeat's worst preparation frame is 5.9759 ms, with a 12.6025 ms producer
interval, 0.4289 ms sync and 6.8766 ms render-to-frame-end. It did not miss a
displayed refresh. Alpha uploads account for 3.4349 ms, pending admission
0.8411 ms, alpha preparation 0.8748 ms. Pipeline state contributes 0.1206 ms and
shader lookup 0.2175 ms; dynamic/static material scopes are below 0.1 ms there.

Seven thresholded upload records belong to gate root 1025705264, generation 4,
sequence 13: ready=true, present=false. They include a 1,344,420-byte stroke
vertex buffer plus 122,220 index bytes, and multiple curve fill batches. Their
reconstruction includes pooled-buffer growth and static/dynamic type recreation.
A separate non-ready gate sequence 12 progresses through pending admission.
This is evidence that hidden, previously ready vector surfaces can still consume
ordinary unbounded upload time. It is not evidence that shader compilation is
the dominant remaining cost.

Next: reproduce ready-but-hidden gate reconstruction in a renderer regression;
check admission ownership and enforce the existing bounded upload policy for
that work while preserving complete-surface visibility and active-viewer priority.
Do not blindly reduce GC/upload constants based on this capture.

## Diagnostic correction and evidence

The stage analyzer's longPresents collection previously included idle-to-input
boundary intervals. It now applies presentIntervalInWindow, matching the phase
analyzer. Boundary samples remain in full samples and are not silently deleted.
A selected 2404 ms idle interval was explicitly rejected before investigating the
actual 17.7989 ms within-window producer delay from the older capture.

An older scheduler CSV was losslessly compressed and round-trip SHA256 verified
before freeing its uncompressed copy; receipt artifacts/native-material-space.json.
Sources, link receipt, captures, PresentMon CSV and analysis are archived with
SHA256 verification. No commit or push was made.

AI assistance has been used to create this output.
