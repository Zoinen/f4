package panel

import (
	"context"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// symlinkTarget reports where the link under the cursor points, for the
// status line and the bottom-border figure (f4#1766).
//
// Drawing runs on the UI thread, so it must never wait for the network. On a
// local file system the answer is a microsecond away and is read on the spot.
// Anywhere else (SFTP, FISH+, SMB, an RPC plugin) it is a round trip, and
// asking for it from inside the draw call froze the interface for as long as
// the link took to answer, on every repaint that found the cursor on a link.
// Those are asked in the background instead: the first call returns "not
// known yet", the answer is kept on the entry, and the panel repaints when it
// arrives. A reload builds new entries, so a stale answer cannot outlive the
// directory it was read from.
func (fp *FileSystemPanel) symlinkTarget(e *FileEntry) (string, bool) {
	if e == nil || !e.IsSymlink || fp.Vfs == nil {
		return "", false
	}
	if e.linkResolved {
		return e.linkTarget, e.linkTarget != ""
	}
	filesystem := fp.Vfs
	full := filesystem.Join(filesystem.GetPath(), e.Name)
	if _, local := filesystem.(*vfs.OSVFS); local {
		target, err := vfs.Readlink(context.Background(), filesystem, full)
		e.linkResolved = true
		if err == nil {
			e.linkTarget = target
		}
		return e.linkTarget, e.linkTarget != ""
	}
	if e.linkPending {
		return "", false
	}
	e.linkPending = true
	vtui.RunAsync(func(task *vtui.TaskContext) {
		target, err := vfs.Readlink(task.Context, filesystem, full)
		task.RunOnUI(func() {
			e.linkPending = false
			if task.Err() != nil {
				return
			}
			e.linkResolved = true
			if err == nil {
				e.linkTarget = target
			}
			vtui.FrameManager.Redraw()
		})
	})
	return "", false
}
