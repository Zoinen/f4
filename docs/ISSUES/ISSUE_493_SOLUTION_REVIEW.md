# Issue #493 solution review

## Pass 1 — direct index replacement

Replace every `table.SelectPos` use with an offset into `hkRows` and adjust
the list whenever filtering changes. Rejected: the table can also reorder rows
for sortable columns, so maintaining a second mapping would duplicate vtui's
selection model and could drift again.

## Pass 2 — rebuild `hkRows` in display order

Keep the existing button callbacks and reorder the application slice whenever
the table changes. Rejected: `vtui.Table` intentionally keeps `Rows` stable
and stores display ordering/filtering separately; copying that state into f4
would couple the dialog to internal widget behavior.

## Pass 3 — resolve the selected row through `Table.RowAt`

Use `table.RowAt(table.SelectPos)` for both Assign and Unbind, then index the
stable `hkRows` slice with the returned source row. This is the common fix for
sorting and QuickSearch because the same mapping covers keyboard navigation,
mouse selection, and Enter activation.

## Safety checks

- out-of-range and nil table selections are ignored;
- both Assign and Unbind use the same mapping;
- a regression test covers a sorted display and a filtered display;
- native Win32 validation will reproduce search, cursor movement, and Enter
  activation in the hotkey configurator.
