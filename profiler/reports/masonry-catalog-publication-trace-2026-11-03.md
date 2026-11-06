# Masonry catalog publication trace, 2026-11-03

Two diagnostic runs captured initialized entry into the 194-row Downloads
catalog at 3840×2160, 160 Hz, DPR 1.75 and Masonry density 152. The second
host changes GallerySession's handling of an incomplete page received after
the complete catalog for the same revision. Both runs enabled verbose media
tracing. The traces support event attribution. Timing comparisons would be
confounded by the tracing and the different initial viewport positions.

| Event | Control o165, left / right ms | Patched o166, left / right ms |
|---|---:|---:|
| Provisional revision 4, 48 rows | 70.67 / 191.67 | 103.00 / 230.30 |
| First 48-row sparse reset, logical count 194 | 220.43 / 226.64 | 276.96 / 281.83 |
| Revision 5 sparse reset, 48 rows | 240.32 / 294.72 | 289.14 / 345.17 |
| Revision 6 reconciliation, 194 rows | 268.19 / 312.20 | 317.39 / 389.51 |

Times are relative to navigation. Bridge events timestamp revision 4 at
apply entry. Other table entries are model apply completions. The first
sparse reset on each side has no enclosing full bridge catalog apply span.
The bridge's row-free panel-state handler also calls the catalog apply API
when deferred-row or readiness flags change. Instrument that path and the
sparse extension rejection before attributing those resets to a specific
flag or row change.

The sequence grows from the provisional prefix to sparse storage and then
reconciles into a complete catalog. Neither run exhibited the independently
reproduced complete-to-sparse regression. The patch consequently did not
eliminate any of these four early sparse resets.

Both runs retained native foreground in every recorded wheel sample. The
control's first delivered input started at logical y=14.286, failing the
first-viewport gate. The patched run passed the initialized-entry gate but
skipped 220 of 3,000 planned input slots. Neither run proves continuous
160 Hz motion.

Raw packets and PresentMon captures are in the local temp
`f4-current-cold-motion-20261031/cold-o165` and `cold-o166` directories.
Verbose media logs are in the local profiler archive's `wheel-curve-probe`
directory under the matching case names.
