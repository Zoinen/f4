# Issue #608 — solution review

## Problem model

The F5 copy/move confirmation dialog in `cmd/f4/actions.go` is 50×11. Its
vertical layout places the operation-mode ComboBox immediately before the
OK/Cancel row. `vtui.ComboBox.Open` deliberately opens its five-row menu below
the field when there is room on the screen, so the menu occupies the button
row and hides the buttons. The issue is therefore a deterministic owner-layout
collision, not a terminal-specific rendering problem.

The same pattern was checked in the other f4 ComboBox dialogs. The compact
dialogs for F5 Copy/Move, Create Link, Make Directory, Delete confirmation,
and the internal Dummy Operation all placed their action row immediately below
the selector. The remaining ComboBox dialogs either have more form rows or
enough vertical room.

## Three candidate fixes

### A — Put the action row before the mode selector (selected)

Keep the compact dialog sizes, but lay out each action row before its selector.
The ComboBoxes remain in their dialogs and keep their keyboard/tab behavior,
while their five-row popups open below the fields and therefore below the
action rows. The change is local to the affected f4 dialogs, leaves shared
ComboBox behavior unchanged, and directly implements the issue's suggested
placement.

### B — Put the buttons before the mode selector

Increase the confirmation dialog height and reserve several blank rows between
the selector and the buttons. This also avoids the overlap, but makes an
infrequently used setting consume more screen space and is less faithful to
the request to put the popup below the buttons.

### C — Teach vtui ComboBox about sibling controls

Make popup placement query its containing dialog and avoid every visible
sibling. This would provide a generic framework solution, but ComboBox menus
are currently independent frames and do not have a container-aware placement
contract. It would require a cross-repository API/design change and could
alter many existing dialogs without a complete layout audit.

## Three-pass review

1. **Correctness:** A moves the only affected button row above the ComboBox;
   the popup opens below the field and cannot intersect either button.
   Selection, default mode, keyboard handling, and callbacks are unchanged.
   The regression test opens the real production dialog and asserts that the
   popup rectangle does not intersect either button.
2. **Adversarial cases:** The mode list is localized, so the menu width may
   grow but its height remains three items. On an extremely short screen,
   ComboBox may flip upward because of its shared screen-bound fallback; that
   can still overlap rows above the field. The local fix targets the supported
   80×25 layout from the report. Making popup placement aware of every
   compact-dialog row is a separate generic vtui change and is not bundled
   into this ticket. Tests cover Copy/Move, Create Link, Make Directory,
   Delete confirmation, and Dummy Operation.
3. **Regression/scope:** Existing dialog-layout checks continue to pass; the
   intentional geometry changes are limited to compact dialogs with this exact
   selector/action-row collision. A native Linux ARM64 smoke run will open the
   affected dialogs, open a mode menu, confirm buttons remain visible, and
   close without performing a file operation.

## Decision

Use A. It is the smallest verified fix for the concrete bug and avoids a
cross-repository popup-placement refactor. Larger generic popup placement can
be proposed separately if another reproducible overlap is found.
