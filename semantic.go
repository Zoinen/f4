package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/fnv"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// SemanticNode экспортирует PanelsFrame в семантическое дерево ShellModel
func (pf *PanelsFrame) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	shell := extui.ShellModel{
		ID:             vtui.SemanticID(pf),
		Title:          strings.TrimSpace(pf.GetTitle()),
		Mode:           "panels",
		ActivePanel:    pf.activeIdx,
		ShowPanels:     pf.showPanels,
		ShowLeftPanel:  pf.showLeftPanel,
		ShowRightPanel: pf.showRightPanel,
		Wide:           pf.wide,
		WidePanel:      pf.widePanel,
		ShowKeyBar:     pf.showKeyBar,
		TerminalBusy:   pf.isPtyBusy(),
		TerminalActive: !pf.showPanels,
	}
	if !pf.showPanels {
		shell.Mode = "terminal"
	}

	for i, panel := range pf.panels {
		if fsp, ok := panel.(*FileSystemPanel); ok {
			shell.Panels = append(shell.Panels, fsp.semanticPanelModel(ctx, i, i == pf.activeIdx))
		}
		if info, ok := pf.altPanels[i].(*InfoPanel); ok {
			shell.InfoPanels = append(shell.InfoPanels, info.semanticModel(i, i == pf.activeIdx))
		}
	}

	if pf.cmdLine != nil {
		shell.CommandLine = pf.cmdLine.semanticModel(ctx)
	}
	if pf.termView != nil {
		shell.Terminal = pf.termView.semanticModel(ctx)
	}
	if MacroMgr != nil && MacroMgr.Recording {
		shell.MacroRecording = true
	}
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		shell.Fallback = true
		shell.FallbackReason = reason
	}

	return shell.ToMap()
}

func (pf *PanelsFrame) semanticGridFallbackReason() string {
	for _, panel := range pf.altPanels {
		if panel != nil && panel.Kind() != "info" {
			return "the QML presentation does not yet support the " + panel.Kind() + " panel"
		}
	}
	if pf.showPanels && (pf.widthDecrement != 0 || pf.leftHeightDecrement != 0 || pf.rightHeightDecrement != 0) {
		return "the QML presentation does not yet support resized-panel layouts"
	}
	return ""
}

func (ip *InfoPanel) semanticModel(side int, active bool) extui.InfoPanelModel {
	model := extui.InfoPanelModel{
		ID:         vtui.SemanticID(ip),
		Side:       side,
		Active:     active,
		Title:      Msg("InfoPanel.Title"),
		BottomHint: Msg("InfoPanel.UnitsHint"),
	}
	row := func(label, value string) {
		model.Rows = append(model.Rows, extui.InfoPanelRowModel{Kind: "row", Label: label, Value: value})
	}
	section := func(title string) {
		model.Rows = append(model.Rows, extui.InfoPanelRowModel{Kind: "section", Label: title})
	}
	blank := func() {
		model.Rows = append(model.Rows, extui.InfoPanelRowModel{Kind: "blank"})
	}

	hostname, _ := os.Hostname()
	username := ""
	if current, err := user.Current(); err == nil {
		username = shortUsername(current.Username)
	}
	row(Msg("InfoPanel.Computer"), hostname)
	row(Msg("InfoPanel.User"), username)
	blank()

	path := ""
	if ip.src != nil && ip.src.vfs != nil {
		path = ip.src.vfs.GetPath()
	}
	fsTitle := Msg("InfoPanel.FilesystemTitle")
	if fs, ok := fsInfo(path); ok {
		if fs.Type != "" {
			fsTitle = fmt.Sprintf("%s (%s)", fsTitle, fs.Type)
		}
		section(fsTitle)
		row(Msg("InfoPanel.Total"), formatBytes(fs.Total))
		row(Msg("InfoPanel.Free"), formatBytes(fs.Free))
		if fs.Label != "" {
			row(Msg("InfoPanel.Label"), fs.Label)
		}
		if fs.Serial != "" {
			row(Msg("InfoPanel.Serial"), fs.Serial)
		}
		row(Msg("InfoPanel.CurrentDir"), path)
		if fs.Mount != "" && fs.Mount != path {
			row(Msg("InfoPanel.Mount"), fs.Mount)
		}
		if fs.MaxFilename > 0 {
			row(Msg("InfoPanel.MaxFilename"), fmt.Sprintf("%d", fs.MaxFilename))
		}
		if fs.Flags != "" {
			row(Msg("InfoPanel.Flags"), fs.Flags)
		}
	} else {
		section(fsTitle)
		row(Msg("InfoPanel.CurrentDir"), path)
	}

	if mem, ok := memInfo(); ok {
		blank()
		section(Msg("InfoPanel.MemoryTitle"))
		row(Msg("InfoPanel.MemLoad"), fmt.Sprintf("%d%%", mem.LoadPercent))
		row(Msg("InfoPanel.MemTotal"), formatBytes(mem.Total))
		row(Msg("InfoPanel.MemFree"), formatBytes(mem.Free))
		if mem.Shared > 0 {
			row(Msg("InfoPanel.MemShared"), formatBytes(mem.Shared))
		}
		if mem.Buffered > 0 {
			row(Msg("InfoPanel.MemBuffered"), formatBytes(mem.Buffered))
		}
		if mem.SwapTotal > 0 {
			row(Msg("InfoPanel.SwapTotal"), formatBytes(mem.SwapTotal))
			row(Msg("InfoPanel.SwapFree"), formatBytes(mem.SwapFree))
		}
	}
	return model
}

// HandleSemanticAction обрабатывает нативные GUI-действия для ViewerView
func (vv *ViewerView) HandleSemanticAction(action map[string]any) bool {
	target := semanticString(action["target"])
	if vtui.SemanticID(vv) != target {
		return false
	}

	switch semanticString(action["action"]) {
	case "viewer.scroll":
		offset := int64(semanticInt(action["offset"]))
		if offset < 0 {
			offset = 0
		}
		if offset > vv.backend.Size() {
			offset = vv.backend.Size()
		}
		if vv.HexMode {
			offset &= ^int64(0xF)
		} else {
			offset = vv.backend.FindLineStart(offset)
		}
		vv.TopOffset = offset
		return true
	case "control.focus":
		vv.SetFocus(true)
		return true
	}
	return false
}

