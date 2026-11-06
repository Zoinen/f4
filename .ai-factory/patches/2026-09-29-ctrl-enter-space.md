# Ctrl+Enter filename argument separator

## Problem

Ctrl+Enter inserted a filename without a trailing space; repeated insertions
concatenated names rather than preparing the next shell argument.

## Root Cause

The shared panel insertion method passed only the escaped filename to the
command edit. Existing tests explicitly expected that behavior.

## Solution

Append a space after shell escaping in the shared method, preserving path-prefix
concatenation and both action-dispatch and direct-key handling.

## Prevention

Test quoted filenames, directory entries, existing path prefixes, and repeated
insertions. The updated regression tests fail without the trailing separator.
