package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// Action represents a bindable command in the application.
type Action struct {
	Name        string
	Label       string
	Description string
	Handler     func() bool
}

var actionRegistry = make(map[string]Action)

// RegisterAction adds an action to the global registry.
func RegisterAction(action Action) {
	actionRegistry[strings.ToLower(action.Name)] = action
}

// RunAction executes an action by name if it exists.
func RunAction(name string) bool {
	if a, ok := actionRegistry[strings.ToLower(name)]; ok && a.Handler != nil {
		return a.Handler()
	}
	return false
}

// GetActions returns a list of all registered actions, sorted by name.
func GetActions() []Action {
	var actions []Action
	for _, a := range actionRegistry {
		actions = append(actions, a)
	}
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].Name < actions[j].Name
	})
	return actions
}

// GetAction returns an action by name.
func GetAction(name string) (Action, bool) {
	a, ok := actionRegistry[strings.ToLower(name)]
	return a, ok
}

func init() {
	withPF := func(fn func(pf *PanelsFrame)) func() bool {
		return func() bool {
			if pf := findPanelsFrameAnyScreen(); pf != nil {
				fn(pf)
				return true
			}
			return false
		}
	}

	withEditor := func(fn func(ev *EditorView)) func() bool {
		return func() bool {
			if vtui.FrameManager == nil {
				return false
			}
			if ev, ok := vtui.FrameManager.GetTopFrame().(*EditorView); ok {
				fn(ev)
				return true
			}
			return false
		}
	}

	withViewer := func(fn func(vv *ViewerView)) func() bool {
		return func() bool {
			if vtui.FrameManager == nil {
				return false
			}
			if vv, ok := vtui.FrameManager.GetTopFrame().(*ViewerView); ok {
				fn(vv)
				return true
			}
			return false
		}
	}

	RegisterAction(Action{
		Name:        "File.Copy",
		Label:       "Copy",
		Description: "Copy selected files or current file",
		Handler:     withPF(func(pf *PanelsFrame) { actionCopyMove(pf, false) }),
	})
	RegisterAction(Action{
		Name:        "File.Move",
		Label:       "Move",
		Description: "Rename or move selected files",
		Handler:     withPF(func(pf *PanelsFrame) { actionCopyMove(pf, true) }),
	})
	RegisterAction(Action{
		Name:        "File.Rename",
		Label:       "Rename",
		Description: "Rename current file",
		Handler:     withPF(func(pf *PanelsFrame) { actionRename(pf) }),
	})
	RegisterAction(Action{
		Name:        "File.Delete",
		Label:       "Delete",
		Description: "Delete selected files",
		Handler:     withPF(func(pf *PanelsFrame) { actionDelete(pf) }),
	})
	RegisterAction(Action{
		Name:        "File.MakeDir",
		Label:       "Make Folder",
		Description: "Create a new directory",
		Handler:     withPF(func(pf *PanelsFrame) { actionMkDir(pf) }),
	})
	RegisterAction(Action{
		Name:        "File.Edit",
		Label:       "Edit",
		Description: "Open file in editor",
		Handler:     withPF(func(pf *PanelsFrame) { actionEditFile(pf) }),
	})
	RegisterAction(Action{
		Name:        "File.View",
		Label:       "View",
		Description: "Open file in viewer",
		Handler:     withPF(func(pf *PanelsFrame) { actionViewFile(pf) }),
	})
	RegisterAction(Action{
		Name:        "File.New",
		Label:       "New File",
		Description: "Create and open a new file in editor",
		Handler:     withPF(func(pf *PanelsFrame) { actionNewFile(pf) }),
	})
	RegisterAction(Action{
		Name:        "File.Find",
		Label:       "Find File",
		Description: "Search for files",
		Handler:     withPF(func(pf *PanelsFrame) { actionFindFile(pf) }),
	})

	RegisterAction(Action{
		Name:        "Panel.Rescan",
		Label:       "Rescan",
		Description: "Refresh panel contents",
		Handler:     withPF(func(pf *PanelsFrame) { pf.RefreshAll() }),
	})
	RegisterAction(Action{
		Name:        "Panel.Swap",
		Label:       "Swap Panels",
		Description: "Swap left and right panels",
		Handler:     withPF(func(pf *PanelsFrame) { vtui.FrameManager.EmitCommand(CmSwapPanels, nil) }),
	})
	RegisterAction(Action{
		Name:        "Panel.Toggle",
		Label:       "Toggle Panels",
		Description: "Show or hide panels",
		Handler: withPF(func(pf *PanelsFrame) {
			pf.exitWide()
			pf.showPanels = !pf.showPanels
			if pf.showPanels && !pf.showLeftPanel && !pf.showRightPanel {
				pf.showLeftPanel = true
				pf.showRightPanel = true
			}
			vtui.FrameManager.HardRefresh()
			if pf.showPanels {
				pf.RefreshAll()
			}
		}),
	})
	RegisterAction(Action{
		Name:        "Panel.FoldersHistory",
		Label:       "Folders History",
		Description: "Show folders history",
		Handler:     withPF(func(pf *PanelsFrame) { actionFoldersHistory(pf) }),
	})
	RegisterAction(Action{
		Name:        "Panel.ViewBrief",
		Label:       "Brief Mode",
		Description: "Set active panel to brief mode",
		Handler:     withPF(func(pf *PanelsFrame) { pf.setPanelViewMode(pf.activeIdx, ViewModeBrief) }),
	})
	RegisterAction(Action{
		Name:        "Panel.ViewMedium",
		Label:       "Medium Mode",
		Description: "Set active panel to medium mode",
		Handler:     withPF(func(pf *PanelsFrame) { pf.setPanelViewMode(pf.activeIdx, ViewModeMedium) }),
	})
	RegisterAction(Action{
		Name:        "Panel.ViewDetailed",
		Label:       "Detailed Mode",
		Description: "Set active panel to detailed mode",
		Handler:     withPF(func(pf *PanelsFrame) { pf.setPanelViewMode(pf.activeIdx, ViewModeDetailed) }),
	})
	RegisterAction(Action{
		Name:        "Panel.ViewWide",
		Label:       "Wide Mode",
		Description: "Set active panel to wide mode",
		Handler:     withPF(func(pf *PanelsFrame) { pf.setWidePanel(pf.activeIdx) }),
	})
	RegisterAction(Action{
		Name:        "Panel.Bookmarks",
		Label:       "Bookmarks",
		Description: "Show folder bookmarks dialog",
		Handler:     withPF(func(pf *PanelsFrame) { ShowBookmarksDialog(pf) }),
	})
	RegisterAction(Action{
		Name:        "Settings.Language",
		Label:       "Language",
		Description: "Open language selection dialog",
		Handler:     withPF(func(pf *PanelsFrame) { actionLanguage(pf) }),
	})
	RegisterAction(Action{
		Name:        "Settings.HelpLanguage",
		Label:       "Help Language",
		Description: "Open help language selection dialog",
		Handler:     withPF(func(pf *PanelsFrame) { actionHelpLanguage(pf) }),
	})
	RegisterAction(Action{
		Name:        "Settings.Plugins",
		Label:       "Plugins Menu",
		Description: "Manage plugins dialog",
		Handler:     withPF(func(pf *PanelsFrame) { actionManagePlugins(pf) }),
	})
	RegisterAction(Action{
		Name:        "Panel.CommandHistory",
		Label:       "Command History",
		Description: "Show command line history",
		Handler:     withPF(func(pf *PanelsFrame) { actionCommandHistory(pf) }),
	})

	RegisterAction(Action{
		Name:        "Settings.Panel",
		Label:       "Panel Settings",
		Description: "Open panel settings dialog",
		Handler:     withPF(func(pf *PanelsFrame) { actionPanelSettings(pf) }),
	})
	RegisterAction(Action{
		Name:        "Settings.Editor",
		Label:       "Editor Settings",
		Description: "Open editor settings dialog",
		Handler:     withPF(func(pf *PanelsFrame) { actionEditorSettings(pf) }),
	})
	RegisterAction(Action{
		Name:        "Settings.Colorer",
		Label:       "Colorer Settings",
		Description: "Open Colorer settings dialog",
		Handler:     withPF(func(pf *PanelsFrame) { actionColorerSettings(pf) }),
	})
	RegisterAction(Action{
		Name:        "Settings.Appearance",
		Label:       "Appearance Settings",
		Description: "Open appearance settings dialog",
		Handler:     withPF(func(pf *PanelsFrame) { actionAppearanceSettings(pf) }),
	})
	RegisterAction(Action{
		Name:        "Settings.Confirmations",
		Label:       "Confirmations Settings",
		Description: "Open confirmations settings dialog",
		Handler:     withPF(func(pf *PanelsFrame) { actionConfirmationsSettings(pf) }),
	})
	RegisterAction(Action{
		Name:        "Settings.Hotkeys",
		Label:       "Hotkey Configuration",
		Description: "Open Hotkey Configurator",
		Handler:     withPF(func(pf *PanelsFrame) { actionHotkeyConfig(pf) }),
	})

	RegisterAction(Action{
		Name:        "Terminal.ViewLog",
		Label:       "View Terminal Log",
		Description: "Open terminal log in viewer",
		Handler:     withPF(func(pf *PanelsFrame) { actionViewTerminalLog(pf) }),
	})
	RegisterAction(Action{
		Name:        "Terminal.EditLog",
		Label:       "Edit Terminal Log",
		Description: "Open terminal log in editor",
		Handler:     withPF(func(pf *PanelsFrame) { actionEditTerminalLog(pf) }),
	})

	// Editor Actions
	RegisterAction(Action{Name: "Editor.Save", Label: "Save", Description: "Save file", Handler: withEditor(func(ev *EditorView) { ev.SaveToFile(nil) })})
	RegisterAction(Action{Name: "Editor.WordWrap", Label: "Word Wrap", Description: "Toggle word wrap", Handler: withEditor(func(ev *EditorView) {
		ev.WordWrap = !ev.WordWrap
		ev.ScrollLeft = 0
		ev.clearCaches()
		ev.ensureCursorVisible()
	})})
	RegisterAction(Action{Name: "Editor.ShowWhitespaces", Label: "Show Whitespaces", Description: "Toggle visible whitespaces", Handler: withEditor(func(ev *EditorView) { ev.ShowWhitespaces = !ev.ShowWhitespaces })})
	RegisterAction(Action{Name: "Editor.Search", Label: "Search", Description: "Find text", Handler: withEditor(func(ev *EditorView) { vtui.FrameManager.EmitCommand(CmSearch, nil) })})
	RegisterAction(Action{Name: "Editor.Replace", Label: "Replace", Description: "Replace text", Handler: withEditor(func(ev *EditorView) { vtui.FrameManager.EmitCommand(CmReplace, nil) })})
	RegisterAction(Action{Name: "Editor.SearchNext", Label: "Search Next", Description: "Continue search", Handler: withEditor(func(ev *EditorView) {
		if LastEditorSearch != "" {
			ev.Search(LastEditorSearch, LastEditorSearchCase, LastEditorSearchReverse, LastEditorSearchRegexp, LastEditorSearchWholeWord, true)
		}
	})})
	RegisterAction(Action{Name: "Editor.Quit", Label: "Quit", Description: "Close editor", Handler: withEditor(func(ev *EditorView) { ev.tryClose() })})
	RegisterAction(Action{Name: "Editor.SwitchToViewer", Label: "Switch to Viewer", Description: "Switch to viewer mode", Handler: withEditor(func(ev *EditorView) { vtui.FrameManager.EmitCommand(CmSwitchToViewer, ev) })})
	RegisterAction(Action{Name: "Editor.CodepageNext", Label: "Next Codepage", Description: "Cycle to next codepage", Handler: withEditor(func(ev *EditorView) {
		next := vfs.GetNextFastSwitchCodepage(ev.Codepage)
		ev.ReloadWithCodepage(next)
		vtui.ShowToast(fmt.Sprintf("Codepage: %d", next), time.Second)
	})})
	RegisterAction(Action{Name: "Editor.CodepageMenu", Label: "Codepage Menu", Description: "Select codepage", Handler: withEditor(func(ev *EditorView) { ev.showCodepageDialog() })})
	RegisterAction(Action{Name: "Editor.SelectAll", Label: "Select All", Description: "Select all text", Handler: withEditor(func(ev *EditorView) {
		ev.rectSelActive = false
		ev.selActive = true
		ev.selAnchorOffset = 0
		lastLine := ev.li.LineCount() - 1
		ev.CursorLine = lastLine
		ev.CursorPos = ev.getLineLength(lastLine)
		ev.ensureCursorVisible()
	})})
	RegisterAction(Action{Name: "Editor.DeleteLine", Label: "Delete Line", Description: "Delete current line", Handler: withEditor(func(ev *EditorView) { ev.DeleteCurrentLine() })})
	RegisterAction(Action{Name: "Editor.Undo", Label: "Undo", Description: "Undo last change", Handler: withEditor(func(ev *EditorView) { ev.Undo() })})
	RegisterAction(Action{Name: "Editor.Redo", Label: "Redo", Description: "Redo last undone change", Handler: withEditor(func(ev *EditorView) { ev.Redo() })})

	// Viewer Actions
	RegisterAction(Action{Name: "Viewer.Quit", Label: "Quit", Description: "Close viewer", Handler: withViewer(func(vv *ViewerView) { vv.Close() })})
	RegisterAction(Action{Name: "Viewer.WrapMode", Label: "Wrap Mode", Description: "Toggle word wrap", Handler: withViewer(func(vv *ViewerView) { vv.WrapMode = !vv.WrapMode })})
	RegisterAction(Action{Name: "Viewer.HexMode", Label: "Hex Mode", Description: "Toggle hex view", Handler: withViewer(func(vv *ViewerView) {
		vv.HexMode = !vv.HexMode
		if vv.HexMode {
			vv.TopOffset &= ^int64(0xF)
		}
	})})
	RegisterAction(Action{Name: "Viewer.Search", Label: "Search", Description: "Find text", Handler: withViewer(func(vv *ViewerView) { vtui.FrameManager.EmitCommand(CmSearch, nil) })})
	RegisterAction(Action{Name: "Viewer.SwitchToEditor", Label: "Switch to Editor", Description: "Switch to editor mode", Handler: withViewer(func(vv *ViewerView) { vtui.FrameManager.EmitCommand(CmSwitchToEditor, vv) })})
	RegisterAction(Action{Name: "Viewer.CodepageNext", Label: "Next Codepage", Description: "Cycle to next codepage", Handler: withViewer(func(vv *ViewerView) {
		next := vfs.GetNextFastSwitchCodepage(vv.Codepage)
		vv.ReloadWithCodepage(next)
		vtui.ShowToast(fmt.Sprintf("Codepage: %d", next), time.Second)
	})})
	RegisterAction(Action{Name: "Viewer.CodepageMenu", Label: "Codepage Menu", Description: "Select codepage", Handler: withViewer(func(vv *ViewerView) { vv.showCodepageDialog() })})
}
