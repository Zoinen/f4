# Fractional displacement estimator calibration

The unchanged desktop-strip estimator was checked offline against 600 known
translations. Ground truth comes from direct analytic sampling of continuous
sinusoids with independent phases across the 32 columns, fixed gain, and 8-bit
quantization. This avoids deriving reference translations with the estimator's
own interpolation method. Five seeds, five integer offsets, eight fractional
offsets and three frequency bands are retained in the results.

525 cases matched: median absolute error 0.004370 physical pixels, p99 0.051347,
maximum 0.051555. All 400 cases in the two lower frequency bands matched.
75 of the 200 highest-frequency cases failed the existing weak-match threshold;
they remain rejected, not zero-error samples. The original integer, flat and
periodic-signal checks also pass. No matcher thresholds or implementation changed.

These results establish synthetic fractional-shift accuracy only. They do not
bound error on arbitrary gallery content, validate masked desktop regions,
prove physical scanout, or pass the application's display/motion performance
gates. Real-image uncertainty and unmeasurable pairs must remain explicit in
the next combined marker/submission/driver-refresh capture.

Reproduce with `python artifacts/calibrate-fractional-motion-20261031.py`.
All cases and the estimator source hash are in
`artifacts/fractional-motion-calibration-20261031.json`. This is an offline
diagnostic addition; no application source, build or running instance changed.
