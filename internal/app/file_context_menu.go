package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/vfs"
)

func init() {
	panel.ContextMenuCommand = func(pf *panel.PanelsFrame, id string) {
		switch id {
		case "copy":
			actionCopyMove(pf, false)
		case "move":
			actionCopyMove(pf, true)
		case "rename":
			actionRename(pf)
		case "duplicate":
			actionCopyInPlace(pf)
		case "trash":
			actionDeleteWithDisposition(pf, vfs.DeleteToTrash, false)
		case "properties":
			actionFileAttributes(pf)
		}
	}
}
