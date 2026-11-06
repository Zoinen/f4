# Hide environment sync echo and bound its interactive handoff

**Date:** 2026-10-21
**Severity:** medium
**Files:** internal/panel/frame.go, internal/panel/frame_procenv.go, internal/terminal/view.go, internal/panel/frame_procenv_test.go, internal/panel/frame_command_darwin_test.go

## Problem

After a long process command, the graphical terminal showed an Environment
Manager wrapper containing private runtime paths and status variable names.

## Root Cause

The value-free wrapper was still 1139 bytes long and could hit the macOS
interactive input limit. Separately, scanning a whole read buffer for its E
acknowledgement unmuted the view before parsing the wrapper echo in that same
buffer. The single-buffer regression reproduced visible technical echo.

## Solution

Stage the wrapper through the existing private-file command handoff, including
a failure acknowledgement if reading that file fails. Process active shell
markers in parser order. Restore muting at the exact acknowledgement boundary,
not in a later UI task; keep deferred input delivery scheduled on the UI.
Diagnostics report acknowledgement success and byte counts, not command text
or environment values.

## Prevention

Test one-buffer and split acknowledgements, preservation of the following
prompt, bounded wire length, verbose/errexit failure behavior and alternating
long commands/environment updates through a real macOS zsh PTY.

## Tags

`#terminal` `#pty` `#envman` `#ordering` `#macos`
