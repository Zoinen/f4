# ConPTY and long lines: what we measured, and what we decided

Context: microsoft/terminal#20626 and the discussion at #20629, where I asked
for optional logical-line markers in the ConPTY output stream. The premise
turned out to be wrong. This is the record, so nobody re-runs the argument
from memory.

## Decision

Ship the current ConPTY redistributable with f4, compiled into the binary.
No markers, no protocol change, no request pending on Microsoft, and
explicitly **not** an old pinned build - an old build is the thing that
breaks long lines.

`conpty.dll` and `OpenConsole.exe` must travel as a matched pair from the
same package and sit in the same directory; `conpty.dll` launches
`OpenConsole.exe` from beside itself. Mismatched versions produce the
failures reported in wezterm#7774.

Size, measured from `Microsoft.Windows.Console.ConPTY.1.25.260710002-preview.nupkg`
(release v1.25.1912.0), uncompressed:

| arch  | conpty.dll | OpenConsole.exe | pair    |
|-------|-----------:|----------------:|--------:|
| x64   |    109,920 |       1,063,224 | 1.12 MB |
| x86   |     88,376 |       1,036,128 | 1.07 MB |
| arm64 |    106,336 |       1,110,368 | 1.16 MB |

The package declares support for Windows 10.0.17763 and above - build 1809,
the release that introduced ConPTY - so there is no older-Windows fallback
to write. Bundling is also the distribution model Microsoft intends for
this component; see microsoft/terminal#17608.

## What was measured

`tools/conpty_probe.py` drives a real pseudoconsole and reads the raw byte
stream. Ten real programs, launched directly with `CreateProcessW`, console
mode `0x0007`, into a 20-column pty: findstr, certutil, reg, tasklist,
ipconfig, PowerShell, CPython, Node, git, bash.

**Current ConPTY passes their long lines through whole.** Verified at the
byte level, not inferred from a summary:

* `python -c "print('A'*200)"` - 200 literal `A` in one run, one CRLF (the
  program's own), zero cursor-positioning sequences.
* `tasklist` - 11,351 bytes, 146 CRLF for 146 output lines, zero `ESC[..H`.

Nothing arrives capped at the pty width. The terminal does the wrapping,
which means the terminal holds the wrap flags and can reflow and copy long
lines by itself, exactly as on Unix.

**The in-box ConPTY on the CI runner is the one that shreds them.** Same
`tasklist`, same launch: 590 CRLF and 582 cursor-positioning sequences for
those 146 logical lines. That is where the "long lines get cut" folklore
comes from, and it is an argument for shipping a current ConPTY rather than
freezing an old one.

## The one place structure is still lost

Text painted through the buffer API rather than written:
`WriteConsoleOutputCharacterW` with 200 characters at (0,0) in an 80-column
pty arrives as `[80, 80, 40]` from the current ConPTY, split by absolute
cursor positioning, against `[200]` from the in-box one.

Not worth pursuing. `SetWrapForced` is called from exactly one place in
conhost, `WriteCharsLegacy` in `src/host/_stream.cpp`; the buffer writes
never set it, so the wrap bit does not exist upstream either. Asking for it
to be forwarded would be asking for a bit nobody has. TUI applications also
render into the window size they queried, so they do not produce logical
lines longer than the row in the first place.

## Also worth knowing

* `ENABLE_VIRTUAL_TERMINAL_PROCESSING` is set by ConPTY, not by the
  application. In the default mode `0x0007` writes go through
  `WriteCharsVT` and are passed through untouched. Only an application that
  clears the bit lands on `WriteCharsLegacy`, where a `\r\n` is appended
  when a write ends exactly on the right margin.
* `cmd.exe` gives its children mode `0x0001` - no wrap at EOL, no VT. In
  that mode nothing wraps at all, so measurements taken through `cmd` say
  nothing about wrapping. Console output mode is per-handle, so it cannot
  be corrected from another process.
* `>CONOUT$` is not a neutral way to hand a program a stdout; it goes
  through cmd and inherits the same `0x0001`.
* Launching a child of a pseudoconsole requires
  `STARTUPINFOEX.StartupInfo.dwFlags = STARTF_USESTDHANDLES` with the three
  std handles left NULL, as in `ConptyConnection::_LaunchAttachedClient`.
  Without the flag `CreateProcessW` copies the parent's handle values into
  the child even with `bInheritHandles` false, and a parent whose stdout is
  a pipe leaves the child writing into an invalid handle.

## What stays in the tree

`tools/conpty_probe.py`, `tools/conpty_probe_child.py` and the
`ConPTY line-wrap probe` workflow stay as they are. They also probe a
bundled `conpty.dll` alongside the in-box one, which is the insurance
policy: if a future ConPTY regresses, the same run shows it against a build
that works. Nothing here depends on that path in normal operation.
