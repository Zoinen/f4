# File fields in the Qt panel

File fields add typed metadata to a panel entry without changing `VFSItem` or
the file-list plugin API. Descriptors are shared by the semantic panel model,
Details columns, sorting, filtering, and grouping. The implementation
publishes EXIF and video fields in the Qt frontend; it does not read metadata in the TUI
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

## Video fields

| Key | Display |
| --- | --- |
| `media.duration` | `4.48 s` below one minute, otherwise `01:01:01.125` |
| `media.bitrate` | overall average bitrate, e.g. `1.6 Mb/s` |
| `media.format` | container format |
| `video.codec` | video codec |
| `video.bitrate` | video stream bitrate |
| `video.resolution` | `1920×1080` |
| `video.frame_rate` | `29.97 fps` |
| `audio.codec` | audio codec |
| `audio.bitrate` | audio stream bitrate |

The thumbnail worker reads these values from the same media player used to
capture video frames. Track fields describe the active video and audio tracks
(or the first track when none is selected). Overall bitrate is calculated from
file size and duration, including container overhead; stream bitrates come from
track metadata. Unsupported, missing, zero, or non-finite values stay blank.

Video metadata uses the `video-metadata-v2` cache schema. A cache hit restores
values and completion without opening the source. If cached thumbnail pixels
survive but metadata is absent, the same worker loads metadata and reuses those
pixels. Source-access failures remain unread. Completed fields are retained for
the same source version when a delayed thumbnail or placeholder carries an
incomplete read. Cached video pixels restore native dimensions and completed fields before
publication; their preview dimensions never replace completed native metadata.
Identical metadata does not invalidate settled thumbnail geometry.

Video presentation combines the stream's surface rotation and mirroring with
the frame's presentation transform. Posters crop the visible viewport after
mapping it through the surface transform already applied by Qt's image
conversion. Display dimensions retain the resulting orientation, including
metadata-only refreshes. The video thumbnail and poster cache revisions exclude
older artifacts that omitted stream rotation or cropped rotated frames into
squares.

HLG and PQ video are converted to the same 100-nit SDR presentation in the
viewer and cached previews, including BT.2020 to sRGB conversion. The video
shader applies tone mapping before filtering, also when reusing its linear
conversion texture. Opaque P010/P016 frames stay opaque in previews: Qt's GPU
image conversion can otherwise place tone-mapping luminance in their alpha
channel. CPU image-conversion fallback also applies HDR-to-SDR processing.

On Windows, HDR video also offers an explicit HDR button when the current
display supports HDR. It opens a separate fullscreen FP16 scRGB surface;
the main window and ordinary playback retain their SDR swapchain and shader.
The HDR shader decodes PQ to absolute luminance and HLG using the 1000-nit
reference OOTF, preserving BT.2020 gamut in linear scRGB (1.0 = 80 nits).
FFmpeg's original video planes remain on the GPU; no SDR image conversion or
eight-bit intermediate is used for that output. Windows HDR is enabled only
on request. Closing that surface, leaving fullscreen, or navigating to another
file restores the previous display setting if F4 changed it. An already
enabled system HDR setting is preserved. Cached posters remain SDR.

The video controls fade out after three seconds without mouse movement in the
viewer and fade back in on movement. Hovering the controls or dragging a slider
keeps them visible. Leaving or deactivating the window cancels hiding. This
behavior is shared by the ordinary viewer and the fullscreen HDR surface.
Seek and volume hit areas span the full control-panel height while their
visible tracks and handles stay centered.

The Qt panel's disk cache keeps one WebP thumbnail per source revision and
thumbnail transform. It replaces that entry when more pixels are available,
up to 1024 pixels on the long edge. A frame at that limit satisfies subsequent
panel requests of any size. Smaller requests reuse any stored frame that
covers their fitted bounds. Older exact-size entries remain readable until
ordinary eviction; resizing never creates another exact-size entry. The
viewer's Fit frames and first-frame video posters are separate artifacts.

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

Grouping publishes file-only ranges from Go through the shared paged group
transport (`panelGroupingV1`), including EXIF groups. Collapse state is keyed
by the selected field as well as the directory and grouping options. The gallery places one header
before each range in Details, Columns, Grid, Icons, and Masonry. Grid and icon
groups start on a new row; column groups occupy a separate horizontal block.
Headers are supplementary visuals, so they do not have model indexes or take
part in selection, keyboard navigation, drag hit testing, or thumbnail
requests. Only headers intersecting the viewport have QML visuals. The file
geometry includes the header space, so scrolling and page navigation continue
to use file indexes while skipping the header area.

## Calculated directory sizes

F3 records every completed directory total in a panel-owned in-memory hash map,
qualified by VFS/session identity and absolute path. Each value holds payload
and directory-entry bytes, so the `DirSizeCountsFolders` preference still applies.
Parent paths are stored once per VFS/session and directory; child totals are
indexed by their existing names. Restoring a listing normalizes its parent once
outside the cache lock, then performs name lookups without allocating full child
paths. A miss for the parent returns without walking the listing.
The cache retains no file rows or second directory listing. Cancelled,
inaccessible and unknown-size subtrees are not published. F3 uses one generic
walk; remote FastScanner backends cannot return these per-directory results.

Navigation restores these hints with `~`, including during bounded, phased and
cached directory loads. Phased filesystem metadata preserves the restored total.
A fresh F3, including a multi-folder scan, replaces the total and clears `~`.
Metadata revisions include the approximation flag, so an unchanged byte count
still updates the visible label. Both immutable snapshots and live paged metadata
preserve the flag; Qt displays the authoritative `sizeText` without reformatting.

GUI entry sizes use spaced groups of three digits and whole binary units. The
complete string, including `~` and the suffix, is at most 11 characters. For
example: `999 999 999`, `976 562 K`, `9 999 999 K`, then `9 765 M`.
Further units are G/T/P/E. Unit changes truncate fractional digits. Unknown file
sizes and unmeasured directory sizes retain their existing blank/type labels.

Qt retains the reported visible metadata range across metadata-only revisions
and cursor prioritization. The range is qualified by path and catalog revision;
a new directory uses its own viewport. Visible sizes continue loading in bounded
eight-row asynchronous chunks after the first frame, pausing during input.
Offscreen rows stay dormant until scrolling brings them into view.

## Pixel geometry and scrolling

The semantic column schema marks default widths with `autoWidth`. Qt measures
those columns with the current panel font; name uses the remaining space and size
reserves three groups of three digits plus a unit. Measurements update on font
family/size, schema, padding or viewport changes, not per entry. Explicitly sized
columns keep their relative widths. Header and row boundaries share the result.

The GUI settings' **Column padding** slider sets a shared horizontal inset
(0–24 logical pixels, default 8) for Details and two/three-column views.
Names, separate extensions, sizes, metadata cells and Details headers use the
same per-edge padding. Column boundaries do not move when padding changes;
only their content area changes. Preview is immediate, Apply saves the GUI
preference, and Cancel restores the previous value.

`MasonryLayout` owns physical-pixel alignment. It observes the host ancestor
positions once per panel, snaps the scrolling viewport in scene coordinates,
and rounds each brick's shared edges using the window DPR. The analytical
density and content extent retain their precision: rounding a row pitch first
would accumulate an error over large catalogs. Hit testing uses the painted
edges and the actual viewport translation. The fixed Details header receives
its parent-origin correction from C++. Group overlays use the shared grouping
layout, including its sticky headers and collapse/navigation behavior.

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
