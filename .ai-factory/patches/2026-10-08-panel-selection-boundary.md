# Shift-selection at a clamped panel cursor

The GUI selection gesture normally excludes the destination index because it
paints entries being left. Applying that rule to a stationary navigation target
made the first/last item impossible to mark or unmark by continuing past an edge.

Include the destination only for stationary Shift navigation. Keep the normal
moving range unchanged so reversing direction still contracts the gesture.
Columns Page navigation also needs the stationary fallback when the spatial
target is unavailable; masonry Page navigation must not return before selection
when neither the cursor nor viewport can move.

Regression: F4GalleryPointerTests::panelBoundarySelection covers both endpoints,
add/remove intent, repeated arrow keys and Page keys in masonry, grid, details,
and columns. The original implementation failed the endpoint checks; after the
fix all 16 data rows pass at DPR 1.75. The existing pointer/modifier regression
also passes. Live verification in the rebuilt app marked then unmarked the last
entry with Shift+Down, keeping the cursor clamped and restoring the original
selection count. Console navigation already marks its starting entry before
clamping and did not require a change.
