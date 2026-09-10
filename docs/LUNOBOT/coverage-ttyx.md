# Coverage: `internal/ttyx`

## Scope

This change adds display-independent coverage for the X11 terminal-session
state machine: focus-event filtering, focus transition notifications, and
the no-display error paths for geometry, key grabs, focus queries, and child
window lookup. Existing Xvfb-backed integration tests remain unchanged.

## Baseline

Codecov on `main` commit `e40b44db248b72deb958a8fe6c70ac6bca31e349` reports
`internal/ttyx` at 28.04% across 699 lines. The package is a real logic
package, but its Xvfb-dependent tests are skipped on CI runners without an X
display.

## Verification

No local Go build or test was run, per the current LUNOBOT instructions.
Static verification is `git diff --check`; GitHub Actions is authoritative.
The exact CI run and resulting Codecov coverage will be recorded here.
