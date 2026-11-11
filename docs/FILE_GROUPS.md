[← Highlighting](HIGHLIGHTING.md) · [Back to README](../README.md)

# File groups

Open **Options → Settings → File groups**. The editor is implemented in Go and
works in a terminal. A graphical frontend may contribute icon catalogs and show
image previews without owning the configuration or command execution.

Each group has classification conditions and four independent features. File
colour themes and icon themes are separate name-based appearance maps; neither
creates another matching rule or another row for the same group:

| Feature | Resolution |
| --- | --- |
| Highlighting | Existing four-state colour cascade, continuation and `UseDefaults` |
| Icons | First matching group with named artwork in the catalog, or an explicit override |
| Sorting | First enabled matching sort group; unclassified files use `10000` |
| Associations | Enabled commands for the invoked action, in group and command order |

An icon-only group cannot stop another group's highlighting or associations.
Directories and the parent row keep their navigation behaviour; association
commands do not intercept them. Directory-first sorting is unchanged.

## Editing

The main page is a full-height table: live sample/icon, group name, conditions
and enabled features. All groups are ordinary editable records: there are no
Built-in, Imported or User labels, source columns or source dividers.
Universal and active-theme rules form one list without divider rows.
Shipped groups are shared across icon themes; legacy user-scoped sets remain
editable for compatibility. Archives, Executables and Temporary files each have
one canonical record. Colour themes supply colours by group name; inactive
themes do not appear as repeated rows.
The headers do not sort. Display order is reversed so general file/folder rules
appear first; evaluation and saved priority order are unchanged. Up/Down moves
the selected row in the displayed direction. Type to search the list.
Space/Insert mark preview rows, independently of the real panels' selection;
cursor and marked states use the same four-state colour cascade as panels.

In the group editor, each icon catalog offers **Automatic** (use the group
artwork or generic override), **No icon**, and named icon choices. The Qt
editor previews the effective choice and shows artwork next to each choice.

The **Icons** selector changes only the editing/preview icon context. It does **not**
change the application's saved icon theme, real panel icons or association context.
The independent **Colours** selector previews file colours in the table and open
panels. Apply saves that selection; Cancel restores the last applied selection.
Neither selector changes the application's window/menu colour style.
Drafts for hidden sets remain in memory. Apply validates and saves the entire
document, including those sets.

The legacy **Group scope** field applies to all features. Universal groups always run
first, followed by the selected theme's groups in their own order. An association
in a universal group survives every icon-theme switch; a thematic association
does not run outside its theme. Colour-style priority, four independent colour
states, attribute overrides and `UseDefaults` are unchanged.

Add always creates a universal group, independently of the preview icon theme.
The scope control is retained only for existing scoped user records.
Duplicate retains scope. Changing scope moves the group to the destination's
tail; new thematic overrides precede the shipped fallback rules. Up/Down moves
within one scope only. Cancel rolls back group edits, never the application's
current icon theme.

Enter, F4, double-click or Edit opens a separate group editor with one scrolling
page and no category sidebar. Sections appear in this order: Matching conditions,
Associations, Visual appearance, Icons, and Sorting.
Feature checkboxes sit in their section headings. Related controls appear in
group boxes below, and disabled controls retain their values.
In Qt, the group name and matching controls are in the left column. The right
column combines appearance controls with a four-row preview of Normal, Marked,
Cursor, and Marked cursor. The sample uses the full rule cascade and base
palette. Each row's Edit button opens a picker for independent text and
background colors. Inherit leaves that component to the highlighting cascade.
Change icon opens the controls for all icon catalogs. The console keeps its
pinned preview and text-based color fields.
Invalid edits report an error and retain the last valid sample;
the preview does not execute associations or compile masks while painting.
The Group name checkbox enables the group and sits above the name editor.
The name editor comes before Matching conditions. Name,
Attributes, Size, and Date and time have checkboxes in their group
headings that enable editing. Name is editable by default, as are
groups with existing restrictions. Unchecked groups remain visible and keep
their saved values and matching behavior. Mask syntax and case matching use
single-selection button groups in the console and Qt. File name masks use a
multiline field. Qt shows comma-delimited chips while the field is unfocused
and wraps the original text while it is being edited. The matching buttons are below the masks. In Qt, Attributes, Size, and Date
and time follow Name in the left column. Narrow dialogs stack the columns.

