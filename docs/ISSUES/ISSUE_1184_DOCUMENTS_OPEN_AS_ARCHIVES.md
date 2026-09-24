# Issue #1184 solution review — office documents opened as archives

Report: `.docx`, `.xlsx` and probably other office formats do not start on
Enter; the panel enters them as archives instead, because they are ZIP
containers. The default action must be opening by extension association.

## Cause

`ArchiveProvider.CanOpen` asks `zipper/archive.DetectFormat` about the name
first. For `report.docx` that answers `""`, so the probe added for
self-extracting archives takes over: `findEmbeddedArchive` scans the file for
the `PK\x03\x04`, `7z…` and `Rar!…` signatures. An office document carries the
ZIP signature at offset 0, so the probe reports an archive and `CanOpen`
returns true.

`PanelEnterAllowed` then refused ordinary Enter only for `offset > 0`, the
executable-stub case. At offset 0 it allowed Enter, and `list.go` mounted the
document as an archive. Measured on a real `.docx`:

| step | result |
| --- | --- |
| `DetectFormat("report.docx")` | `""` |
| `findEmbeddedArchive` | `{format: zip, suffix: .zip, offset: 0}`, found |
| `CanOpen` | true |
| `PanelEnterAllowed` | true — the bug |

Everything reachable this way shares the shape: `.docx`, `.xlsx`, `.pptx`,
`.odt`, `.jar`, `.apk`, `.epub`. A double click hits the same code, because
`frame.go` turns it into `VK_RETURN`. The content probe arrived with
"archive: open local SFX containers for browsing" and was gated behind
Ctrl+PgDn for `offset > 0` only, which is the gap these files fell into.

## Change

`PanelEnterAllowed` now decides on the name first, on the rule that the name
outranks the content for the default action:

- the name declares an archive format — Enter opens the archive, as before.
  `nameDeclaresArchive` answers from the name alone, covering what
  `DetectFormat` selects an engine from plus the split-volume names
  (`name.7z.001`, `name.z01`, `name.r00`) that only the content probe
  recognizes. `DetectFormat` itself is not used for this: it falls back to
  reading the file when the extension says nothing, and it is handed a bare
  base name here, so its answer would depend on the process working directory;
- the name declares something else (`.docx`, `.jar`, `.exe`) — Enter goes to
  that extension's association, and Ctrl+PgDn still browses the container;
- the name has no extension at all — nothing to associate, so the content
  decides, minus the self-extracting case, exactly as before;
- the parent is not the local file system — unchanged. Inside another archive
  or on a remote one there is no association and no system opener to hand
  Enter to, so the content stays the only thing to go on.

## Tests

- `plugins/archive/issue1184_test.go`: documents and packages keep `CanOpen`
  (Ctrl+PgDn) but lose ordinary Enter; archive names, split volumes and
  extension-less archives keep it; an SFX stays out of it with and without an
  extension; a table for `nameDeclaresArchive`.
- `internal/panel/issue1184_test.go`: Enter on a `.docx` row is not consumed
  by the panel, reaches `Execute` once, and leaves the VFS alone, while
  Ctrl+PgDn mounts the archive VFS.

Both fail before the change and pass after it. Nothing here is
platform-specific: the reproduction and the fix were measured on Linux.

## Amendment: packages, and the rule as a setting

The rule above answered the question from the shape of the name -- any
extension that is not an archive's keeps Enter for its association -- which
is wider than what the issue reported. It reported office documents; it also
caught `.jar` and `.apk`, and neither ancestor treats those that way. Far
Manager's arclite lists `*.jar` and `*.[ah]pk` among the names Enter may open
(`arclite/options.cpp`), and far2l's `KnownDocumentTypes`
(`multiarc/src/MultiArc.cpp`) does not mention them, so both browse a package
on Enter.

The deduction is therefore replaced by a list, and the list by a setting:

- `nameDeclaresArchive` is gone. In its place `PanelEnterAllowed` asks
  `ArchiveEnterExcludeMask` -- a far2l file mask under `[Panel]`, so `|` still
  carves an exception out of it -- whether this name keeps Enter for its
  association. A name nobody listed is left to its content, which is how both
  ancestors treat everything off their own lists.
- The default is far2l's `KnownDocumentTypes` verbatim, plus `*.epub`, which
  this issue named and far2l's list does not. Every document the issue
  reported behaves exactly as it did.
- `.jar`, `.apk` and the other packages now open on Enter, as they do in Far
  and in far2l. Anyone who wants them back names them in the mask.

Far keeps its list in a setting for the same reason (`use_include_masks` and
friends), and it is the half worth copying: whether Enter should browse a
`.whl` or launch a `.cbz` is a preference, and a preference belongs in
`f4.ini` rather than in an argument about defaults.

Tests: `TestIssue1184PackagesFollowTheirContent` pins the packages and shows
the mask putting one of them back; `TestIssue1184EnterExcludeMask` replaces
the `nameDeclaresArchive` table, covering the default list, an empty mask and
the `|` exception. `TestIssue1184DocumentsKeepEnterForAssociation` is
unchanged apart from losing `lib.jar` and gaining a mixed-case name.