// HandleSemanticAction глобально маршрутизирует семантические действия из внешнего GUI
func HandleSemanticAction(action map[string]any) bool {
	if action == nil {
		return false
	}
	actionName := semanticString(action["action"])
	if semanticString(action["target"]) == "app" && (actionName == "presentation.toggle" || actionName == "toggle_presentation") {
		toggleGuiPresentation()
		return true
	}
	if semanticString(action["target"]) == "app" && actionName == "workspace.switch" {
		idx := semanticInt(action["index"])
		if vtui.FrameManager == nil || idx < 0 || idx >= len(vtui.FrameManager.Screens) {
			return false
		}
		vtui.FrameManager.SwitchScreen(idx)
		return true
	}
	if kind, _ := action["kind"].(string); kind == "command" {
		return vtui.FrameManager.EmitCommand(semanticInt(action["command"]), action["args"])
	}
	if actionName == "menuBar.itemSelect" || actionName == "menuBar.itemActivate" {
		if mb := vtui.FrameManager.GetActiveMenuBar(); mb != nil {
			menuIndex := semanticInt(action["menuIndex"])
			itemIndex := semanticInt(action["index"])
			if menuIndex >= 0 && menuIndex < len(mb.Items) &&
				itemIndex >= 0 && itemIndex < len(mb.Items[menuIndex].SubItems) &&
				!mb.Items[menuIndex].SubItems[itemIndex].Separator {
				if actionName == "menuBar.itemSelect" {
					// A pointer-hover action is deliberately non-activating. It may
					// arrive after the pointer has already moved to another top-level
					// menu, so accepting it must never reopen the stale submenu.
					if !mb.Active || mb.SelectPos != menuIndex {
						return false
					}
					menu := activeMenuBarSubmenu(mb, semanticString(action["target"]))
					if menu == nil {
						return false
					}
					menu.SetSelectPos(itemIndex)
					return true
				}

				// Click activation is atomic and may legitimately target a submenu
				// which QML was previewing before Go had materialized it.
				mb.Active = true
				if mb.SelectPos != menuIndex {
					mb.ActivateSubMenu(menuIndex)
				}
				if menu := activeMenuBarSubmenu(mb, ""); menu != nil {
					menu.SetSelectPos(itemIndex)
					handled := menu.ProcessKey(&vtinput.InputEvent{
						Type:           vtinput.KeyEventType,
						KeyDown:        true,
						VirtualKeyCode: vtinput.VK_RETURN,
						InputSource:    "qt_semantic",
					})
					if handled {
						// Semantic actions execute outside FrameManager's normal
						// keyboard-dispatch loop. Complete the same post-dispatch
						// lifecycle here: close the menu bar and remove every frame
						// whose command marked it done (for example viewer/editor
						// after File > Exit).
						vtui.FrameManager.EmitCommand(vtui.CmMenuClose, nil)
						frames := append([]vtui.Frame(nil),
							vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)...)
						for i := len(frames) - 1; i >= 0; i-- {
							if frames[i].IsDone() && frames[i].GetType() == vtui.TypeMenu {
								vtui.FrameManager.RemoveFrame(frames[i])
							}
						}

						frames = append(frames[:0],
							vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)...)
						closeWholeScreen := len(vtui.FrameManager.Screens) > 1
						hasDoneFrame := false
						for _, frame := range frames {
							if frame.IsDone() {
								hasDoneFrame = true
								continue
							}
							if frame.GetType() != vtui.TypeDesktop {
								closeWholeScreen = false
							}
						}
						if hasDoneFrame && closeWholeScreen {
							// Viewer/editor live on their own vtui screen. Closing
							// that screen restores the previous commander panels;
							// merely removing the document frame would expose its
							// otherwise-empty Desktop fallback.
							vtui.FrameManager.CloseActiveScreen()
						} else {
							for i := len(frames) - 1; i >= 0; i-- {
								if frames[i].IsDone() {
									vtui.FrameManager.RemoveFrame(frames[i])
								}
							}
						}
						vtui.FrameManager.Redraw()
					}
					return handled
				}
			}
		}
		return false
	}
	if actionName == "menu_bar_activate" || actionName == "menuBar.activate" || actionName == "menuBar.toggle" {
		if mb := vtui.FrameManager.GetActiveMenuBar(); mb != nil {
			idx := semanticInt(action["index"])
			if idx >= 0 && idx < len(mb.Items) {
				if actionName == "menuBar.toggle" && mb.Active && mb.SelectPos == idx {
					frames := vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
					for i := len(frames) - 1; i >= 0; i-- {
						if menu, _ := appFrameVMenu(frames[i]); menu != nil {
							frames[i].Close()
							break
						}
					}
					mb.Active = false
					vtui.FrameManager.Redraw()
					return true
				}
				mb.Active = true
				mb.ActivateSubMenu(idx)
				vtui.FrameManager.Redraw()
				return true
			}
		}
	}

	activeIdx := vtui.FrameManager.ActiveIdx
	frames := vtui.FrameManager.GetActiveFrames(activeIdx)
	for i := len(frames) - 1; i >= 0; i-- {
		if h, ok := frames[i].(vtui.SemanticActionHandler); ok && h.HandleSemanticAction(action) {
			vtui.FrameManager.Redraw()
			return true
		}
	}

	target := semanticString(action["target"])
	if target == "" {
		return false
	}
	for i := len(frames) - 1; i >= 0; i-- {
		if handleSemanticFrameAction(frames[i], target, action) {
			vtui.FrameManager.Redraw()
			return true
		}
	}
	return false
}

func activeMenuBarSubmenu(mb *vtui.MenuBar, target string) *vtui.VMenu {
	if vtui.FrameManager == nil || mb == nil {
		return nil
	}
	frames := vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
	for i := len(frames) - 1; i >= 0; i-- {
		menu, _ := appFrameVMenu(frames[i])
		if menu == nil || !appIsMenuBarSubmenu(frames[i], mb) {
			continue
		}
		if target != "" && vtui.SemanticID(frames[i]) != target {
			continue
		}
		return menu
	}
	return nil
}

func toggleGuiPresentation() GuiPresentationMode {
	AppConfig.GuiPresentation = nextGuiPresentationMode(AppConfig.GuiPresentation)
	SaveConfig()
	if vtui.FrameManager != nil {
		vtui.FrameManager.HardRefresh()
		message := Msg("Presentation.GUI")
		if AppConfig.GuiPresentation == GuiPresentationText {
			message = Msg("Presentation.Text")
		}
		vtui.ShowToast(message, 2*time.Second)
	}
	return AppConfig.GuiPresentation
}

func handleSemanticFrameAction(frame vtui.Frame, target string, action map[string]any) bool {
	if vtui.SemanticID(frame) == target {
		switch semanticString(action["action"]) {
		case "close", "dialog.close", "window.close":
			frame.Close()
			return true
		case "menu_activate", "menu.activate":
			if menu, _ := appFrameVMenu(frame); menu != nil {
				idx := semanticInt(action["index"])
				if idx >= 0 && idx < len(menu.Items) && !menu.Items[idx].Separator {
					menu.SetSelectPos(idx)
					return menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
				}
			}
		case "menu_select", "menu.select":
			if menu, _ := appFrameVMenu(frame); menu != nil {
				idx := semanticInt(action["index"])
				if idx >= 0 && idx < len(menu.Items) && !menu.Items[idx].Separator {
					menu.SetSelectPos(idx)
					return true
				}
			}
		case "menu_scroll", "menu.scroll":
			if menu, _ := appFrameVMenu(frame); menu != nil {
				menu.ScrollBy(semanticInt(action["delta"]))
				return true
			}
		}
	}
	if c, ok := frame.(vtui.Container); ok {
		return handleSemanticChildrenAction(c.GetChildren(), target, action)
	}
	return false
}

func handleSemanticChildrenAction(children []vtui.UIElement, target string, action map[string]any) bool {
	for _, child := range children {
		if vtui.SemanticID(child) == target {
			return handleSemanticElementAction(child, action)
		}
		if c, ok := child.(vtui.Container); ok {
			if handleSemanticChildrenAction(c.GetChildren(), target, action) {
				return true
			}
		}
	}
	return false
}

