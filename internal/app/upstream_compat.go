package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/vfs"
)

// These aliases keep package-private names used by upstream files and tests
// available while the fork's public entry points remain the canonical ones.
type coreAPI = CoreAPI

func getLongVersionInfo() string { return GetLongVersionInfo() }

func openEditFileIn(pf *panel.PanelsFrame, path string) { OpenEditFileIn(pf, path) }

func actionOpenViewer(pf *panel.PanelsFrame, v vfs.VFS, path string) {
	ActionOpenViewer(pf, v, path)
}

func actionPanelSettings(pf *panel.PanelsFrame) { ActionPanelSettings(pf) }
