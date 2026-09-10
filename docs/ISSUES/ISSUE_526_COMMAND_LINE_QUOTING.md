# Issue #526 follow-up solution review

## Problem model

PR #573 stopped the command line from sending unmatched POSIX single and
double quotes to the one-shot shell runner, but the guard did not recognize
backticks. On POSIX shells an unmatched backtick starts command substitution
and waits for a closing delimiter, which makes the f4 tab appear frozen. The
same character is ordinary text for the Windows `cmd.exe` dialect.

## Candidate solutions

1. **Chosen: track shell quote contexts with a small stack.** Extend the
   existing guard to recognize POSIX backticks, including backticks inside
   double quotes and quoted/escaped content, while preserving Windows literal
   backticks and existing POSIX/Windows escape rules.
2. **Rejected: special-case a lone backtick with a string count.** Counting
   characters would mis-handle escaped delimiters, backticks inside single
   quotes, and command substitutions embedded in double quotes.
3. **Rejected: kill or time out the child shell after it hangs.** That leaves
   the user without a useful error, makes a valid long-running command
   indistinguishable from malformed input, and does not prevent backend-specific
   UI freezes while the child is waiting for continuation input.

## Review pass 1 — shell fidelity

- POSIX `'`, `"`, and `` ` `` contexts are tracked independently and can
  return to an outer context after a nested delimiter closes.
- Backslash escaping remains active where the existing scanner allowed it;
  content inside POSIX single quotes stays literal.
- Windows keeps caret escaping and treats both apostrophes and backticks as
  ordinary command text, matching `cmd.exe` behavior.

## Review pass 2 — user-visible behavior

- An unmatched backtick is rejected before command history or PTY dispatch.
- The existing error dialog path is reused, so the panels remain visible and
  the user can close the message and correct the command.
- Tests cover the parser and the real PanelsFrame Enter path, including the
  guarantee that no malformed command reaches the PTY.

## Review pass 3 — compatibility and risk

- The scanner is linear in command length and allocates only for nested quote
  contexts.
- The change is platform-neutral at compile time; the runtime dialect switch
  keeps the fix POSIX-only, so Windows command syntax is not regressed.
- The Linux Arch + gogpu report cannot be reproduced with the available
  Fedora Wayland backend, but the generic cause is prevented before any
  backend or shell process is involved.

## Decision

Keep PR #573's existing single/double quote protection and make the guard
model the complete set of relevant POSIX delimiters. This removes the actual
source of the hang while preserving valid commands and the Windows dialect.
