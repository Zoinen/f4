# Identical catalog row replay, 2026-11-01

## Result

The candidate skips replacement of an already loaded row when its authoritative
inputs are unchanged. Both candidate captures skip all five repeated 64-row
metadata batches, retaining the existing facades and decoded state. This removes
measured synchronous work, but **does not establish constant 160 Hz or eliminate
startup scrolling stalls**. The normal portable application has not been replaced.

## Evidence for the change

The source-field-only `cold-t1` probe records 320 row comparisons, including 136
existing facades, in 56,859 spans with zero overflow. Of these comparisons, 306
have a zero mask, ten have only the published-thumbnail presence bit, and four
have derived original-size/settled differences plus a vector-source presence bit.
The observed authoritative source and display fields do not change. Presence
bits are not differences. The probe did not observe `thumbnailKind` or the
`fileFieldsRead` flag, so its mask alone is not a complete equality predicate.

The implementation compares parsed authoritative fields, including source
revision/access, geometry classification, selection, display metadata and style.
It validates the complete batch, including duplicate IDs/rows, before committing
changed rows. An all-identical batch still resumes the existing pipelines.
Actual source changes retain the previous replacement/notification path.

Decoded dimensions, thumbnail pixels, vector leases and definitive failures are
derived state and must survive identical transport replay. Newly available cache
dimensions, orientation, EXIF and typed fields must still publish. The cache
progress check requires `isCached`: a video's settled 16:9 placeholder is not
evidence of newly cached geometry.

## Regression checks

The current native test executable passes **58 selected QtTest cases**, with no
failures. Coverage includes facade identity/no notifications on identical replay,
19 changed-input cases, atomic validation of mixed batches, new cached dimensions,
orientation and typed metadata, cache-off decoded portrait videos, retryable VFS
errors, viewer failure refill and bounded viewport materialization, alongside the
previous catalog, video and vector regressions. This is not the full test suite.

The new cache-off video case fails on the original implementation: decoded
1080×1920 dimensions become 16×9 after replay. An earlier naive equality check
also failed the new cache-progress regression (empty dimensions instead of
96×72); the guarded implementation passes both. These failures are retained.

At DPR 1.75, all five Details/Columns/Grid/Icons/Masonry vector panel captures are
pixel-identical to the control. The existing regression maps actual caption and
vector leaves to scene coordinates and checks their transforms. The Masonry
capture was inspected, including its caption. These bounded fixtures do not
prove every bitmap format or full viewer interaction.

## Scrolling captures

Control is the previous role-only candidate, not the old blanket-role baseline.
Both arms use the same motion recorder and bounded census; only the row
transaction object changes. The field-mask probe is absent from both arms.
Actual Downloads has 192 entries, Masonry density 152, 3840×2160, DPR 1.75 and
160 Hz. Gallery caches are empty before QML; the OS file cache is not flushed.
No Computer Use helper is present in the before/after checks. No compilation or
analysis ran during timing. Each input observation lasts 30 seconds.

| Case | Arm | Cached frames at input start | Display gaps >9.375 ms | Dropped presents | Maximum display interval | Rows installed in repeated batches |
|---|---|---:|---:|---:|---:|---:|
| cold-v1 | control | 48 | 1 | 0 | 25.0047 ms | 5 × 64 |
| cold-v2 | candidate | 48 | 0 | 0 | 6.4811 ms | 0 |
| cold-v5 | candidate | 3, with 26 pending | 3 | 0 | 50.0044 ms | 0 |
| cold-v4 | control | 48 | 0 | 0 | 6.4463 ms | 5 × 64 |

The order is control/candidate/candidate/control. `cold-v3` was rejected by the
unchanged 150 MiB free-space guard before launching an application or collector;
its attempt directory is retained. `cold-v5` is its fresh-ID replacement.

All five candidate `applyRows` spans have zero pending rows and take
0.0001–0.0003 ms; this excludes validation/parsing. In `cold-v5`, two complete
outer apply-catalog-row spans overlapping an early gap still take 0.4794 and
0.4486 ms. Control installation spans reach 3.2567 and 4.6078 ms respectively,
mostly before input. These inclusive wall-clock spans are not exclusive CPU
measurements and nested spans must not be summed.

The control's 25 ms miss crosses input start. The less-warm candidate retains a
50.0044 ms boundary interval plus two post-start gaps of 12.5181 and 12.4820 ms.
Its measured render-signal range peaks at 3.8470 ms. The remaining gaps cannot
be attributed to row replacement, nor localized to a new CPU/wait cause from
these counters alone. The unequal initial loading states prevent a fair claim
that the candidate improves or worsens total display misses.

Coverage holds are zero in all four runs. Motion analysis retains unmatched
commits, frames without commits and all excluded wheel pairs: it produces no
valid wheel physical-motion error sample. Small matrix-float errors therefore
do not prove the 2 px displayed-motion requirement. No optical claim is made.

## Build identity, storage and rollout

Candidate diagnostic host SHA-256:
`ddd628de63b06da8cae40d1993045061bd723c739615f1380e0529c7cfc4410c`.
Native test SHA-256:
`7035f54301b229578e01699bdf890bbafb61072a3b538bdc051011ad6aa6bb3e`.
The Windows GUI subsystem and system-DLL import audits pass for this host.
It is a worktree-only diagnostic override, not a regenerated embedded payload or
a completed portable rollout. The normal Go/Qt processes remain unchanged.

Disk-space recovery compressed closed raw data without changing hashes,
removed explicitly identified superseded test executables while retaining their
sources/objects/logs, and consolidated duplicate build paths using verified
links. Raw capture attempts were not deleted. Storage receipts are included in
the experiment manifest; the storage guard was not lowered.

The next attribution target is the remaining cold scene-publication/startup
interval, with a CPU/scheduler capture that separates execution from waits.
Initialized-app folder entry, quiescent full-folder warm wheel/middle-scroll,
wheel motion/optical verification, sustained runs and full portable gates remain
outstanding. This candidate is not promoted on a claim of no-spike performance.

AI assistance has been used to create this output.
