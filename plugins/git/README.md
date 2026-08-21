# Built-in Git plugin

The Git plugin is an in-process f4 plugin backed exclusively by the public
`github.com/go-git/go-git/v6` API. During this development phase f4 keeps that
upstream import path and pins it, through `go.mod`, to the public
[`Zoinen/go-git`](https://github.com/Zoinen/go-git) fork. It never runs
`git.exe`, `git-lfs`, `gpg`, or `ssh`. Repository hooks are the exception:
they are programs explicitly configured by the repository owner.

## Fast navigation

Entering a local directory only reads the session cache on the UI path. A
cancellable worker immediately checks its `.git` directory or `gitdir:` file.
When that is absent, it waits 300 ms before looking up parent directories.
Both positive and negative answers are refreshed on every visit and never
persisted between f4 sessions. Branch labels use the same debounce and are
read from the cache while the command prompt is rendered.

The normal panel receives cached extended attributes and a status prefix:

| Prefix | Meaning |
| --- | --- |
| `+` | staged |
| `~` | unstaged |
| `±` | both layers changed |
| `?` | untracked |
| `!` | conflict |
| `·` | ignored |

Directories receive an aggregate state. Colours are semantic f4 palette roles,
resolved while rendering, rather than values retained by the plugin.

## Status panel

Use **Git: Status** from the plugin command menu while a local filesystem panel
is active. The passive panel becomes a linked virtual view with staged rows,
a non-interactive separator, then unstaged rows. The source real panel remains
the navigation target and status state is refreshed asynchronously.

| Key | Action |
| --- | --- |
| F2 | commit dialog (multiline message; optional OpenPGP/SSH signature) |
| F3 | view a unified diff for the selected layer |
| F4 | edit a text-only patch safely |
| Space | stage or unstage one homogeneous selected group |
| F7 | open Git history in the passive panel |

The F4 flow restores the original selected patch to its recorded base, applies
the edited patch, and restores the original changes if the edited patch fails.
Binary diffs, gitlinks, ignored rows, and unresolved conflicts stay read-only.
Commit hooks run in Git order (`pre-commit`, `prepare-commit-msg`,
`commit-msg`, `post-commit`). Checking **Sign commit** never silently creates
an unsigned commit: the configured `user.signingKey` must resolve as an
OpenPGP or SSH key; X.509 returns an explicit unsupported error.

## History panel

History is a lazy, bounded (200 commit) VFS. At its root F2 cycles HEAD DAG,
all local refs, and first-parent traversal. Enter a full-hash commit directory,
then F2 switches between changed files and the full snapshot. F4 edits a
session-only overlay for an historical text file; F5 copies the overlay when
present, otherwise the commit's after-blob. No historical overlay changes the
worktree, index, or Git object database.

## Current core boundaries

The fork includes cancellable status, diff, stage/unstage, text-patch apply,
hook, signing, and local LFS pointer/object-store APIs. The LFS store validates
SHA-256 and keeps objects in `.git/lfs/objects`, but f4 intentionally does not
yet implement LFS HTTP/SSH transfer, locking, fetch, or push. Partial-clone
objects must already be present locally.
