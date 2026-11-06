# Rejected experiment: bounded visited-root retention, 2026-10-20

The constant-160 Hz goal is not achieved. This experiment is rejected for production
because its measured effect does not justify the extra retained scene state.

## Change tested

The real native scroll controller retained only previously visible delegate roots in
its existing proven prepared camera range. Prepared but unseen roots remained blocked.
SVG presentation still used the exact visible viewport; no preparation/decode window
was enlarged. Root visits reset on recycled entry identity, camera owner replacement,
leaving the retention range, and detach. Binding did not mark a visit before commit
published the synchronized camera geometry revision.

A new real-controller regression failed on both D3D11 and OpenGL before the change:
'A recently visible root was discarded inside the prepared camera range'. Afterward,
the two new cases and seven existing behavior/rendering cases pass on both backends:
11 checks including setup/cleanup, zero failures or skips. Software fallback passes.
Visible captions are checked at DPR 1.75 for scene origins and unit transforms, and
the settled capture is exactly equal to the GUI-camera reference. An initial test
incorrectly treated parked QML-visible leaves as displayed; entry-0 physical y=-631.75
was entirely outside the clip. The final test explicitly intersects the actual clip.
One initial D3D11 combined run exited abnormally after the failed assertion; the full
corrected rerun passed both backends. Do not characterize that first run as green.

## Measurement

Same diagnostic workload/probes as native-scroll-svg-upload-details-2026-10-20.md.
Four 10-second wheel windows, fresh process, 186 Downloads entries, density152,
DPR1.75, 160 Hz, default GC5000us, no general frame recorder or concurrent build/test.
Thresholded SVG bytes before: cold167,423,472; warm165,492,144.
After: cold163,625,760; warm164,503,296 (only about 0.6% lower warm).
Thresholded fill+stroke upload times before: cold81.8582ms, warm84.3463ms.
After: cold80.3601ms, warm81.4804ms. These small differences are not persuasive given
capture variance and a thresholded sampler.
Maximum public preparation before6.6819ms; after6.8460ms. Producer intervals over
9.375ms before71; after83. These are producer gaps, not displayed missed refreshes.
The source-based hypothesis did not establish a useful end-to-end improvement.

## Disposition

Archive the exact implementation, tests, generated probes, raw captures and identity
receipts, then remove this experiment from production. Keep the previous verified
normal application alive. Next probe should record geometry identity, gate ownership
and generation to distinguish re-entry from repeated upload of unchanged visible SVGs.
