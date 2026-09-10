package panel

import (
	"github.com/unxed/f4/vfs"
)

// DispatchPanelAction gives a virtual manager VFS first refusal on a semantic
// panel action. The action is independent of the key or menu entry that
// invoked it, so user hotkey remapping does not break plugin behavior.
func DispatchPanelAction(pf *PanelsFrame, action vfs.PanelAction, paths []string) bool {
	if pf == nil {
		return false
	}
	fsp := pf.GetActivePanel()
	if fsp == nil || fsp.Vfs == nil {
		return false
	}
	handler, ok := fsp.Vfs.(vfs.PanelActionHandler)
	if !ok {
		return false
	}
	// Handlers run synchronously on the UI thread. Give them an immutable
	// snapshot so selection changes made by a dialog cannot mutate the input.
	return handler.HandlePanelAction(pf, action, append([]string(nil), paths...))
}

func SelectedPanelActionPaths(fsp *FileSystemPanel) []string {
	if fsp == nil || fsp.Vfs == nil {
		return nil
	}
	names := fsp.GetSelectedNames()
	paths := make([]string, 0, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		if name == ".." {
			paths = append(paths, fsp.Vfs.Dir(fsp.Vfs.GetPath()))
			continue
		}
		paths = append(paths, fsp.Vfs.Join(fsp.Vfs.GetPath(), name))
	}
	return paths
}