func handleSemanticElementAction(el vtui.UIElement, action map[string]any) bool {
	switch semanticString(action["action"]) {
	case "focus", "control.focus":
		el.SetFocus(true)
		return true
	case "activate", "control.activate":
		return el.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
	case "toggle", "control.toggle":
		return el.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE, Char: ' ', InputSource: "qt_semantic"})
	case "set_text", "control.setText":
		if edit, ok := el.(*vtui.Edit); ok {
			edit.SetText(semanticString(action["text"]))
			if edit.OnTextChange != nil {
				edit.OnTextChange(edit.GetText())
			}
			return true
		}
	case "insert_text", "control.insertText":
		if edit, ok := el.(*vtui.Edit); ok {
			edit.InsertString(semanticString(action["text"]))
			return true
		}
	case "select", "control.select":
		idx := semanticInt(action["index"])
		switch w := el.(type) {
		case *vtui.RadioGroup:
			if idx >= 0 && idx < len(w.Items) {
				w.SetData(idx)
				return true
			}
		case *vtui.ListBox:
			if idx >= 0 && idx < len(w.Items) {
				w.SetSelectPos(idx)
				return true
			}
		case *vtui.ComboBox:
			if idx >= 0 && idx < len(w.Menu.Items) {
				w.Menu.SetSelectPos(idx)
				w.Edit.SetText(w.Menu.Items[idx].Text)
				return true
			}
		}
	}
	return false
}

func (pf *PanelsFrame) HandleSemanticAction(action map[string]any) bool {
	switch semanticString(action["action"]) {
	case "activate_panel", "panel.activate":
		side := semanticInt(action["side"])
		if side >= 0 && side < len(pf.panels) {
			pf.activeIdx = side
			pf.lastKey = 0
			if fsp, ok := pf.panels[side].(*FileSystemPanel); ok {
				fsp.clearFastFindForSemanticPointerIntent()
			}
			return true
		}
	case "panel_cursor", "panel.cursor":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			idx, ok := fsp.semanticEntryIndex(action)
			if !ok {
				return false
			}
			// A native GUI row click targets an item and activates its panel as
			// one visual operation. Applying those intents atomically prevents
			// an intermediate scene from highlighting the panel's old cursor.
			if semanticBool(action["activate"]) {
				pf.setActivePanelForAction(action)
				pf.lastKey = 0
			}
			fsp.clearFastFindForSemanticPointerIntent()
			fsp.SetCursorIndex(idx)
			return true
		}
	case "panel_pointer", "panel.pointer":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			phase := semanticString(action["phase"])
			button := semanticString(action["button"])
			if phase == "up" || phase == "cancel" {
				fsp.semanticPointerRelease()
				return true
			}
			if phase == "click" {
				// The click is intentionally represented in the semantic stream,
				// while all immediate state changes happen on mouse-down just as
				// they do in the terminal frontend.
				return true
			}
			idx, ok := fsp.semanticEntryIndex(action)
			if !ok {
				return false
			}
			if phase == "down" || phase == "doubleClick" {
				pf.setActivePanelForAction(action)
				pf.lastKey = 0
				fsp.clearFastFindForSemanticPointerIntent()
			}
			switch button {
			case "right":
				if phase == "down" {
					return fsp.semanticRightPointerDown(idx)
				}
				if phase == "move" {
					return fsp.semanticRightPointerMove(idx)
				}
				if phase == "doubleClick" {
					return fsp.semanticRightPointerDoubleClick(idx)
				}
			case "middle":
				if phase == "down" {
					fsp.SetCursorIndex(idx)
					return pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
				}
			case "left":
				if phase == "down" || phase == "move" {
					fsp.SetCursorIndex(idx)
					return true
				}
				if phase == "doubleClick" {
					fsp.SetCursorIndex(idx)
					return pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
				}
			}
		}
	case "panel_open", "panel.open":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			idx, ok := fsp.semanticEntryIndex(action)
			if !ok {
				return false
			}
			pf.setActivePanelForAction(action)
			fsp.clearFastFindForSemanticPointerIntent()
			fsp.SetCursorIndex(idx)
			return pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
		}
	case "panel_toggle_selection", "panel.toggleSelection":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			idx, ok := fsp.semanticEntryIndex(action)
			if !ok {
				return false
			}
			if rawRevision, present := action["selectionRevision"]; present && semanticInt64(rawRevision) != fsp.selectionRevision {
				return false
			}
			fsp.clearFastFindForSemanticPointerIntent()
			fsp.ToggleSelection(idx)
			return true
		}
	case "panel_set_selection", "panel.setSelection":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			if !fsp.applySemanticSelection(action) {
				return false
			}
			fsp.clearFastFindForSemanticPointerIntent()
			return true
		}
	case "panel_set_presentation", "panel.setPresentation":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			if !fsp.SetPresentation(PanelPresentation(semanticString(action["presentation"]))) {
				return false
			}
			pf.updateMenuCheckmarks()
			return true
		}
	case "panel_set_view_mode", "panel.setViewMode":
		mode, ok := parseViewModeName(semanticString(action["viewMode"]))
		if !ok {
			return false
		}
		side := pf.panelIndexForSemanticAction(action)
		if side < 0 {
			return false
		}
		if mode == ViewModeWide {
			pf.setWidePanel(side)
		} else {
			pf.setPanelViewMode(side, mode)
		}
		return true
	case "panel_set_gallery_layout", "panel.setGalleryLayout":
		fsp := pf.panelForSemanticAction(action)
		if fsp == nil {
			return false
		}
		modeValue := semanticString(action["layoutMode"])
		if modeValue == "" {
			modeValue = semanticString(action["galleryLayoutMode"])
		}
		mode, ok := parseGalleryLayoutMode(modeValue)
		if !ok {
			return false
		}
		columns := semanticInt(action["columnCount"])
		if columns == 0 {
			columns = fsp.effectiveGalleryColumnCount()
		}
		if !fsp.SetGalleryLayout(mode, columns) {
			return false
		}
		pf.exitWide()
		pf.updateMenuCheckmarks()
		return true
	case "panel_set_gallery_density", "panel.setGalleryDensity":
		fsp := pf.panelForSemanticAction(action)
		if fsp == nil {
			return false
		}
		modeValue := semanticString(action["layoutMode"])
		if modeValue == "" {
			modeValue = semanticString(action["galleryLayoutMode"])
		}
		if modeValue == "" {
			modeValue = string(fsp.effectiveGalleryLayoutMode())
		}
		mode, ok := parseGalleryLayoutMode(modeValue)
		if !ok {
			return false
		}
		return fsp.SetGalleryDensity(mode, semanticInt(action["density"]))
	case "panel_sort", "panel.sort":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			mode, ok := parseSortModeName(semanticString(action["mode"]))
			if !ok {
				return false
			}
			pf.setActivePanelForAction(action)
			pf.lastKey = 0
			fsp.clearFastFindForSemanticPointerIntent()
			fsp.SetSortMode(mode)
			pf.updateMenuCheckmarks()
			return true
		}
	case "panel_sort_menu", "panel.sortMenu":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			pf.setActivePanelForAction(action)
			pf.lastKey = 0
			fsp.clearFastFindForSemanticPointerIntent()
			actionSortMenu(pf)
			return true
		}
	case "panel_refresh", "panel.refresh":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			fsp.ReadDirectory()
			return true
		}
	case "submit_command", "command.submit":
		if text := semanticString(action["text"]); text != "" && pf.cmdLine != nil {
			pf.cmdLine.Edit.SetText(text)
		}
		closeActiveAutocompleteMenus()
		return pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
	case "complete_command", "command.complete":
		// QML keeps autocomplete selection local so arrow-key repeats never
		// wait for a semantic-scene round trip.  Tab commits that explicit
		// selection, if any, but an untouched menu merely closes and preserves
		// the exact command line.
		if text, present := action["text"]; present && pf.cmdLine != nil {
			pf.cmdLine.Edit.SetText(semanticString(text))
		}
		closeActiveAutocompleteMenus()
		return true
	case "set_command_text", "command.setText":
		if pf.cmdLine != nil {
			pf.cmdLine.Edit.SetText(semanticString(action["text"]))
			return true
		}
	case "emit_command", "command.emit":
		return vtui.FrameManager.EmitCommand(semanticInt(action["command"]), action["args"])
	}
	return false
}

