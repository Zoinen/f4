# Shared group artwork resources

Group icon keys are the lowercase display group name with spaces replaced by
hyphens. These resources declare artwork aliases only; they do not classify
files or add theme-specific matching rules.

## Zoin rename and catalog handoff

SVG bytes and `ICON_LICENSES.txt` are preserved. `manifest.json` names the
canonical files. CMake embeds canonical paths and explicit compatibility aliases
for every renamed legacy resource URL.

The shared catalog now maps each old `Asset.<old>` field to
`Asset.<group-key> = qrc:/F4QtHost/icons/zoin/<group-key>.svg` using this table.
Rows with identical names need no field change.

| Old key | Group key |
| --- | --- |
| bash | shell-scripts |
| c | c-source |
| claude | claude |
| cpp | cpp-source |
| css | stylesheets |
| database | databases |
| default | files |
| docker | docker |
| folder-up | parent-folder |
| folder | folders |
| font | fonts |
| git | git |
| go | go-source |
| html | web-pages |
| image | images |
| javascript | javascript |
| json | json |
| markdown | markdown |
| npm | npm |
| python | python |
| ruby | ruby |
| rust | rust |
| svg | vector-images |
| swift | swift |
| table | tables |
| text | text-files |
| typescript | typescript |
| yml | yaml |
| zip | archives |
| react | react |

Fallbacks become `Fallback.file = asset:files`,
`Fallback.folder = asset:folders`, and
`Fallback.parent = asset:parent-folder`. Legacy direct qrc URLs continue to
resolve, but catalog asset-key compatibility is owned by the catalog/resolver.
This preparation does not modify `filegroups.default.ini` or the provider.

## Lucide catalog handoff

The 15 catalog artwork files use group names in all eight stroke directories
(`video` already had its canonical name). The catalog can keep each row as
`Asset.<group-key> = asset:<original-source-basename>` because old QRC URLs
alias the canonical files. The primary prepared resource URL
is `qrc:/F4QtHost/icons/files/<stroke-step>/<group-key>.svg`, with stroke steps
1 through 8. Original artwork URLs remain available for overrides and virtual
panel device/action rows. Menu and toolbar artwork is unchanged.

| Canonical prepared basename | Original Lucide source / legacy QRC basename |
| --- | --- |
| databases | database |
| files | file |
| parent-folder | folder-up |
| folders | folder |
| images | image |
| text-files | file-text |
| archives | file-archive |
| executables | file-terminal |
| audio | music |
| video | video |
| source-code | file-code |
| books | book-open |
| configuration | file-cog |
| locked-files | file-lock |
| temporary-files | file-clock |

Lucide fallbacks use the same three group keys as Zoin. The provider resolves
the catalog's declared raw artwork through compatibility aliases. The declarative
`files/group-names.txt` maps canonical output names to original Lucide sources;
both CMake resource enumeration and `PrepareFileIcons` consume it.
`files/names.txt` remains the original artwork regeneration input. The other 17
prepared chrome/plugin resources retain their original physical filenames.

## Verification

`F4GroupIconResourceTest` checks canonical and legacy Zoin paths, the manifest,
license resource, and every Lucide group alias at all eight stroke widths against
the source bytes. It runs without provider or GUI state and is registered with
CTest. The host and all existing icon test targets embed the same alias resource
helper from `cmake/GroupIconResources.cmake`.
