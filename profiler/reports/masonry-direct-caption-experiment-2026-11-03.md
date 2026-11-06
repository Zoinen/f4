# Masonry direct caption experiment, 2026-11-03

The o178 diagnostic factory instantiates `MasonryEntryDelegate` directly.
It removes the extra `GalleryMasonryEntryDelegate` wrapper item and its
forwarding binding. It keeps the mode host and its Loader. The change
was compiled into the diagnostic host only.

The capture passed first-viewport and native foreground checks at
physical 160 Hz, DPR 1.75 and 194 entries. It skipped 176 scheduled inputs.
Compared with o176, construction elapsed median was 10.05 ms versus
9.75 ms, p95 was 56.46 ms versus 64.82 ms, and coverage holds rose from
217 to 243. Both runs had three displayed intervals above 9.375 ms.
The candidate's longest interval was 62.49 ms.

The elapsed construction data includes suspended time. This single
comparison shows no convincing coverage benefit. The candidate was
kept out of the production source and did not proceed to rendered
caption and scene-space alignment verification.