The sections expose masks/dialect/case behaviour, required and
excluded attributes, byte-size bounds, timestamp choice, absolute or relative
dates, all four colour states, marker, continuation, `UseDefaults`, theme icon
references, native/plugin priority, sort number and associations.

The colour matrix edits the selected colour theme's entry for the group name,
not the group itself. Renaming a group selects the new name's entry; it does not
copy or delete the previous entry. **Highlight conditions** exposes a matching
condition separately from classification. For example, executable extensions
select artwork, while highlighting can match any file with the Executable
attribute, including extensionless programs. Neither rule is silently widened.
The overview mentions a differing highlighting condition in its Conditions cell.
Group order determines the cascade independently of the selected colour theme.

Associations has separate command lists for Open (Enter), View (F3), Edit (F4),
and their alternate actions. Add, name, enable, and reorder commands within
each action. Every command belongs to the file group being edited. One match
runs directly. When several enabled commands match across file groups, the
chooser shows each command and its source group. Commands support multiple
lines and the existing Far substitutions.

Child OK merges edits into the Settings draft, without saving a file. Child
Cancel/Escape discards only that child's changes. Cancelling a newly added group
also removes its provisional record. Settings Apply saves; Settings Cancel
discards remaining unapplied edits. Delete removes the group and its command
sets, regardless of where it originated. It stays deleted after Apply and
restart. Up/Down reorder only within the same scope.

Use Ctrl+Backspace on a field or More → Reset group to
copy the corresponding standard values. **Restore standard set**
replaces the entire group list, order and command sets after confirmation.
It changes the draft only: Apply saves it and Cancel restores the committed
working set. An empty edited value is an explicit clear, not a reset.
Unknown keys and unavailable theme references remain in the document rather
than being discarded.

The user file includes a marked, generated help block describing the current
format, colours, attribute precedence and commands. Applying edited File groups
refreshes only that block, in the same atomic save as the groups. Startup, preview and
Cancel never update the file. Personal comments, unmarked legacy documentation
and unknown sections are retained; malformed help markers stop Apply rather
than risk deleting personal text.

Colour fields are editable expressions, including `F_WHITE | B_BLUE`, palette
references such as `foreground:Panel.Text`, and RGB expressions such as
`foreground:#AABBCC | background:#112233`. The four states are independent.

More → Sample opens the selected row's sample filename and contributing-rule
explanation. The overview evaluates against the complete ordered active set,
not merely the row's own colour or icon fields. A filename
alone has no reliable size/timestamp/permission metadata, so metadata-dependent
conditions remain deferred. Testing a match never executes a command.
Use **More → Use panel entry** to evaluate the selected panel item's cached
metadata without a filesystem read.

More → Themes creates, duplicates, renames or disables rule sets. The application's
active set cannot be disabled here. These sets are separate from artwork
catalogs: several sets can reuse Zoin artwork, or a colour-only set can have no
catalog. Unknown sets and catalog IDs are preserved.

Frontend defaults may declare `LegacyIconPrefix` on a theme descriptor. Imported
legacy rules' duplicated `Icon`/`Icon.*` references under that prefix become
automatic group-named artwork, so bundled defaults cannot override other themes.
Colours and commands remain universal; custom icon overrides are preserved.
This repair is read-only until Apply and backs up an already unified document
to `highlight.ini.pre-filegroups-icon-scopes` before saving.

