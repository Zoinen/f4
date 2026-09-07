# Issue #266 solution review

## Reproduction

1. Open a directory containing at least two objects.
2. Mark two files or directories in the active panel.
3. Press `Ctrl+A` or choose `File attributes` from the `Files` menu.
4. Change an attribute and press `Set`.

Before this fix, `actionFileAttributes` called `getRawSelectedName()`. That
always returned only the row under the cursor, even when the panel had an
explicit multi-selection, so the dialog could update only one object.

## Candidate solutions

### 1. Repeat the old single-file dialog from the action

The action could open the same dialog once per selected path. This would make
all objects reachable, but would require the user to repeat the same change and
would be especially poor for remote or mixed file selections.

### 2. Apply the first item's complete metadata to every selected object

This is simple, but copying the first object's full metadata would overwrite
object-specific fields such as Windows directory/reparse/compression flags and
could change unrelated ownership or timestamps.

### 3. Use one dialog with a stable target snapshot and apply edited fields to all

The selected paths and their metadata are captured before the asynchronous
dialog opens. On Set, edited common fields are applied to every target while
target-specific metadata is retained. The Windows path edits only the ordinary
Read-only/Hidden/System/Archive bits and preserves each target's advanced bits.

Selected solution: option 3.

## Three-pass review

### Pass 1 — functional correctness

- `GetSelectedNames()` supplies all explicit selections and still falls back to
  the cursor for the normal single-object case.
- Each selected path is `Lstat`-checked before opening the dialog, so symlink
  metadata is not silently replaced by target metadata.
- Unix mode/owner/group/mtime edits are sent to every target.
- Windows ordinary flags and last-write time are sent to every target.
- A selected `..` entry is excluded by the existing selection contract.

### Pass 2 — safety and compatibility

- The asynchronous operation uses an immutable path/metadata snapshot; later
  cursor movement cannot retarget the operation.
- Windows advanced flags are merged per target rather than copied from the
  first row.
- Symlink target editing remains available for a single symlink and is not
  ambiguously applied to a multi-selection.
- Existing single-file dialog entry points remain wrappers around the new
  target-aware implementation.
- Set errors now remain visible instead of closing the Windows dialog silently.

### Pass 3 — scope and regression risk

- The action registry already contains `File.Attributes` in the `Files` menu;
  a menu regression test now guards that discoverability path.
- No file-operation or selection semantics were changed outside attributes.
- Tests cover the action using two selected entries and the Windows metadata
  merge behavior that protects per-object advanced flags.
- Native Windows validation must confirm that the real TTY action updates both
  selected objects, not merely that the helper receives two paths.
