package main

import "github.com/unxed/f4/vfs"

// dispatchPanelAction gives a virtual manager VFS first refusal on a semantic
// panel action. The action is independent of the key or menu entry that
// invoked it, so user hotkey remapping does not break plugin behavior.
func dispatchPanelAction(pf *PanelsFrame, action vfs.PanelAction, paths []string) bool {
	if pf == nil {
		return false
	}
	fsp := pf.getActivePanel()
	if fsp == nil || fsp.vfs == nil {
		return false
	}
	handler, ok := fsp.vfs.(vfs.PanelActionHandler)
	if !ok {
		return false
	}
	// Handlers run synchronously on the UI thread. Give them an immutable
	// snapshot so selection changes made by a dialog cannot mutate the input.
	return handler.HandlePanelAction(pf, action, append([]string(nil), paths...))
}

func selectedPanelActionPaths(fsp *FileSystemPanel) []string {
	if fsp == nil || fsp.vfs == nil {
		return nil
	}
	names := fsp.GetSelectedNames()
	paths := make([]string, 0, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		if name == ".." {
			paths = append(paths, fsp.vfs.Dir(fsp.vfs.GetPath()))
			continue
		}
		paths = append(paths, fsp.vfs.Join(fsp.vfs.GetPath(), name))
	}
	return paths
}
