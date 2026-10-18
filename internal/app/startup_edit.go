package app

import (
	"path/filepath"
	"sync"

	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// startupEditor is the editor the command line's -e named, from the moment
// it is asked for until showEditor builds it: the absolute path of the file and
// what to do once that editor is closed. Opening can take a while (a progress
// dialog, the Colorer schema download), and showEditor is where the editor
// exists, so the two are matched by path.
var startupEditor struct {
	mu       sync.Mutex
	path     string
	onClosed func()
}

// openStartupEditFileIn opens the -e file, and f4 leaves with it as far2l's
// -e does (issue #1974): `f4 --edit file` typed at the command line of f4 is
// a session of its own, and when the editor is closed it is the way back to
// that command line, not a second set of panels on the way.
//
// A file that is already open in an editor of this session is only switched
// to: closing that editor was never what the command line asked for.
func openStartupEditFileIn(pf *panel.PanelsFrame, path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		openEditFileIn(pf, path)
		return
	}
	v := vfs.NewOSVFS(filepath.Dir(abs))
	if ev, _ := FindOpenedEditor(v, abs); ev == nil {
		armStartupEditor(abs, startupEditorClosed(pf, terminal.ClientOwnsSession()))
	}
	ActionOpenEditor(pf, v, abs)
}

// armStartupEditor makes onClosed the reaction to closing the editor that
// showEditor builds for the absolute path abs.
func armStartupEditor(abs string, onClosed func()) {
	startupEditor.mu.Lock()
	defer startupEditor.mu.Unlock()
	startupEditor.path = abs
	startupEditor.onClosed = onClosed
}

// takeStartupEditorHook returns what to do when the editor just built for path
// is closed, if that is the -e editor, and forgets it.
func takeStartupEditorHook(v vfs.VFS, path string) func() {
	if osvfs, ok := v.(*vfs.OSVFS); ok {
		if abs, err := osvfs.Abs(path); err == nil {
			path = abs
		}
	}
	startupEditor.mu.Lock()
	defer startupEditor.mu.Unlock()
	if startupEditor.onClosed == nil || startupEditor.path != path {
		return nil
	}
	hook := startupEditor.onClosed
	startupEditor.path, startupEditor.onClosed = "", nil
	return hook
}

// leaveSharedSession lets this terminal go from a session it joined. A variable
// so that a test can tell it from ending the session.
var leaveSharedSession = func() { vtui.FrameManager.Stop() }

// startupEditorClosed is what happens when the -e editor is closed. A session
// started for this launch ends, unless another editor still holds unsaved
// work -- then it stays for the user to finish that. A session the launch
// joined goes on running; only this terminal is let go, as when it drops.
func startupEditorClosed(pf *panel.PanelsFrame, ownsSession bool) func() {
	return func() {
		frames := vtui.FrameManager
		frames.PostTask(func() {
			if !ownsSession {
				leaveSharedSession()
				return
			}
			if anyEditorModified() {
				return
			}
			pf.QuitNow()
		})
	}
}

// anyEditorModified reports whether an open editor has unsaved changes.
func anyEditorModified() bool {
	for _, screen := range vtui.FrameManager.Screens {
		if screen == nil {
			continue
		}
		for _, f := range screen.Frames {
			if ev, ok := f.(*editor.EditorView); ok && !ev.IsDone() && ev.Modified {
				return true
			}
		}
	}
	return false
}