**More → Themes → Properties → Tint icons with file colour** controls whether
icons use file-group colours rather than the artwork's original colours.
Panel icons retain their normal foreground when the cursor moves onto them;
persistent marking remains a separate state. There is no blanket opacity
reduction for graphical panel icons (hidden-entry dimming still applies).
Shipped Lucide and Zoin sets enable it; disable it to retain original artwork
colours. Native icons always retain their original colours.

## Standard set and working set

The application retains a read-only standard set as a starting point and an
explicit reset source:

1. Embedded, read-only `internal/filegroups/highlight.default.ini`, existing
   colour-style INI contributions, and optional frontend catalog/default data.
2. Profile-local `highlight.ini`, containing the complete editable working set
   after Apply (`Mode = snapshot`).

Without a saved working set, the editor starts from the standard set. Once
saved, its records, fields and order stand on their own. Application updates do
not silently reinsert deleted groups or replace edited values. Restore standard
set explicitly copies the current shipped set, including newly shipped groups.
Older sparse Version 2/3 files still load with their original inheritance rules;
the first Apply materializes their effective list as a snapshot.

The current standard set has 46 groups. The Current colour theme includes the
former development-profile highlights; symbols, projects and libraries have
shared groups, while additional archive/media/source masks extend existing ones.

The shipped Qt contribution is `qt/host/icons/filegroups.default.ini`. It defines
Lucide, Zoin and Native artwork catalogs, without any matching rules. Shared
conditions live in `internal/filegroups/highlight.default.ini`. Artwork and rendering
remain frontend resources; toolbar/menu icons are not file-group policy.

### Group-named artwork

A theme's icon keys are normalized group names: lowercase letters/numbers with
punctuation and spaces replaced by hyphens. `Archives` selects `archives.svg`;
`Go source` selects `go-source.svg`. Matching is still controlled by the group's
conditions, not by the icon theme. Rename a group to select different artwork.

Catalog membership and predicates are compiled with the document, never during
painting. Missing artwork falls through to another applicable group and finally
the selected catalog's file/folder/parent fallback. Explicit icon overrides,
including `none`, remain supported; Native/plugin priority is unchanged.

Every shipped catalog asset has an ordinary shared group. The old `qt.zoin.*`
and `qt.lucide.*` factory aliases migrate through the explicit `LegacyIDs` table.
Unmodified aliases fold into shared groups; edited conditions/commands remain
independent user records. Reading and preview do not write the profile. Apply
backs up a repaired Version-3 file as `highlight.ini.pre-filegroups-group-icons`.

Zoin resources have group-named SVG files. Lucide's prepared stroke variants
expose the same group-named resource paths; original artwork inputs and old URLs
remain available to preserve overrides and toolbar/menu rendering. See
[`qt/host/icons/GROUP_ASSETS.md`](../qt/host/icons/GROUP_ASSETS.md).

### Independent file colour themes

`internal/filegroups/colors.default.ini` supplies two file colour themes:

- **Current**: the existing group colours, including all four colour states.
  An existing profile's active colours are captured in memory on migration,
  preserving its appearance until Apply writes the new representation.
- **Zoin**: the base RGB hue extracted from each of the 30 shipped Zoin SVGs.
  Opacity changes do not create a second hue. Only normal foregrounds are set;
  cursor/marked states retain the normal panel selection treatment.

The selected theme is `FileGroups.ColorTheme`. Entries use the same normalized
group names as artwork, not group IDs or masks:

```ini
[FileColorTheme.zoin]
Name = "Zoin"
Color.archives.NormalColor = "foreground:#F79F57"
Color.images.NormalColor = "foreground:#F74175"
```

Each name can define `NormalColor`, `SelectedColor`, `CursorColor` and
`SelectedCursorColor`. Empty values explicitly clear a state. Missing entries
allow later matching groups to supply colours. Markers, highlighting enablement,
continuation, `UseDefaults`, conditions, sorting and commands remain group-owned.
Icon selection and file-colour selection are independent. The existing UI
`ColorStyle` still controls window/menu colours, not the file-colour theme.

