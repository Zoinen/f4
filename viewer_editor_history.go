package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

const viewerEditorHistoryID = "viewer-editor"

type viewerEditorHistoryMode string

const (
	historyModeView viewerEditorHistoryMode = "view"
	historyModeEdit viewerEditorHistoryMode = "edit"
)

type viewerEditorHistoryEntry struct {
	Path     string                  `json:"path"`
	Display  string                  `json:"display"`
	Mode     viewerEditorHistoryMode `json:"mode"`
	Local    bool                    `json:"local,omitempty"`
	VFSType  string                  `json:"vfs_type,omitempty"`
	VFSTitle string                  `json:"vfs_title,omitempty"`
}

func loadViewerEditorHistory() []viewerEditorHistoryEntry {
	if vtui.GlobalHistoryProvider == nil {
		return nil
	}
	encoded := vtui.GlobalHistoryProvider.LoadHistory(viewerEditorHistoryID)
	entries := make([]viewerEditorHistoryEntry, 0, len(encoded))
	for _, item := range encoded {
		var entry viewerEditorHistoryEntry
		if json.Unmarshal([]byte(item), &entry) != nil || entry.Path == "" {
			continue
		}
		if entry.Display == "" {
			entry.Display = entry.Path
		}
		if entry.Mode != historyModeEdit {
			entry.Mode = historyModeView
		}
		entries = append(entries, entry)
	}
	return entries
}

func saveViewerEditorHistory(entries []viewerEditorHistoryEntry) {
	if vtui.GlobalHistoryProvider == nil {
		return
	}
	encoded := make([]string, 0, len(entries))
	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err == nil {
			encoded = append(encoded, string(data))
		}
	}
	vtui.GlobalHistoryProvider.SaveHistory(viewerEditorHistoryID, encoded)
}

func rememberViewerEditorHistory(fs vfs.VFS, path string, mode viewerEditorHistoryMode) {
	if fs == nil || path == "" || vtui.GlobalHistoryProvider == nil {
		return
	}
	// The terminal log is generated live and has no stable file to reopen.
	if _, transient := fs.(*TerminalLogVFS); transient {
		return
	}

	entry := viewerEditorHistoryEntry{
		Path:    path,
		Display: path,
		Mode:    mode,
		VFSType: fmt.Sprintf("%T", fs),
	}
	if local, ok := fs.(*vfs.OSVFS); ok {
		if absolute, err := local.Abs(path); err == nil {
			entry.Path = absolute
			entry.Display = absolute
		}
		entry.Local = true
	} else if titled, ok := fs.(vfs.TitleProvider); ok {
		entry.VFSTitle = titled.GetTitle()
		if entry.VFSTitle != "" && !strings.HasPrefix(path, entry.VFSTitle+":") {
			entry.Display = entry.VFSTitle + ":" + path
		}
	}

	entries := loadViewerEditorHistory()
	filtered := make([]viewerEditorHistoryEntry, 0, len(entries)+1)
	filtered = append(filtered, entry)
	for _, old := range entries {
		if sameViewerEditorHistoryFile(old, entry) {
			continue
		}
		filtered = append(filtered, old)
	}
	if len(filtered) > 100 {
		filtered = filtered[:100]
	}
	saveViewerEditorHistory(filtered)
}

func sameViewerEditorHistoryFile(a, b viewerEditorHistoryEntry) bool {
	if a.Local != b.Local || a.VFSType != b.VFSType || a.VFSTitle != b.VFSTitle {
		return false
	}
	if a.Local {
		return sameFolderHistoryPath(a.Path, b.Path)
	}
	return a.Path == b.Path
}

func viewerEditorHistoryVFS(pf *PanelsFrame, entry viewerEditorHistoryEntry) vfs.VFS {
	if entry.Local {
		return vfs.NewOSVFS(filepath.Dir(entry.Path))
	}
	for _, panel := range []*FileSystemPanel{pf.getActivePanel(), pf.getInactivePanel()} {
		if panel == nil || fmt.Sprintf("%T", panel.vfs) != entry.VFSType {
			continue
		}
		title := ""
		if titled, ok := panel.vfs.(vfs.TitleProvider); ok {
			title = titled.GetTitle()
		}
		if title == entry.VFSTitle {
			return panel.vfs
		}
	}
	return nil
}

