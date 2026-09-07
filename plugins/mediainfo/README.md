# MediaInfo plugin

This package implements f4's built-in, metadata-oriented MediaInfo plugin in
pure Go. It reads through `vfs.VFS`, so local and remote-panel files follow the
same path and no temporary OS copy or native MediaInfo library is required.

## Entry points

- **F11 → Media information** analyzes the item under the active-panel cursor
  and opens a resizable report window.
- **Ctrl+Q** invokes the registered fast Quick View provider for supported
  audio/video containers. Images and text subtitles retain f4's native preview
  precedence.
- **`MediaInfo:path`** analyzes an absolute or active-VFS-relative path. The
  prefix is configurable and an empty prefix disables it.
- **F4** in a report opens a collision-safe, unsaved
  `<name>.MediaInfo.txt` buffer in the editor. On a VFS that cannot guarantee
  an atomic no-replace rename, the first save fails safely instead of risking
  an overwrite; the report remains in the editor so its content can be copied.
- **`Plugin.Call("f4.mediainfo", ...)`** exposes the Far-compatible macro
  result `(ok, count, keys, values)`. The original plugin GUID
  `919C1FC6-A571-4642-99DF-BDACE840ED18` is also accepted.

Configuration is stored at `<f4-config>/plugins/mediainfo.json`. It controls
menu visibility, Quick View, editor-first reports, command prefix, English or
Russian report labels, and a custom Inform template.

## Backend coverage

The analyzer currently covers:

- MP4/MOV/M4A/3GP/3G2 and fragmented ISO base media;
- Matroska and WebM;
- AVI, WAVE, RF64/BW64, and AIFF/AIFC;
- MPEG audio/MP3, FLAC, and Ogg Vorbis/Opus/FLAC/Theora;
- JPEG, PNG/APNG, GIF, BMP, TIFF and common TIFF-based RAW files, WebP,
  HEIF/HEIC, and AVIF;
- SRT, WebVTT, ASS/SSA, TTML, MicroDVD, and binary EBU STL subtitles.

The parser is deliberately bounded by context, read, operation, element,
stream, tag, chapter, value, cache-weight, and rendered-output limits. Fast
mode has smaller limits for interactive Quick View; detailed mode trades more
I/O for richer reports. Unsupported or partially decoded metadata is reported
without turning the package into a media decoder.

Transport streams and disc structures, FLV, ASF/WMV/WMA, RealMedia,
MXF/GXF/LXF, DCP/IMF, VobSub, and PGS are outside the first implementation.
BigTIFF, exact HEIF primary-item association, and multiplexed or chained Ogg
are also not fully decoded.

## Templates and macros

The Inform subset supports `Section;template`, `%Field%`, optional `[ ... ]`
groups, `\n`/`\r`/`\t`, and `Section_Begin`, `Section_Middle`, and
`Section_End`. A Go `text/template` is selected when the source contains
`{{`. Template source and generated output are bounded during execution.

Macro arguments follow the original plugin conventions: booleans or integers
select technical versus image fields, `{Key,Key}` is an exact key filter, and
the last non-empty string is the requested path. Nested string slices are
accepted as an additional exact-filter form. Keys are canonical English IDs;
order and duplicate pairs are preserved.

Run `go test ./plugins/mediainfo` and `go vet ./plugins/mediainfo` for the
focused backend and integration checks.
