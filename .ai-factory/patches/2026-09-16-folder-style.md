# Restore original folder collage presentation

The external-catalog folder card used a transparent interior and left-aligned,
single-line caption instead of the original ZoinGallery presentation.

Compared against standalone BrickDelegate.qml and ZGStyle/Style.qml. Restored
the filled blue folder shape, opaque interior, spacing and centered two-line
caption with right elision. Kept folder interaction and physical-pixel snapping.

Regression first failed with horizontalAlignment 1, expected 4. The corrected
test covers fill and frame colors, wrapping, grid/caption separation and leaf
scene origins/unit transforms at DPR 1.75. Inspected the captured raster.
Full Masonry modes and DPR suites, embedded-only QML checks, Windows import
audit and embedded payload/preview broker tests passed.

When porting a visual feature, compare its complete original delegate and style
constants, including fill layers and caption geometry, before simplifying it.
