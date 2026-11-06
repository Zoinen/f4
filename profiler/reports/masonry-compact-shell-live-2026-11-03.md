# Masonry compact shell live capture, 2026-11-03

The compact shell candidate passed the first-viewport and sampled
foreground checks at 160 Hz and DPR 1.75 with 194 panel entries.
Verbose media and scheduler tracing were disabled. Input began
240.711 ms after navigation at y=0. The recorder skipped 195 of
3,000 scheduled inputs.

The 4,787 displayed intervals had a 6.283 ms p99 and zero dropped
present rows. Three intervals exceeded 9.375 ms, at 68.744, 24.996,
and 12.504 ms. The first two followed gaps between Qt frames of
61.74 and 23.03 ms. The third gap had a Qt frame already in
progress. This run did not reproduce the control's single-spike
pattern and cannot establish a cadence improvement from the patch.

The motion recorder identified 217 coverage-hold samples and a
maximum requested-to-visible displacement of 475.007 physical pixels.
The patch removes the full shell notification for compact activation
and title changes in the regression test. Frame and coverage limits
still fail in live scrolling.
