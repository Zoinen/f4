# Settings content scrollbar gutter

The attached vertical ScrollBar was parented to the clipped native settings
Flickable, so its default right alignment overlaid the last 16 logical pixels of
content while leaving the dialog's 24-pixel right padding unused.

Regression before correction at DPR 1.75: viewport right=1883 physical pixels,
scrollbar left=1855, a 28-pixel overlap. The new check failed before the fix.

Keep the ScrollBar attached for scrolling behavior, but visually parent it to
SettingsDialogBody and position it in the actual gutter supplied by GenericDialog.
Leave four logical pixels to the dialog edge and preserve the viewport width.
Tie visibility explicitly to the native viewport now that it has a different parent.

After correction: viewport right=1883, scrollbar left=1890 physical pixels.
Regression checks non-overlap, right-edge clearance, scrollbar and handle physical
origins/dimensions, plus the existing leaf unit transforms and category interactions.
175% GUI and Gallery screenshots inspected. Native settings and category CTests,
resource-only QML import, compiled-host startup, and Windows import audit pass.

Prevention: attached scroll controls intended for a gutter must live outside the
clipped viewport; test scene-space bounds against both content and dialog edges.

Delivery: regenerated embedded payload, passed targeted Go payload tests, rebuilt and launched only this worktree's f4-zoin.exe --gui=qt. Verified the running extracted host matches the newly built host.
