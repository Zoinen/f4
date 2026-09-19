# Unified native Settings categories

## Request and cause
GUI, Gallery & cache and Terminal colors hid the semantic settings heading and
footer and painted their own titles, padding and save/close controls. The terminal
palette also presented normal and bright colors as an unpaired list.

## Change
- Reuse the actual semantic category heading and Apply / OK / Cancel delegates.
- Keep native page drafts alive across category switches; validate visited pages
  before applying, route successful footer actions to Go, and restore live GUI
  values on Cancel or dismissal. Unchanged Gallery drafts avoid cache maintenance.
- Remove native-page outer insets and redundant headings/footers; keep standalone
  theme-window controls intact.
- Use F4CheckBox for Overrides and existing F4 controls for the editor.
- Pair palette indices 0..7 and 8..15 in two columns, and preview them in two rows.
  Use rounded swatches/preview without borders. Fit the editor beside the palette
  at normal widths, stack it below at narrow widths.

## Regression evidence
Before integration, nativeSettingsPagePreservesConfigurator failed because the
shared semantic category title was hidden (.diagnostics/settings-unified-before.txt).
The extended test checks every visible text/raster leaf at DPR 1.75 for integral
physical origins and identity unit-vector transforms; it also checks palette
frame/swatch edges, paired placement, native draft retention, validation, Apply,
OK, Cancel and rollback when the dialog is removed. No fractional-coordinate
failures were observed in this change. Rendered GUI, Gallery, palette and narrow
palette captures were inspected.

## Verification
- Static Qt host and F4QuickViewSurfaceTests build.
- Eight QtTest cases including setup/cleanup: native settings, standalone theme
  restoration/options/pixel grid, resource-only QML imports, compiled-host startup.
- CTest: F4NativeSettingsPixelGridTest, F4QuickViewThemeDialogPixelGridTest,
  F4SettingsCategoryInteractionPixelGridTest, F4SemanticDialogControlsPixelGridTest.
- Portable Windows import audit passes.

## Prevention
New native settings pages should provide resetDraft/applyDraft (and optional
validateDraft) to SettingsDialogBody and reuse its semantic heading/footer rather
than introducing independent save/close controls or outer padding.

Delivery: regenerated embedded host; CGO_ENABLED=0 launcher built; targeted embedded-payload Go tests passed. Replaced only this worktree's f4-zoin.exe and launched --gui=qt. Running extracted Qt host hash matches the new static build.