func closeActiveAutocompleteMenus() {
	if vtui.FrameManager == nil {
		return
	}
	for _, frame := range vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx) {
		if _, ok := frame.(*vtui.AutoCompleteMenu); ok {
			frame.Close()
		}
	}
}

func (pf *PanelsFrame) setActivePanelForAction(action map[string]any) {
	side := pf.panelIndexForSemanticAction(action)
	if side >= 0 && side < len(pf.panels) {
		pf.activeIdx = side
	}
}

func (pf *PanelsFrame) panelIndexForSemanticAction(action map[string]any) int {
	side := pf.activeIdx
	if _, present := action["side"]; present {
		side = semanticInt(action["side"])
	}
	if side < 0 || side >= len(pf.panels) {
		return -1
	}
	return side
}

func (pf *PanelsFrame) panelForSemanticAction(action map[string]any) *FileSystemPanel {
	side := pf.panelIndexForSemanticAction(action)
	if side < 0 {
		return nil
	}
	if fsp, ok := pf.panels[side].(*FileSystemPanel); ok {
		return fsp
	}
	return nil
}

// Native FileSystemPanel mouse presses leave fast-find before applying their
// pointer action. Gallery sends the equivalent intent through semantic
// actions, so clear both the visible mode and its stale query there as well.
func (fp *FileSystemPanel) clearFastFindForSemanticPointerIntent() {
	if !fp.fastFindMode && fp.fastFindStr == "" {
		return
	}
	fp.fastFindMode = false
	fp.fastFindStr = ""
}

func (fp *FileSystemPanel) semanticEntryIndex(action map[string]any) (int, bool) {
	fp.updateSemanticRevisions()
	if rawRevision, ok := action["catalogRevision"]; ok && semanticInt64(rawRevision) != fp.catalogRevision {
		return 0, false
	}

	if entryID := semanticString(action["entryId"]); entryID != "" {
		sourceKind, _ := fp.semanticSourceInfo()
		for idx, entry := range fp.entries {
			id, _ := fp.semanticEntryMetadata(entry, sourceKind)
			if id == entryID {
				return idx, true
			}
		}
		return 0, false
	}

	if _, ok := action["index"]; !ok {
		return 0, false
	}
	// Preserve the v2 index action behavior: cursor/open clamp through
	// SetCursorIndex and selection toggles outside the range are harmless.
	return semanticInt(action["index"]), true
}

func (fp *FileSystemPanel) applySemanticSelection(action map[string]any) bool {
	fp.updateSemanticRevisions()
	if rawRevision, ok := action["catalogRevision"]; ok && semanticInt64(rawRevision) != fp.catalogRevision {
		return false
	}
	if rawRevision, ok := action["selectionRevision"]; ok && semanticInt64(rawRevision) != fp.selectionRevision {
		return false
	}

	entryIDs := semanticStringSlice(action["entryIds"])
	if entryID := semanticString(action["entryId"]); entryID != "" {
		entryIDs = append(entryIDs, entryID)
	}
	wanted := make(map[int]struct{}, len(entryIDs))
	sourceKind, _ := fp.semanticSourceInfo()
	byID := make(map[string]int, len(fp.entries))
	for idx, entry := range fp.entries {
		id, _ := fp.semanticEntryMetadata(entry, sourceKind)
		byID[id] = idx
	}
	for _, id := range entryIDs {
		idx, ok := byID[id]
		if !ok {
			return false
		}
		wanted[idx] = struct{}{}
	}
	for _, idx := range semanticIntSlice(action["indices"]) {
		if idx < 0 || idx >= len(fp.entries) {
			return false
		}
		wanted[idx] = struct{}{}
	}

	mode := strings.ToLower(semanticString(action["mode"]))
	if mode == "" {
		mode = "replace"
	}
	switch mode {
	case "replace":
		for idx := range fp.entries {
			_, selected := wanted[idx]
			fp.SetItemSelected(idx, selected)
		}
	case "add":
		for idx := range wanted {
			fp.SetItemSelected(idx, true)
		}
	case "remove":
		for idx := range wanted {
			fp.SetItemSelected(idx, false)
		}
	case "toggle":
		for idx := range wanted {
			fp.ToggleSelection(idx)
		}
	default:
		return false
	}
	return true
}

func (fp *FileSystemPanel) semanticSourceInfo() (sourceKind string, previewCapable bool) {
	provider, ok := fp.vfs.(vfs.LocalPathProvider)
	if !ok {
		return "vfs", false
	}
	if _, err := provider.LocalPath(fp.vfs.GetPath()); err != nil {
		return "local", false
	}
	return "local", true
}

func (fp *FileSystemPanel) semanticEntryMetadata(entry *fileEntry, sourceKind string) (entryID, localPath string) {
	if provider, ok := fp.vfs.(vfs.LocalPathProvider); ok {
		path := fp.vfs.Join(fp.vfs.GetPath(), entry.Name)
		if resolved, err := provider.LocalPath(path); err == nil {
			localPath = resolved
		}
	}
	identity := sourceKind + "\x00" + localPath
	if localPath == "" {
		identity = sourceKind + "\x00" + fmt.Sprintf("%T", fp.vfs) + "\x00" + fp.vfs.GetPath() + "\x00" + entry.Name
	}
	sum := sha256.Sum256([]byte(identity))
	return fmt.Sprintf("entry-%x", sum[:16]), localPath
}

func writeSemanticFingerprintString(h hash.Hash, value string) {
	var size [8]byte
	binary.LittleEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = h.Write(size[:])
	_, _ = h.Write([]byte(value))
}

func semanticMTimeNanos(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixNano()
}

