# Column-view text physical-pixel alignment

Ctrl+2/Ctrl+3 labels inherited fractional scene X positions from the column
stride and host panel placement. At DPR 1.75 the regression measured the first
filename at physical X=66.9375. The shared icons were already aligned.

ColumnsEntryDelegate now corrects filename and extension text through the entire
ancestor chain, uses a natural-height text item for vertical placement, and
aligns the extension extent for right-aligned text. The regression covers two
and three columns, separate extensions, fractional host position/width, visible
text and icon origins and unit transforms, and rendered captures. It is included
in the Gallery visual DPR matrix, including 1.75.

## Real F4 host follow-up: mixed-monitor DPI

The isolated view missed a separate raster-size defect. In the real 3840x2076
F4 window at DPR 1.75, the column icon at physical (33,253) had an aligned
origin and a 28x28 physical extent, but its provider request was 32x32. The
same folder icon in the details panel requested 28x28 and looked different.
New column delegates completed while detached from their window, so Qt used
the maximum desktop DPR (2) and retained that texture after attachment.

GalleryEntryPreview now starts the IconImage request only once Window.window
exists. No image cache or resampling workaround is involved. The real F4
panels now produce pixel-identical folder icons. Live Ctrl+2 measurements
checked 221 text/icon leaves at DPR 1.75 with no fractional origins.

F4GalleryMixedDpiPixelTest runs the real GalleryPanelHost, bridge and icon
provider with simulated 175% and 200% monitors. It checks final leaf positions,
unit transforms and exact rendered icon pixels across attachment, column
switching and resizing. Before the source-loading fix, each sampled icon
differed in 166 pixels from its native 28x28 raster; afterward it matches.
