# Icon notification candidate, 2026-11-01

## Implementation and result

`GalleryEntryVisual` now gives `iconKey` and `iconPath` a dedicated
`iconChanged` notification. It emits only when one of those values changes.
The complete snapshot is still installed before notification, and the original
`mediaChanged` signal and `MediaChange` bit remain available for existing
consumers. No new setting, layout, rounding, codec, resource or GC policy is
introduced.

This removes a verified redundant QML binding evaluation on thumbnail
publication. It passes the selected correctness/rendering checks, but the
scrolling comparison **does not establish a reliable display-cadence win**.
The source change remains an uncommitted candidate; it has not replaced the
normal portable application. Constant 160 Hz is still unproven.

## Regression before implementation

The new native test creates actual QML bindings that read the icon key, icon path
and image source through native read counters. Publishing a thumbnail URL and
known dimensions must update the image-source binding and legacy media signal
without re-evaluating either unchanged icon binding.

Before changing the product source, the test reproduces the problem against
the original implementation: `iconKeyReads` becomes **2 instead of 1**. The
failure is preserved. The fixed version passes the test, including subsequent
icon-key and icon-path replacement, unchanged-snapshot replay and clearing the
snapshot. It checks observable bindings, not merely the new signal name.

The current candidate native executable passes **59 selected QtTest cases**.
Ten additional hidden-fallback and empty-video cases pass in a separate run.
These counts include their respective QtTest setup/cleanup cases and are not
presented as 69 distinct product tests or as the full suite. Coverage retains
sparse/catalog transactions, newly cached metadata, failure recovery, bounded
prefetch/materialization, video fallback and SVG classification/navigation.

The implementation changes only `GalleryEntryVisual.cpp/.h`; the regression is
in `tests/GallerySessionTest.cpp`. Native generated build files are preserved.
The experiment compiles the new visual object and a complete copied moc unit
with the regenerated visual metadata, rather than mixing old and new moc
definitions. The class's instance data layout is unchanged.

Five Details/Columns/Grid/Icons/Masonry captures at DPR 1.75 are pixel-identical
to the control. Actual caption/vector scene-origin and unit-transform checks
execute in the native regression. The Details output was inspected, including
its smaller size caption. These fixtures do not claim exhaustive optical
verification of every file-icon format. No static layout is modified.

## Scrolling comparison

The old and candidate hosts have the same six-phase census, runtime budget
observer, native motion recorder and other objects. The candidate adds only
the visual and moc overrides. Both arms explicitly use the default GC setting;
native reads verify **5000 µs** in all four processes. No builds, tests or
offline analysis run during captures. Desktop-observer checks before/after are
clean. Each uses actual Downloads, 192 entries, Masonry density 152, real
160 Hz/DPR 1.75, and 30 seconds of rapid wheel input from the first viewport.
Gallery cache storage is empty before QML; the OS cache is not flushed.

| Case | Arm | Cached frames / pending at input start | Display intervals >9.375 ms | Dropped presents | Maximum display interval | Median / maximum commit |
|---|---|---:|---:|---:|---:|---:|
| cold-n11 | control | 22 / 14 | 0 | 0 | 6.4750 ms | 0.8019 / 9.5184 ms |
| cold-n12 | candidate | 2 / 25 | 3 | 0 | 50.0124 ms | 0.8508 / 11.4036 ms |
| cold-n13 | candidate | 12 / 15 | 1 | 0 | 12.4901 ms | 0.9755 / 11.0495 ms |
| cold-n14 | control | 26 / 10 | 1 | 0 | 12.4967 ms | 1.0269 / 10.0365 ms |

The controls start with more content ready. This is not a matched cold-loading
state and cannot establish that the candidate caused the difference in misses.
All misses remain in the results. Their associated present timestamps relative
to input start are +6.029/+20.136/+36.038 ms in n12, +8.615 ms in n13 and
+24.603 ms in n14. This timestamp is the present, not an assertion that the
entire display interval lies after input start; startup-boundary gaps remain.

No long displayed interval is associated with a later present in these four
observations, despite occasional long GUI commits. This distinction matters:
an elapsed GUI spike is not automatically a displayed-frame miss. It makes the
first interactive scroll during incomplete loading a higher-priority next
target than repeating broad steady-state GC tuning.

Visual-binding inclusive totals are 1130.2730/1034.1770/1009.6498/1038.5872 ms
in control/candidate/candidate/control order. Completion totals are
388.4611/416.8423/449.1479/411.5254 ms. These elapsed phase totals include
instrumentation and possible waits and do not prove an overall speedup.
The candidate is retained for its proven reduction of unrelated binding work,
not promoted on a claim of smoother display delivery.

Coverage holds are zero throughout. Native motion analysis retains unmatched
commits (0/0/1/1), excluded wheel pairs (4794/4792/4793/4794), and frames without
motion commits in the raw analyses. No valid physical wheel-error samples or
optical-motion proof are produced. This is not a quiescent full-folder warm
tour or an initialized-app cold folder-entry verification.

## Build identity and state

Candidate host SHA-256:
`2c28f2372757bc527e1c69f527a0254fdb8d8b9ea3421186d9929828fb33167d`.
Control host SHA-256:
`14009d9735a71c6118b6da725d0b47b761d5ddb5e119d579a39a359a4596ac4b`.
Native candidate test SHA-256:
`49d7ad4669fd338e89e55cb5f8198d0129a9841370eca1d5b2efdb90dce48a20`.

The Windows GUI subsystem and system-DLL import audit pass for the diagnostic
host. This manual worktree override is not a rebuilt embedded payload or a
completed portable release. The normal ac68c2e7/653cbd8a application is unchanged.
No commit or push is performed.

Closed historical native tests and symbol-link outputs are copied to the
dedicated P: profiler archive, hash-verified and kept at their original paths
via symlinks. Closed JSON/CSV compression retains identical hashes. No raw
capture is deleted, and no external app, VM, display setting or affinity is
changed.

## Next acceptance work

Measure the initialized-app folder-entry and first-scroll path with incomplete
content, preserving visible preview priority and all boundary misses. Then
verify full-folder warm wheel/middle motion, physical and rendered movement,
sustained cadence and portable rollout gates. The candidate's tests and reduced
binding reads do not replace these requirements.

AI assistance has been used to create this output.