func (fp *FileSystemPanel) semanticFingerprints() (uint64, uint64) {
	catalog := fnv.New64a()
	selection := fnv.New64a()
	selectedEntryIDs := make([]string, 0)
	sourceKind, previewCapable := fp.semanticSourceInfo()
	writeSemanticFingerprintString(catalog, sourceKind)
	writeSemanticFingerprintString(catalog, fp.vfs.GetPath())
	// This presentation option changes the normalized display fields consumed
	// by unified Columns/Details without changing any VFS entry. Treat it as a
	// catalog change so persistent Gallery sessions receive the new appearance
	// instead of retaining their previous name split indefinitely.
	if AppConfig.SeparateFileExtensions {
		_, _ = catalog.Write([]byte{1})
	} else {
		_, _ = catalog.Write([]byte{0})
	}
	if previewCapable {
		_, _ = catalog.Write([]byte{1})
	} else {
		_, _ = catalog.Write([]byte{0})
	}
	for _, entry := range fp.entries {
		entryID, localPath := fp.semanticEntryMetadata(entry, sourceKind)
		writeSemanticFingerprintString(catalog, entryID)
		writeSemanticFingerprintString(catalog, localPath)
		writeSemanticFingerprintString(catalog, entry.Name)
		writeSemanticFingerprintString(catalog, entry.Mode)
		var value [8]byte
		binary.LittleEndian.PutUint64(value[:], uint64(entry.Size))
		_, _ = catalog.Write(value[:])
		binary.LittleEndian.PutUint64(value[:], uint64(semanticMTimeNanos(entry.MTime)))
		_, _ = catalog.Write(value[:])
		flags := byte(0)
		if entry.IsDir {
			flags |= 1 << 0
		}
		if entry.IsHidden {
			flags |= 1 << 1
		}
		if entry.IsExecutable {
			flags |= 1 << 2
		}
		if entry.IsCached {
			flags |= 1 << 3
		}
		if entry.SizeCalculated {
			flags |= 1 << 4
		}
		_, _ = catalog.Write([]byte{flags})
		if entry.Selected {
			selectedEntryIDs = append(selectedEntryIDs, entryID)
		}
	}
	// SelectionRevision represents the selected identity set, independently
	// of catalog ordering. Sorting already advances CatalogRevision; advancing
	// SelectionRevision as well rejects otherwise valid incremental selection
	// intents and creates needless state churn in native Gallery clients.
	sort.Strings(selectedEntryIDs)
	for _, entryID := range selectedEntryIDs {
		writeSemanticFingerprintString(selection, entryID)
	}
	return catalog.Sum64(), selection.Sum64()
}

func (fp *FileSystemPanel) updateSemanticRevisions() {
	catalog, selection := fp.semanticFingerprints()
	if !fp.semanticCatalogInitialized || catalog != fp.semanticCatalogFingerprint {
		fp.semanticCatalogFingerprint = catalog
		fp.semanticCatalogInitialized = true
		fp.catalogRevision++
	}
	if !fp.semanticSelectionInitialized || selection != fp.semanticSelectionFingerprint {
		fp.semanticSelectionFingerprint = selection
		fp.semanticSelectionInitialized = true
		fp.selectionRevision++
	}
}

type semanticPanelStaticCache struct {
	catalogRevision       int64
	highlightRevision     int64
	separateFileExtension bool
	entries               []extui.FileEntryModel
	highlightStyles       map[string]extui.HighlightStyleModel
	totalSize             int64
}

func (fp *FileSystemPanel) semanticStaticPanelData(sourceKind string) *semanticPanelStaticCache {
	cache := fp.semanticStaticCache
	if cache != nil &&
		cache.catalogRevision == fp.catalogRevision &&
		cache.highlightRevision == GlobalFileHighlighter.Revision &&
		cache.separateFileExtension == AppConfig.SeparateFileExtensions {
		return cache
	}

	cache = &semanticPanelStaticCache{
		catalogRevision:       fp.catalogRevision,
		highlightRevision:     GlobalFileHighlighter.Revision,
		separateFileExtension: AppConfig.SeparateFileExtensions,
		entries:               make([]extui.FileEntryModel, 0, len(fp.entries)),
		highlightStyles:       make(map[string]extui.HighlightStyleModel),
	}
	for i, entry := range fp.entries {
		entryID, localPath := fp.semanticEntryMetadata(entry, sourceKind)
		displayBaseName := entry.Name
		displayExtension := ""
		if AppConfig.SeparateFileExtensions && !entry.NoExtension && !entry.IsDir && entry.Name != ".." {
			if base, extension := splitFileExtension(entry.Name); extension != "" {
				displayBaseName = base
				displayExtension = extension
			}
		}
		highlightStyleID, highlightStyle := GlobalFileHighlighter.SemanticStyle(&entry.VFSItem)
		if highlightStyleID != "" {
			cache.highlightStyles[highlightStyleID] = highlightStyle
		}
		if !entry.IsDir {
			cache.totalSize += entry.Size
		}
		cache.entries = append(cache.entries, extui.FileEntryModel{
			Index:            i,
			EntryID:          entryID,
			Name:             entry.Name,
			DisplayBaseName:  displayBaseName,
			DisplayExtension: displayExtension,
			LocalPath:        localPath,
			Size:             entry.Size,
			SizeText:         semanticFileSize(entry),
			IsDir:            entry.IsDir,
			IsUp:             entry.Name == "..",
			IsHidden:         entry.IsHidden,
			IsExecutable:     entry.IsExecutable,
			IsCached:         entry.IsCached,
			SizeCalculated:   entry.SizeCalculated,
			MTime:            entry.MTime.Format("2006-01-02 15:04"),
			MTimeNanos:       semanticMTimeNanos(entry.MTime),
			Version:          fmt.Sprintf("%d:%d", semanticMTimeNanos(entry.MTime), entry.Size),
			Mode:             entry.Mode,
			HighlightStyleID: highlightStyleID,
		})
	}
	fp.semanticStaticCache = cache
	return cache
}

