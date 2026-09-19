# Drive-menu text centering and filesystem width

The menu's ordinary labels used a full-row Text with AlignVCenter, while detail
labels used natural text height. Center the font's common cap/descender ink sample
(Ag) on the row and retain one baseline for labels and drive detail columns.
Dropdowns and section headers keep their existing layout. Pixel-snap the final
leaf origins and preserve identity transforms.

Before correction the default-font regression measured text center 174.25 versus
row center 172.5 physical pixels. NTFS measured 31 logical pixels in Text but was
allocated only 29.1429 by FontMetrics plus nearest-pixel rounding. The NTFS
regression failed before implementation. Measure filesystem and styled capacity
text with the same Text renderer used by the row and round widths outward. Reduce
the capacity/filesystem gap from 24 to 12 logical pixels, including preferred menu
width and its edge insets. The rendered capacity string was also elided before
switching its measurement to Text; regression now requires it to fit.

Coverage: Segoe UI, selected and plain labels, disabled icons, actual drive rows,
filesystem/capacity no-elision, column gap, all affected text/image scene origins
and identity unit vectors at DPR 1.75, and a rendered menu capture. Existing nested
menu and static QML startup tests remain part of verification.

Prevention: do not substitute FontMetrics advance for a QML Text's actual implicit
width when allocating columns; reserve outward-rounded geometry. Center a common
font ink sample rather than relying on full-row line-box alignment or a fixed
pixel nudge.

Verified: 3 focused CTests, static resource/compiled-host startup, Windows import audit, embedded payload Go tests. Inspected 175% capture. Rebuilt and launched only this worktree's f4-zoin.exe --gui=qt; running extracted host hash matches the new static host.
