# Adaptive Settings fields

- Rounded category selection using the common 4 DIP radius.
- Typed fillWidth metadata for elastic controls; native nested loaders preserve
  symmetric group padding during growth and shrinkage. Baseline regression:
  profile input left inset16/right inset0.
- Radio caption and choices form one semantic field. Native glyph measurement
  selects inline, next-row horizontal, or vertical arrangement. Terminal layout
  and action IDs remain unchanged; no caption-text heuristics.
- Regression checks cover all three modes at 175%, subsequent-field clearance,
  elastic inputs/buttons, native text origins and unit transforms. Render each
  mode; preserve existing keyboard/focus and category drag tests.