Apply atomically saves both themes and the selected ID alongside the groups.
Unknown fields and unavailable themes survive. Switching colours shares compiled
predicates; it does not reload directories, recompile masks or alter associations.
The first conversion of an existing unified profile is backed up as
`highlight.ini.pre-filegroups-colors`. Startup and Cancel do not write it.

### Legacy colour-scheme migration

Built-in style INIs use `FileGroup = type.archives` (or another stable group ID)
on their existing Highlight sections. The Go adapter attaches those fields to
the canonical group under `Palette.<hex UTF-8 style name>.<field>`:

```ini
[Group.type.executables]
Palette.4d6f6465726e.NormalColor = "foreground:#72EE48"
Palette.4d6f6465726e.IncludeAttributes = "Executable"
Palette.4d6f6465726e.ExcludeAttributes = "Directory"
```

Here `4d6f6465726e` is `Modern`. This is an import representation only: migration
extracts the active variant's colours into Current and retains its condition on
the group. Each legacy variant owns its complete highlighting
condition; missing predicate fields do not borrow unrelated classification
restrictions. Its `Order` retains historical palette order. Universal explicit
highlighting fields remain supported; `Highlight.Mask`, `Highlight.Case` and
the other `Highlight.<condition>` fields override highlighting without changing
icon, sorting or command conditions. All predicates compile with the snapshot.

Old `style.<hash>.Highlight_N` factory identities migrate through declared
`LegacyID` ancestry into these variants. Field edits, explicit clears and unknown
keys survive. Rules extended with commands, icons or other independent features
remain separate rather than having their behaviour silently changed. Saving a
repaired Version-3 document backs up the original as
`highlight.ini.pre-filegroups-palettes`. Startup and Cancel never write it.

Standard groups have stable named IDs; added groups and applications receive
persistent random IDs. IDs never depend on the current row number. A saved
snapshot contains all effective fields and command slots, rather than patches
against a changing factory list.

| Operation | Representation |
| --- | --- |
| Standard starting values | Copied when creating/resetting a working set |
| Explicit clear | Key present with an empty quoted value |
| Disable group | `Enabled = "false"` |
| Disable feature | e.g. `Icons.Enabled = "false"` |
| Delete | Omit the group and its commands from the working set |
| Reset | Copy the standard field, feature, group or whole set |

## Version 3 format

Optional theme-set descriptors retain their separate defaults/overrides layer:

```ini
[FileGroupTheme.photos]
Name = "Photo rules"
IconCatalog = "zoin"
TintIcons = "true"
Enabled = "true"
```

A group's `IconTheme = "photos"` selects its whole-rule scope; `Icon.zoin`
selects an artwork reference. They are not interchangeable. Existing Version 3
files without descriptors materialize referenced theme IDs in memory.

```ini
[FileGroups]
Version = 3
Mode = snapshot
ColorTheme = current
Order = user-images

[Group.user-images]
Name = "Images"
Enabled = "true"
IconTheme = ""
Mask = "*.png;*.jpg|thumb*"
MaskSyntax = "far"
Case = "insensitive"
ExcludeAttributes = "Directory"
Highlight.Enabled = "true"
Icons.Enabled = "true"
Icon.zoin = "asset:images"
OverrideNativeIcon = "false"
Sort.Enabled = "true"
Sort.Number = "20"
Associations.Enabled = "true"
CommandOrder = "editor"

[FileColorTheme.current]
Name = "Current"
Color.images.NormalColor = "foreground:#AABBCC"

[Command.user-images/editor]
Name = "Image editor"
Edit.Enabled = "true"
Edit = "image-editor !.!\n  --reuse-window"
```

