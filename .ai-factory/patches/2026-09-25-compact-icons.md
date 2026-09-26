# Restore compact panel icon size

## Horizontal slot follow-up

The icon inset alone left the old oversized square slot in place, shifting
icons and filenames right. Size the compact slot from the scaled icon extent
plus the configured slot/icon difference. At default density this restores
the original slot width; both the slot and icon still scale with zoom.
Extended detailsZoom with slot-width assertions: the padded case failed
before the change (18.285714 vs 12 DIP). All eight Windows cases pass at DPR
1.75 after the fix, including visual-leaf alignment and captures. Embedded
QML imports pass. Removed the obsolete, non-running generated
f4-zoin-compact-icons-20260925.exe to make room for its replacement; it is
reproducible from source.

## Follow-up clarification

The fixed-size correction below was superseded: icons must remain scalable.
Compact icons now subtract a configurable per-edge vertical inset from row
height. F4 derives that inset from its default row height and 16-DIP icon
size, so standard rows retain the previous icon size while zoom still grows
icons. Thumbnail slots and text placement remain unchanged.

The updated zoom check reproduced the fixed-size regression before this
correction. All eight native Windows cases pass at DPR 1.75, including
larger insets, scene-space leaf alignment, unit transforms and captures.
Embedded QML import verification passes. Portable payload and executable
were rebuilt. Removed only the obsolete, non-running generated executable
f4-zoin-icon-routes-20260924.exe to free build space; it can be rebuilt.

## Root cause

The Gallery merge included row-height-dependent fallback icon sizing. At a
28-DIP row and DPR 1.75 this rendered a 22.285714-DIP icon instead of the
previous 16-DIP icon.

## Fix

Restore the configured compact icon size in Details and Columns. Keep the
growing thumbnail slot, portrait centering, source identity and scene-pixel
correction unchanged. No catalog, scheduling or thumbnail cache changes.

## Verification

Extended the existing detailsZoom test with 28-DIP rows and fixed-size icon
assertions, including large Details and Columns rows. It failed before the
fix with actual 22.285714 versus expected 16. Native Windows execution at
actual DPR 1.75 passes all six cases, including leaf scene coordinates, unit
transforms and rendered captures. Reviewed the native 28-DIP capture.
The offscreen backend lacks the native fonts and fails an unrelated filename
truncation assertion; the native run passes that assertion too.

Static host rebuilt; embedded-only QML import check passes. Regenerated the
embedded payload and rebuilt the worktree-specific Go executable.

## Prevention

Keep compact fallback-icon dimensions independent of thumbnail-slot zoom.
Retain explicit size assertions alongside pixel-grid checks.
