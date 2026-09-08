# Issue #703 solution review

## Evidence

The follow-up comments clarify two separate behaviors:

1. `Ctrl+Alt+Shift+T` must copy the active frame identity used by the help
   system, not the visible host/window title and not merely a transient title.
2. `Ctrl+Alt+RightClick` must work in the Windows GUI and TTY paths. The
   current Windows ANSI run reached `FrameManager` with `Mouse{Btn:Right,
   Mods:Alt,Ctrl}`, but the existing vtui translator found no target on the
   f4 main panel or a VMenu row.

The repository has no additional image or linked attachment for this issue.

## Candidate solutions

### Candidate 1: change the host title function

Make `currentWindowTitle` return the active frame identity. Rejected: that
would alter the terminal/GUI title and the documented `Far.Title` macro value.

### Candidate 2: use only the active frame title

Use `GetTopFrame().GetTitle()` for `App.CopyWindowTitle`. This was the earlier
partial fix, but the owner clarified that the stable help identity is the
useful value. Rejected as the primary value, retained only as a fallback for
legacy frames with no help topic.

### Candidate 3: extend the f4 event-filter boundary

Use `Frame.GetHelp()` first for the action, then handle translator misses in
f4's existing `EventFilter`. The fallback recognizes a right-button press
with Ctrl+Alt modifiers, resolves the visible f4 menu bar, VMenu rows,
command line, panel titles, table headers, and file cells, and creates the
same report format as vtui's existing translator. Selected: it fixes the
custom f4 rendering paths without changing the shared vtui dependency or
the normal mouse dispatcher.

## Three-pass review

### Pass 1: correctness

`currentFrameIdentity` returns the top frame's non-empty `GetHelp()` value,
which is the identifier consumed by the help system. It trims and falls back
to `GetTitle()` only for frames that do not declare help metadata, and falls
back to the host title only while the frame stack is empty.

The translator fallback checks `KeyDown`, a right-button bit rather than an
exact button value, both Ctrl and Alt modifier bits, and rejects mouse-motion
events. It is called from the already-installed f4 event filter, after the
shared vtui dispatch has had the opportunity to handle ordinary widgets.

### Pass 2: blast radius and lifecycle

The host title path and `Far.Title` remain unchanged. Ordinary clicks continue
through the existing event filter and `PanelsFrame.ProcessMouse`; only the
specific Ctrl+Alt right-button press is consumed after a concrete visible
target is resolved. The temporary translator element is not inserted into
the widget tree, so it cannot retain owners, focus, or stale panel data.

The target resolver follows the normal global-menu/frame priority and stops
at the top hit frame, avoiding accidental inspection of a frame behind a
modal dialog. Duplicate inherited help topics are collapsed in the report so
f4 adapters do not emit `Panels -> Panels`.

### Pass 3: regression and factual validation

The regression tests cover modifier shape filtering, VMenu row resolution,
menu-bar resolution, command-line resolution, report context, and help
identity precedence over a different visible dialog title.

On Windows 11 x64, a fresh binary from upstream base `459f268f` was run in a
real ANSI TTY. Repeated Ctrl+Alt+RightClick tests produced the translator
toast and the native clipboard contained reports for both a panel cell and a
Right-menu item. The event trace showed the physical mouse event being
consumed by the f4 event filter; the normal panel selection/menu action did
not run.

Focused tests pass. The full `cmd/f4` package still has the pre-existing
environment-specific failure `TestEditorView_PasteConvertsInternalClipboardCodepage`
(`"╧ЁштхЄ"` vs `"Привет"`) on this Windows host; the new tests are independent
of that failure. `go vet ./cmd/f4` reports only the repository's existing
Windows unsafe-pointer warnings.
