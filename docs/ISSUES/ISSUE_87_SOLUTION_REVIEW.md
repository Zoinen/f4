# Issue 87 solution review

## Reproduction

Issue 87 reports that starting F4 inside the built-in F4 terminal on Windows
breaks mouse wheel navigation, left/right clicks, menu cursor rendering, and
progress rendering. The real Windows validation session reproduced the input
part: the outer F4 forwarded SGR mouse sequences, while the nested F4 used the
Windows native reader. Windows ConPTY converted those bytes to `MOUSE_EVENT`
records; the nested reader saw both button clicks as `ButtonState=3`, so it
could not distinguish left and right clicks. Wheel events remained visible but
the same protocol mismatch made the nested session unreliable.

## Three candidate fixes

1. Teach `TranslateMouseInput` or the Windows reader to infer button identity
   from the lossy ConPTY `MOUSE_EVENT` records. This cannot recover whether a
   release was left or right, needs stateful heuristics, and could corrupt
   ordinary native-console mouse input.
2. Add a second Windows-specific mouse wire format to the outer F4 and a
   matching decoder to the nested F4. This would be proprietary, would not
   help other terminal applications, and would create another protocol mode to
   keep synchronized with ConPTY and the terminal mirror.
3. Pass the same child environment on Windows as on Unix, keep the existing
   `F4_NESTED=1` marker, and make a nested F4 select the ANSI reader by default.
   The outer F4 already mirrors Win32/Kitty/mouse protocol state and forwards
   the standard SGR bytes; the nested ANSI reader can parse those bytes without
   the lossy ConPTY conversion. An explicit `--input` remains authoritative.

## Three-pass review of candidate 3

### Pass 1: correctness

`pty_windows.go` now supplies the environment block built by
`terminalChildEnv`, so the Windows shell receives `F4_NESTED=1` just like
Unix shells. At startup, only a nested Windows F4 with no explicit `--input`
gets `vtinput.InputMode="ansi"`. The outer F4 continues to receive native
console events and translates them to standard SGR; the nested F4 parses those
events directly, preserving left/right/release and wheel semantics.

### Pass 2: concurrency and lifecycle

The environment block is immutable for the lifetime of `CreateProcess` and is
kept alive until the call returns. No shared reader or PTY state is changed.
The explicit mode check prevents command-line behavior from being overwritten,
and the change affects only a child process marked by `F4_NESTED=1`.

### Pass 3: scope and regressions

Top-level Windows F4 keeps the native ConPTY reader, Unix behavior is
unchanged, and non-F4 programs still receive the ordinary terminal protocol.
The added pure mode-selection tests cover nested, explicit, top-level, and
Unix cases. Native validation must confirm that a nested F4 receives distinct
left/right SGR events and that wheel, F9, and progress rendering remain usable.
