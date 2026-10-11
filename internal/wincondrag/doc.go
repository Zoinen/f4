// Package wincondrag drags files out of f4 running in a Windows console
// (conhost, OpenConsole, Windows Terminal) into any other Windows
// application: Explorer, a browser upload box, a chat window (issue #1604).
//
// A console program owns no window. The console window belongs to conhost
// or to the terminal, mouse input reaches f4 as console input records, and
// OLE's DoDragDrop run from f4 has no window of its own to capture the mouse
// with: its modal loop never sees a button release and hangs. The field log
// of an earlier attempt attached to #1604 went through every variant of that
// (a bare goroutine, a timer, synthetic releases, SendInput) and every one
// hung or dropped on nothing.
//
// What works is the technique of the Far Manager plugin Burlak
// (github.com/refaim/burlak, src/drag/ToolWindow.cpp and src/core/Session.cpp),
// followed here step by step:
//
//  1. A tool thread with OleInitialize, a message pump and one hidden layered
//     window at alpha 1 -- invisible, but hit-testable. No parent and no
//     owner: a window related to conhost's attaches the two input queues
//     (see internal/wincon's package comment).
//  2. While the physical button is still held, the dragged paths become a
//     shell data object on the tool thread (SHParseDisplayName,
//     SHCreateShellItemArrayFromIDLists, BindToHandler(BHID_DataObject)).
//  3. The physical button is released synthetically (mouse_event LEFTUP), so
//     the console sees an ordinary release and the panel's own gesture ends.
//  4. The tool window is placed over the host window under the pointer,
//     captures the mouse, arms a one-second timer and a synthetic press
//     (mouse_event MOVE|LEFTDOWN) follows the release in the input stream.
//     Because it follows a release, it is a fresh click, and it lands on the
//     tool window as WM_LBUTTONDOWN. The earlier attempt skipped the release
//     in step 3 and pressed an already pressed button, which is why its start
//     worked only sometimes.
//  5. WM_LBUTTONDOWN runs SHDoDragDrop on the tool thread, from inside the
//     window procedure. From there on it is an ordinary OLE drag with the
//     real button held, and the real release ends it.
//
// If the press never arrives, the timer disarms after a second and the drag
// reports failure instead of leaving the backend busy forever -- the other
// thing the earlier attempt lacked.
//
// Every step is logged with the "CONSOLE_DND:" prefix when f4 runs with
// --debug, so a failure in the field names the step that failed.
package wincondrag