An empty or absent `IconTheme` is universal. Other values are stable theme IDs:
`lucide`, `zoin`, `system` (displayed as Native), or `generic` for a pure-Go
frontend without a declared theme. Unknown IDs survive round trips and activate
only when that exact ID is selected. `IconTheme` restricts the whole group;
`Icon.zoin`, `Icon.lucide`, `Icon.system` and `Icon.*` only choose resources.
A universal group can still contain different resources for each theme.

The six command keys are `Execute`, `AltExec`, `View`, `AltView`, `Edit` and
`AltEdit`, each with a corresponding `.Enabled` key. Deleted command sets are
omitted in snapshot mode. Legacy `Disabled = "true"` command overrides remain
readable.

Group and command values written by f4 use Go-style quoted strings. A newline
is encoded as `\n`; a literal Windows backslash is encoded as `\\`. Legacy
unquoted Windows commands are imported literally, never interpreted as escapes.

Whole-line `#`/`;` comments, unknown keys, unknown sections and unknown icon-theme
IDs are retained. There are no trailing comments: a `#` in a value can be a
colour literal. Unsupported document versions and malformed INI are diagnosed
and left untouched.

### Conditions

`MaskSyntax = "far"` supports comma/semicolon include lists, `|` exclusions and
`/regular expressions/`. Imported highlighting/sort rules use `legacy-glob`:
comma-separated `filepath.Match` globs with their original case semantics.
Imported associations retain Far masks and platform-sensitive case matching.
The editor never silently converts one mask dialect into the other.

`Case` accepts `insensitive`, `sensitive` or `platform`. New groups use Far masks.
Attributes are Directory, Hidden, Executable, ReadOnly, System, Archive, Symlink
and Junction. An attribute cannot be both required and excluded. Executable
does not mean a Unix directory's traversal permission.

`SizeAbove`/`SizeBelow` are inclusive byte bounds; zero means unlimited.
`DateType` is Modified, Created or Accessed. Absolute dates use
`YYYY-MM-DD HH:MM:SS` (UTC, as in the legacy parser); relative bounds accept Go
durations or whole days such as `48h`, `30m`, `2d`, with `DateRelative = "true"`.

## Migration and saving

Legacy `[Highlight_N]`, `[SortGroup_N]` and associations entries are imported in
memory, in their original order. Associations initially remain separate groups,
even when masks resemble a highlighting rule. Four colour states, aliases,
markers, cascades, `UseDefaults`, sort numbers and command states are retained.

Opening Settings or previewing does not migrate files on disk. On the first
successful Apply, f4 backs up the original highlighting file as
`highlight.ini.pre-filegroups`, then atomically publishes the new `highlight.ini`.
The old associations file is never changed. A conflicting existing backup is
reported rather than overwritten.

Converting a previously unified sparse document with no other pending migration
uses `highlight.ini.pre-filegroups-snapshot`. Once `Mode = snapshot` is saved,
missing groups mean deletion, not inheritance. Unknown modes are diagnosed
rather than guessed. The standard set remains embedded for explicit reset.

Version 2 documents are migrated in memory and saved as Version 3 only on Apply,
with an exact `highlight.ini.pre-filegroups-v3` backup. Version 2 readers reject
the new version rather than silently executing thematic commands universally.
Factory `LegacyID` declarations explicitly map old factory icon IDs to scoped
replacements; no user ID prefix is interpreted. Added highlighting, sorting or
associations on those factory groups move once to `<old-ID>.universal`, retaining
conditions, command IDs and order. The scoped replacements keep only icons.
Identity collisions produce a diagnostic without overwriting either group.
Ordinary legacy user rules remain universal. Parent/folder/file catch-alls end
each factory thematic set; the neutral embedded set is scoped to `generic`.

Apply validates the document and checks the original file fingerprints. If an
external program changed the file, saving is refused and the draft remains open.
Use **Reload from disk** to explicitly discard the draft and load external edits,
or leave the editor open to preserve the draft. A failed write does not activate
unsaved commands or establish a new baseline.

## Runtime preview

