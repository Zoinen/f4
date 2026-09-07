package main

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/sheet"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// activePanelsFrame returns the panels frame of the current workspace.
//
// It walks the frame stack instead of looking only at the top frame: while a
// menu dropdown or the command palette is open, that popup is the top frame,
// and a check against the top frame alone reports that there are no panels
// exactly when the user is choosing this command.
func activePanelsFrame() *PanelsFrame {
	if vtui.FrameManager == nil {
		return nil
	}
	frames := vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
	for index := len(frames) - 1; index >= 0; index-- {
		if pf, ok := frames[index].(*PanelsFrame); ok {
			return pf
		}
	}
	return nil
}

// findSheetWorkspace switches to an existing spreadsheet workspace, if any.
func findSheetWorkspace() bool {
	if vtui.FrameManager == nil {
		return false
	}
	for index, screen := range vtui.FrameManager.Screens {
		if screen == nil {
			continue
		}
		for _, frame := range screen.Frames {
			if _, ok := frame.(*SheetFrame); ok {
				vtui.FrameManager.SwitchScreen(index)
				return true
			}
		}
	}
	return false
}

// actionSpreadsheet opens the spreadsheet, loading the file under the cursor
// when it is one this editor understands.
func actionSpreadsheet() bool {
	if vtui.FrameManager == nil {
		return false
	}
	path := selectedSpreadsheetPath()
	if path == "" && findSheetWorkspace() {
		return true
	}
	frame := NewSheetFrame()
	if path != "" {
		sheetOpen(frame, path)
	}
	vtui.FrameManager.AddScreenHeadless(frame)
	return true
}

// selectedSpreadsheetPath returns the panel selection when it looks like a
// spreadsheet: the native SQLite format, a workbook or a CSV file.
func selectedSpreadsheetPath() string {
	pf := activePanelsFrame()
	if pf == nil {
		return ""
	}
	name := pf.GetSelectedName()
	if name == "" || name == ".." {
		return ""
	}
	fs, ok := pf.GetActivePanelVFS().(*vfs.OSVFS)
	if !ok || fs == nil {
		return ""
	}
	path, err := fs.Abs(fs.Join(fs.GetPath(), name))
	if err != nil {
		return ""
	}
	path = filepath.Clean(path)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".xlsx", ".csv":
		return path
	// .f4s is what the native format used to be called. Sheets written under
	// the old name keep opening; IsSheetFile is what decides in every case.
	case ".f4s", ".db", ".sqlite", ".sqlite3":
		if sheet.IsSheetFile(context.Background(), path) {
			return path
		}
	}
	return ""
}

func init() {
	RegisterAction(Action{
		Name:        "App.Spreadsheet",
		Area:        "Shell",
		Label:       "Spreadsheet",
		LabelKey:    "Action.App.Spreadsheet",
		Description: "Open the spreadsheet workspace",
		DescKey:     "Action.App.Spreadsheet.Desc",
		// Shift+F11 belongs to Settings.PluginConfiguration, which is in this
		// same area, so binding it here took the shortcut away from it and
		// left it with none. Alt+F9 is no good either: window managers claim
		// it for resize, and the registry knows nothing about what the outside
		// world has already taken.
		DefaultKeys: []string{"CtrlAltS"},
		MenuPath:    "Commands",
		Handler:     actionSpreadsheet,
	})
}