func (fp *FileSystemPanel) semanticPanelModel(ctx *vtui.SemanticContext, side int, active bool) extui.PanelModel {
	fp.updateSemanticRevisions()
	galleryLayoutMode := fp.effectiveGalleryLayoutMode()
	galleryLayoutRevision := fp.galleryLayoutRevision
	if galleryLayoutRevision < 1 {
		galleryLayoutRevision = 1
	}
	sourceKind, previewCapable := fp.semanticSourceInfo()
	static := fp.semanticStaticPanelData(sourceKind)
	entries := append([]extui.FileEntryModel(nil), static.entries...)
	selectedCount := 0
	var selectedSize int64
	for i, entry := range fp.entries {
		entries[i].Selected = entry.Selected
		if entry.Selected {
			selectedCount++
			selectedSize += entry.Size
		}
	}
	cursorEntryID := ""
	if cursor := fp.GetCursorIndex(); cursor >= 0 && cursor < len(entries) {
		cursorEntryID = entries[cursor].EntryID
	}
	columns := make([]extui.PanelColumnModel, 0, len(fp.table.Columns))
	for index, column := range fp.table.Columns {
		mode, sortable := fp.columnSortMode(index)
		alignment := "left"
		if column.Alignment == vtui.AlignRight {
			alignment = "right"
		}
		columns = append(columns, extui.PanelColumnModel{
			Index:     index,
			Title:     column.Title,
			Width:     column.Width,
			Alignment: alignment,
			SortMode:  sortModeName(mode),
			Sortable:  sortable,
		})
	}

	return extui.PanelModel{
		ID:                     vtui.SemanticID(fp),
		Side:                   side,
		Active:                 active,
		Path:                   fp.vfs.GetPath(),
		Title:                  fp.currentTitle,
		ViewMode:               viewModeName(fp.effectiveViewMode()),
		Presentation:           string(parsePanelPresentation(string(fp.presentation))),
		GalleryLayoutMode:      string(galleryLayoutMode),
		GalleryColumnCount:     fp.effectiveGalleryColumnCount(),
		GalleryDensity:         fp.galleryDensity(galleryLayoutMode),
		GalleryLayoutRevision:  galleryLayoutRevision,
		SourceKind:             sourceKind,
		PreviewCapable:         previewCapable,
		CatalogRevision:        fp.catalogRevision,
		SelectionRevision:      fp.selectionRevision,
		HighlightRevision:      GlobalFileHighlighter.Revision,
		HighlightStyles:        static.highlightStyles,
		CursorEntryID:          cursorEntryID,
		SortMode:               sortModeName(fp.sortMode),
		SortReverse:            fp.sortReverse,
		SeparateFileExtensions: AppConfig.SeparateFileExtensions,
		Cursor:                 fp.GetCursorIndex(),
		Top:                    fp.table.TopPos,
		Loading:                fp.isLoading,
		FastFind:               fp.fastFindMode,
		FastFindText:           fp.fastFindStr,
		SelectedCount:          selectedCount,
		SelectedSize:           selectedSize,
		TotalCount:             len(fp.entries),
		TotalSize:              static.totalSize,
		Columns:                columns,
		GalleryColumns:         fp.semanticGalleryColumns(),
		Entries:                entries,
	}
}

// semanticGalleryColumns is intentionally independent from the panel's saved
// legacy ViewMode. The reusable Details renderer always has a stable Name +
// Size schema, even when the text fallback remains Brief or Medium.
func (fp *FileSystemPanel) semanticGalleryColumns() []extui.PanelColumnModel {
	width := fp.X2 - fp.X1 + 1
	nameWidth := width - 14
	if nameWidth < 5 {
		nameWidth = 5
	}
	title := func(base string, mode SortMode) string {
		if fp.sortMode != mode {
			return base
		}
		if fp.sortIsAscending() {
			return base + " ↑"
		}
		return base + " ↓"
	}
	return []extui.PanelColumnModel{
		{
			ID:        "name",
			Role:      "name",
			Index:     0,
			Title:     title(Msg("Panel.Column.Name"), SortName),
			Width:     nameWidth,
			Alignment: "left",
			SortMode:  sortModeName(SortName),
			Sortable:  true,
		},
		{
			ID:        "size",
			Role:      "size",
			Index:     1,
			Title:     title(Msg("Panel.Column.Size"), SortSize),
			Width:     panelSizeColumnWidth,
			Alignment: "right",
			SortMode:  sortModeName(SortSize),
			Sortable:  true,
		},
	}
}

func semanticFileSize(entry *fileEntry) string {
	if entry.IsDir {
		if entry.SizeCalculated {
			return formatIntWithSpaces(entry.Size)
		}
		if entry.Name == ".." {
			return Msg("Panel.UpDir")
		}
		return ""
	}
	return formatIntWithSpaces(entry.Size)
}

func viewModeName(mode ViewMode) string {
	switch mode {
	case ViewModeBrief:
		return "brief"
	case ViewModeDetailed:
		return "detailed"
	case ViewModeWide:
		return "wide"
	default:
		return "medium"
	}
}

func parseViewModeName(name string) (ViewMode, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "brief":
		return ViewModeBrief, true
	case "medium":
		return ViewModeMedium, true
	case "detailed", "details":
		return ViewModeDetailed, true
	case "wide":
		return ViewModeWide, true
	default:
		return ViewModeMedium, false
	}
}

func sortModeName(mode SortMode) string {
	switch mode {
	case SortExt:
		return "extension"
	case SortTime:
		return "time"
	case SortSize:
		return "size"
	case SortUnsorted:
		return "unsorted"
	default:
		return "name"
	}
}

func parseSortModeName(name string) (SortMode, bool) {
	switch name {
	case "name":
		return SortName, true
	case "extension":
		return SortExt, true
	case "time":
		return SortTime, true
	case "size":
		return SortSize, true
	case "unsorted":
		return SortUnsorted, true
	default:
		return SortUnsorted, false
	}
}

func (cl *CommandLine) semanticModel(ctx *vtui.SemanticContext) *extui.CommandLineModel {
	model := &extui.CommandLineModel{
		ID:         vtui.SemanticID(cl),
		Visible:    cl.IsVisible(),
		Focused:    cl.IsFocused(),
		Prompt:     cl.Prompt,
		PromptRuns: semanticRunsFromCells(cl.RichPrompt),
		Text:       cl.Edit.GetText(),
		Empty:      cl.IsEmpty(),
		InputX:     cl.Edit.X1 - cl.X1,
	}
	rendered := semanticRenderSurface(cl.X1, cl.Y1, cl.X2, cl.Y2, cl.DisplayObject)
	if len(rendered.Rows) > 0 {
		model.Runs = rendered.Rows[0]
	}
	model.CursorX = rendered.CursorX
	model.CursorPrefixRuns = rendered.CursorPrefixRuns
	model.CursorVisible = rendered.CursorVisible
	model.CursorShape = rendered.CursorShape
	return model
}

func (tv *TerminalView) semanticModel(ctx *vtui.SemanticContext) *extui.TerminalModel {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	buf := tv.Lines
	if tv.UseAltScreen {
		buf = tv.AltLines
	}
	offset := 0
	if !tv.UseAltScreen {
		lowestRow := 0
		for y := tv.Height - 1; y >= 0; y-- {
			if tv.rowHasText(y) {
				lowestRow = y
				break
			}
		}
		if tv.CursorY > lowestRow {
			lowestRow = tv.CursorY
		}
		if lowestRow < tv.Height-1 {
			offset = (tv.Height - 1) - lowestRow
		}
	}

	var rows []extui.TextRowModel
	for y := 0; y < tv.Height && y < len(buf); y++ {
		drawY := y + offset
		if tv.UseAltScreen {
			drawY = y
		}
		if drawY < 0 || drawY >= tv.Height {
			continue
		}
		rows = append(rows, extui.TextRowModel{
			Index: drawY,
			Runs:  semanticRunsFromCells(buf[y]),
		})
	}

	return &extui.TerminalModel{
		ID:        vtui.SemanticID(tv),
		Title:     tv.Title,
		Visible:   tv.IsVisible(),
		Focused:   tv.IsFocused(),
		AltScreen: tv.UseAltScreen,
		Busy:      tv.Muted,
		CursorX:   tv.CursorX,
		CursorY:   tv.CursorY + offset,
		Rows:      rows,
	}
}

