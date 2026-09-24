package panel

import (
	"path/filepath"
	"strings"

	"github.com/unxed/vtui"
)

// ApplyCurrentExtensionSelection implements Far Manager's Ctrl+Gray +/-
// selection command. The main-keyboard aliases offer it without a numpad.
// Folders form a separate group; the parent entry is never marked.
func (fp *FileSystemPanel) ApplyCurrentExtensionSelection(state bool) {
	idx := fp.GetCursorIndex()
	if idx < 0 || idx >= len(fp.Entries) {
		return
	}
	current := fp.Entries[idx]
	if current.Name == ".." {
		return
	}
	extension := selectionExtension(current)
	fp.SaveSelection()
	matched := 0
	for i, entry := range fp.Entries {
		if entry.Name == ".." || entry.IsDir != current.IsDir {
			continue
		}
		if current.IsDir || strings.EqualFold(selectionExtension(entry), extension) {
			fp.SetItemSelected(i, state)
			matched++
		}
	}
	vtui.DebugLog("[FIX:panel-selection] current-extension state=%t folders=%t matched=%d", state, current.IsDir, matched)
	vtui.FrameManager.Redraw()
}

func selectionExtension(entry *FileEntry) string {
	if entry.NoExtension {
		return ""
	}
	return filepath.Ext(entry.Name)
}
