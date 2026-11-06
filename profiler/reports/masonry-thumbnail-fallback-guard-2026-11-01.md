# Deferred thumbnail fallback resolution — 2026-11-01

The artifact-only candidate suppresses GalleryFallbackIconHost.modelIconSource resolution while preview.thumbnailExpected is true. Ordinary file icons still resolve; disabling thumbnails or definitive thumbnail failure restores resolution. GalleryEntryPreview.markerNeeded also requires !thumbnailExpected, so suppressing the icon URL does not create an unnecessary marker renderer.

No product QML source was changed or promoted. Candidate source copies, qmlcache objects and matching raw resource overrides live under `C:/Users/xs/AppData/Local/Temp/f4-thumbnail-fallback-guard-20261101`. Control SHA-256: `29b53ffdce8e3d657e12f4b2e91ed4029f8f0d778bbcb604093a15c0a5697245`; candidate: `a2818ee41e4eca8c105b095433263edacc5695055be75b3f3c3c00fee46f6607`.

## Behavior and rendered checks

Five data rows cover Masonry, Grid, Icons, Columns and Details: normal file, cached image dimensions before thumbnail source availability, thumbnails disabled, re-enabled pending thumbnail, definitive failed source and restoration of the ordinary row. Actual visible Text/Image leaves have named origins mapped to window contentItem at DPR 1.75 and unit transforms asserted. Five captures per mode are saved for each arm.

The control rejects all five rows when run against the candidate's deferred-resolution expectation. The initial one-file candidate passes that state check but creates a fallback marker while metadata already promises a thumbnail. A new assertion reproduces that side effect in all five modes. Adding the marker gate fixes it. The final candidate and control each pass 10 QtTest results including fixtures, all five new rows, the existing pending-thumbnail failure check and two parked-bitmap lifetime cases. The candidate import-isolation test passes three results. Native Direct3D and the Basic control style are used. GUI PE subsystem and system-only DLL audits pass.

RGB and alpha comparison finds 20 of 25 final candidate/control captures exactly identical (all phases in Masonry, Grid, Icons and Columns). All five Details captures differ by seven pixels in the header region. A repeat of the unchanged control differs by two pixels; that demonstrates some baseline variability but does not explain every candidate/control difference. Keep the remaining pixel-equivalence uncertainty open. Existing test font/TLS warnings and the import-only undefined QPointF/PinchArea warnings remain. This generic fixture and its captures do not establish all SVG/video/source-transition fidelity; those wider checks would be required before rollout.

## Performance status

The original A/B/B/A runs cold-u11/u12/u13/u14 cannot establish a win: Windows foreground ownership was not validated, and almost all presents were dropped. An independent frozen-host check reproduced the problem and observed foreground=Parsec even while Qt reported active=true. See [native foreground findings](D:/Code/f4-zoin/profiler/reports/masonry-native-foreground-admission-2026-11-01.md).

The final control setup was interrupted by a harness receipt bookkeeping assertion before launch. A shallow copy had preserved the shared published-host hash inside a saved stage receipt. The original receipts were retained, all stage inputs reverified, and the harness changed to a deep copy. The unused cold-u14 case then completed in its original final-control order. No capture case was restarted or silently overwritten.

No scrolling performance gain is claimed. Next run a new foreground-validated A/B/B/A comparison on the corrected diagnostic graph. Retain the full cold/warm/middle scope and physical-motion verification requirements.

> AI assistance has been used to create this output.
