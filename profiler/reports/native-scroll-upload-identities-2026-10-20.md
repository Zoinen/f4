# SVG upload identity and admission bypass, 2026-10-20

The 160 Hz without spikes goal remains incomplete. No new production optimization
from this experiment is retained. The normal worktree application remains alive on
the earlier verified build; only a diagnostic copy was rebuilt and run.

## Method

Extend the previous slow-upload probe with numeric first-node, geometry, vertex-data,
renderer-root, vector-gate-root, generation and gate-sequence identities. Record
ready/present flags from the nearest ancestor gate. This payload is collected after
the timed upload ends; it performs no filesystem access or JSON work during input.
Arrays remain bounded/pre-touched and samples are thresholded at 0.1 ms.

Same native controller, Downloads186, Masonry152, actual DPR1.75 and screen160Hz,
default GC5000us, no general frame recording, two cold and two warm 10-second wheel
windows with 30 ms input spacing. Builds/tests/analysis/UI Automation are stopped
during measurement. Identity sidecars include current source, archive and probe hashes.
All captures finished successfully; native sample/gate capacities were not exhausted.

## Results

Cold: 291 slow SVG uploads / 166,886,640 bytes, 290 distinct full identities.
Warm: 302 / 166,786,560 bytes, 302 distinct full identities.
Identity tuple is (gate root, generation, gate sequence, first node, geometry,
vertex-data storage). Excluding vertex-data storage from the tuple does not change
repeat counts: one cold repeat, zero warm repeats. The one cold repeat is not ready
or present. These are thresholded observations; they do not prove absence of fast
reuploads, or byte-identical contents at the same storage address.

221 cold samples already report ready+present (128,599,776 bytes / 61.4839 ms).
239 warm samples already report ready+present (131,347,584 bytes / 63.3222 ms).
The largest warm stroke uploads are 2,054,448 vertex bytes +186,768 index bytes,
ready+present, generation4, but different gate sequence162 and171.
Thus recurring equal sizes alone were insufficient evidence that one unchanged
visible geometry was repeatedly uploaded. The identity probe supersedes that inference.

Public preparation maximum per block: 5.4175, 7.0475, 6.3941, 5.9135 ms.
The worst cold frame spends5.3595ms in alphaUploads. Producer intervals >9.375ms:
17+17+16+18=68 over40seconds. Display intervals are independently recorded in the
phase summary and must not be equated with producer intervals.

## Source evidence and next implementation target

F4VectorGateFrame skips ownership tracking for gates that are ready and present.
The ordinary opaque/alpha upload loops upload batches without an owner or with a
ready owner. Only non-ready owner batches enter the bounded pending-admission loop
(32 batches,1MiB soft budget,2ms soft clock; one oversize batch may progress alone).
GalleryVectorUploadGate persists a prepared hint after initial readiness;
f4ConfigureVectorGate ORs that hint into gate readiness. Readiness therefore cannot
by itself prove that the current renderer batch still has valid uploaded geometry.
The measurements show most thresholded SVG bytes take the unbounded ready route.

Next: add a renderer regression for a previously prepared SVG whose renderer batch
must be reconstructed, then make upload admission cover that work without exposing
partial vector surfaces. Preserve rendering order, active-viewer priority, no raster
fallback, geometry correctness, and progress for one oversize batch. Measure the
result against this baseline rather than enlarging decode/viewport windows.

## Rejected experiment restoration

The bounded visited-root retention experiment is archived separately. Its five source
and test files were restored to archived pre-experiment bytes after normalized content
comparison and SHA256 validation. An initial verification mismatch was only CRLF/LF;
restoring archived bytes removed that difference too. The pre-experiment controller
SHA256 is10aa8844312a61787190da96fd6aa4f69ce8470e76fc459f0cf06651501edbef.
