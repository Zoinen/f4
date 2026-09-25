# File fields in the Qt panel

File fields add typed metadata to a panel entry without changing `VFSItem` or
the file-list plugin API. Descriptors are shared by the semantic panel model,
Details columns, sorting, filtering, and grouping. This first implementation
publishes EXIF fields in the Qt frontend; it does not read metadata in the TUI
or run a second scanner.

## EXIF fields

The descriptor registry currently contains:

| Key | Type | Display |
| --- | --- | --- |
| `exif.exposure_time` | number, seconds | `1/125 с` or `0.5 с` |
| `exif.iso` | integer | `800` |
| `exif.f_number` | number | `f/2.8` |
| `exif.focal_length_35mm` | number, millimetres | `75 мм` |
| `exif.lens_focal_range` | range, millimetres | `24–70 мм` (fixed lenses display one value) |
| `exif.camera_model` | text | camera make and model when supplied |
| `exif.lens_model` | text | lens make and model when supplied |

Values come from decoder metadata. Missing focal-equivalent and focal-range
tags remain absent; neither is inferred from a model string. Invalid,
non-finite, zero, or incomplete numeric values are omitted.

Each value has one of three states: unread, read with no value, or known. An
image dimension, thumbnail, or preview does not imply that the EXIF pass has
finished. A completed decoder pass marks every absent descriptor as missing.
Temporary source-access failures remain unread so that the existing metadata
retry policy can run.

## Data path and identity

ZoinGallery extracts fields in the existing image-metadata pass used by the
gallery. LibRaw remains the primary RAW decoder and exposes the standard
`FocalLengthIn35mmFilm` tag on its lens-info structure. We read that parsed
value from the existing decoder pass; no second file open or metadata walk is
needed. It never derives an equivalent from actual focal length or a lens
name. Results are
stored alongside that pass in the versioned metadata cache
(`image-metadata-v3`). Cache hits, including batched hits, restore both typed
values and read completion. File-field values live beside panel entries;
they do not modify VFS records or attributes sent to plugins.

The Qt host advertises `panelFileFieldsV1`. When negotiated, ZoinGallery sends
`panel.fileFields.update` or groups cached thumbnail results in one
`panel.fileFields.updateBatch` action. Each result carries the source key,
content version, directory-reading generation, completion, and typed values;
the batch carries the panel ID once. Go applies a batch before rebuilding the
panel projection. It accepts results only when the panel and identity fields
still match the current entry. Presentation
changes such as sort, filter, and group do not change the content generation;
navigation and content changes invalidate delayed results. Identical results
do not trigger another panel projection. Existing Go-to-Qt file attributes
remain on their separate stream.

## Panel behavior

The View menu's **Columns…** editor controls Details columns and order. Added
file-field columns start hidden. Drag any boundary in the Details header to
resize its two neighbouring columns; relative widths are saved with the
per-side gallery session. The sort menu and
Details headers use the same descriptors. Numeric fields compare numerically,
text fields use panel text comparison, and focal ranges compare by minimum and
then maximum. Equal values use file name as a stable tie-breaker. Known values
sort before unread and missing values in both directions.

The **Filter…** editor supports flat conditions combined with **All** or
**Any**. Numeric fields support comparisons; text supports equality and
substring search; ranges support equality and focal-length inclusion. Every
field supports **has value** and **has no value**. Exposure accepts a fraction
or a number of seconds. An unread field is indeterminate: it remains visible
unless another known condition proves the row does not match. The panel shows
the number of files still awaiting metadata, and the filter is reevaluated as
the existing gallery metadata pass completes. Name search continues to work
alongside field filters, and parent-directory navigation remains available.

File-field sort, filter, group, and Details-column choices are part of the
per-side gallery session state. Old sessions without these keys load with
default file-field settings.

Grouping publishes file-only ranges from Go. The gallery places one header
before each range in Details, Columns, Grid, Icons, and Masonry. Grid and icon
groups start on a new row; column groups occupy a separate horizontal block.
Headers are supplementary visuals, so they do not have model indexes or take
part in selection, keyboard navigation, drag hit testing, or thumbnail
requests. Only headers intersecting the viewport have QML visuals. The file
geometry includes the header space, so scrolling and page navigation continue
to use file indexes while skipping the header area.

## Pixel geometry and scrolling

`MasonryLayout` owns physical-pixel alignment. It observes the host ancestor
positions once per panel, snaps the scrolling viewport in scene coordinates,
and rounds each brick's shared edges using the window DPR. The analytical
density and content extent retain their precision: rounding a row pitch first
would accumulate an error over large catalogs. Hit testing uses the painted
edges and the actual viewport translation. Group headers share that viewport;
the fixed Details header receives its parent-origin correction from C++.

Details text and compact previews only round local extents, insets, and
centering. They do not observe `contentY` or walk the host ancestor chain for
each cell. Scrolling therefore moves the common viewport without reevaluating
every EXIF label's pixel correction. DPR 1.75 tests cover fractional host
placement, scrolling, resizing, text and raster leaves, and unit transforms.
The opt-in `detailsScrollCpuProfile` test (`F4_DETAILS_SCROLL_PROFILE=1`)
measures the synchronous layout update for 96 entries and seven extra columns;
it deliberately excludes file reads and frame rendering.

For EXIF grouping, unread files and files whose completed read has no value
form separate groups. Grouping and filtering consume the values already
delivered by the thumbnail metadata pass; they do not start a second read.
