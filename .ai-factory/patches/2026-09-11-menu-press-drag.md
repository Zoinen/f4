# Menu bar press/drag/release

The QML bar opened from onClicked (release). It now opens onPressed and keeps
its mouse grab while routing window-space movement/release to the topmost live
menu. Menu item pointer selection and activation share the ordinary click path;
submenu hover timers follow the grabbed pointer. Disabled items and separators
never activate; releasing on the bar leaves its popup open. No layout changes.

Regression: menuBarPressDragReleaseActivatesItem, five cases at DPR 1.75.
Original release handler fails before release with no menuBar.toggle action;
fixed version selects and activates exactly once. Related menu/overlay tests,
static QML loading and import audit pass. Built and launched the embedded static
Qt frontend from the worktree binary.
