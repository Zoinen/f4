# Issue #601 solution review

## Problem model

The renderer f4 uses is decided entirely at invocation time. `--gui[=backend]`
and `--tty[=backend]` are the only levers, plus the `f4-gui.exe` filename
check, and everything else falls to `shouldTryGui()`/`tryRunDefaultGui()`
probing the environment on every start. A user whose answer never changes --
the reporter's `f4.exe --gui=gogpu` -- has to retype it every time, and there
is no way to express it once.

The request has three parts that are easy to conflate but are actually
independent: *which renderer family f4 opens* (console or window), *which
console backend* it uses (`ansi`/`winapi`), and *which graphics backend* it
uses (`win32`/`gogpu`/`ebiten`/`x11`/`wayland`). Automatic selection already
answers all three; the configuration has to be able to pin any subset of them
and leave the rest on detection.

## Three candidate solutions

### A — One `DefaultBackend` string

A single setting holding `gogpu`, `ansi`, or empty, with the mode inferred
from which family the name belongs to. Smallest diff, but it collapses the
three questions into one: it cannot express "start in a window, and let the
backend be detected", nor keep a console backend preference that only applies
on the runs that end up in a console. It also makes `win32` ambiguous, since
that name exists in both families.

### B — Three independent settings (selected)

`[Startup] Mode`, `GuiBackend` and `TTYBackend`, each defaulting to automatic,
resolved after argument parsing so the command line keeps precedence. The mode
selects the family, each backend applies to its own family whenever that family
is used, including on the automatic path. This matches the issue's own
decomposition ("отдельные пункты настроек для tty и графики" plus the
mode checkbox) and leaves every existing invocation behaving exactly as before.

### C — Persisted last-used backend

Record whatever backend succeeded and reuse it next time. Requires no UI at
all, but it makes startup depend on invisible state: a single `--gui=x11`
troubleshooting run would silently become permanent, and there would be no
obvious way to inspect or reset the choice. Rejected as too implicit for a
setting that decides whether the program opens a window.

## Three-pass review of option B

1. **Correctness pass.** Precedence is explicit: a `--gui`/`--tty` flag or a
   GUI-named executable sets `startupChoiceGiven` and suppresses the configured
   mode; a backend named on the command line sets `guiBackendGiven`/
   `ttyBackendGiven` and suppresses the configured backend. With an absent
   `[Startup]` section every value resolves to automatic, so an existing
   settings.ini reproduces today's behavior byte for byte — pinned by
   `TestStartupConfigDefaultsAreAuto`. `--gui=auto` and `--tty=auto` are the
   escape hatch for overriding a configured backend on one run, which would
   otherwise be unreachable once a backend is pinned.

2. **Adversarial pass.** The two directions are deliberately asymmetric.
   Config values go through `normalizeStartupGuiBackend`/
   `normalizeStartupTTYBackend`, which map unknown names to automatic: a
   settings.ini copied from another machine, or written by a newer build, must
   not be able to leave f4 unable to start. Command line values pass through
   unchanged so a typo still produces a visible `RunGui` error instead of
   silently starting something else. For the same reason `runGuiBackend` falls
   back to `tryRunDefaultGui()` when a *configured* backend fails — a static
   build without FFI, a missing GPU stack, a display server that is not running
   — while a backend named on the command line keeps failing hard. Combo
   selections are read back through bounds-clamping helpers, and
   `startupBackendChoices` copies rather than aliases its source slice.

3. **Regression pass.** The change is confined to startup resolution and the
   new dialog. `SelectedTTYBackend`'s existing consumers are untouched: both
   `winapi` and `win32` were already accepted downstream
   (`main.go`, `console_passthrough.go`), so canonicalizing the stored spelling
   to `winapi` is invisible to them. `RunGui`'s `qt` and `ext:` external-UI
   routes are preserved by the normalizer. Backend names are shown untranslated
   in the dialog on purpose, so what the user picks matches what the
   documentation and the command line call it.

## Validation target

`go build`, `go vet` and the `cmd/f4` suite run locally, including the
localization gates (`TestLangConsistency`, `TestNoNewHardcodedUIStrings`) and
the new `startup_backend_test.go`. What the local environment cannot exercise
is the part that only exists on other platforms: the Win32/GDI and Wine paths,
and an actual `gogpu`/`ebiten`/`wayland` window. The configured-backend
fallback in particular is best confirmed on a machine where the pinned backend
genuinely fails.
