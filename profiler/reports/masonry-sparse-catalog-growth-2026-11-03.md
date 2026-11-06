# Masonry sparse catalog growth, 2026-11-03

The provisional-to-sparse regression now checks that the existing persistent
row index survives a logical row count increase from 1 to 6,275. The old
implementation emitted one model reset. The candidate preserves the row and
emits an insertion for the new tail. It reuses the sparse extension's
validation for every materialized row before changing storage. Ten targeted
Qt checks passed.

In the 194-row Downloads diagnostic at 160 Hz and DPR 1.75, the first
48-to-194 transition on each panel changed from `sparse-reset` in o166 to
`sparse-extended` in o167. Each panel still reset when revision 5 published
its authoritative 48-row page. The final 194-row publication still
reconciled both panels. This removes two model resets from the observed
entry sequence. It does not establish a frame-time improvement because both
runs enabled synchronous verbose trace output. The o167 verifier also
rejected its first delivered wheel input at logical y=14.286. All 2,787
recorded wheel inputs retained native foreground.

The next point to attribute is the sparse extension rejection during
revision 5. The remaining reset may be necessary if the authoritative page
changes row order or source identity. Distinguishing those changes from
appearance or enrichment updates determines whether the model can retain
existing delegates and work for individual rows.