Valid group edits preview after a short debounce. Invalid edits retain the last
valid appearance. Masks and predicates compile on document changes, not while
painting. No directory reload is needed to recolour entries or change sort groups.
Cursor, marked entries and viewport anchors survive a group-driven resort.
Theme changes select cached immutable views sharing the original compiled
predicates. Only an effective sorting change requests a resort. No catalog or
configuration files are read to switch themes.

Associations always use the committed snapshot. Apply saves first, then changes
the committed baseline. Cancel, Escape or closure discards only unapplied preview
changes, including when Apply was used earlier in the same dialog. Preview
ownership prevents one editor's cancellation from cancelling another's preview.
Colour-style changes still own the active palette; file-group rollback does not
restore a captured, stale palette.

Alternate actions `File.AssociationAltExecute`, `File.AssociationAltView` and
`File.AssociationAltEdit` are bindable without default shortcuts. Alt+F3 remains
hex view, and existing Open/View/Edit bindings are unchanged.

## Icon extension contract

References are typed: `asset:<catalog name>`, `file:<path>`, `native:<name>` or
`none`. Legacy custom URLs remain readable. `Icon.<theme-id>` applies to one
theme, while `Icon.*` is generic. Within one group, the specific reference wins;
across groups, the first applicable group wins.

With **Override native/plugin icon** off, supplied plugin artwork wins, and
Native theme artwork wins over a group's choice. With it on, the explicit group
choice wins. OS lookup remains exclusive to the Native theme. Missing assets
fall back through the theme catalog. SVG aspect ratio and device-pixel-ratio
rendering are unchanged.

The optional startup `fileGroupDefaults` contribution uses the same versioned INI
group format plus `[IconTheme.<id>]` descriptors. `Asset.<name>` lists catalog
assets; `Fallback.file`, `Fallback.folder` and `Fallback.parent` declare fallback
references. It is sent once, not with every panel frame.

The optional ExtUI hello field `fileGroupIconTheme` supplies the actual saved
GUI theme before SetupUI/first scene. `fileGroupIconThemeV1` advertises the
extension. Later changes send only `{type: "file_group_icon_theme", iconTheme:
"zoin"}`. They are applied on the Go UI thread; omitting a theme selects
`generic`. Shift+F12 changes presentation, not this application context. Catalogs
and factory contributions are not re-sent. Resource fallback may cross artwork
catalogs, but never activates rules belonging to another theme.

Semantic styles add optional `iconChoices` entries containing `kind`, `value`,
`groupId`, `order` and `overrideNative`, plus explicit `iconSource` provenance.
Clients without this extension can continue using the existing `icon`/`iconKey`
fields. Document headers carry an optional `fileStyle` through the same resolver.
Settings fields may specify `IconTheme`; terminal clients keep an ordinary
editable value, and graphical clients can add a catalog preview.

The optional `fileGroupsOverview` control projects independent Go-owned icon and
colour selectors (`overview-theme` and `overview-color-theme` layout roles), a
table and actions. Table rows carry stable IDs, separator labels, cached
`appearance`, sample filename/directory and preview marking. `previewCatalog`
selects an independent icon resolver; it never changes the application's theme.
Optional `headerControl` renders a real accessible group-title checkbox. The
ordinary child checkbox remains available to older clients. Predicates and
colours are evaluated before painting; QML adds no filename-matching policy.
The optional `fileGroupSample` control carries `previewStyle`, `previewName`,
`previewCatalog` and `previewDirectory` outside the scrollable content. Matrix
edit controls add `colorMatrixCell` and a resolved `colorPreview` patch. These
fields have typed Go transport support; older clients retain the expression
fields and can ignore the additional presentation data.

## See also

- [Highlighting](HIGHLIGHTING.md): legacy colour syntax and cascade details.
- [Keymap](KEYMAP.md): binding alternate association actions.
- [Plugins](PLUGINS.md): filesystem/provider integration.
