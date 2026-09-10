# Issue #165 solution review

## Candidate 1: keep whole-chunk pattern matching

- Keeps the current implementation small and preserves its existing behavior.
- Fails when ConPTY splits a long `cd /d "..." & rem f4_sync` line between reads; the partial command reaches the terminal renderer and can add visible lines.
- Rejected.

## Candidate 2: remove every visible `cd /d` fragment after rendering

- Would hide commands even when the command is split.
- Cannot safely repair already-rendered terminal state and could erase legitimate user-entered commands that happen to begin with `cd /d`.
- Rejected.

## Candidate 3: stream-aware pending command buffer (chosen)

- Detects the Windows synchronization marker across arbitrary input chunk boundaries, holds only an incomplete marker, and removes it once the complete command and line ending arrive.
- Preserves ordinary output and non-f4 commands after a quoted `cd`, while retaining the existing screen-clearing behavior for the hidden synchronization command.
- Bounds the pending buffer so malformed or unrelated long input cannot be retained indefinitely.

## Three-pass review

### Pass 1: correctness

- Input preceding a partial marker is emitted immediately; only the possible technical command suffix is retained.
- Complete markers are removed with their CR/LF terminator, including when the terminator is split across reads.
- A normal command after `" & ` remains visible; f4's exact `rem f4_sync` marker is the only command treated as hidden.

### Pass 2: compatibility and failure modes

- Existing Windows and cross-platform ANSI behavior is unchanged for complete chunks, including UTF-8 handling and non-sync commands.
- The pending bytes are copied before the caller's read buffer can be reused.
- A malformed command exceeding the bounded buffer is emitted rather than retained forever.

### Pass 3: scope and regression risk

- The fix is isolated to the shared f4 terminal parser and benefits every Windows PTY directory synchronization, not just one path length.
- Regression tests cover both a long path split inside the path and a split inside the `cd /d` marker.
- Full f4 tests and a native Windows x64 run that navigated into a deeply nested path were used for validation.
