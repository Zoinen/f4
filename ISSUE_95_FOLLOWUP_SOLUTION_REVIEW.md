# Issue #95 follow-up solution review

## Reproduction

The first fix covered the normal f4 terminal path, but it did not cover the
host/Far console path. In that mode `PanelsFrame.ProcessKey` sends a TAB to the
PTY after the f4 command line declines it, while the command being completed is
still only in f4's overlay. On an ANSI host console the regular autocomplete
popup is intentionally suppressed because vtui cannot repaint the host screen
safely, so the reporter sees no change.

## Candidate solutions

1. Handle a plain TAB in the host-console command line with the existing
   `vtui.AutoCompleteMenu` and VFS path-hint provider. On a WinAPI overlay,
   push the menu so the existing native popup renderer can display it. On an
   ANSI overlay, accept the menu's first selectable match directly and redraw
   the command line; this keeps the host screen safe while still completing the
   command. **Chosen.**
2. Remove `AutoCompleteSuppressed` for ANSI host consoles and let vtui draw its
   normal menu. Rejected: the normal frame renderer targets f4's screen buffer,
   not the live host console, and would repaint or erase host-console history.
3. Forward TAB to the shell or implement a second filesystem scanner in the
   host-console code. Rejected: the command is held by f4 until Enter, and a
   second scanner would bypass remote/plugin VFSes, existing ranking, quoting,
   colors, and replacement-span handling.

## Three-pass review of the chosen solution

### Pass 1 — behavior

- A plain TAB with a non-empty command line asks the same autocomplete pipeline
  used by panel mode for history and VFS path matches.
- Host WinAPI consoles retain the popup and its keyboard navigation.
- Host ANSI consoles complete the selected first item immediately and repaint
  the overlay, so the command is visibly changed and TAB never reaches the
  shell behind it.

### Pass 2 — compatibility and scope

- Ctrl/Alt/Shift-modified TAB, an empty line, disabled autocomplete, no-match
  input, and overlays disabled continue through the existing shell path.
- The fix is platform-neutral at the dispatch boundary; only the already
  existing `consoleOverlayUsesWinAPI` capability decides whether to push or
  accept the menu.
- Path completion continues to use the active/passive panel VFS selection,
  timeout, fuzzy matching, quote preservation, and replacement spans from
  `path_hints.go`.

### Pass 3 — regression and failure modes

- A regression test models the reported host-console state with a real temporary
  directory and verifies `cd sub` becomes `cd subdir/` without any PTY write.
- Existing command-line, path-hint, host-console, and full f4 tests remain the
  validation gate; the ARM64 binary is also rebuilt and exercised live.
- If no completion candidate exists, the helper declines the event rather than
  changing the line or swallowing unrelated shell input.

## Conclusion

The host console must complete f4-owned input before raw PTY forwarding. Reusing
the existing menu keeps behavior consistent, while the ANSI direct-accept path
honors the renderer's safety constraint instead of drawing a popup into an
unreadable host buffer.
