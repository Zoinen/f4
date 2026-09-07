# Issue #261 solution review

## Scope

Issue #261 reported three color areas as unreliable or unclear:

* `Help.Box` did not appear to apply.
* `Editor.Scrollbar` and `Viewer.Scrollbar` appeared to be applied
  inconsistently.
* The roles of `Scrollbar` and `Table.Box` were unclear, and a follow-up
  comment reported `WarnDialog.Button.Selected` as ineffective.

The current check was performed from `upstream/main` at `4227e8a6` on Windows
11 x64 in an ANSI TTY.

## Candidate review

1. **Color parsing and slot mapping.** `cmd/f4/colors.go` maps
   `Help.Box` to `vtui.ColHelpBox`, maps `Panel.Scrollbar`, `Viewer.Scrollbar`,
   and `Editor.Scrollbar` to dedicated f4 slots, maps `Table.Box` to
   `vtui.ColTableBox`, maps generic `Scrollbar` to `vtui.ColScrollBar`, and
   maps `WarnDialog.Button.Selected` to `vtui.ColWarnSelectedButton`.
   `WarnDialog.Edit.Selected` intentionally shares `vtui.ColWarnEdit` with the
   warning edit field because vtui's warning palette has no separate selected
   edit slot. No runtime mapping defect remains.
2. **Widget consumers.** The panel, generic table/tree, help view, warning
   dialog, editor, and viewer all consume the mapped palette indices. Dedicated
   editor/viewer scrollbar tests and panel rendering tests are present, and
   the current vtui `ScrollBar` defaults to the shared `ColScrollBar` slot.
3. **Configuration/documentation.** The checked-in generated table still
   listed `WarnDialog.Edit.Selected` as `ColWarnSelectedButton`, contradicting
   the runtime mapping and the current vtui behavior. This residual user-facing
   defect was corrected, and a regression test now checks every documented row
   against `ColorSlots`.

## Three review passes

### Pass 1 — functional correctness

The existing focused tests passed for Help.Box, the editor/viewer/panel
scrollbars, Table.Box separators, and warning-dialog palette selection. The
new documentation consistency test also passes after correcting the stale row.

### Pass 2 — regression and scope check

The change is documentation plus a read-only consistency assertion; it does
not change palette indices, rendering, input, or configuration parsing. It
cannot alter runtime color behavior. The assertion uses the repository's
`ColorSlots` as the source of truth and will catch another stale generated row.

### Pass 3 — native Windows validation

An isolated portable profile applied distinct colors for all reported slots.
The freshly built binary showed:

* the panel scrollbar using `#112233` on `#445566`;
* a real warning save dialog selected button using `#ff00ff` on `#123456`;
* the Help frame using `#778899` on `#aabbcc`.

The process exited normally through the real F10 confirmation dialog. The
native ANSI output therefore confirms that the current upstream runtime loads
and renders the relevant custom colors; the only remaining defect found in
the issue's surface was the stale documentation row.

## Decision

Submit the documentation correction and its regression test. No functional
palette change is justified by the current evidence.