func (vv *ViewerView) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	rows := vv.semanticRows()
	width := vv.X2 - vv.X1 + 1
	if vv.scrollBar != nil {
		width--
	}
	rendered := semanticRenderSurface(vv.X1, vv.Y1+1,
		vv.X1+width-1, vv.Y2, vv.DisplayObject)
	rows = semanticRowsWithRenderedRuns(rows, rendered.Rows)
	mode := "text"
	if vv.HexMode {
		mode = "hex"
	}

	surface := extui.SurfaceModel{
		ID:        vtui.SemanticID(vv),
		Kind:      "viewer",
		Title:     vv.GetTitle(),
		Path:      vv.path,
		BaseName:  semanticBaseName(vv.vfs, vv.path),
		Mode:      mode,
		HexMode:   vv.HexMode,
		WrapMode:  vv.WrapMode,
		Busy:      vv.Busy,
		TopOffset: vv.TopOffset,
		Size:      vv.backend.Size(),
		Rows:      rows,
	}
	return surface.ToMap()
}

func (vv *ViewerView) semanticRows() []extui.TextRowModel {
	if vv.backend == nil {
		return nil
	}
	width := vv.X2 - vv.X1 + 1
	if vv.scrollBar != nil {
		width--
	}
	contentHeight := vv.Y2 - vv.Y1
	if width <= 0 || contentHeight <= 0 {
		return nil
	}
	if vv.Busy {
		return []extui.TextRowModel{{Index: 0, Text: " [ Loading... ] "}}
	}
	var rows []extui.TextRowModel
	if vv.HexMode {
		currOffset := vv.TopOffset &^ 0xF
		for y := 0; y < contentHeight && currOffset < vv.backend.Size(); y++ {
			data, err := vv.backend.ReadAt(currOffset, 16)
			if err != nil && err != piecetable.ErrLoading {
				break
			}
			rows = append(rows, extui.TextRowModel{
				Index:  y,
				Offset: currOffset,
				Text:   semanticHexLine(currOffset, data),
			})
			currOffset += 16
		}
		return rows
	}

	currOffset := vv.TopOffset
	for y := 0; y < contentHeight; y++ {
		if currOffset >= vv.backend.Size() {
			break
		}
		data, err := vv.backend.ReadAt(currOffset, width*4)
		if err == piecetable.ErrLoading {
			rows = append(rows, extui.TextRowModel{Index: y, Offset: currOffset, Text: " [ Loading... ] "})
			break
		}
		if err != nil || len(data) == 0 {
			break
		}
		lineLen, textLen := semanticViewerLineLen(data, width, vv.WrapMode)
		rows = append(rows, extui.TextRowModel{Index: y, Offset: currOffset, Text: string(data[:textLen])})
		if lineLen <= 0 {
			break
		}
		currOffset += int64(lineLen)
	}
	return rows
}

func semanticHexLine(offset int64, data []byte) string {
	hexPart := ""
	asciiPart := ""
	for i := 0; i < 16; i++ {
		if i < len(data) {
			hexPart += fmt.Sprintf("%02X ", data[i])
			r := rune(data[i])
			if r < 32 || r > 126 {
				r = '.'
			}
			asciiPart += string(r)
		} else {
			hexPart += "   "
		}
		if i == 7 {
			hexPart += " "
		}
	}
	return fmt.Sprintf("%010X: %s | %s", offset, hexPart, asciiPart)
}

func semanticViewerLineLen(data []byte, width int, wrap bool) (lineLen int, textLen int) {
	visualWidth := 0
	tabSize := 8
	if AppConfig.EditorTabSize > 0 {
		tabSize = AppConfig.EditorTabSize
	}
	for lineLen < len(data) {
		r, size := utf8.DecodeRune(data[lineLen:])
		if r == '\n' {
			lineLen += size
			return lineLen, textLen
		}
		if r == '\r' {
			lineLen += size
			continue
		}
		rw := 1
		if r == '\t' {
			rw = tabSize - (visualWidth % tabSize)
		} else {
			rw = runewidth.RuneWidth(r)
			if rw <= 0 {
				rw = 1
			}
		}
		if wrap && visualWidth+rw > width {
			return lineLen, textLen
		}
		visualWidth += rw
		lineLen += size
		textLen = lineLen
		if !wrap && visualWidth >= width {
			return lineLen, textLen
		}
	}
	return lineLen, textLen
}

func (ev *EditorView) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	rows := ev.semanticRows()
	width := ev.X2 - ev.X1 + 1
	if ev.scrollBar != nil {
		width--
	}
	rendered := semanticRenderSurface(ev.X1, ev.Y1+1,
		ev.X1+width-1, ev.Y2, ev.DisplayObject)
	rows = semanticRowsWithRenderedRuns(rows, rendered.Rows)

	surface := extui.SurfaceModel{
		ID:                 vtui.SemanticID(ev),
		Kind:               "editor",
		Title:              ev.GetTitle(),
		Path:               ev.filePath,
		BaseName:           semanticBaseName(ev.vfs, ev.filePath),
		Busy:               ev.IsBusy(),
		Dirty:              ev.modified,
		Saving:             ev.saving,
		WordWrap:           ev.WordWrap,
		Overtype:           ev.overtype,
		CursorLine:         ev.CursorLine,
		CursorPos:          ev.CursorPos,
		CursorVisualRow:    rendered.CursorY,
		CursorVisualColumn: rendered.CursorX,
		CursorVisible:      rendered.CursorVisible,
		CursorShape:        rendered.CursorShape,
		ScrollTop:          ev.ScrollTopRow,
		ScrollLeft:         ev.ScrollLeft,
		Selection:          ev.selActive,
		Rows:               rows,
		Autocomplete:       ev.semanticAutocomplete(),
	}
	return surface.ToMap()
}

// GetText возвращает текущий текст редактора из PieceTable
func (ev *EditorView) GetText() string {
	if ev.pt == nil {
		return ""
	}
	return ev.pt.String()
}

// HandleSemanticAction обрабатывает нативные GUI-действия для EditorView
func (ev *EditorView) HandleSemanticAction(action map[string]any) bool {
	target := semanticString(action["target"])
	if vtui.SemanticID(ev) != target {
		return false
	}

	switch semanticString(action["action"]) {
	case "editor.setText":
		text := semanticString(action["text"])
		ev.SetText(text)
		return true
	case "editor.insertText":
		text := semanticString(action["text"])
		ev.PasteText(text)
		return true
	case "editor.deleteSelection":
		ev.DeleteSelection()
		return true
	case "editor.undo":
		ev.Undo()
		return true
	case "editor.redo":
		ev.Redo()
		return true
	case "editor.save":
		ev.SaveToFile(nil)
		return true
	case "editor.search":
		pattern := semanticString(action["pattern"])
		caseSensitive := semanticBool(action["case"])
		reverse := semanticBool(action["reverse"])
		next := semanticBool(action["next"])
		ev.Search(pattern, caseSensitive, reverse, false, false, next)
		return true
	case "control.focus":
		ev.SetFocus(true)
		return true
	}
	return false
}

