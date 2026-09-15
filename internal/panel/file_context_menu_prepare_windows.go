package panel

import (
	"github.com/unxed/f4/internal/filemenu"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func (pf *PanelsFrame) prepareFileMenu() {
	if vtui.FrameManager == nil || vtui.FrameManager.GetTopFrame() != pf {
		return
	}
	s := captureFileMenuSelection(pf)
	if s == nil {
		filemenu.Prepare(nil)
		return
	}
	// Background work is limited to direct local paths. Mounted virtual targets
	// still go through their live identity/permission checks on explicit request.
	switch s.filesystem.(type) {
	case *vfs.OSVFS, *vfs.DisksVFS:
		paths, _ := s.localPaths()
		filemenu.Prepare(paths)
	default:
		filemenu.Prepare(nil)
	}
}
