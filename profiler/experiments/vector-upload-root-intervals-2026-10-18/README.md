# Vector upload gate with owned transform-root intervals

Experimental successor to the v2 checkpoint; not enabled by the dependency
recipe or embedded in the normal worktree application.

`qt-upload-gate.patch` is self-contained and applies from qtdeclarative after
the integrated stable-interval recipe. It replaces the earlier gate patches;
do not apply them together. Apply-check passes against LF-normalized copies
of both base source files. The optional interface remains v2, so it uses the
committed gallery consumer and protocol without a Go or QML API change.

Only registered vector transform roots take the new path. When an assigned
ancestor has free drawing-order capacity, creating a root transfers up to 64
padding orders and tags the funding/assigned ancestors for a partial rebuild.
The existing unpublished-reservation ledger also tracks these roots. Removal
returns assigned ranges or exact unpublished reservations through the existing
safe refund path. Ordinary Qt transform roots retain their original behavior;
unsafe or unassigned cases retain the full-rebuild fallback.

The native `vectorUploadRootsAvoidUnrelatedFullRebuilds` regression fails on
the preceding prototype: one full rebuild occurs when adding the root. With
this patch it verifies 16 insertion/removal cycles, zero full rebuilds during
those cycles, preserved sibling pixels, overflow outside the small item bounds,
and whole physical-pixel origins/unit transforms at DPR 1.75.

The complete native suite passes 14 results on default Windows RHI and OpenGL.
The focused gallery SVG suite passes 26 results. Real scrolling results remain
inconsistent; see the report below. This is not an approved rollout candidate.

[Measurement report](../../reports/vector-upload-real-scrolling-2026-10-18.md)