func openViewerEditorHistoryEntry(pf *PanelsFrame, entry viewerEditorHistoryEntry, mode viewerEditorHistoryMode) bool {
	fs := viewerEditorHistoryVFS(pf, entry)
	if fs == nil {
		vtui.ShowMessage(Msg("History.ViewEditTitle"), Msg("History.SourceUnavailable"), []string{Msg("vtui.Ok")})
		return false
	}
	if mode == historyModeEdit {
		actionOpenEditor(pf, fs, entry.Path)
	} else {
		actionOpenViewer(pf, fs, entry.Path)
	}
	return true
}

func actionViewerEditorHistory(pf *PanelsFrame) {
	entries := loadViewerEditorHistory()
	if len(entries) == 0 {
		vtui.ShowMessage(Msg("History.Title"), Msg("History.EmptyViewEdit"), []string{Msg("vtui.Ok")})
		return
	}

	paths := make([]HistoryRecord, len(entries))
	modes := make([]string, len(entries))
	for i, entry := range entries {
		paths[i] = HistoryRecord{Name: entry.Display}
		if entry.Mode == historyModeEdit {
			modes[i] = Msg("History.Mode.Edit")
		} else {
			modes[i] = Msg("History.Mode.View")
		}
	}

	menu := vtui.NewVMenu(Msg("History.ViewEditTitle"))
	menu.SetHelp("HistoryViewEdit")
	search := newHistorySearch(menu, paths, Msg("History.ViewEditHint"))
	search.setSecondaryWidth(modes, true, 10)

	openCurrent := func(override viewerEditorHistoryMode) {
		idx, _, ok := search.selected()
		if !ok || idx < 0 || idx >= len(entries) {
			return
		}
		mode := override
		if mode == "" {
			mode = entries[idx].Mode
		}
		if openViewerEditorHistoryEntry(pf, entries[idx], mode) {
			search.cleanup()
			menu.Close()
		}
	}
	menu.OnAction = func(int) { openCurrent("") }
	menu.OnKeyDown = func(e *vtinput.InputEvent) bool {
		shift := e.ControlKeyState&vtinput.ShiftPressed != 0
		ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
		alt := e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
		if search.processKey(e) {
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_ESCAPE || e.VirtualKeyCode == vtinput.VK_F10 {
			search.cleanup()
			return false
		}

		idx, _, ok := search.selected()
		if !ok || idx < 0 || idx >= len(entries) {
			return false
		}
		switch e.VirtualKeyCode {
		case vtinput.VK_RETURN:
			openCurrent("")
			return true
		case vtinput.VK_F3:
			openCurrent(historyModeView)
			return true
		case vtinput.VK_F4:
			openCurrent(historyModeEdit)
			return true
		}

		if (e.VirtualKeyCode == vtinput.VK_DELETE || e.VirtualKeyCode == vtinput.VK_BACK) && shift {
			entries = append(entries[:idx], entries[idx+1:]...)
			search.deleteSelected()
			saveViewerEditorHistory(entries)
			if len(entries) == 0 {
				search.cleanup()
				menu.Close()
			}
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_DELETE && !ctrl && !alt && !shift {
			pathNames := extractNames(paths)
			confirmAndClearHistory(Msg("History.ViewEditTitle"), viewerEditorHistoryID, &pathNames, func() {
				entries = nil
			}, search, menu)
			return true
		}
		if (e.VirtualKeyCode == vtinput.VK_C || e.VirtualKeyCode == vtinput.VK_INSERT) && ctrl && !alt && !shift {
			go vtui.SetClipboard(entries[idx].Path)
			return true
		}
		return false
	}

	vtui.FrameManager.Push(menu)
}
