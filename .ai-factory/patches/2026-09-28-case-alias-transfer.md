# Case-alias transfers and hidden queue confirmations

**Date:** 2026-09-28
**Files:** internal/fileops/local.go, internal/fileops/ops.go, internal/fileops/ops_case_test.go, internal/app/actions.go, internal/app/actions_copy_conflict_test.go, docs/VFS.md
**Severity:** critical

## Problem

Copying `_1.MMM` onto `_1.mmm` stalled the queue on case-insensitive macOS.
With unconditional overwrite, a temporary regression fixture reproduced source
truncation. The user's original file was intact.

## Root Cause

Path string comparison only accounted for Windows case folding, not filesystem
identity or hard links. Existing case aliases also bypassed optimized move.
Panel actions omitted the originating frame when enqueueing transfers, placing
overwrite confirmation in a hidden queue workspace while its worker retained
the disk resource reservation. A live goroutine dump confirmed this wait.

## Solution

Compare local objects using os.SameFile before copying, including after conflict
renaming. Allow native case-only moves only for aliases rather than distinct
directory entries. Pass the originating panels frame through copy/move dispatch.

Regression tests reproduced both failures before the fix and pass afterward:
case aliases, hard links, optimized rename, subsequent queued copy, and four
panel action paths retaining conflict-dialog ownership. Fileops and VFS suites
and focused app tests pass. Go and Qt builds succeeded.

Live GUI checks on disposable files verified rejection of case-alias self-copy,
subsequent successful copy, case-only rename, and visible overwrite confirmation.
Checksums matched after testing. Cancelling the original stalled operation let
the user's pending `_2.mmm` copy finish; both originals and copy match in SHA256.

## Prevention

- Test object identity, not just path spelling, before destructive copy opens.
- Keep remote paths out of local filesystem identity checks.
- Test case-insensitive aliases and distinct case-sensitive directory entries.
- Test real action dispatch with a different queue-frame anchor and assert the
  active workspace does not change during interactive confirmation.
- Assert a failed task releases its reservation and the next task completes.

## Tags

`#filesystem-identity` `#macos` `#data-loss` `#queue` `#dialog-ownership`
