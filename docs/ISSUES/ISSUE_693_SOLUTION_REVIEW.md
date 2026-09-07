# Issue #693 solution review

## Problem observed

The generic Linux release is built with `goffi_static`.  That makes the
binary fully static, but also makes foreign-function loading unavailable.
When the user explicitly selects `--gui=gogpu`, vtui creates a native window
before the first Wayland/library call fails.  The result is a blank window and
an opaque backend failure instead of a usable f4 session or an actionable
message.

The current ARM64 host reproduced this with a statically linked binary:

```text
GOGPU_HOST: app.Run() exited with: wayland: failed to open libwayland:
... goffi: FFI is disabled in this build ...
```

## Candidate solutions

1. **f4-side backend preflight (chosen).** Check goffi availability before
   calling vtui for `gogpu`, `wayland`, or `ebiten`. Return a clear error for an
   explicitly requested unavailable backend; automatic selection can continue
   to the next backend. Keep X11 available because it has a pure-Go path in
   these static artifacts.
2. **vtui-side preflight.** Add the same check inside each vtui GUI host.
   This would protect every vtui consumer, but requires a vtui change, a new
   vtui release, and an f4 dependency update. It is a useful follow-up, but it
   is broader than the regression reported here and would delay the f4 fix.
3. **Fake or silently downgrade the GPU host.** Keep the window alive and
   render through a substitute without reporting that the requested backend is
   unavailable. This hides the missing capability, can leave a blank window,
   and makes explicit backend selection misleading; it is rejected.

## Three review passes

### Pass 1: static-build behavior

The preflight uses `ffi.Available()`, the capability API provided by the
goffi fork already used by f4. It is false for `goffi_static` and true for
normal dynamic builds (and for Windows, where the static tag is intentionally
ignored). Thus the check affects only binaries that cannot perform the calls
the selected backends require.

### Pass 2: fallback and explicit selection

`RunGui` checks before constructing a renderer or window. Automatic selection
receives an ordinary backend error and continues to X11 when that display is
available. An explicit `--gui=gogpu`, `--gui=wayland`, or `--gui=ebiten` fails
immediately with the reason and suggests `--gui=x11` or `--tty=ansi`. No
window is created in either case.

### Pass 3: portability and compatibility

The helper is shared by Linux, macOS, and Windows entry points, where the
existing goffi module provides the capability query. Other Unix targets use a
small no-op shim because goffi is not part of their f4 GUI build path. External
UI backends (`qt` and `ext:`) bypass the check because f4 does not own their
loading path. The change uses only the existing goffi module and does not
alter renderer setup, session state, or the X11/ANSI paths.

The regression tests exercise the backend classification and unavailable
error without opening a display. The static ARM64 binary is also built and
run to verify that explicit `--gui=gogpu` now fails before the old blank-window
path.
