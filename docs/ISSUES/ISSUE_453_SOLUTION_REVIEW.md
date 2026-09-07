# Issue #453 solution review

## Problem model

The localized overwrite dialog fix is already present in merged PR #587, but
the validation that accompanied it did not exercise the production
`AskOverwrite` constructor. The language test built a representative row of
buttons directly, while the general layout test could be skipped locally
because its dependency globs omitted `file_ops.go` and
`dialog_button_layout.go`. That allowed the validator to report a green result
without proving that the real conflict dialog stayed inside its frame.

## Candidate solutions

1. **Chosen: validate the production dialog in every bundled language.** Start
   `AskOverwrite`, inspect the actual `vtui.Window` with `vtui.AssertLayout`,
   and close it through the real Escape path for each language pack. Add the
   production layout files to the validator cache dependencies.
2. **Rejected: add another screenshot or PTY-only test.** A screenshot can
   reproduce one locale and terminal size, but it does not cover all captions
   or provide a stable assertion about frame boundaries.
3. **Rejected: widen the dialog or remove the validator check.** This hides
   the symptom at one size and permits the same overflow in another language
   or smaller terminal; it also leaves the validator's blind spot intact.

## Review pass 1 — fidelity

- The new test constructs the exact `AskOverwrite` dialog used by file
  operations rather than duplicating its button layout.
- It runs against every embedded language pack, including scripts that were
  previously excluded from the synthetic test.
- `vtui.AssertLayout` checks both frame clearance and each widget's painted
  width.

## Review pass 2 — coverage and maintenance

- The local dependency cache now invalidates when the production file-op or
  button-row layout changes, so the broad validator cannot be silently stale.
- The test drives Escape and waits for `AskOverwrite` to return, ensuring the
  asynchronous UI path is cleaned up for every language.
- No platform-specific screenshot assumption is embedded in the regression.

## Review pass 3 — risk

- The merged production layout code remains unchanged; this follow-up only
  makes its regression proof match the real dialog.
- The test uses the existing silent screen and normal task pump, so it does
  not access the user's desktop or clipboard.
- If a future translation overflows, the failing subtest names the language
  and the exact layout violation.

## Decision

Keep PR #587's production fix and strengthen the validator around the actual
dialog construction. This directly addresses the owner's concern about why
the validator missed the report without widening the UI or weakening layout
rules.
