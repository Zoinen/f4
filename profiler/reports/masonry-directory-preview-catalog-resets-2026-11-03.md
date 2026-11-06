# Directory preview catalog resets, 2026-11-03

The o168 diagnostic logged the first row that rejected sparse extension
during each authoritative page update. In both panels, row 1 differed only
in its directory preview source descriptor. The logical count stayed 194.
The probe logged field names and row indices, with no descriptor values.

The regression reproduces this with an initially unbound directory
preview. It checks that the authoritative update preserves the persistent
row index, emits no model reset, and enables the preview. It failed with one
reset on the old model. Eleven targeted Qt checks passed with the fix.

In o169 the two authoritative 48-row page updates both extended the sparse
catalog in place. The earlier provisional expansions also extended it in
place, and both final publications reconciled all 194 rows. The initial
folder changes still perform their normal model resets. This comparison
uses verbose tracing to establish the publication sequence and does not
measure the frame-time benefit of removing the resets. The o169 capture
also failed the first-viewport gate at logical y=14.286. All 2,771 delivered
wheel samples retained native foreground.
