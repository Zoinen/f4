# File context menus

Use **Files → File Context Menu…**, the command palette, or **Apps** in a
file panel. The shortcut is configurable and is not claimed in editors,
dialogs or embedded terminal applications. Right-click and Shift+F10 retain
their existing meanings.

Keyboard invocation anchors the popup below the focused panel row, accounting
for scrolling and multi-column views. The anchor is captured before background
work starts. Native popups receive desktop coordinates from the host window;
when the host cannot reliably expose them (for example, a terminal emulator),
F4 shows its own menu at the same cell position. Menus fit within the screen.
The explicit-position entry point accepts mouse cell and desktop coordinates
for future mouse invocation; right-click behavior is unchanged.
Windows maps the cell anchor through the rendered client area, so zoom and
display scaling do not reuse nominal font dimensions. Pending native requests
draw no loading dialog; Escape still cancels them.

Windows uses the classic shell context menu, including shell extensions.
The helper initializes Common Controls v6 visual styles and follows the Windows
application light/dark preference. High-contrast mode keeps its system colors.
Dark popup support uses optional, version-checked Windows theme exports; when
unavailable, the menu keeps the normal system visual style. This is Explorer's
classic **Show more options** menu, not the compact Windows 11 presentation.
macOS recreates common Finder actions in a native Cocoa menu; Finder
extensions, Tags and Share are not included. Linux uses a themed F4 menu,
with Open With discovery through GIO when a desktop session provides it.
F4's menu is also the fallback when native presentation is unavailable.

Selections are captured before opening the menu. If the source VFS,
directory or selection changes, a pending F4 action is discarded. Marked
items take precedence; the parent entry is never targeted. Unsupported
actions are disabled in Cocoa and omitted from F4's menu.

Remote and archive panels can use mounts owned by this F4 process when the
mount has the same provider session. Cross-process mount records are not
trusted as proof of identity; browse such mounts by their local path.
Without a matching mount, F4 actions operate directly on the VFS originals.
The command creates neither temporary file copies nor mounts. Read-only
mounts use only nonmutating actions. Trash never falls back to permanent
deletion.

Native menus run in a private mode of the same executable, communicating
over pipes. They do not initialize panels or embedded shells. Windows reuses
one helper and its OLE thread, owner window and loaded shell extensions.
After a local selection settles for 150 ms, F4 prepares its shell menu in
the background. Pressing Apps reuses that menu after checking file identity,
size, modification time and attributes; invocation checks the targets again.
Changing the selection or the files rebuilds the menu, and invoking an action
discards it. Rapid navigation coalesces pending preparation requests. Virtual
paths are resolved only on an explicit menu request, using the mount checks
described above.

Prepared menus avoid process startup and shell-menu construction on the input
path. The Windows popup also skips its opening animation. A newly selected
item can still need construction if Apps is pressed
before preparation finishes. Shell extensions control that construction cost.
The helper continues processing native messages while idle, so Properties
and other asynchronous shell UI remain usable. Application exit closes the
helper; cancelling a blocked request terminates it. A crashed helper is
replaced on the next request. A native command with an uncertain outcome is
never automatically replayed through the fallback. macOS uses a helper per
menu request.

Native popups follow OS appearance; F4 popups use the active F4 palette.
On a headless system, use the built-in View/Edit/file-operation actions;
desktop opening is only available when the relevant desktop service exists.
