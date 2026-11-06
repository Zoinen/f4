# Masonry shell publication cost, 2026-11-03

The o175 diagnostic times synchronous delivery of
`SurfaceRegistry::shellChanged` and records the changed top-level keys.
It logs after delivery, so trace formatting and writing are outside the
reported signal duration.

| Start after navigation | Changed keys | Synchronous delivery |
|---:|---|---:|
| 6.602 ms | activePanel, title | 12.835 ms |
| 115.931 ms | terminal | 4.141 ms |
| 120.675 ms | activePanel | 13.149 ms |
| 237.272 ms | panels, terminal | 8.311 ms |

The two activation publications each exceeded the 6.25 ms display
period without changing the panel descriptors. The controller already
publishes typed active-panel state and a dedicated activation
acknowledgment. This is a concrete candidate for separating chrome-only
updates from the signal that rebuilds the retained shell projection.
Any change must preserve the registry's latest backing state, including
when the QML projection is created after the update. Terminal changes
need their own analysis because the retained shell has terminal
consumers. These timings are diagnostic and do not measure cadence
without verbose tracing.