func (ev *EditorView) semanticRows() []extui.TextRowModel {
	if ev.pt == nil || ev.li == nil || ev.engine == nil {
		return nil
	}
	ev.ensureEngineWidth()
	height := ev.Y2 - ev.Y1
	if height <= 0 {
		return nil
	}
	startLogLine, startFragIdx := ev.engine.GetLogLineAtVisualRow(ev.ScrollTopRow)
	var rows []extui.TextRowModel
	for logIdx := startLogLine; logIdx < ev.li.LineCount() && len(rows) < height; logIdx++ {
		frags := ev.engine.GetFragments(logIdx)
		baseVRow := ev.engine.GetRowOffset(logIdx)
		for fIdx, frag := range frags {
			if logIdx == startLogLine && fIdx < startFragIdx {
				continue
			}
			data, err := ev.pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
			text := string(data)
			if err == piecetable.ErrLoading {
				text = " [ Loading... ] "
			} else if err != nil {
				text = ""
			}
			rows = append(rows, extui.TextRowModel{
				Index:       len(rows),
				VisualRow:   baseVRow + fIdx,
				LogicalLine: logIdx,
				Offset:      int64(frag.ByteOffsetStart),
				Text:        text,
			})
			if len(rows) >= height {
				break
			}
		}
	}
	return rows
}

func (ev *EditorView) semanticAutocomplete() map[string]any {
	if !ev.acEnabled || len(ev.acMatches) == 0 || ev.acCurrentIdx < 0 || ev.acCurrentIdx >= len(ev.acMatches) {
		return nil
	}
	match := ev.acMatches[ev.acCurrentIdx]
	if len(match) <= len(ev.acPrefix) {
		return nil
	}
	return map[string]any{
		"prefix": ev.acPrefix,
		"tail":   match[len(ev.acPrefix):],
		"index":  ev.acCurrentIdx,
	}
}

func semanticRunsFromCells(cells []vtui.CharInfo) []extui.RunModel {
	if len(cells) == 0 {
		return nil
	}
	var runs []extui.RunModel
	var b strings.Builder
	var attr uint64
	haveRun := false
	flush := func() {
		if !haveRun {
			return
		}
		runs = append(runs, extui.RunModel{
			Text:       b.String(),
			Attr:       attr,
			Foreground: semanticAttrColor(attr, true),
			Background: semanticAttrColor(attr, false),
			Bold:       attr&vtui.ForegroundIntensity != 0,
			Underline:  attr&vtui.CommonLvbUnderscore != 0,
			Strikeout:  attr&vtui.CommonLvbStrikeout != 0,
		})
		b.Reset()
	}
	for _, cell := range cells {
		if cell.Char == vtui.WideCharFiller {
			continue
		}
		ch := cellRune(cell.Char)
		if !haveRun {
			attr = cell.Attributes
			haveRun = true
		} else if cell.Attributes != attr {
			flush()
			attr = cell.Attributes
			haveRun = true
		}
		b.WriteRune(ch)
	}
	flush()
	return runs
}

type semanticRenderedSurface struct {
	Rows             [][]extui.RunModel
	CursorPrefixRuns []extui.RunModel
	CursorX          int
	CursorY          int
	CursorVisible    bool
	CursorShape      string
}

func semanticRenderSurface(x1, y1, x2, y2 int, render func(*vtui.ScreenBuf)) semanticRenderedSurface {
	result := semanticRenderedSurface{CursorX: -1, CursorY: -1}
	if render == nil || x2 < x1 || y2 < y1 {
		return result
	}
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(max(1, x2+1), max(1, y2+1))
	scr.ThemePalette = &vtui.ThemePalette
	render(scr)
	cursorX, cursorY, visible, shape := scr.GetCursorStateForTesting()
	result.Rows = make([][]extui.RunModel, 0, y2-y1+1)
	for y := y1; y <= y2; y++ {
		cells := make([]vtui.CharInfo, 0, x2-x1+1)
		for x := x1; x <= x2; x++ {
			cells = append(cells, scr.GetCell(x, y))
		}
		result.Rows = append(result.Rows, semanticRunsFromCells(cells))
		if visible && y == cursorY && cursorX >= x1 && cursorX <= x2 {
			prefixLength := min(cursorX-x1, len(cells))
			result.CursorPrefixRuns = semanticRunsFromCells(cells[:prefixLength])
		}
	}
	if visible && cursorX >= x1 && cursorX <= x2 && cursorY >= y1 && cursorY <= y2 {
		result.CursorX = cursorX - x1
		result.CursorY = cursorY - y1
		result.CursorVisible = true
		switch shape {
		case vtui.CursorShapeBlock:
			result.CursorShape = "block"
		default:
			result.CursorShape = "underline"
		}
	}
	return result
}

func semanticRowsWithRenderedRuns(rows []extui.TextRowModel, rendered [][]extui.RunModel) []extui.TextRowModel {
	if len(rendered) == 0 {
		return rows
	}
	byIndex := make(map[int]extui.TextRowModel, len(rows))
	for _, row := range rows {
		byIndex[row.Index] = row
	}
	result := make([]extui.TextRowModel, 0, len(rendered))
	for index, runs := range rendered {
		row, ok := byIndex[index]
		if !ok {
			row = extui.TextRowModel{Index: index}
		}
		row.Text = ""
		row.Runs = runs
		result = append(result, row)
	}
	return result
}

func semanticAttrColor(attr uint64, foreground bool) string {
	reverse := attr&vtui.CommonLvbReverse != 0
	if reverse {
		foreground = !foreground
	}
	var rgb uint32
	if foreground {
		if attr&vtui.IsFgRGB != 0 {
			rgb = vtui.GetRGBFore(attr)
		} else {
			rgb = vtui.ThemePalette[vtui.GetIndexFore(attr)]
		}
		if attr&vtui.ForegroundDim != 0 {
			rgb = ((rgb>>16&0xff)/2)<<16 | ((rgb>>8&0xff)/2)<<8 | (rgb&0xff)/2
		}
	} else if attr&vtui.IsBgRGB != 0 {
		rgb = vtui.GetRGBBack(attr)
	} else {
		rgb = vtui.ThemePalette[vtui.GetIndexBack(attr)]
	}
	return fmt.Sprintf("#%06x", rgb&0xffffff)
}

func cellRune(ch uint64) rune {
	if ch == 0 || ch > utf8.MaxRune || (ch >= 0xD800 && ch <= 0xDFFF) {
		return ' '
	}
	return rune(ch)
}

func semanticBaseName(v interface{ Base(string) string }, path string) string {
	if path == "" {
		return ""
	}
	if v != nil {
		return v.Base(path)
	}
	return filepath.Base(path)
}

func semanticString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func semanticInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func semanticInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int8:
		return int64(n)
	case int16:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case uint:
		return int64(n)
	case uint8:
		return int64(n)
	case uint16:
		return int64(n)
	case uint32:
		return int64(n)
	case uint64:
		if n > uint64(^uint64(0)>>1) {
			return 0
		}
		return int64(n)
	case float32:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}

func semanticStringSlice(v any) []string {
	switch values := v.(type) {
	case []string:
		return append([]string(nil), values...)
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return out
	}
	return nil
}

func semanticIntSlice(v any) []int {
	switch values := v.(type) {
	case []int:
		return append([]int(nil), values...)
	case []any:
		out := make([]int, 0, len(values))
		for _, value := range values {
			out = append(out, semanticInt(value))
		}
		return out
	}
	return nil
}

func semanticBool(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	if n, ok := v.(int); ok {
		return n != 0
	}
	if f, ok := v.(float64); ok {
		return f != 0
	}
	return false
}
