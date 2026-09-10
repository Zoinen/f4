package fileops

import (
	"github.com/unxed/vtui"
)

// BackgroundWorkspace supplies the screen an operation sent to the background
// runs in front of. A file operation put in the background keeps its progress
// dialog on screen, and the dialog needs a workspace behind it: the root
// returns a copy of the panels, so the user goes on working in the original
// while the copy sits under the dialog.
//
// The root sets it. It may also return nil, in a run that has no panels to
// copy — an editor started on a file, a test.
var BackgroundWorkspace func() vtui.Frame

// BackgroundScreen returns the screen to put behind a progress dialog, or nil
// to run the dialog headless. Mode 1 is the background mode; everything else
// keeps the dialog in front of whatever is already on screen.
//
// Headless is the honest fallback for an unwired seam and for a run with no
// panels alike: a progress dialog with nothing behind it is worse than none.
func BackgroundScreen(mode int) vtui.Frame {
	if mode != 1 || BackgroundWorkspace == nil {
		return nil
	}
	return BackgroundWorkspace()
}

// HandleWorkspaceFork answers the "fork this workspace" command for the frames
// this package owns. Forking duplicates the panels, which is the application's
// business, so the root supplies it.
//
// The default declines, and declining is the honest answer rather than a
// degraded one: a run with no workspaces to fork has nothing to do here, and
// the command falls through to the base window exactly as it would have.
var HandleWorkspaceFork = func(cmd int, args any) bool { return false }
