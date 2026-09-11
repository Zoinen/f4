# Multiline semantic dialog fields

The vtui MultiLineEdit had no semantic projection or action handler, so Settings
exported empty space for association/user-menu commands. Add a multiLineEdit
kind with full logical text, rune-index cursor/selection and owner-routed pointer
selection. Keep all editing in Go and render a read-only native TextEdit with
scrollbars, disabled styling and scene-space pixel alignment.

Validation: missing projection reproduced before implementation; owner selection,
Unicode replacement, nested keyboard focus and one change callback tested.
Settings command edits reach the draft through the actual semantic route. Native
projection preserves fields. QML regression covers text/selection, pointer actions,
overflow scrollbar and TextEdit scene origins/unit transforms at DPR 1.75, plus a
rendered capture. Full operations tests: 50 passed, 1 opt-in profile skipped.
Static-host embedded-import smoke and Windows system-DLL import audit passed.
