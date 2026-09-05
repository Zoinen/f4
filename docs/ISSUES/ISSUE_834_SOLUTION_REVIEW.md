# Issue #834 — solution review

## Problem model

Editor and Viewer title bars currently reduce a file to its final name. That
is ambiguous when several files with the same name are open, while simply
printing a full path can collide with the codepage, mode, percentage, and
cursor fields on the right. The feature therefore needs a persistent opt-in,
must work with local and virtual VFS paths, and must keep the filename visible
when the title bar is narrow.

## Three candidate fixes

### A — Always show the full path

Replace the basename in both title bars with the path. This removes ambiguity,
but changes the default layout and still lets a long path overwrite or obscure
the status fields.

### B — Add one shared opt-in and title-bar truncation (selected)

Add `Interface.DisplayFullPathInTitle`, expose it in Appearance settings, and
use a shared VFS-aware helper for Editor and Viewer. Keep workspace-tab labels
compact, and make the common TopBar middle-truncate its left title when the
right-side status fields need room. This preserves the default behavior while
making the opt-in useful on both local and remote filesystems.

### C — Add separate per-view settings or reserve a fixed path width

Give Editor and Viewer independent controls, or reserve a fixed number of
columns for the path. Separate controls duplicate a preference, while a fixed
width wastes space for short paths and still does not adapt to terminal size.

## Three-pass review

### Pass 1: correctness

The selected setting defaults to off, so existing title and workspace-tab
behavior remains unchanged. When enabled, the title bar and the descriptive
Editor/Viewer frame title use the complete VFS path. The TopBar computes the
available left width after the right status string and keeps both path ends,
including the filename, using display-cell-aware grapheme truncation.

### Pass 2: VFS and lifecycle edge cases

The full-path branch returns the opaque VFS path directly instead of applying
the host `filepath` rules to an SFTP, archive, or other virtual path. Basename
display still delegates to `VFS.Base` when available. Internal frame and
workspace identities intentionally remain compact, and generated Editor
titles continue to take precedence. Missing config keys default to the old
basename behavior.

### Pass 3: regression and scope

Tests cover config round-tripping, the Appearance checkbox, local Editor and
Viewer title bars, compact workspace tabs, and narrow/wide-character TopBar
truncation. The implementation is shared by the two requested views; the
unrelated image and video title identities are not changed.

## Decision

Use B. It provides the requested opt-in without consuming title-bar space by
default, preserves the status fields, and generalizes cleanly across the VFS
implementations already supported by f4.
